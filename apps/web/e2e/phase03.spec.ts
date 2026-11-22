import { expect, test, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'

const run = randomUUID().slice(0, 8)
const memberUsername = `phase03_member_${run}`
const teamName = `Phase03 ZERO_R ${run}`

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
async function saveAction(page: Page, name: string) {
  await page.getByRole('button', { name, exact: true }).click()
  // A successful response is followed by a workspace reload and remounted task detail.
  await expect(page.getByRole('status')).toContainText('操作已保存，工作区已更新。')
  await expect(page.getByRole('button', { name: '刷新工作区', exact: true })).toBeEnabled()
}
async function taskTab(page: Page, name: string) {
  await page
    .getByRole('navigation', { name: '任务详情导航', exact: true })
    .getByRole('button', { name, exact: true })
    .click()
}
async function reportProgress(page: Page, progress: number, summary: string) {
  await taskTab(page, '工作进展')
  const slider = page.getByRole('slider', { name: '当前进度', exact: true })
  await slider.focus()
  await slider.press('Home')
  for (let value = 0; value < progress; value++) await slider.press('ArrowRight')
  await expect(slider).toHaveValue(String(progress))
  await page.getByLabel('本次工作内容', { exact: true }).fill(summary)
  await saveAction(page, '提交进展')
  await expect(page.getByRole('region', { name: '工作进展', exact: true })).toContainText(summary)
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
  await saveAction(page, '创建任务')
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
  await page.fill('#team-name', teamName)
  await page.fill('#team-description', '项目与任务验收工作区')
  await page.locator('form').first().getByRole('button', { name: '创建团队', exact: true }).click()
  const row = page.locator('li[data-team-id]', { hasText: teamName })
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
    data: { username: memberUsername, display_name: 'Lin', password },
  })
  expect(account.status()).toBe(201)
  member = (await account.json()).data.id
  const invitation = await page.request.post(`/api/v1/teams/${team}/invitations`, {
    headers: { 'X-Nexora-CSRF': '1' },
    data: { username: memberUsername, role: 'member' },
  })
  expect(invitation.status()).toBe(201)
  const invitationID = (await invitation.json()).data.id
  await logout(page)
  await login(page, memberUsername)
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
  await expect(page.getByText(`@${memberUsername}`, { exact: true })).toBeVisible()
})

test('real UI: create three levels and assign leaf owner and collaborator', async ({ page }) => {
  await login(page)
  await workspace(page)
  top = await createTask(page, '研发规划', '', 100, root)
  middle = await createTask(page, '后端服务', top, 50, root)
  leaf = await createTask(page, '接口实现', middle, 3, member)
  await selectTask(page, '接口实现')
  await page.getByRole('button', { name: '编辑任务', exact: true }).click()
  await page.getByRole('checkbox', { name: /@root$/, exact: true }).check()
  await saveAction(page, '保存任务')
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
  await login(page, memberUsername)
  await workspace(page)
  await expect(page.getByRole('button', { name: /研发规划/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /后端服务/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '新建任务', exact: true })).toHaveCount(0)
  await selectTask(page, '接口实现')
  await saveAction(page, '开始任务')
  await reportProgress(page, 60, '完成接口主体实现，继续补齐回归验证')
  await expect(page.getByTestId('project-progress')).toHaveText('60.00%')
  await expect(page.getByRole('button', { name: '编辑任务', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '移除', exact: true })).toHaveCount(0)
})

test('real UI: an executing task group accepts new children and moved leaves', async ({ page }) => {
  await login(page)
  await workspace(page)
  await createTask(page, '增量子任务', middle, 2, root)
  await selectTask(page, '增量子任务')
  await taskTab(page, '设置')
  await page.getByLabel('取消原因', { exact: true }).fill('取消增量计划，保留验证历史')
  await saveAction(page, '取消任务')
  await expect(page.getByTestId('project-progress')).toHaveText('60.00%')
  await createTask(page, '待移入计划', '', 2, root)
  await selectTask(page, '待移入计划')
  await taskTab(page, '设置')
  await page.getByLabel('移动到父任务', { exact: true }).selectOption(middle)
  await saveAction(page, '移动子树')
  await expect(page.getByRole('heading', { name: '待移入计划', exact: true })).toBeVisible()
  const tree = await page.request.get(`/api/v1/teams/${team}/projects/${project}/tasks`)
  const moved = (await tree.json()).data.tasks.find(
    (t: { title: string }) => t.title === '待移入计划',
  )
  expect(moved.parent_id).toBe(middle)
  expect(moved.depth).toBe(3)
  await taskTab(page, '设置')
  await page.getByLabel('取消原因', { exact: true }).fill('结束结构验证')
  await saveAction(page, '取消任务')
  await expect(page.getByTestId('project-progress')).toHaveText('60.00%')
})

