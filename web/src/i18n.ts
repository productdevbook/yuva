import { i18n } from "@lingui/core"

import { messages as en } from "@/locales/en/messages.po"
import { messages as tr } from "@/locales/tr/messages.po"

export const locales = { en: "English", tr: "Türkçe" } as const
export type Locale = keyof typeof locales

i18n.load({ en, tr })

export function initialLocale(): Locale {
  try {
    const saved = localStorage.getItem("locale")
    if (saved === "tr" || saved === "en") {
      return saved
    }
  } catch {
    // Storage can be blocked; fall back to the browser language.
  }
  return navigator.language.toLowerCase().startsWith("tr") ? "tr" : "en"
}

export function activate(locale: Locale) {
  i18n.activate(locale)
  document.documentElement.lang = locale
  try {
    localStorage.setItem("locale", locale)
  } catch {
    // Storage can be blocked; the choice then lasts for this page only.
  }
}

export { i18n }
