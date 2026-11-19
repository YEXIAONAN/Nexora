import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ContributionDraftEditor from './ContributionDraftEditor.vue'
import ContributionVersionPanel from './ContributionVersionPanel.vue'
import type { ContributionVersion } from '@/api/contributions'
const version: ContributionVersion = {
  id: 'v1',
  case_id: 'c1',
  version: 1,
  status: 'draft',
  supersedes_version_id: null,
  proposed_by: 'editor',
  summary: '人工说明',
  evidence_snapshot: { original: { reference: '审核原文' } },
  proposed_at: '2026-11-19T00:00:00Z',
  submitted_at: null,
  submitted_digest: null,
  shares: [],
  confirmation: null,
  total_bps: 0,
  ready_to_submit: false,
}
function setup(busy = false) {
  return mount(ContributionDraftEditor, { props: { version: { ...version }, members: [], busy } })
}
describe('manual contribution draft', () => {
  it('rejects fractions below one base point instead of rounding', async () => {
    const w = setup()
    await w.get('input[inputmode="decimal"]').setValue('33.333')
    await w.findAll('input').at(-1)!.setValue('无舍入')
    await w.findAll('form')[1]!.trigger('submit')
    expect(w.emitted('share')).toBeUndefined()
    expect(w.text()).toContain('请输入 0.01 至 100.00')
    w.unmount()
  })
  it('retains existing evidence when saving manual notes', async () => {
    const w = setup()
    await w.findAll('textarea')[1]!.setValue('新增整理证据')
    await w.findAll('input')[0]!.setValue('补充证据')
    await w.findAll('form')[0]!.trigger('submit')
    expect(w.emitted('save')?.[0]).toEqual([
      {
        summary: '人工说明',
        evidence_snapshot: { original: { reference: '审核原文' }, notes: '新增整理证据' },
        reason: '补充证据',
      },
    ])
    w.unmount()
  })
  it('disables the editor while a write is pending', () => {
    const w = setup(true)
    expect(w.get('fieldset').attributes('disabled')).toBeDefined()
    w.unmount()
  })
  it('requires a reason to remove an unconfirmed share', async () => {
    const share = {
      user_id: 'contributor',
      share_bps: 10000,
      basis: '依据',
      statement: '说明',
      evidence_refs: [],
      membership_state: 'active' as const,
    }
    const w = setup()
    await w.setProps({ version: { ...version, shares: [share], total_bps: 10000 } })
    const remove = w.findAll('button').find((b) => b.text() === '删除此份额')!
    expect(remove.attributes('disabled')).toBeDefined()
    await w.findAll('input').at(-1)!.setValue('撤销重复项')
    await remove.trigger('click')
    expect(w.emitted('remove')?.[0]).toEqual(['contributor', '撤销重复项'])
    w.unmount()
  })
  it('emits precise base points from manually entered text', async () => {
    const share = {
      user_id: 'contributor',
      share_bps: 3333,
      basis: '依据',
      statement: '说明',
      evidence_refs: [],
      membership_state: 'active' as const,
    }
    const w = setup()
    await w.setProps({ version: { ...version, shares: [share], total_bps: 3333 } })
    await w
      .findAll('button')
      .find((b) => b.text() === '编辑此份额')!
      .trigger('click')
    await w.findAll('input').at(-1)!.setValue('人工修改')
    await w.findAll('form')[1]!.trigger('submit')
    expect(w.emitted('share')?.[0]).toEqual([
      'contributor',
      { share_bps: 3333, basis: '依据', statement: '说明', evidence_refs: [], reason: '人工修改' },
    ])
    w.unmount()
  })
})
describe('immutable contribution version presentation', () => {
  it.each([
    ['superseded', '已被后续版本替代'],
    ['overturned', '已被裁定推翻'],
    ['draft', '草案 · 尚未正式核定'],
  ] as const)('shows distinct historical semantics for %s', (status, label) => {
    const w = mount(ContributionVersionPanel, { props: { version: { ...version, status } } })
    expect(w.text()).toContain(label)
    expect(w.findAll('button')).toHaveLength(0)
    w.unmount()
  })
  it('preserves a historical confirmation without edit controls', () => {
    const confirmation = {
      id: 'conf1',
      case_version_id: 'v1',
      confirmed_by: 'appraiser',
      rationale: '独立审核',
      version_digest: 'a'.repeat(64),
      membership_snapshot: {},
      confirmed_at: '2026-11-19T00:00:00Z',
    }
    const w = mount(ContributionVersionPanel, {
      props: { version: { ...version, status: 'overturned', confirmation } },
    })
    expect(w.get('[data-testid="immutable-confirmation"]').text()).toContain('conf1')
    expect(w.text()).toContain('不可变核定记录')
    expect(w.findAll('input,textarea,button')).toHaveLength(0)
    w.unmount()
  })
})
