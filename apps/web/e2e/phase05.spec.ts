import { expect, test, type APIResponse, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import type { components } from '../src/api/schema'

// Every request uses the real isolated API and browser login session. There are
// no route handlers, fake permissions, database inserts or frontend mocks.
type S = components['schemas']
const password = process.env.E2E_ROOT_PASSWORD ?? ''
const headers = { 'X-Nexora-CSRF': '1' }
const people = [
  { username: 'phase05_worker', name: '成果贡献者', id: '', invitation: '' },
  { username: 'phase05_reviewer', name: '成果验收人', id: '', invitation: '' },
  { username: 'phase05_appraiser', name: '独立核定人', id: '', invitation: '' },
  { username: 'phase05_arbiter', name: '独立裁定人', id: '', invitation: '' },
]
let team = '',
  project = '',
  task = '',
  caseID = '',
  confirmation = '',
  firstVersion = ''
const base = () => `/api/v1/teams/${team}/projects/${project}`
const projectURL = () => `/teams/${team}/projects/${project}`
const casePath = () => `${base()}/contributions/${caseID}`
const caseURL = () => `${projectURL()}/contributions/${caseID}`

test.describe.configure({ mode: 'serial' })

async function data<T>(
  response: Pick<APIResponse, 'status' | 'text' | 'json'>,
  status = 200,
): Promise<T> {
  expect(response.status(), await response.text()).toBe(status)
  return (await response.json()).data as T
}
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
async function current(page: Page) {
  return data<S['ContributionCaseDetail']>(await page.request.get(casePath()))
}
async function command(page: Page, reason: string) {
  const d = await current(page)
  return {
    expected_version: d.case.revision,
    case_version_id: d.latest.id,
    client_request_id: randomUUID(),
    reason,
  }
}
async function manualShare(page: Page) {
  await page.getByRole('combobox', { name: '贡献者', exact: true }).selectOption(people[0]!.id)
  await page.getByLabel('贡献份额（%）', { exact: true }).fill('100.00')
  await page.getByLabel('贡献依据', { exact: true }).fill('人工评价实际成果及证据')
  await page.getByLabel('贡献说明', { exact: true }).fill('完成全部实际成果，并提供验收证据')
  await page.getByRole('button', { name: '添加贡献证据引用', exact: true }).click()
  await page.getByLabel('证据来源', { exact: true }).fill('accepted_submission')
  await page.getByLabel('证据引用', { exact: true }).fill(`正式验收任务 ${task} 的成果附件`)
  await page.getByLabel('份额修改原因', { exact: true }).fill('人工填写 10000 整数基点')
  const share = page.waitForResponse(
    (r) => r.request().method() === 'PUT' && r.url().endsWith(`/shares/${people[0]!.id}`),
  )
  await page.getByRole('button', { name: '保存贡献份额', exact: true }).click()
  const shared = await share
  expect(shared.status()).toBe(200)
  expect(shared.request().postDataJSON().share_bps).toBe(10000)
  await expect(page.getByTestId('contribution-latest')).toContainText('100.00%')
}
async function mobile(page: Page) {
  await page.setViewportSize({ width: 390, height: 844 })
  await expect
    .poll(() =>
      page
        .getByRole('complementary', { name: '主导航' })
        .evaluate((element) => Math.ceil(element.getBoundingClientRect().right)),
    )
    .toBeLessThanOrEqual(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(
    false,
  )
}
async function submit(page: Page, reason: string) {
  await page.getByLabel('操作原因', { exact: true }).fill(reason)
  const response = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith(`/${caseID}/submit`),
  )
  await page.getByRole('button', { name: '提交待核定', exact: true }).click()
  expect((await response).status()).toBe(200)
  await expect(page.getByTestId('contribution-latest')).toContainText('待独立核定')
}
async function confirm(page: Page) {
  await page.getByLabel('操作原因', { exact: true }).fill('独立核对验收、证据与人工贡献份额')
  await page.getByLabel('我已理解：正式核定将形成不可变核定历史', { exact: true }).check()
  const response = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith(`/${caseID}/confirm`),
  )
  await page.getByRole('button', { name: '正式核定', exact: true }).click()
  const result = await data<S['ContributionCommandResult']>(await response)
  await expect(page.getByTestId('contribution-effective')).toContainText('100.00%')
  return result.id
}

