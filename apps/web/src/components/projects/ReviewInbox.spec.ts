import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import { pendingReviews, type PendingReview } from '@/api/submissions'
import ReviewInbox from './ReviewInbox.vue'
vi.mock('@/api/submissions', () => ({ pendingReviews: vi.fn() }))
it('does not show the old Team review inbox after switching Teams', async () => {
  let resolve!: (v: PendingReview[]) => void
  vi.mocked(pendingReviews)
    .mockReturnValueOnce(
      new Promise((r) => {
        resolve = r
      }),
    )
    .mockResolvedValueOnce([])
  const w = mount(ReviewInbox, { props: { team: 'A' }, global: { stubs: { RouterLink: true } } })
  await w.setProps({ team: 'B' })
  await flushPromises()
  resolve([
    {
      submission_version_id: 'old',
      task_title: 'Old confidential task',
      project_name: 'Old Team',
    } as PendingReview,
  ])
  await flushPromises()
  expect(w.text()).not.toContain('Old confidential task')
  expect(w.text()).toContain('暂无需要你独立审核')
})
