// Keep decimal strings exact: money and token counts never pass through floating point arithmetic.
export function tokenText(value: string | undefined): string {
  const n = BigInt((value || '0').split('.')[0] || '0')
  if (n === 0n) return '0'
  if (n < 1000n) return '＜1K'
  const [unit, suffix]: [bigint, string] = n >= 1000000000n ? [1000000000n, 'B'] : n >= 1000000n ? [1000000n, 'M'] : [1000n, 'K']
  const hundredths = n * 100n / unit
  return `${hundredths / 100n}${hundredths % 100n ? `.${String(hundredths % 100n).padStart(2, '0').replace(/0$/, '')}` : ''}${suffix}`
}
export function moneyText(value: string | undefined): string {
  const [whole = '0', fraction = ''] = (value || '0').split('.')
  const decimals = fraction.slice(0, 6).replace(/0+$/, '')
  if (whole === '0' && !decimals && /[1-9]/.test(fraction)) return '¥＜0.000001'
  return `¥${whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}${decimals ? `.${decimals}` : '.00'}`
}
