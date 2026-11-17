import { expect, test, type Page } from '@playwright/test'

const rootPassword = process.env.E2E_ROOT_PASSWORD ?? ''
const alicePassword = 'alice-e2e-password-1'

let teamId = ''
let secondTeamId = ''

test.describe.configure({ mode: 'serial' })

test.beforeAll(() => {
  if (!rootPassword) {
    throw new Error('E2E_ROOT_PASSWORD must be provided by infra/scripts/e2e.sh')
  }
})

async function login(page: Page, username: string, password: string): Promise<void> {
  await page.goto('/login')
  await page.fill('#login-username', username)
  await page.fill('#login-password', password)
  await page.click('button[type="submit"]')
  await expect(page).toHaveURL(/\/teams$/)
}

async function logout(page: Page): Promise<void> {
  await page.click('button[aria-label="退出登录"]')
  await expect(page).toHaveURL(/\/login/)
}

async function openMembers(page: Page, id: string): Promise<void> {
  await page.goto(`/teams/${id}/members`)
  await expect(page.locator('h2')).toContainText(/加载中…|ZERO_R|Platform Ops/)
}

test('unauthenticated visitors are redirected to the login page', async ({ page }) => {
  await page.goto('/teams')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.locator('h1')).toHaveText('登录')
  await page.goto('/invitations')
  await expect(page).toHaveURL(/\/login\?redirect=/)
})

test('ROOT logs in and creates the first team through the UI', async ({ page }) => {
  await login(page, 'root', rootPassword)
  await expect(page.locator('header')).toContainText('ROOT')

  await page.fill('#team-name', 'ZERO_R')
  await page.fill('#team-description', '核心研发团队')
  await page.locator('form').first().locator('button[type="submit"]').click()
  await expect(page.getByRole('status')).toContainText('团队「ZERO_R」已创建')

  const row = page.locator('li[data-team-id]', { hasText: 'ZERO_R' })
  await expect(row).toContainText('所有者')
  teamId = (await row.getAttribute('data-team-id')) ?? ''
  expect(teamId).not.toBe('')

  const stored = await page.evaluate(() => Object.keys(window.localStorage))
  expect(stored.filter((key) => /token|session|password/i.test(key))).toEqual([])
})

test('ROOT creates a member account through the real admin API', async ({ page }) => {
  await login(page, 'root', rootPassword)
  const response = await page.context().request.post('/api/v1/admin/users', {
    headers: { 'X-Nexora-CSRF': '1' },
    data: { username: 'alice', password: alicePassword, display_name: 'Alice' },
  })
  expect(response.status()).toBe(201)
})

test('ROOT invites alice through the members page', async ({ page }) => {
  await login(page, 'root', rootPassword)
  await openMembers(page, teamId)
  await page.fill('#invite-username', 'alice')
  await page.selectOption('#invite-role', 'member')
  await page.getByRole('button', { name: '发出邀请' }).click()
  await expect(page.getByRole('status')).toContainText('已向 alice 发出邀请')
  const pendingRow = page.locator('li', { hasText: 'alice' }).first()
  await expect(pendingRow).toContainText('有效期至')
  await expect(pendingRow.getByRole('button', { name: '取消邀请' })).toBeVisible()
})

test('alice accepts the invitation from her inbox', async ({ page }) => {
  await login(page, 'alice', alicePassword)
  await expect(page.getByText('你还没有加入任何团队')).toBeVisible()

  await page.goto('/invitations')
  const row = page.locator('li[data-invitation-id]', { hasText: 'ZERO_R' })
  await expect(row).toContainText('成员')
  await row.getByRole('button', { name: '接受' }).click()
  await expect(page.getByRole('status')).toContainText('已加入团队「ZERO_R」')

  await page.goto('/teams')
  await expect(page.locator('li[data-team-id]', { hasText: 'ZERO_R' })).toContainText('成员')
})

test('a plain member sees no management controls', async ({ page }) => {
  await login(page, 'alice', alicePassword)
  await openMembers(page, teamId)
  await expect(page.locator('#invite-username')).toHaveCount(0)
  await expect(page.locator('select[aria-label^="变更"]')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '移除' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '退出团队' })).toBeVisible()
})

test('ROOT promotes alice to admin and the change is reflected', async ({ page }) => {
  await login(page, 'root', rootPassword)
  await openMembers(page, teamId)
  await page.selectOption('select[aria-label="变更 alice 的角色"]', 'admin')
  await expect(page.getByRole('status')).toContainText('已将 alice 的角色变更为管理员')
  await expect(page.locator('li[data-user-id]', { hasText: 'alice' })).toContainText('管理员')
})

test('ROOT removes alice and her team access ends immediately', async ({ page }) => {
  await login(page, 'root', rootPassword)
  await openMembers(page, teamId)
  await page
    .locator('li[data-user-id]', { hasText: 'alice' })
    .getByRole('button', { name: '移除' })
    .click()
  await expect(page.getByRole('status')).toContainText('已移除成员 alice')
  await expect(page.locator('li[data-user-id]', { hasText: 'alice' })).toHaveCount(0)

  await logout(page)
  await login(page, 'alice', alicePassword)
  await expect(page.getByText('你还没有加入任何团队')).toBeVisible()
  await page.goto(`/teams/${teamId}/members`)
  await expect(page.getByRole('alert')).toContainText('团队不存在，或你已不是该团队的成员')
})

test('the last owner cannot leave the team', async ({ page }) => {
  await login(page, 'root', rootPassword)
  await openMembers(page, teamId)
  await page.getByRole('button', { name: '退出团队' }).click()
  await expect(page.getByRole('alert')).toContainText(
    'the team must retain at least one active owner',
  )
  await expect(page.locator('li[data-user-id]', { hasText: 'root' })).toContainText('所有者')
})

test('the topbar switches between multiple teams', async ({ page }) => {
  await login(page, 'root', rootPassword)
  await page.fill('#team-name', 'Platform Ops')
  await page.locator('form').first().locator('button[type="submit"]').click()
  await expect(page.getByRole('status')).toContainText('团队「Platform Ops」已创建')
  const secondRow = page.locator('li[data-team-id]', { hasText: 'Platform Ops' })
  secondTeamId = (await secondRow.getAttribute('data-team-id')) ?? ''
  expect(secondTeamId).not.toBe('')
  expect(secondTeamId).not.toBe(teamId)

  const selector = page.locator('select[aria-label="当前团队"]')
  await expect(selector.locator('option')).toHaveCount(2)
  await expect(selector).toHaveValue(secondTeamId)
  await expect(secondRow).toContainText('当前团队')

  await selector.selectOption(teamId)
  await expect(page.locator('li[data-team-id]', { hasText: 'ZERO_R' })).toContainText('当前团队')
  await expect(secondRow).not.toContainText('当前团队')
})

test('a revoked session returns to the login page with an expiry notice', async ({ page }) => {
  await login(page, 'root', rootPassword)
  await openMembers(page, teamId)
  await page.context().clearCookies()
  await page.getByRole('button', { name: '刷新' }).click()
  await expect(page).toHaveURL(/\/login\?expired=1/)
  await expect(page.getByRole('status')).toContainText('会话已过期或已被撤销')
})
