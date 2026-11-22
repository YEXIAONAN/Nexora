import { expect, test, type Page, type Response } from '@playwright/test'

const password = process.env.E2E_ROOT_PASSWORD ?? ''
const run = Date.now().toString(36)
const headers = { 'X-Nexora-CSRF': '1' }
const people = {
  owner: { username: `a2_${run}_owner`, name: 'ALPHA2 测试·团队负责人', id: '' },
  lead: { username: `a2_${run}_lead`, name: 'ALPHA2 测试·项目负责人', id: '' },
  worker: { username: `a2_${run}_worker`, name: 'ALPHA2 测试·执行成员', id: '' },
  reviewer: { username: `a2_${run}_reviewer`, name: 'ALPHA2 测试·成果审核人', id: '' },
  appraiser: { username: `a2_${run}_appraiser`, name: 'ALPHA2 测试·贡献核定人', id: '' },
  bonus: { username: `a2_${run}_bonus`, name: 'ALPHA2 测试·奖金核定人', id: '' },
  outsider: { username: `a2_${run}_outsider`, name: 'ALPHA2 测试·其他团队', id: '' },
}
let team = '',
  project = '',
  leaf = '',
  caseID = ''
const projectURL = () => `/teams/${team}/projects/${project}`
const api = () => `/api/v1${projectURL()}`
test.describe.configure({ mode: 'serial' })
test.use({ actionTimeout: 15_000 })

async function login(page: Page, username = 'root') {
  await page.context().clearCookies()
  await page.goto('/login')
  await page.locator('#login-username').fill(username)
  await page.locator('#login-password').fill(password)
  await page.locator('button[type="submit"]').click()
  await expect(page).toHaveURL(/\/teams$/)
}
async function result(response: Response, status = 200) {
  expect(response.status(), await response.text()).toBe(status)
  return (await response.json()).data
}
async function screenshot(page: Page, name: string) {
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.evaluate(() => window.scrollTo({ top: 0, behavior: 'instant' }))
  await page.screenshot({
    path: `../../.screenshots/alpha2-${name}.png`,
    fullPage: true,
    animations: 'disabled',
  })
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
}
async function createTask(
  page: Page,
  title: string,
  parent = '',
  collaborator = '',
  shortcut = false,
) {
  if (shortcut) await page.goto(`${projectURL()}?createTask=1`)
  else await page.getByRole('button', { name: '新建任务', exact: true }).click()
  await page.getByLabel('任务标题', { exact: true }).fill(title)
  await page.getByLabel('任务负责人', { exact: true }).selectOption(people.worker.id)
  await page.getByLabel('父任务', { exact: true }).selectOption(parent)
  await page.getByLabel('工作量权重', { exact: true }).fill('1')
  if (collaborator) await page.getByRole('checkbox', { name: new RegExp(collaborator) }).check()
  const response = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/tasks'),
  )
  await page.getByRole('button', { name: '创建任务', exact: true }).click()
  const task = await result(await response, 201)
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '创建任务', exact: true })).toHaveCount(0)
  return task.id as string
}

test('Alpha.2: real registration has no implicit platform or team administration', async ({
  page,
}) => {
  test.setTimeout(180_000)
  expect(password).not.toBe('')
  for (const person of Object.values(people)) {
    await page.context().clearCookies()
    await page.goto('/register')
    await page.locator('#register-username').fill(person.username)
    await page.locator('#register-display-name').fill(person.name)
    await page.locator('#register-password').fill(password)
    await page.locator('#register-confirm').fill(password)
    const registered = page.waitForResponse(
      (r) => r.request().method() === 'POST' && r.url().includes('/auth/register'),
    )
    await page.getByRole('button', { name: '创建账号', exact: true }).click()
    await result(await registered, 201)
    await login(page, person.username)
    const me = await page.request.get('/api/v1/auth/me')
    expect(me.status()).toBe(200)
    person.id = (await me.json()).data.id
    await expect(page.getByRole('button', { name: '创建团队', exact: true })).toHaveCount(0)
    expect(
      (await page.request.post('/api/v1/teams', { headers, data: { name: 'forbidden' } })).status(),
    ).toBe(403)
    expect((await page.request.get('/api/v1/admin/users')).status()).toBe(403)
  }
  await screenshot(page, 'member-empty')
})

