import { describe, expect, it } from 'vitest'
import {
  bonusRoundStatusLabel,
  bonusRoundStatusTone,
  bonusVersionStatusLabel,
  formatBonusAmount,
} from './bonusPresentation'

describe('bonusPresentation', () => {
  it('formats CNY fen without losing cents', () => {
    expect(formatBonusAmount(0)).toBe('¥0.00')
    expect(formatBonusAmount(5)).toBe('¥0.05')
    expect(formatBonusAmount(123456)).toBe('¥1,234.56')
    expect(formatBonusAmount(100000000)).toBe('¥1,000,000.00')
  })

  it('labels rounds and versions without implying payment', () => {
    expect(bonusRoundStatusLabel('confirmed')).toBe('已核定')
    expect(bonusRoundStatusLabel('pending_review')).toBe('待核定')
    expect(bonusVersionStatusLabel('superseded')).toBe('已被替代')
    expect(bonusRoundStatusTone('confirmed')).toBe('success')
    expect(bonusRoundStatusTone('abandoned')).toBe('danger')
    expect(bonusRoundStatusTone('draft')).toBe('neutral')
  })
})