test('real acceptance creates the immutable source for contribution accounting', async ({
  page,
}) => {
  test.setTimeout(120_000)
  expect(password).not.toBe('')
  await login(page)
  const root = await data<S['User']>(await page.request.get('/api/v1/auth/me'))
  const createdTeam = await data<S['Team']>(
    await page.request.post('/api/v1/teams', {
      headers,
      data: { name: 'Phase05 真实贡献核算', description: '隔离真实业务 E2E' },
    }),
    201,
  )
  team = createdTeam.id
  for (const person of people) {
    const account = await data<S['User']>(
      await page.request.post('/api/v1/admin/users', {
        headers,
        data: { username: person.username, display_name: person.name, password },
      }),
      201,
    )
    person.id = account.id
    const invitation = await data<S['TeamInvitation']>(
      await page.request.post(`/api/v1/teams/${team}/invitations`, {
        headers,
        data: { username: person.username, role: 'member' },
      }),
      201,
    )
    person.invitation = invitation.id
  }
  for (const person of people) {
    await logout(page)
    await login(page, person.username)
    expect(
      (
        await page.request.post(`/api/v1/invitations/${person.invitation}/accept`, { headers })
      ).status(),
    ).toBe(200)
  }
  await logout(page)
  await login(page)
  const createdProject = await data<S['Project']>(
    await page.request.post(`/api/v1/teams/${team}/projects`, {
      headers,
      data: { name: '贡献治理真实闭环', lead_id: root.id },
    }),
    201,
  )
  project = createdProject.id
  for (const person of people) {
    expect(
      (
        await page.request.post(`${base()}/members`, {
          headers,
          data: { user_id: person.id },
        })
      ).status(),
    ).toBe(201)
  }
  expect(
    (
      await page.request.post(`${base()}/reviewers`, {
        headers,
        data: { user_id: people[1]!.id, reason: '独立验收成果' },
      })
    ).status(),
  ).toBe(201)
  const createdTask = await data<S['Task']>(
    await page.request.post(`${base()}/tasks`, {
      headers,
      data: { title: '可追溯验收成果', owner_id: people[0]!.id, weight: 1 },
    }),
    201,
  )
  task = createdTask.id
  await logout(page)
  await login(page, people[0]!.username)
  await page.goto(projectURL())
  await page.getByRole('button', { name: '开始任务', exact: true }).click()
  await page.getByLabel('成果说明', { exact: true }).fill('正式验收的实现与证据，不自动计算贡献')
  await page.getByLabel('成果附件', { exact: true }).setInputFiles({
    name: 'contribution-evidence.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('Actual contribution source evidence\n'),
  })
  await page.getByRole('button', { name: '提交新成果版本', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('80.00%')
  await logout(page)
  await login(page, people[1]!.username)
  await page.goto(projectURL())
  await page.getByLabel('审核意见', { exact: true }).fill('独立核验成果和实际附件')
  await page.getByRole('button', { name: '通过当前版本', exact: true }).click()
  await expect(page.getByTestId('project-progress')).toHaveText('100.00%')
  const accepted = await data<S['Task']>(await page.request.get(`${base()}/tasks/${task}`))
  expect(accepted.status).toBe('completed')
  expect(accepted.accepted_review_id).toBeTruthy()
})

test('lead manually drafts 10000 bps while server and UI reject unauthorized appraisal', async ({
  page,
}) => {
  await login(page)
  await page.setViewportSize({ width: 1440, height: 960 })
  await page.goto(projectURL())
  await page.getByRole('link', { name: '贡献核算', exact: true }).click()
  const candidate = page.getByTestId('contribution-candidate').filter({ hasText: '可追溯验收成果' })
  await page.getByLabel('建案原因', { exact: true }).fill('基于真实正式验收建立贡献案例')
  await page.screenshot({ path: '../../.cache/phase05-m4-workspace-desktop.png', fullPage: true })
  const create = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/contributions'),
  )
  await candidate.getByRole('button', { name: '创建贡献案例', exact: true }).click()
  const result = await data<S['ContributionCommandResult']>(await create, 201)
  caseID = result.id
  await expect(page).toHaveURL(new RegExp(`/contributions/${caseID}$`))
  await expect(page.getByTestId('contribution-effective')).toContainText('尚无正式有效版本')
  await page.getByLabel('草案说明', { exact: true }).fill('人工逐案核对后形成的贡献提案')
  await page.getByLabel('证据整理说明', { exact: true }).fill('正式成果及验收附件，人工阅读核对')
  await page.getByLabel('修改原因', { exact: true }).fill('整理真实验收证据')
  await page.getByRole('button', { name: '保存草案说明', exact: true }).click()
  await manualShare(page)
  const draft = await current(page)
  expect(draft.effective).toBeNull()
  expect(draft.case.undisputed_confirmed).toBe(false)
  await submit(page, '份额精确合计 10000，交由独立人员核定')
  await expect(page.getByRole('button', { name: '正式核定', exact: true })).toHaveCount(0)
  expect(
    (
      await page.request.post(`${casePath()}/confirm`, {
        headers,
        data: await command(page, '提案人尝试自行核定必须被服务端拒绝'),
      })
    ).status(),
  ).toBe(403)
  await logout(page)
  await login(page, people[0]!.username)
  await page.goto(caseURL())
  await expect(page.getByRole('button', { name: '正式核定', exact: true })).toHaveCount(0)
  expect(
    (
      await page.request.post(`${casePath()}/confirm`, {
        headers,
        data: await command(page, '受益人不得核定自己的贡献'),
      })
    ).status(),
  ).toBe(403)
})

test('independent appraisal is immutable and only an explicit personal POST records receipt', async ({
  page,
}) => {
  await login(page)
  await page.goto(`${projectURL()}/contribution-appraisal`)
  for (const person of people.slice(2)) {
    await page.getByLabel('核定授权原因', { exact: true }).fill('指定独立核定与争议裁定人员')
    await page.getByRole('combobox', { name: '选择核定人', exact: true }).selectOption(person.id)
    await page.getByRole('button', { name: '授予核定权', exact: true }).click()
    await expect(page.getByText(`${person.name} · 有效授权`, { exact: true })).toBeVisible()
  }
  await logout(page)
  await login(page, people[2]!.username)
  await page.goto(`${projectURL()}/contribution-appraisal`)
  await page.getByRole('link', { name: new RegExp(caseID) }).click()
  await expect(page.getByText('contribution-evidence.txt', { exact: false }).first()).toBeVisible()
  const evidence = await data<S['ContributionEvidence']>(
    await page.request.get(`${casePath()}/evidence`),
  )
  const attachmentPath = `${base()}/tasks/${evidence.task_id}/submissions/${evidence.submission_version_id}/attachments/${evidence.attachments[0]!.id}`
  // M3 exposes authenticated evidence metadata, while original Phase04 file
  // downloads retain task permissions. Appraiser authorization does not grant
  // ordinary task access; M4 must not invent a download bypass.
  expect((await page.request.get(attachmentPath)).status()).toBe(404)
  await logout(page)
  await login(page)
  await page.goto(projectURL())
  await page.getByRole('button', { name: '项目成员', exact: true }).click()
  await page
    .getByLabel('授权或撤销原因', { exact: true })
    .fill('明确授予既有成果读取权限，使独立核定人可核对实际附件')
  await page.getByLabel('选择审核人', { exact: true }).selectOption(people[2]!.id)
  await page.getByRole('button', { name: '授予审核权', exact: true }).click()
  await expect(page.getByText('独立核定人 · 已授权审核', { exact: true })).toBeVisible()
  await logout(page)
  await login(page, people[2]!.username)
  await page.goto(caseURL())
  const file = await page.request.get(attachmentPath)
  expect(file.status()).toBe(200)
  expect(await file.text()).toBe('Actual contribution source evidence\n')
  expect(evidence.reviewer_id).toBe(people[1]!.id)
  expect((await current(page)).permissions.can_confirm).toBe(true)
  confirmation = await confirm(page)
  const official = await current(page)
  firstVersion = official.effective!.id
  expect(official.effective!.confirmation!.id).toBe(confirmation)
  expect(official.effective!.shares[0]!.share_bps).toBe(10000)
  await expect(page.getByLabel('贡献份额（%）', { exact: true })).toHaveCount(0)
  expect(
    (
      await page.request.patch(`${casePath()}/draft`, {
        headers,
        data: {
          ...(await command(page, '已核定历史不得修改')),
          summary: '不应保存的历史篡改',
          evidence_snapshot: {},
        },
      })
    ).status(),
  ).toBe(409)
  await logout(page)
  await login(page)
  await page.goto(caseURL())
  await page.getByLabel('操作原因', { exact: true }).fill('准备本人查看正式结果和独立确认收件')
  await page
    .getByTestId(`prepare-notice-${people[0]!.id}`)
    .getByRole('button', { name: '准备收件告知', exact: true })
    .click()
  await expect
    .poll(async () => {
      const notices = await data<S['ContributionNotice'][]>(
        await page.request.get(`${casePath()}/notices`),
      )
      return notices.length
    })
    .toBe(1)
  const prepared = await data<S['ContributionNotice'][]>(
    await page.request.get(`${casePath()}/notices`),
  )
  expect(prepared[0]!.receipt).toBeNull()
  expect(
    (
      await page.request.post(`${casePath()}/notices/${prepared[0]!.id}/receipt`, {
        headers,
        data: {
          expected_version: (await current(page)).case.revision,
          case_version_id: firstVersion,
          confirmation_id: confirmation,
          content_digest: official.effective!.confirmation!.version_digest,
          client_request_id: randomUUID(),
          received: true,
        },
      })
    ).status(),
  ).toBe(403)
  const projectBeforeReceipt = await data<S['Project']>(await page.request.get(base()))
  expect(
    (
      await page.request.post(`${base()}/archive`, {
        headers,
        data: { expected_version: projectBeforeReceipt.version },
      })
    ).status(),
  ).toBe(409)
  await logout(page)
  await login(page, people[0]!.username)
  let receiptPosts = 0
  page.on('request', (request) => {
    if (request.method() === 'POST' && /\/receipt$/.test(request.url())) receiptPosts++
  })
  await page.goto(`/teams/${team}/contribution-results`)
  const card = page.getByTestId('contribution-result').filter({ hasText: '人工逐案核对' })
  await expect(card).toContainText('100.00%')
  await expect(card).toContainText('确认收到，仅表示你已收到此结果，不代表同意贡献分配。')
  await page.reload()
  await expect(card).toContainText('100.00%')
  expect(receiptPosts).toBe(0)
  const notices = await data<S['ContributionNotice'][]>(
    await page.request.get(`${casePath()}/notices`),
  )
  expect(notices[0]!.receipt).toBeNull()
  await mobile(page)
  await page.screenshot({ path: '../../.cache/phase05-m4-results-mobile.png', fullPage: true })
  const acknowledged = card.getByLabel('我已阅读此结果及收件说明', { exact: true })
  await acknowledged.focus()
  await expect(acknowledged).toBeFocused()
  await page.keyboard.press('Space')
  await expect(acknowledged).toBeChecked()
  await page.keyboard.press('Tab')
  await expect(card.getByRole('button', { name: '确认已收到', exact: true })).toBeFocused()
  const received = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/receipt'),
  )
  await page.keyboard.press('Enter')
  const response = await received
  expect(response.status()).toBe(200)
  expect(response.request().postDataJSON()).toMatchObject({
    case_version_id: firstVersion,
    confirmation_id: confirmation,
    content_digest: official.effective!.confirmation!.version_digest,
    received: true,
  })
  expect(response.request().postDataJSON().client_request_id).toMatch(/^[0-9a-f-]{36}$/)
  expect(response.request().postDataJSON().expected_version).toBeGreaterThan(0)
  await expect(card).toContainText('已确认收到')
  expect(receiptPosts).toBe(1)
  const actual = await data<S['ContributionNotice'][]>(
    await page.request.get(`${casePath()}/notices`),
  )
  expect(actual[0]!.receipt?.subject_id).toBe(people[0]!.id)
})

