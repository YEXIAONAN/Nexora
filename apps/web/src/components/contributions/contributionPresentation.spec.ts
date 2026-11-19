import { describe, expect, it } from 'vitest'

import { ApiError } from '@/api/client'
import type { ContributionConfirmation, ContributionNotice } from '@/api/contributions'
import {
  canSubmitShares,
  caseStatusLabel,
  contributionError,
  formatBasisPoints,
  percentToBasisPoints,
  totalBasisPoints,
  verifiedReceipt,
  versionStatusLabel,
} from './contributionPresentation'

describe('manual integer basis points', () => {
  it.each([
    ['0', 0],
    ['100', 10000],
    ['33.33', 3333],
    ['33.34', 3334],
    ['0.01', 1],
    ['10.1', 1010],
    [' 12.34 ', 1234],
  ])('converts %s exactly without floating-point multiplication', (value, expected) => {
    expect(percentToBasisPoints(value as string)).toBe(expected)
  })

  it.each(['', '-1', '100.01', '0.001', '1e2', 'NaN', 'Infinity', '1,2', '0x10', '01', '.5'])(
    'rejects invalid or inexact percentage %s',
    (value) => {
      expect(percentToBasisPoints(value)).toBeNull()
    },
  )

  it('permits incomplete draft totals but requires exactly 10000 to submit', () => {
    const draft = [{ share_bps: 3333 }, { share_bps: 3333 }]
    expect(totalBasisPoints(draft)).toBe(6666)
    expect(canSubmitShares(draft)).toBe(false)
    expect(canSubmitShares([...draft, { share_bps: 3334 }])).toBe(true)
    expect(canSubmitShares([...draft, { share_bps: 3333 }])).toBe(false)
    expect(canSubmitShares([...draft, { share_bps: 3335 }])).toBe(false)
    expect(canSubmitShares([])).toBe(false)
    expect(canSubmitShares([{ share_bps: 0 }, { share_bps: 10000 }])).toBe(false)
    expect(canSubmitShares([{ share_bps: 9999.5 }, { share_bps: 0.5 }])).toBe(false)
  })

  it('formats basis points as precise percentage text', () => {
    expect(formatBasisPoints(1)).toBe('0.01%')
    expect(formatBasisPoints(3333)).toBe('33.33%')
    expect(formatBasisPoints(10000)).toBe('100.00%')
    expect(formatBasisPoints(0.1)).toBe('无效份额')
  })
})

describe('governance semantics', () => {
  it('distinguishes disputed, revising, superseded and overturned states', () => {
    expect(caseStatusLabel('disputed')).toBe('存在争议')
    expect(caseStatusLabel('revising')).toBe('修订中')
    expect(versionStatusLabel('superseded')).toBe('已被替代')
    expect(versionStatusLabel('overturned')).toBe('已被推翻')
    expect(versionStatusLabel('draft')).toContain('非正式贡献')
  })

  it.each([401, 403, 404, 409, 422, 500])(
    'explains API status %s and server field errors',
    (status) => {
      const text = contributionError(
        new ApiError('服务端解释', status, undefined, undefined, {
          share_bps: '仅允许整数基点',
        }),
      )
      expect(text).toContain('服务端解释')
      expect(text).toContain('仅允许整数基点')
    },
  )

  it('explains network failure', () => {
    expect(contributionError(new TypeError('fetch failed'))).toContain('无法连接')
  })
})

const confirmation: ContributionConfirmation = {
  id: 'confirmation',
  case_version_id: 'version',
  confirmed_by: 'independent-appraiser',
  rationale: '核对通过',
  version_digest: 'a'.repeat(64),
  membership_snapshot: {},
  confirmed_at: '2026-11-19T05:00:00Z',
}
const notice: ContributionNotice = {
  id: 'notice',
  case_id: 'case',
  case_version_id: 'version',
  confirmation_id: 'confirmation',
  recipient_id: 'holder',
  created_by: 'editor',
  channel: 'authenticated_ack',
  prepared_at: '2026-11-19T05:01:00Z',
  receipt: {
    id: 'receipt',
    notice_id: 'notice',
    case_version_id: 'version',
    confirmation_id: 'confirmation',
    subject_id: 'holder',
    content_digest: 'a'.repeat(64),
    verified_at: '2026-11-19T05:02:00Z',
    sent_at: '2026-11-19T05:01:00Z',
    acknowledged_at: '2026-11-19T05:02:00Z',
  },
}

describe('actual authenticated receipt evidence', () => {
  it('requires actual server proof; a prepared notice alone is not received', () => {
    expect(verifiedReceipt({ ...notice, receipt: null }, confirmation)).toBe(false)
    expect(verifiedReceipt(notice, confirmation)).toBe(true)
    expect(verifiedReceipt({ ...notice, channel: 'manual_unverified' }, confirmation)).toBe(false)
  })

  it.each([
    ['notice_id', 'different-notice'],
    ['case_version_id', 'old-version'],
    ['confirmation_id', 'old-confirmation'],
    ['subject_id', 'other-person'],
    ['content_digest', 'b'.repeat(64)],
    ['verified_at', ''],
    ['acknowledged_at', 'not-a-time'],
    ['sent_at', ''],
  ])('rejects mismatched receipt evidence %s', (field, value) => {
    expect(
      verifiedReceipt(
        {
          ...notice,
          receipt: { ...notice.receipt!, [field]: value },
        },
        confirmation,
      ),
    ).toBe(false)
  })

  it('binds notice to the exact displayed confirmation and version', () => {
    expect(verifiedReceipt({ ...notice, confirmation_id: 'other' }, confirmation)).toBe(false)
    expect(verifiedReceipt({ ...notice, case_version_id: 'other' }, confirmation)).toBe(false)
  })
})
