import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import * as api from '@/api/submissions'
import ReviewersPanel from './ReviewersPanel.vue'
vi.mock('@/api/submissions', () => ({
  listReviewers: vi.fn(),
  grantReviewer: vi.fn(),
  revokeReviewer: vi.fn(),
}))
it('binds reviewer revocation to confirmed scope and explicit grant version', async () => {
  const grant: api.Reviewer = {
    user_id: 'r',
    username: 'reviewer',
    display_name: 'Reviewer',
    default_lead: false,
    active: true,
    version: 4,
    granted_by: 'lead',
    granted_at: '2026-11-18T00:00:00Z',
    reason: 'Independent review',
  }
  vi.mocked(api.listReviewers).mockResolvedValue([grant])
  vi.mocked(api.revokeReviewer).mockResolvedValue([])
  const w = mount(ReviewersPanel, {
    props: { team: 'A', project: 'P', canManage: true, members: [] },
  })
  await flushPromises()
  await w.get('input').setValue('Rotate reviewer')
  await w
    .findAll('button')
    .find((b) => b.text() === '撤销审核权')!
    .trigger('click')
  await flushPromises()
  expect(api.revokeReviewer).toHaveBeenCalledWith(
    'A',
    'P',
    expect.objectContaining({ user_id: 'r', version: 4 }),
    'Rotate reviewer',
  )
})