test('Alpha.2: administrator assigns owners and teams remain isolated', async ({ page }) => {
  await login(page)
  for (const [name, owner] of [
    [`ALPHA2 隔离验收 ${run}`, people.owner],
    [`ALPHA2 其他团队 ${run}`, people.outsider],
  ] as const) {
    await page.locator('#team-name').fill(name)
    await page.locator('#team-description').fill('本次自动化验收测试数据，不用于正式团队')
    await page.locator('#team-owner').selectOption(owner.id)
    const response = page.waitForResponse(
      (r) => r.request().method() === 'POST' && r.url().endsWith('/teams'),
    )
    await page.getByRole('button', { name: '创建团队', exact: true }).click()
    const created = await result(await response, 201)
    if (owner === people.owner) team = created.id
    await expect(page.getByRole('status')).toContainText('已创建')
  }
  expect((await page.request.get(`/api/v1/teams/${team}`)).status()).toBe(404)
  await screenshot(page, 'admin-teams')
  await login(page, people.owner.username)
  expect((await page.request.get('/api/v1/admin/users')).status()).toBe(403)
  await page.goto(`/teams/${team}/members`)
  for (const person of [
    people.lead,
    people.worker,
    people.reviewer,
    people.appraiser,
    people.bonus,
  ]) {
    await page.locator('#invite-username').fill(person.username)
    const response = page.waitForResponse(
      (r) => r.request().method() === 'POST' && r.url().endsWith('/invitations'),
    )
    await page.getByRole('button', { name: '发出邀请', exact: true }).click()
    await result(await response, 201)
    await expect(page.locator('#invite-username')).toHaveValue('')
  }
  await screenshot(page, 'team-members')
  for (const person of [
    people.lead,
    people.worker,
    people.reviewer,
    people.appraiser,
    people.bonus,
  ]) {
    await login(page, person.username)
    await page.goto('/invitations')
    await page.getByRole('button', { name: '接受', exact: true }).click()
    await expect(page.getByRole('status')).toContainText('已加入团队')
  }
})

test('Alpha.2: team owner creates project, members, reviewers and three-level tasks', async ({
  page,
}) => {
  test.setTimeout(120_000)
  await login(page, people.owner.username)
  await page.goto(`/teams/${team}/projects`)
  await page.getByRole('button', { name: '新建项目', exact: true }).click()
  await page.getByLabel('项目名称', { exact: true }).fill(`ALPHA2 研发闭环 ${run}`)
  await page.getByLabel('项目描述', { exact: true }).fill('验收、贡献与人工奖金分配均独立核定')
  await page.getByLabel('项目负责人', { exact: true }).selectOption(people.lead.id)
  const response = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/projects'),
  )
  await page.getByRole('button', { name: '创建项目', exact: true }).click()
  project = (await result(await response, 201)).id
  await expect(page.getByRole('button', { name: '新建任务', exact: true })).toHaveCount(0)
  await login(page, people.lead.username)
  await page.goto(projectURL())
  await page.getByRole('button', { name: '项目成员', exact: true }).click()
  for (const person of [people.worker, people.reviewer, people.appraiser, people.bonus]) {
    await page.getByLabel('添加 Team 成员', { exact: true }).selectOption(person.id)
    await page.getByRole('button', { name: '添加成员', exact: true }).click()
    await expect(page.getByText(`@${person.username}`, { exact: true })).toBeVisible()
  }
  await page.getByLabel('授权或撤销原因', { exact: true }).fill('测试成果独立审核授权')
  await page.getByLabel('选择审核人', { exact: true }).selectOption(people.reviewer.id)
  await page.getByRole('button', { name: '授予审核权', exact: true }).click()
  await expect(
    page.getByText(`${people.reviewer.name} · 已授权审核`, { exact: true }),
  ).toBeVisible()
  await login(page, people.lead.username)
  await page.goto(projectURL())
  const top = await createTask(page, 'ALPHA2 研发计划', '', '', true)
  const middle = await createTask(page, 'ALPHA2 核心模块', top)
  leaf = await createTask(page, 'ALPHA2 交付实现', middle, people.lead.name)
  await screenshot(page, 'project-workspace')
  await page.reload()
  await expect(page.getByRole('button', { name: /ALPHA2 交付实现/ })).toBeVisible()
  const tree = await page.request.get(`${api()}/tasks`)
  expect((await tree.json()).data.tasks.map((t: { depth: number }) => t.depth).sort()).toEqual([
    1, 2, 3,
  ])
})