test('dispute requires an independent ruling and overturned history remains visible', async ({
  page,
}) => {
  await login(page, people[0]!.username)
  await page.goto(caseURL())
  await page.getByLabel('异议原因', { exact: true }).fill('本人已收到结果，但对贡献依据有异议')
  await page.getByRole('button', { name: '发起异议', exact: true }).click()
  await expect(
    page.getByText('本人已收到结果，但对贡献依据有异议', { exact: false }).first(),
  ).toBeVisible()
  const disputed = await current(page)
  expect(disputed.case.case_workflow_status).toBe('disputed')
  expect(disputed.case.undisputed_confirmed).toBe(false)
  const disputes = await data<S['ContributionDispute'][]>(
    await page.request.get(`${casePath()}/disputes`),
  )
  expect(disputes[0]!.resolution).toBeNull()
  await logout(page)
  await login(page, people[2]!.username)
  await page.goto(caseURL())
  await expect(page.getByRole('button', { name: '推翻原核定', exact: true })).toHaveCount(0)
  expect(
    (
      await page.request.post(`${casePath()}/disputes/${disputes[0]!.id}/resolutions`, {
        headers,
        data: {
          expected_version: (await current(page)).case.revision,
          client_request_id: randomUUID(),
          reason: '原核定人不得自行裁定争议',
          decision: 'overturn',
        },
      })
    ).status(),
  ).toBe(403)
  await logout(page)
  await login(page, people[3]!.username)
  await page.goto(caseURL())
  await page.getByLabel('裁定原因', { exact: true }).fill('独立核查争议后推翻，重新整理贡献依据')
  await page.getByRole('button', { name: '推翻原核定', exact: true }).click()
  await expect(page.getByText('已被推翻', { exact: false }).first()).toBeVisible()
  const overturned = await current(page)
  expect(overturned.effective).toBeNull()
  expect(overturned.latest.status).toBe('draft')
  const versions = await data<S['ContributionVersion'][]>(
    await page.request.get(`${casePath()}/versions`),
  )
  expect(versions.find((version) => version.id === firstVersion)?.status).toBe('overturned')
  expect(versions.find((version) => version.id === firstVersion)?.confirmation?.id).toBe(
    confirmation,
  )
  const actual = await data<S['ContributionDispute'][]>(
    await page.request.get(`${casePath()}/disputes`),
  )
  expect(actual[0]!.resolution?.decision).toBe('overturn')
})

