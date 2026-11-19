import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { ContributionEvidence } from '@/api/contributions'
import ContributionEvidencePanel from './ContributionEvidencePanel.vue'
const evidence: ContributionEvidence = {
  task_id: 'task1',
  task_title: '正式成果',
  task_owner_id: 'worker',
  submission_version_id: 'submission1',
  author_id: 'worker',
  summary: '真实验收内容',
  evidence_references: [],
  submitted_at: '2026-11-19T00:00:00Z',
  review_id: 'review1',
  reviewer_id: 'reviewer',
  decision: 'approve',
  comment: '已验收',
  reviewed_at: '2026-11-19T00:00:00Z',
  attachments: [
    {
      id: 'attachment1',
      uploader_id: 'worker',
      file_name: 'evidence.txt',
      byte_size: 20,
      sha256: 'a'.repeat(64),
    },
  ],
}
describe('contribution evidence attachment access', () => {
  it('explains original-task authorization and preserves the workspace if download is denied', () => {
    const w = mount(ContributionEvidencePanel, {
      props: { team: 'team1', project: 'project1', evidence },
    })
    expect(w.text()).toContain('核定授权不会自动增加附件权限')
    expect(w.text()).toContain('通过现有审核人授权明确提供读取权限，核对证据后再核定')
    const link = w.get('a')
    expect(link.attributes('href')).toBe(
      '/api/v1/teams/team1/projects/project1/tasks/task1/submissions/submission1/attachments/attachment1',
    )
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toContain('noopener')
    expect(w.text()).toContain('SHA256：' + 'a'.repeat(64))
    w.unmount()
  })
})