test('Alpha.2: member reports work, blockers and next steps without implicit acceptance', async ({
  page,
}) => {
  await login(page, people.worker.username)
  await page.goto('/my-tasks')
  await expect(page.getByText('ALPHA2 交付实现', { exact: true })).toBeVisible()
  await screenshot(page, 'my-tasks')
  await page.goto(`${projectURL()}?task=${leaf}`)
  await expect(page.getByRole('button', { name: '编辑任务', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '开始任务', exact: true }).click()
  await page.getByRole('slider', { name: '当前进度', exact: true }).fill('99')
  await page
    .getByLabel('本次工作内容', { exact: true })
    .fill('接口实现与回归测试已完成，等待独立验收')
  await page.getByLabel('已完成工作（可选）', { exact: true }).fill('完成真实 API 与 Vue 联调')
  await page.getByLabel('下一步计划（可选）', { exact: true }).fill('提交成果并安排独立审核')
  await page.getByLabel('当前问题（可选）', { exact: true }).fill('需要审核人员确认验收范围')
  await page
    .getByLabel('交付证明（可选，每行一条链接 / Commit / PR）', { exact: true })
    .fill('ALPHA2 验收记录：接口与页面联调')
  await page.getByRole('button', { name: '提交进展', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('操作已保存')
  await expect(page.getByTestId('project-progress')).toHaveText('79.00%')
  await page.reload()
  await expect(page.getByText('需要审核人员确认验收范围', { exact: true })).toBeVisible()
  await screenshot(page, 'progress-report')
  const task = (await (await page.request.get(`${api()}/tasks/${leaf}`)).json()).data
  expect(task.status).toBe('in_progress')
  expect(task.accepted_review_id).toBeNull()
  await page.getByRole('button', { name: '交付与验收', exact: true }).click()
  await page.getByLabel('成果说明', { exact: true }).fill('ALPHA2 测试成果，包含实际实现与回归证据')
  await page.getByLabel('成果附件', { exact: true }).setInputFiles({
    name: 'alpha2-evidence.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('ALPHA2 disposable acceptance evidence\n'),
  })
  await page.getByRole('button', { name: '提交新成果版本', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('80.00%')
  await login(page, people.reviewer.username)
  await page.goto(`${projectURL()}?task=${leaf}`)
  await page.getByRole('button', { name: '交付与验收', exact: true }).click()
  await page
    .getByLabel('审核意见', { exact: true })
    .fill('独立查看实际交付与附件，通过本次测试验收')
  await page.getByRole('button', { name: '通过当前版本', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('100.00%')
  await page.getByRole('button', { name: '交付与验收', exact: true }).click()
  await screenshot(page, 'submission-review')
})

test('Alpha.2: independent contribution confirmation provides bonus evidence', async ({ page }) => {
  test.setTimeout(120_000)
  await login(page, people.lead.username)
  await page.goto(`${projectURL()}/contribution-appraisal`)
  await page.getByLabel('核定授权原因', { exact: true }).fill('ALPHA2 指定独立贡献核定人')
  await page
    .getByRole('combobox', { name: '选择核定人', exact: true })
    .selectOption(people.appraiser.id)
  await page.getByRole('button', { name: '授予核定权', exact: true }).click()
  await expect(page.getByText(`${people.appraiser.name} · 有效授权`, { exact: true })).toBeVisible()
  await page.goto(`${projectURL()}/contributions`)
  await page.getByLabel('建案原因', { exact: true }).fill('基于独立验收成果建立贡献认定')
  const response = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/contributions'),
  )
  await page
    .getByTestId('contribution-candidate')
    .filter({ hasText: 'ALPHA2 交付实现' })
    .getByRole('button', { name: '创建贡献案例', exact: true })
    .click()
  caseID = (await result(await response, 201)).id
  await expect(page).toHaveURL(new RegExp(`/contributions/${caseID}$`))
  await page
    .getByLabel('草案说明', { exact: true })
    .fill('人工认定已验收成果的贡献，不按工时自动推算')
  await page.getByLabel('证据整理说明', { exact: true }).fill('读取正式成果与验收意见')
  await page.getByLabel('修改原因', { exact: true }).fill('整理贡献证据')
  await page.getByRole('button', { name: '保存草案说明', exact: true }).click()
  await page.getByRole('combobox', { name: '贡献者', exact: true }).selectOption(people.worker.id)
  await page.getByLabel('贡献份额（%）', { exact: true }).fill('100.00')
  await page.getByLabel('贡献依据', { exact: true }).fill('完成本案例全部交付内容')
  await page.getByLabel('贡献说明', { exact: true }).fill('仅此案例内的人工贡献认定')
  await page.getByRole('button', { name: '添加贡献证据引用', exact: true }).click()
  await page.getByLabel('证据来源', { exact: true }).fill('accepted_submission')
  await page.getByLabel('证据引用', { exact: true }).fill(`已验收任务 ${leaf}`)
  await page.getByLabel('份额修改原因', { exact: true }).fill('逐项证据人工评议')
  await page.getByRole('button', { name: '保存贡献份额', exact: true }).click()
  await expect(page.getByTestId('contribution-latest')).toContainText('100.00%')
  await page.getByLabel('操作原因', { exact: true }).fill('提交独立核定')
  await page.getByRole('button', { name: '提交待核定', exact: true }).click()
  await expect(page.getByTestId('contribution-latest')).toContainText('待独立核定')
  await login(page, people.appraiser.username)
  await page.goto(`${projectURL()}/contributions/${caseID}`)
  await page.getByLabel('操作原因', { exact: true }).fill('独立确认真实验收与人工贡献份额')
  await page.getByLabel('我已理解：正式核定将形成不可变核定历史', { exact: true }).check()
  await page.getByRole('button', { name: '正式核定', exact: true }).click()
  await expect(page.getByTestId('contribution-effective')).toContainText('100.00%')
  await screenshot(page, 'contribution-confirmed')
  await login(page, people.outsider.username)
  expect((await page.request.get(`${api()}/tasks/${leaf}`)).status()).toBe(404)
  expect((await page.request.get(`${api()}/contributions/${caseID}`)).status()).toBe(404)
  await page.goto(projectURL())
  await expect(page.getByRole('alert')).toBeVisible()
  await screenshot(page, 'forbidden')
})

test('Alpha.2: manual bonus allocation, independent confirmation, publication and simulated payment', async ({
  page,
}) => {
  test.setTimeout(180_000)
  await login(page, people.owner.username)
  await page.goto(`${projectURL()}/bonus-rounds`)
  await page.getByRole('button', { name: '创建奖金批次', exact: true }).click()
  await page.getByLabel('来源标识', { exact: true }).fill(`alpha2-award-${run}`)
  await page.getByLabel('批次名称', { exact: true }).fill('ALPHA2 独立奖金验收')
  await page
    .getByLabel('真实资金来源', { exact: true })
    .fill('隔离测试资金，不代表真实支付或正式团队资金')
  await page.getByRole('combobox', { name: '金额负责人', exact: true }).selectOption(people.lead.id)
  await page
    .getByRole('combobox', { name: '独立核定人', exact: true })
    .selectOption(people.bonus.id)
  await page.getByLabel('创建理由', { exact: true }).fill('独立真实来源语义的人工方案验收')
  const created = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/bonus-rounds'),
  )
  await page.getByRole('button', { name: '创建批次', exact: true }).click()
  const round = (await result(await created, 201)).round_id
  const roundURL = `${projectURL()}/bonus-rounds/${round}`
  const roundAPI = `${api()}/bonus-rounds/${round}`
  await page.getByRole('link', { name: '进入新批次与授权管理 →', exact: true }).click()
  await expect(page.getByText(/你尚未获得此批次的金额访问授权/)).toBeVisible()
  expect((await page.request.get(`${roundAPI}/workspace`)).status()).toBe(404)

  await login(page, people.lead.username)
  await page.goto(roundURL)
  await page.getByRole('button', { name: '编辑人工方案', exact: true }).click()
  await page.getByLabel('方案总预算（元）', { exact: true }).fill('10.00')
  const sources = page.getByRole('combobox', { name: '添加已确认贡献来源', exact: true })
  await sources.selectOption({ index: 1 })
  await page.getByRole('button', { name: '添加来源', exact: true }).click()
  await page.getByLabel('来源预算（元）', { exact: true }).fill('9.00')
  await page.getByLabel('来源预算理由', { exact: true }).fill('人工批准该验收贡献的来源预算')
  await page.getByRole('checkbox', { name: '具备奖金资格', exact: true }).check()
  await page.getByLabel('人工金额（元）', { exact: true }).fill('9.01')
  await expect(page.getByRole('alert')).toContainText('成员金额超出来源预算')
  await page.getByLabel('人工金额（元）', { exact: true }).fill('8.00')
  await page
    .getByLabel('资格与金额理由', { exact: true })
    .fill('有效成员，人工确认资格及金额，不采用自动公式')
  await page.getByLabel('本次保存理由', { exact: true }).fill('保留来源余额1元与方案余额1元')
  await screenshot(page, 'bonus-plan-preview')
  await page.getByRole('button', { name: '保存人工方案', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '人工方案保存成功' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('button', { name: '编辑人工方案', exact: true })).toBeVisible()
  const staleRevision = (await (await page.request.get(roundAPI)).json()).data.revision

  async function transition(label: string, reason: string) {
    await page.getByRole('button', { name: label, exact: true }).click()
    await page.getByLabel('操作理由', { exact: true }).fill(reason)
    await page
      .getByRole('checkbox', { name: '我已核对当前方案，了解上述操作影响', exact: true })
      .check()
    await page.getByRole('button', { name: `确认${label}`, exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: `${label}成功` })).toBeVisible()
  }
  await transition('提交独立核定', '提交人工资格、预算与明细供独立核定')
  expect(
    (
      await page.request.post(`${roundAPI}/submit`, {
        headers,
        data: {
          expected_revision: staleRevision,
          client_request_id: crypto.randomUUID(),
          reason: '过期版本必须拒绝',
        },
      })
    ).status(),
  ).toBe(409)
  await expect(page.getByRole('button', { name: '独立核定', exact: true })).toHaveCount(0)
  await login(page, people.bonus.username)
  await page.goto(roundURL)
  await transition('退回修改', '补充检查后可重新提交，不覆盖历史')
  await login(page, people.lead.username)
  await page.goto(roundURL)
  await transition('提交独立核定', '已复核来源与资格理由，再次提交')
  await login(page, people.bonus.username)
  await page.goto(roundURL)
  await transition('独立核定', '核对正式贡献、资格与人工金额，本人无任何利益冲突')
  await screenshot(page, 'bonus-independent-confirmation')

  await login(page, people.worker.username)
  expect((await page.request.get(`${roundAPI}/workspace`)).status()).toBe(404)
  const unpublished = await page.request.get(`/api/v1/teams/${team}/bonus-results/mine`)
  expect((await unpublished.json()).data.items).toEqual([])
  await page.goto(roundURL)
  await expect(page.getByText(/你尚未获得此批次的金额访问授权/)).toBeVisible()
  await login(page, people.lead.username)
  await page.goto(roundURL)
  await transition('发布正式结果', '发布已独立核定的本人可见结果，不执行转账')
  await page
    .getByRole('button', { name: `记录 ${people.worker.name} 的模拟发放`, exact: true })
    .click()
  await page.getByLabel('模拟发放参考号', { exact: true }).fill(`SIMULATION-${run}`)
  await page.getByLabel('记录理由', { exact: true }).fill('隔离环境验证模拟记录，非银行支付')
  await page
    .getByRole('checkbox', { name: '我确认这仅是模拟发放记录，不执行实际转账', exact: true })
    .check()
  await page.getByRole('button', { name: '确认记录模拟发放', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '模拟发放记录成功' })).toBeVisible()
  await expect(page.getByRole('button', { name: '新建修订版本', exact: true })).toHaveCount(0)
  await screenshot(page, 'bonus-payment-history')

  await login(page, people.worker.username)
  await page.goto(`/teams/${team}/bonus-results`)
  await expect(page.getByText('ALPHA2 独立奖金验收', { exact: true })).toBeVisible()
  await expect(page.getByText('¥8.00', { exact: true })).toBeVisible()
  await expect(page.getByText('已发布 · 已记录模拟发放', { exact: true })).toBeVisible()
  await screenshot(page, 'my-bonus')
  await page.setViewportSize({ width: 390, height: 844 })
  const navigation = page.getByRole('complementary', { name: '主导航', exact: true })
  await expect(navigation).not.toBeInViewport()
  await page.getByRole('button', { name: '打开导航', exact: true }).click()
  await expect(navigation).toBeInViewport()
  await page.getByRole('button', { name: '关闭导航', exact: true }).click()
  await expect(navigation).not.toBeInViewport()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({
    path: '../../.screenshots/alpha2-my-bonus-mobile.png',
    fullPage: true,
    animations: 'disabled',
  })
  await login(page)
  expect((await page.request.get(`${roundAPI}/workspace`)).status()).toBe(404)
  await login(page, people.outsider.username)
  expect((await page.request.get(roundAPI)).status()).toBe(404)
})