test('manual revisions preserve the effective version until fresh independent confirmation', async ({
  page,
}) => {
  await login(page)
  await page.goto(caseURL())
  await page.getByLabel('草案说明', { exact: true }).fill('根据独立裁定重新整理的第二版提案')
  await page.getByLabel('修改原因', { exact: true }).fill('补充并更正争议贡献依据')
  await page.getByRole('button', { name: '保存草案说明', exact: true }).click()
  await manualShare(page)
  await submit(page, '裁定后修订重新提交，原确认保留')
  await logout(page)
  await login(page, people[2]!.username)
  await page.goto(caseURL())
  await confirm(page)
  const second = await current(page)
  expect(second.effective!.version).toBe(2)
  await logout(page)
  await login(page)
  await page.goto(caseURL())
  await page
    .getByLabel('操作原因', { exact: true })
    .fill('继续修订说明，正式结果在再次核定前保持有效')
  await page.getByRole('button', { name: '创建修订草案', exact: true }).click()
  await expect(page.getByTestId('contribution-latest')).toContainText('草案')
  const revision = await current(page)
  expect(revision.latest.id).not.toBe(revision.effective!.id)
  expect(revision.effective!.id).toBe(second.effective!.id)
  expect(revision.latest.shares).toEqual([])
  expect(revision.latest.total_bps).toBe(0)
  expect(revision.effective!.total_bps).toBe(10000)
  expect(revision.case.case_workflow_status).toBe('revising')
  await expect(page.getByTestId('contribution-effective')).toContainText('100.00%')
  await page
    .getByLabel('草案说明', { exact: true })
    .fill('第三版：完善说明，人工份额仍为 10000 基点')
  await page.getByLabel('修改原因', { exact: true }).fill('完善正式贡献说明')
  await page.getByRole('button', { name: '保存草案说明', exact: true }).click()
  await manualShare(page)
  await submit(page, '修订提案再次独立核定')
  await logout(page)
  await login(page, people[2]!.username)
  await page.goto(caseURL())
  await confirm(page)
  const third = await current(page)
  expect(third.latest.id).toBe(third.effective!.id)
  expect(third.effective!.version).toBe(3)
  const history = await data<S['ContributionVersion'][]>(
    await page.request.get(`${casePath()}/versions`),
  )
  expect(history.find((version) => version.version === 1)?.status).toBe('overturned')
  expect(history.find((version) => version.version === 2)?.status).toBe('superseded')
  expect(history.find((version) => version.version === 2)?.confirmation?.id).toBe(
    second.effective!.confirmation!.id,
  )
  await page.screenshot({
    path: '../../.cache/phase05-m4-contribution-desktop.png',
    fullPage: true,
  })
  await mobile(page)
  await page.screenshot({ path: '../../.cache/phase05-m4-contribution-mobile.png', fullPage: true })
  await logout(page)
  await login(page)
  const projectBeforeNewReceipt = await data<S['Project']>(await page.request.get(base()))
  expect(
    (
      await page.request.post(`${base()}/archive`, {
        headers,
        data: { expected_version: projectBeforeNewReceipt.version },
      })
    ).status(),
  ).toBe(409)
})

