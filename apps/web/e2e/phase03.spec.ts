import { expect, test, type Page } from '@playwright/test'

const password = process.env.E2E_ROOT_PASSWORD ?? ''
let team = '',
  project = '',
  root = '',
  member = '',
  top = '',
  middle = '',
  leaf = '',
  successor = ''
const projectURL = () => `/teams/${team}/projects/${project}`
test.describe.configure({ mode: 'serial' })
async function login(page: Page, username = 'root') {
  await page.goto('/login')
  await page.fill('#login-username', username)
  await page.fill('#login-password', password)
  await page.locator('button[type="submit"]').click()
  await expect(page).toHaveURL(/\/teams$/)
}
async function logout(page: Page) {
  await page.getByRole('button', { name: '退出登录', exact: true }).click()
  await expect(page).toHaveURL(/\/login/)
}
async function workspace(page: Page) {
  await page.goto(projectURL())
  await expect(page.getByRole('heading', { name: 'ZERO_R 项目闭环', exact: true })).toBeVisible()
}
async function selectTask(page: Page, title: string) {
  await page.getByRole('button', { name: new RegExp(title) }).click()
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
}
async function createTask(
  page: Page,
  title: string,
  parent: string,
  weight: number,
  owner: string,
) {
  await page.getByRole('button', { name: '新建任务', exact: true }).click()
  await page.getByLabel('任务标题', { exact: true }).fill(title)
  await page.getByLabel('任务负责人', { exact: true }).selectOption(owner)
  await page.getByLabel('父任务', { exact: true }).selectOption(parent)
  await page.getByLabel('工作量权重', { exact: true }).fill(String(weight))
  const response = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/tasks'),
  )
  await page.getByRole('button', { name: '保存新任务', exact: true }).click()
  const result = await response
  expect(result.status()).toBe(201)
  const id = (await result.json()).data.id as string
  await expect(page.getByRole('button', { name: new RegExp(title) })).toBeVisible()
  return id
}

test('real UI: login → select Team → create project', async ({ page }) => {
  expect(password).not.toBe('')
  await login(page)
  const me = await page.request.get('/api/v1/auth/me')
  root = (await me.json()).data.id
  await page.fill('#team-name', 'Phase03 ZERO_R')
  await page.fill('#team-description', '项目与任务验收工作区')
  await page.locator('form').first().getByRole('button', { name: '创建团队', exact: true }).click()
  const row = page.locator('li[data-team-id]', { hasText: 'Phase03 ZERO_R' })
  await expect(row).toBeVisible()
  team = (await row.getAttribute('data-team-id')) ?? ''
  await row.getByRole('button', { name: '项目', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`/teams/${team}/projects$`))
  await page.getByRole('button', { name: '新建项目', exact: true }).click()
  await page.getByLabel('项目名称', { exact: true }).fill('ZERO_R 项目闭环')
  await page.getByLabel('项目描述', { exact: true }).fill('真实 API、三级任务、精确进度与依赖')
  await page.getByLabel('项目负责人', { exact: true }).selectOption(root)
  await page.getByRole('button', { name: '创建项目', exact: true }).click()
  await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+$/)
  project = page.url().split('/').at(-1) ?? ''
  await expect(page.getByTestId('project-progress')).toHaveText('0.00%')
})

test('real API membership → UI project participation', async ({ page }) => {
  await login(page)
  const account = await page.request.post('/api/v1/admin/users', {
    headers: { 'X-Nexora-CSRF': '1' },
    data: { username: 'phase03_member', display_name: 'Lin', password },
  })
  expect(account.status()).toBe(201)
  member = (await account.json()).data.id
  const invitation = await page.request.post(`/api/v1/teams/${team}/invitations`, {
    headers: { 'X-Nexora-CSRF': '1' },
    data: { username: 'phase03_member', role: 'member' },
  })
  expect(invitation.status()).toBe(201)
  const invitationID = (await invitation.json()).data.id
  await logout(page)
  await login(page, 'phase03_member')
  const accepted = await page.request.post(`/api/v1/invitations/${invitationID}/accept`, {
    headers: { 'X-Nexora-CSRF': '1' },
  })
  expect(accepted.status()).toBe(200)
  await logout(page)
  await login(page)
  await workspace(page)
  await page.getByRole('button', { name: '项目成员', exact: true }).click()
  await page.getByLabel('添加 Team 成员', { exact: true }).selectOption(member)
  await page.getByRole('button', { name: '添加成员', exact: true }).click()
  await expect(page.getByText('@phase03_member', { exact: true })).toBeVisible()
})

test('real UI: create three levels and assign leaf owner and collaborator', async ({ page }) => {
  await login(page)
  await workspace(page)
  top = await createTask(page, '研发规划', '', 100, root)
  middle = await createTask(page, '后端服务', top, 50, root)
  leaf = await createTask(page, '接口实现', middle, 3, member)
  await selectTask(page, '接口实现')
  await page.getByRole('button', { name: '编辑任务', exact: true }).click()
  await page.getByLabel('协作者（可多选）', { exact: true }).selectOption(root)
  await page.getByRole('button', { name: '保存任务', exact: true }).click()
  await expect(page.getByRole('heading', { name: '接口实现', exact: true })).toBeVisible()
  const tree = await page.request.get(`/api/v1/teams/${team}/projects/${project}/tasks`)
  const data = (await tree.json()).data
  expect(data.tasks.map((t: { depth: number }) => t.depth).sort()).toEqual([1, 2, 3])
  expect(data.tasks.find((t: { id: string }) => t.id === leaf).collaborator_ids).toEqual([root])
  expect(data.progress.total_weight).toBe('3')
})

