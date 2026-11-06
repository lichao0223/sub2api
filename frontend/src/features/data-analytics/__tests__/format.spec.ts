import { describe, expect, it } from 'vitest'
import { moneyText, tokenText } from '../format'

describe('analytics exact string formatting', () => {
  it('uses compact units for every nonzero token value', () => {
    expect(['0','1','999','1000','1234567','1000000000','9007199254740993'].map(tokenText))
      .toEqual(['0','＜1K','＜1K','1K','1.23M','1B','9007199.25B'])
  })
  it('does not round tiny positive procurement costs to zero', () => {
    expect(moneyText('0.000000000001')).toBe('¥＜0.000001')
    expect(moneyText('9007199254740993.123456789012')).toBe('¥9,007,199,254,740,993.123456')
  })
})