test('departed members receive intake acknowledgment without ordinary Team data access', async ({
  page,
}) => {
  await login(page, people[0]!.username)
  await page.goto(caseURL())
  await expect(page.getByTestId('contribution-effective')).toContainText('V3')
  // These identifiers were actually displayed before departure, not disclosed
  // by a new result endpoint or inferred from intake verification.
  const held = await current(page)
  const version = held.effective!
  await logout(page)
  await login(page)
  await page.goto(`/teams/${team}/members`)
  await page
    .locator(`li[data-user-id="${people[0]!.id}"]`)
    .getByRole('button', { name: '移除', exact: true })
    .click()
  await expect(page.getByRole('status')).toContainText('已移除成员 phase05_worker')
  await logout(page)
  await login(page, people[0]!.username)
  expect((await page.request.get(`${base()}/contribution-results/mine`)).status()).toBe(404)
  expect((await page.request.get(casePath())).status()).toBe(404)
  expect((await page.request.get(base())).status()).toBe(404)
  await page.goto(`/teams/${team}/contribution-results`)
  await expect(page.getByTestId('contribution-result')).toHaveCount(0)
  await expect(page.getByText('可追溯验收成果', { exact: true })).toHaveCount(0)
  await page.goto('/contribution-dispute-requests')
  await page.getByLabel('团队 ID', { exact: true }).fill(team)
  await page.getByLabel('项目 ID', { exact: true }).fill(project)
  await page.getByLabel('案例 ID', { exact: true }).fill(caseID)
  await page.getByLabel('核定版本 ID', { exact: true }).fill(version.id)
  await page.getByLabel('核定记录 ID', { exact: true }).fill(version.confirmation!.id)
  await page
    .getByLabel('本人异议声明', { exact: true })
    .fill('本人对离队前查看的第三版核定提出异议请求')
  const intake = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().endsWith('/contribution-dispute-requests'),
  )
  await page.getByRole('button', { name: '提交本人请求', exact: true }).click()
  const accepted = await intake
  expect(accepted.status()).toBe(202)
  const received = await data<S['ContributionIntake']>(accepted, 202)
  expect(Object.keys(received).sort()).toEqual(['received', 'request_id'])
  expect(received.received).toBe(true)
  await expect(page.getByRole('status')).toContainText('请求已受理')
  await expect(page.getByRole('status')).toContainText('受理不表示核验成功')
  await expect(page.getByText('可追溯验收成果', { exact: true })).toHaveCount(0)
  await expect(page.getByText('贡献治理真实闭环', { exact: true })).toHaveCount(0)
  expect((await page.request.get(casePath())).status()).toBe(404)
})
