import { setupI18n, type I18n, type Messages } from "@lingui/core"
import { messages as en } from "@/locales/en/messages.po"
import { messages as de } from "@/locales/de/messages.po"
import { messages as tr } from "@/locales/tr/messages.po"
import { bcp47, type Locale } from "@/lib/routes"

const catalogs: Record<Locale, Messages> = { en, tr, de }
const cache = new Map<Locale, I18n>()

export function i18nFor(locale: Locale): I18n {
  let i18n = cache.get(locale)
  if (!i18n) {
    i18n = setupI18n({ locale, locales: [bcp47[locale]], messages: { [locale]: catalogs[locale] } })
    cache.set(locale, i18n)
  }
  return i18n
}
