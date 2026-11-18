import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'

describe('translation HTML sanitization', () => {
  it('neutralizes iframe srcdoc when escapeParameter is enabled (GHSA-83rm-6c8r-g728)', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'en',
      escapeParameter: true,
      warnHtmlMessage: false,
      messages: { en: { message: '<iframe srcdoc="<b>untrusted</b>"></iframe>' } },
    })

    const document = new DOMParser().parseFromString(i18n.global.t('message'), 'text/html')
    const iframe = document.querySelector('iframe')
    expect(iframe).not.toBeNull()
    expect(iframe!.hasAttribute('srcdoc')).toBe(false)
  })
})
