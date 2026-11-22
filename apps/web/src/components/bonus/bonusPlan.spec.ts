import { describe, expect, it } from 'vitest'
import { buildBonusPlan, draftFromSource, sumDraftLines, type DraftSource } from './bonusPlan'
import { bonusAmountInput, formatBonusAmount, parseBonusAmount } from './bonusPresentation'

function source(): DraftSource {
  return {
    confirmation_id: 'confirmed',
    task_title: '设计工作',
    budget: '0.30',
    reason: '来源预算理由',
    lines: [
      {
        user_id: 'a',
        display_name: '甲',
        share_bps: 8000,
        eligible: true,
        amount: '0.10',
        reason: '人工评估',
      },
      {
        user_id: 'b',
        display_name: '乙',
        share_bps: 2000,
        eligible: true,
        amount: '0.20',
        reason: '特殊奖励',
      },
    ],
  }
}

describe('bonus money and manual plans', () => {
  it.each([
    ['0', 0],
    ['0.01', 1],
    ['0.29', 29],
    ['1.2', 120],
    ['0001.20', 120],
    [' 1.23 ', 123],
    ['9000000000000.00', 900000000000000],
  ])('parses %s exactly', (input, expected) => {
    expect(parseBonusAmount(input as string)).toBe(expected)
  })
  it.each([
    '',
    '1e2',
    'NaN',
    'Infinity',
    '-1',
    '+1',
    '.5',
    '1.',
    '0.001',
    '1,000',
    '9000000000000.01',
    '9999999999999999999999',
    '1 2',
  ])('rejects invalid or overflowing input %s', (input) => {
    expect(parseBonusAmount(input)).toBeNull()
  })
  it('round trips the maximum without float rounding and formats aggregate integers', () => {
    expect(bonusAmountInput(900000000000000)).toBe('9000000000000.00')
    expect(formatBonusAmount(900000000000000n)).toBe('¥9,000,000,000,000.00')
    expect(formatBonusAmount(-1n)).toBe('-¥0.01')
  })
  it('preserves manually chosen amounts instead of proportionally distributing', () => {
    const draft = source()
    expect(sumDraftLines(draft)).toBe(30n)
    expect(buildBonusPlan('0.30', [draft]).sources[0]?.lines.map((l) => l.amount_minor)).toEqual([
      10, 20,
    ])
  })
  it('forces ineligible amounts to zero while keeping the human reason', () => {
    const draft = source()
    draft.lines[0]!.eligible = false
    draft.lines[0]!.amount = '99.00'
    expect(buildBonusPlan('0.30', [draft]).sources[0]?.lines[0]).toEqual({
      user_id: 'a',
      eligible: false,
      amount_minor: 0,
      reason: '人工评估',
    })
    draft.lines[0]!.reason = ''
    expect(() => buildBonusPlan('0.30', [draft])).toThrow('无资格也需填写')
  })
  it('blocks both source overspend and overall budget overspend', () => {
    const draft = source()
    draft.budget = '0.29'
    expect(() => buildBonusPlan('1.00', [draft])).toThrow('成员金额超出来源预算')
    draft.budget = '0.30'
    expect(() => buildBonusPlan('0.29', [draft])).toThrow('各来源预算合计超出总预算')
  })
  it('starts all confirmed shares at zero without silently assigning eligibility', () => {
    const draft = draftFromSource({
      confirmation_id: 'confirmation',
      case_version_id: 'v1',
      task_title: '工作',
      shares: [{ user_id: 'a', display_name: '甲', share_bps: 10000 }],
    })
    expect(draft.lines).toEqual([
      {
        user_id: 'a',
        display_name: '甲',
        share_bps: 10000,
        eligible: false,
        amount: '0.00',
        reason: '',
      },
    ])
  })
})