test('real participant: only assigned task is visible and progress is authoritative', async ({
  page,
}) => {
  await login(page, 'phase03_member')
  await workspace(page)
  await expect(page.getByRole('button', { name: /研发规划/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /后端服务/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '新建任务', exact: true })).toHaveCount(0)
  await selectTask(page, '接口实现')
  await page.getByRole('button', { name: '开始任务', exact: true }).click()
  await page.getByLabel('实际进度（0–99%）', { exact: true }).fill('60')
  await page.getByRole('button', { name: '记录进度', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('60.00%')
  await expect(page.getByRole('button', { name: '编辑任务', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '移除', exact: true })).toHaveCount(0)
})

test('real UI: 99% execution caps aggregation at 79% and dependencies block', async ({ page }) => {
  await login(page)
  await workspace(page)
  await selectTask(page, '接口实现')
  await page.getByLabel('实际进度（0–99%）', { exact: true }).fill('99')
  await page.getByRole('button', { name: '记录进度', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('79.00%')
  successor = await createTask(page, '集成验证', '', 1, root)
  await selectTask(page, '集成验证')
  await page.getByLabel('选择前置任务', { exact: true }).selectOption(leaf)
  await page.getByRole('button', { name: '添加依赖', exact: true }).click()
  await expect(page.getByText('依赖阻塞', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '开始任务', exact: true })).toHaveCount(0)
  await expect(page.getByTestId('project-progress')).toHaveText('59.25%')
  await page.getByLabel('忽略原因', { exact: true }).fill('负责人批准并行验证')
  await page.getByRole('button', { name: '忽略此依赖', exact: true }).click()
  await expect(page.getByText('已显式忽略', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '开始任务', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '开始任务', exact: true }).click()
  await expect(page.getByLabel('实际进度（0–99%）', { exact: true })).toBeVisible()
  await page.screenshot({ path: '../../.cache/phase03-workspace-desktop.png', fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.screenshot({ path: '../../.cache/phase03-workspace-mobile.png', fullPage: true })
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  )
  expect(overflow).toBe(false)
})

test('real UI: weight history, milestone plan, edit project and archive read-only', async ({
  page,
}) => {
  await login(page)
  await workspace(page)
  await selectTask(page, '接口实现')
  await page.getByLabel('新权重', { exact: true }).fill('1')
  await page.getByLabel('权重修改原因', { exact: true }).fill('重新估算工作量')
  await page.getByRole('button', { name: '变更权重并保留快照', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('39.50%')
  await page.getByRole('button', { name: '进度历史', exact: true }).click()
  await expect(page.getByText('weight_changed', { exact: true })).toBeVisible()
  await expect(page.getByText('59.25%', { exact: true }).first()).toBeVisible()
  await page.getByRole('button', { name: '里程碑', exact: true }).click()
  await page.getByLabel('里程碑名称', { exact: true }).fill('联调计划')
  await page.getByLabel('里程碑说明', { exact: true }).fill('完成接口联调准备')
  await page.getByRole('button', { name: '添加里程碑', exact: true }).click()
  await expect(page.getByRole('heading', { name: '联调计划', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '项目设置', exact: true }).click()
  await page.getByLabel('项目描述', { exact: true }).fill('项目设置经真实 API 更新')
  await page.getByRole('button', { name: '保存项目', exact: true }).click()
  await expect(page.getByText('项目设置经真实 API 更新', { exact: true }).first()).toBeVisible()
  await page.getByRole('button', { name: '归档并设为只读', exact: true }).click()
  await expect(page.getByText('已归档 · 只读', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '任务树', exact: true }).click()
  await selectTask(page, '接口实现')
  await expect(page.getByRole('button', { name: '记录进度', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '新建任务', exact: true })).toHaveCount(0)
  const write = await page.request.post(
    `/api/v1/teams/${team}/projects/${project}/tasks/${successor}/transition`,
    {
      headers: { 'X-Nexora-CSRF': '1' },
      data: { status: 'cancelled', reason: 'late write', expected_version: 4 },
    },
  )
  expect(write.status()).toBe(409)
})

test('real UI: Team switch leaves the previous project context', async ({ page }) => {
  await login(page)
  await workspace(page)
  const options = await page
    .getByLabel('当前团队', { exact: true })
    .locator('option')
    .evaluateAll((nodes) => nodes.map((n) => (n as HTMLOptionElement).value))
  const other = options.find((id) => id !== team)
  expect(other).toBeTruthy()
  await page.getByLabel('当前团队', { exact: true }).selectOption(other!)
  await expect(page).toHaveURL(new RegExp(`/teams/${other}/projects$`))
  await expect(page.getByText('ZERO_R 项目闭环', { exact: true })).toHaveCount(0)
})