test('real UI: 99% execution caps aggregation at 79% and dependencies block', async ({ page }) => {
  await login(page)
  await workspace(page)
  await selectTask(page, '接口实现')
  await reportProgress(page, 99, '接口实现与回归已完成，等待正式交付验收')
  await expect(page.getByTestId('project-progress')).toHaveText('79.00%')
  successor = await createTask(page, '集成验证', '', 1, root)
  await selectTask(page, '集成验证')
  await taskTab(page, '关联')
  await page.getByLabel('添加前置任务').selectOption(leaf)
  await saveAction(page, '添加依赖')
  await taskTab(page, '关联')
  await expect(page.getByText('依赖阻塞', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '开始任务', exact: true })).toHaveCount(0)
  await expect(page.getByTestId('project-progress')).toHaveText('59.25%')
  await page.getByLabel('忽略原因', { exact: true }).fill('负责人批准并行验证')
  await saveAction(page, '忽略')
  await taskTab(page, '关联')
  await expect(page.getByText('已忽略', { exact: true })).toBeVisible()
  await expect(page.getByText(/负责人批准并行验证/)).toBeVisible()
  await expect(page.getByRole('button', { name: '开始任务', exact: true })).toBeVisible()
  await saveAction(page, '开始任务')
  await expect(page.getByRole('slider', { name: '当前进度', exact: true })).toBeVisible()
  await page.screenshot({
    path: '../../.cache/phase03-workspace-desktop.png',
    fullPage: true,
    animations: 'disabled',
  })
  await page.getByRole('button', { name: '主题：跟随系统', exact: true }).click()
  await page.getByRole('button', { name: '主题：浅色', exact: true }).click()
  await page.screenshot({
    path: '../../.cache/phase03-workspace-dark.png',
    fullPage: true,
    animations: 'disabled',
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('complementary', { name: '主导航' })).not.toBeInViewport()
  await page.screenshot({
    path: '../../.cache/phase03-workspace-mobile.png',
    fullPage: true,
    animations: 'disabled',
  })
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  )
  expect(overflow).toBe(false)
})

test('real UI: an inactive collaborator can be explicitly removed and the task saved', async ({
  page,
}) => {
  await login(page)
  await workspace(page)
  await selectTask(page, '接口实现')
  await page.getByRole('button', { name: '编辑任务', exact: true }).click()
  await page.getByLabel('任务负责人', { exact: true }).selectOption(root)
  await page.getByRole('checkbox', { name: `Lin @${memberUsername}`, exact: true }).check()
  const saved = page.waitForResponse(
    (r) => r.request().method() === 'PUT' && r.url().endsWith(`/tasks/${leaf}`),
  )
  await saveAction(page, '保存任务')
  expect((await saved).status()).toBe(200)
  await expect(page.getByRole('button', { name: '保存任务', exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '接口实现', exact: true })).toBeVisible()
  await page.goto(`/teams/${team}/members`)
  await page
    .locator(`li[data-user-id="${member}"]`)
    .getByRole('button', { name: '移除', exact: true })
    .click()
  await expect(page.getByRole('status')).toContainText(`已移除成员 ${memberUsername}`)
  await workspace(page)
  await selectTask(page, '接口实现')
  await page.getByRole('button', { name: '编辑任务', exact: true }).click()
  await page.getByRole('button', { name: '移除失效协作者 Lin', exact: true }).click()
  await page.getByLabel('任务描述', { exact: true }).fill('成员离队后显式修复任务分配')
  await saveAction(page, '保存任务')
  await expect(page.getByText('成员离队后显式修复任务分配', { exact: true })).toBeVisible()
  const response = await page.request.get(`/api/v1/teams/${team}/projects/${project}/tasks/${leaf}`)
  expect((await response.json()).data.collaborator_ids).toEqual([])
})

test('real UI: weight history, milestone plan, edit project and archive read-only', async ({
  page,
}) => {
  await login(page)
  await workspace(page)
  await selectTask(page, '接口实现')
  await taskTab(page, '设置')
  await page.getByLabel('新权重', { exact: true }).fill('1')
  await page.getByLabel('权重修改原因', { exact: true }).fill('重新估算工作量')
  await saveAction(page, '变更权重并保留快照')
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
  await expect(page.getByRole('button', { name: '提交进展', exact: true })).toHaveCount(0)
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
