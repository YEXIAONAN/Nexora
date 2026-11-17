import { expect, test, type Page } from '@playwright/test'
import { readFile } from 'node:fs/promises'
const password = process.env.E2E_ROOT_PASSWORD ?? ''
const headers = { 'X-Nexora-CSRF': '1' }
let team = '',
  project = '',
  root = '',
  reviewer = '',
  task = '',
  v1 = '',
  v2 = ''
const path = () => `/api/v1/teams/${team}/projects/${project}/tasks/${task}`
const url = () => `/teams/${team}/projects/${project}`
test.describe.configure({ mode: 'serial' })
async function login(page: Page, user = 'root') {
  await page.goto('/login')
  await page.fill('#login-username', user)
  await page.fill('#login-password', password)
  await page.locator('button[type="submit"]').click()
  await expect(page).toHaveURL(/\/teams$/)
}
async function logout(page: Page) {
  await page.getByRole('button', { name: '退出登录', exact: true }).click()
  await expect(page).toHaveURL(/\/login/)
}
async function workspace(page: Page) {
  await page.goto(url())
  await expect(
    page.getByRole('heading', { name: 'Phase04 真实成果验收', exact: true }),
  ).toBeVisible()
  await expect(page.getByRole('heading', { name: '成果与验收', exact: true })).toBeVisible()
}
test('real UI creates Team, project and task with an independent project member', async ({
  page,
}) => {
  await login(page)
  root = (await (await page.request.get('/api/v1/auth/me')).json()).data.id
  await page.fill('#team-name', 'Phase04 ZERO_R')
  await page.fill('#team-description', 'Immutable evidence and independent review')
  await page.locator('form').first().getByRole('button', { name: '创建团队', exact: true }).click()
  const row = page.locator('li[data-team-id]', { hasText: 'Phase04 ZERO_R' })
  await expect(row).toBeVisible()
  team = (await row.getAttribute('data-team-id')) ?? ''
  await row.getByRole('button', { name: '项目', exact: true }).click()
  await page.getByRole('button', { name: '新建项目', exact: true }).click()
  await page.getByLabel('项目名称', { exact: true }).fill('Phase04 真实成果验收')
  await page.getByLabel('项目负责人', { exact: true }).selectOption(root)
  await page.getByRole('button', { name: '创建项目', exact: true }).click()
  await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+$/)
  project = page.url().split('/').at(-1) ?? ''
  const account = await page.request.post('/api/v1/admin/users', {
    headers,
    data: { username: 'phase04_reviewer', display_name: '独立审核人', password },
  })
  expect(account.status()).toBe(201)
  reviewer = (await account.json()).data.id
  const invitation = await page.request.post(`/api/v1/teams/${team}/invitations`, {
    headers,
    data: { username: 'phase04_reviewer', role: 'member' },
  })
  expect(invitation.status()).toBe(201)
  const invite = (await invitation.json()).data.id
  await logout(page)
  await login(page, 'phase04_reviewer')
  expect(
    (await page.request.post(`/api/v1/invitations/${invite}/accept`, { headers })).status(),
  ).toBe(200)
  await logout(page)
  await login(page)
  await page.goto(url())
  await expect(
    page.getByRole('heading', { name: 'Phase04 真实成果验收', exact: true }),
  ).toBeVisible()
  await page.getByRole('button', { name: '项目成员', exact: true }).click()
  await page.getByLabel('添加 Team 成员', { exact: true }).selectOption(reviewer)
  await page.getByRole('button', { name: '添加成员', exact: true }).click()
  await expect(page.getByText('@phase04_reviewer', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '任务树', exact: true }).click()
  await page.getByRole('button', { name: '新建任务', exact: true }).click()
  await page.getByLabel('任务标题', { exact: true }).fill('交付可验证成果')
  await page.getByLabel('工作量权重', { exact: true }).fill('3')
  await page.getByLabel('任务负责人', { exact: true }).selectOption(root)
  const created = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/tasks'),
  )
  await page.getByRole('button', { name: '保存新任务', exact: true }).click()
  const result = await created
  expect(result.status()).toBe(201)
  task = (await result.json()).data.id
  await page.getByRole('button', { name: '开始任务', exact: true }).click()
  await expect(page.getByLabel('成果说明', { exact: true })).toBeVisible()
  await expect(page.getByTestId('project-progress')).toHaveText('0.00%')
})
test('real UI submits V1 with a reference and private attachment, remains at 80%', async ({
  page,
}) => {
  await login(page)
  await workspace(page)
  await page.getByLabel('成果说明', { exact: true }).fill('V1 实现与验证说明')
  await page.getByRole('button', { name: '添加证据链接', exact: true }).click()
  await page.getByLabel('证据类型', { exact: true }).selectOption('pull_request')
  await page
    .getByLabel('证据地址', { exact: true })
    .fill('https://github.com/YEXIAONAN/Nexora/pull/3')
  await page.getByLabel('证据标签', { exact: true }).fill('参考实现')
  await page.getByLabel('成果附件', { exact: true }).setInputFiles({
    name: 'evidence-v1.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('V1 actual evidence\n'),
  })
  const submitted = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/submissions'),
  )
  await page.getByRole('button', { name: '提交新成果版本', exact: true }).click()
  const result = await submitted
  expect(result.status()).toBe(201)
  v1 = (await result.json()).data.id
  await expect(page.getByTestId('project-progress')).toHaveText('80.00%')
  await expect(page.getByTestId('submission-detail')).toContainText('V1 实现与验证说明')
  await expect(page.getByLabel('审核意见', { exact: true })).toHaveCount(0)
  const current = (await (await page.request.get(path())).json()).data
  expect(
    (
      await page.request.post(`${path()}/submissions/${v1}/reviews`, {
        headers,
        data: { expected_version: current.version, decision: 'approve', comment: 'Self approval' },
      })
    ).status(),
  ).toBe(403)
  const downloaded = page.waitForEvent('download')
  await page.getByRole('link', { name: '下载 evidence-v1.txt', exact: true }).click()
  const file = await downloaded
  expect(file.suggestedFilename()).toBe('evidence-v1.txt')
  expect(await readFile((await file.path())!, 'utf8')).toBe('V1 actual evidence\n')
})
test('explicit reviewer grant unlocks the real inbox and V1 rejection', async ({ page }) => {
  await login(page)
  await workspace(page)
  await page.getByRole('button', { name: '项目成员', exact: true }).click()
  await page.getByLabel('授权或撤销原因', { exact: true }).fill('独立验收项目成果')
  await page.getByLabel('选择审核人', { exact: true }).selectOption(reviewer)
  await page.getByRole('button', { name: '授予审核权', exact: true }).click()
  await expect(page.getByText('独立审核人 · 已授权审核', { exact: true })).toBeVisible()
  await logout(page)
  await login(page, 'phase04_reviewer')
  await page.goto(`/teams/${team}/projects`)
  await page
    .getByRole('region', { name: '待我审核' })
    .getByRole('link', { name: /交付可验证成果/ })
    .click()
  await expect(page.getByTestId('submission-detail')).toContainText('V1 实现与验证说明')
  await page.getByLabel('审核意见', { exact: true }).fill('V1 缺少回归验证，请补正')
  await page.getByRole('button', { name: '驳回当前版本', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('0.00%')
  await expect(page.getByTestId('review-decision')).toContainText('审核驳回')
  await expect(page.getByTestId('review-decision')).toContainText('V1 缺少回归验证，请补正')
})
test('author corrects and submits V2 while V1 history remains immutable', async ({ page }) => {
  await login(page)
  await workspace(page)
  await page.getByLabel('补正原因', { exact: true }).fill('补充全部回归证据')
  await page.getByRole('button', { name: '进入补正', exact: true }).click()
  await expect(page.getByLabel('成果说明', { exact: true })).toBeVisible()
  await page.getByLabel('成果说明', { exact: true }).fill('V2 已补充完整回归验证')
  await page.getByLabel('成果附件', { exact: true }).setInputFiles({
    name: 'evidence-v2.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('V2 independently verified evidence\n'),
  })
  const submitted = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/submissions'),
  )
  await page.getByRole('button', { name: '提交新成果版本', exact: true }).click()
  const result = await submitted
  expect(result.status()).toBe(201)
  const version = (await result.json()).data
  v2 = version.id
  expect(version.version).toBe(2)
  await expect(page.getByTestId('project-progress')).toHaveText('80.00%')
  await page.getByRole('button', { name: 'V1', exact: true }).click()
  await expect(page.getByTestId('review-decision')).toContainText('V1 缺少回归验证，请补正')
  await expect(page.getByTestId('submission-detail')).toContainText('V1 实现与验证说明')
  await page.getByRole('button', { name: 'V2 · 当前版本', exact: true }).click()
  await expect(page.getByTestId('submission-detail')).toContainText('V2 已补充完整回归验证')
})
test('independent reviewer approves only V2 and the project reaches 100%', async ({ page }) => {
  await login(page, 'phase04_reviewer')
  await page.goto(`/teams/${team}/projects`)
  await page
    .getByRole('region', { name: '待我审核' })
    .getByRole('link', { name: /交付可验证成果/ })
    .click()
  await expect(page.getByTestId('submission-detail')).toContainText('V2 已补充完整回归验证')
  const current = (await (await page.request.get(path())).json()).data
  expect(
    (
      await page.request.post(`${path()}/submissions/${v1}/reviews`, {
        headers,
        data: { expected_version: current.version, decision: 'approve', comment: 'Stale V1' },
      })
    ).status(),
  ).toBe(409)
  await page.getByLabel('审核意见', { exact: true }).fill('已独立验证 V2 成果和回归证据')
  await page.getByRole('button', { name: '通过当前版本', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('100.00%')
  await expect(page.getByTestId('review-decision')).toContainText('审核通过')
  await expect(page.getByTestId('review-decision')).toContainText('独立审核人')
  await expect(page.getByLabel('成果说明', { exact: true })).toHaveCount(0)
  const done = (await (await page.request.get(path())).json()).data
  expect(done.status).toBe('completed')
  expect(done.current_submission_version_id).toBe(v2)
  expect(done.accepted_review_id).toBeTruthy()
  expect(done.effective_progress_bps).toBe(10000)
  await page.screenshot({
    path: '../../.cache/phase04-completed.png',
    fullPage: true,
    animations: 'disabled',
  })
  await page.emulateMedia({ colorScheme: 'dark' })
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.screenshot({
    path: '../../.cache/phase04-dark.png',
    fullPage: true,
    animations: 'disabled',
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('complementary', { name: '主导航' })).not.toBeInViewport()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(
    false,
  )
  await expect(page.getByTestId('project-progress')).toHaveText('100.00%')
  await page.screenshot({
    path: '../../.cache/phase04-mobile.png',
    fullPage: true,
    animations: 'disabled',
  })
})
