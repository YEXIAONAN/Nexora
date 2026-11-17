import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '@/api/submissions'
import { transitionTask, type Task } from '@/api/tasks'
import SubmissionPanel from './SubmissionPanel.vue'
vi.mock('@/api/submissions', () => ({
  listVersions: vi.fn(),
  submitVersion: vi.fn(),
  reviewVersion: vi.fn(),
  uploadAttachment: vi.fn(),
  attachmentURL: vi.fn(() => '/private'),
}))
vi.mock('@/api/tasks', () => ({ transitionTask: vi.fn() }))
const task = (extra: Partial<Task> = {}): Task => ({
  id: 'task',
  team_id: 'A',
  project_id: 'P',
  parent_id: null,
  depth: 1,
  title: 'Delivery',
  description: '',
  owner_id: 'author',
  collaborator_ids: [],
  priority: 'normal',
  planned_start_at: null,
  planned_end_at: null,
  status: 'awaiting_review',
  actual_progress: 99,
  effective_progress_bps: 8000,
  weight: 1,
  is_leaf: true,
  blocked: false,
  can_execute: false,
  can_cancel: false,
  can_submit: false,
  can_review: true,
  current_submission_version_id: 'V2',
  accepted_review_id: null,
  cancellation_reason: '',
  version: 7,
  created_by: 'author',
  created_at: '2026-11-18T00:00:00Z',
  updated_at: '2026-11-18T00:00:00Z',
  ...extra,
})
const version = (id = 'V2', number = 2): api.SubmissionVersion => ({
  id,
  team_id: 'A',
  project_id: 'P',
  task_id: 'task',
  submission_id: 'submission',
  version: number,
  author_id: 'author',
  author_name: 'Author',
  summary: `Evidence ${id}`,
  references: [],
  attachments: [],
  submitted_at: '2026-11-18T00:00:00Z',
  based_on_task_version: 6,
  review: null,
})
function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}
const button = (w: ReturnType<typeof mount>, text: string) =>
  w.findAll('button').find((b) => b.text() === text)!
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.listVersions).mockResolvedValue([version(), version('V1', 1)])
  vi.mocked(api.reviewVersion).mockResolvedValue({} as never)
})
describe('version-bound delivery UI', () => {
  it('binds review to the confirmed current version and task concurrency version', async () => {
    const w = mount(SubmissionPanel, { props: { team: 'A', project: 'P', task: task() } })
    await flushPromises()
    await w.get('textarea').setValue('Verified current evidence')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(api.reviewVersion).toHaveBeenCalledWith('A', 'P', 'task', 'V2', {
      expected_version: 7,
      decision: 'approve',
      comment: 'Verified current evidence',
    })
    expect(w.emitted('changed')).toHaveLength(1)
  })
  it('shows historical evidence without allowing an old-version review', async () => {
    const w = mount(SubmissionPanel, { props: { team: 'A', project: 'P', task: task() } })
    await flushPromises()
    await button(w, 'V1').trigger('click')
    expect(w.get('[data-testid="submission-detail"]').text()).toContain('Evidence V1')
    expect(w.find('form').exists()).toBe(false)
  })
  it('rejects late Team/Project responses and clears old drafts', async () => {
    const old = deferred<api.SubmissionVersion[]>()
    vi.mocked(api.listVersions)
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce([version('NEW', 1)])
    const w = mount(SubmissionPanel, { props: { team: 'A', project: 'P', task: task() } })
    await w.setProps({
      team: 'B',
      project: 'Q',
      task: task({ id: 'other', version: 2, current_submission_version_id: 'NEW' }),
    })
    await flushPromises()
    old.resolve([version('STALE', 1)])
    await flushPromises()
    expect(w.text()).toContain('Evidence NEW')
    expect(w.text()).not.toContain('STALE')
  })
  it('does not emit an old review result after a context switch', async () => {
    const old = deferred<Awaited<ReturnType<typeof api.reviewVersion>>>()
    vi.mocked(api.reviewVersion).mockReturnValueOnce(old.promise)
    const w = mount(SubmissionPanel, { props: { team: 'A', project: 'P', task: task() } })
    await flushPromises()
    await w.get('textarea').setValue('Review A')
    await w.get('form').trigger('submit')
    await w.setProps({ team: 'B', project: 'Q', task: task({ id: 'other', version: 1 }) })
    await flushPromises()
    old.resolve({} as never)
    await flushPromises()
    expect(w.emitted('changed')).toBeUndefined()
    expect(w.text()).not.toContain('Review A')
  })
  it('uploads actual files before binding their IDs to a new immutable version', async () => {
    vi.mocked(api.listVersions).mockResolvedValue([])
    vi.mocked(api.uploadAttachment).mockResolvedValue({ id: 'file' } as api.Attachment)
    vi.mocked(api.submitVersion).mockResolvedValue(version('V1', 1))
    const w = mount(SubmissionPanel, {
      props: {
        team: 'A',
        project: 'P',
        task: task({
          status: 'in_progress',
          can_submit: true,
          current_submission_version_id: null,
        }),
      },
    })
    await flushPromises()
    await w.get('textarea').setValue('Reproducible delivery')
    const file = new File(['Evidence'], 'result.txt', { type: 'text/plain' })
    Object.defineProperty(w.get('input[type="file"]').element, 'files', {
      value: [file],
      configurable: true,
    })
    await w.get('input[type="file"]').trigger('change')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(api.uploadAttachment).toHaveBeenCalledWith('A', 'P', 'task', file, expect.any(String))
    expect(api.submitVersion).toHaveBeenCalledWith(
      'A',
      'P',
      'task',
      expect.objectContaining({
        attachment_ids: ['file'],
        expected_version: 7,
        summary: 'Reproducible delivery',
      }),
    )
    expect(w.emitted('changed')).toHaveLength(1)
  })
  it('stops the submit chain if the task changes while upload is in flight', async () => {
    vi.mocked(api.listVersions).mockResolvedValue([])
    const upload = deferred<api.Attachment>()
    vi.mocked(api.uploadAttachment).mockReturnValueOnce(upload.promise)
    const w = mount(SubmissionPanel, {
      props: { team: 'A', project: 'P', task: task({ status: 'in_progress', can_submit: true }) },
    })
    await flushPromises()
    await w.get('textarea').setValue('Old draft')
    const file = new File(['Evidence'], 'result.txt')
    Object.defineProperty(w.get('input[type="file"]').element, 'files', {
      value: [file],
      configurable: true,
    })
    await w.get('input[type="file"]').trigger('change')
    await w.get('form').trigger('submit')
    await w.setProps({
      task: task({ id: 'new-task', version: 1, status: 'pending', can_submit: false }),
    })
    await flushPromises()
    upload.resolve({ id: 'old-file' } as api.Attachment)
    await flushPromises()
    expect(api.submitVersion).not.toHaveBeenCalled()
    expect(w.emitted('changed')).toBeUndefined()
    expect(w.text()).not.toContain('Old draft')
  })
  it('requires explicit correction reason and never infers reviewer authority from participation', async () => {
    const w = mount(SubmissionPanel, {
      props: {
        team: 'A',
        project: 'P',
        task: task({ status: 'rejected', can_review: false, can_execute: true }),
      },
    })
    await flushPromises()
    await w.get('input').setValue('Address feedback')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(transitionTask).toHaveBeenCalledWith(
      'A',
      'P',
      expect.objectContaining({ id: 'task' }),
      'in_progress',
      'Address feedback',
    )
    expect(api.reviewVersion).not.toHaveBeenCalled()
  })
})
