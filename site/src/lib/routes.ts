export const locales = [
  "en",
  "ar",
  "bn",
  "ca",
  "cs",
  "da",
  "de",
  "el",
  "es",
  "es-MX",
  "fi",
  "fr",
  "fr-CA",
  "gu",
  "he",
  "hi",
  "hr",
  "hu",
  "id",
  "it",
  "ja",
  "kn",
  "ko",
  "ml",
  "mr",
  "ms",
  "nb",
  "nl",
  "or",
  "pa",
  "pl",
  "pt-BR",
  "pt-PT",
  "ro",
  "ru",
  "sk",
  "sl",
  "sv",
  "ta",
  "te",
  "th",
  "tr",
  "uk",
  "ur",
  "vi",
  "zh-Hans",
  "zh-Hant",
] as const
export type Locale = (typeof locales)[number]
export const defaultLocale: Locale = "en"
export const rtl: readonly Locale[] = ["ar", "he", "ur"]
export const dir = (locale: Locale) => (rtl.includes(locale) ? "rtl" : "ltr")

export const SITE = process.env.SITE_URL ?? "http://localhost:4321"
const tags: Partial<Record<Locale, string>> = { en: "en-US", tr: "tr-TR", de: "de-DE" }
const regions: Record<string, string> = {
  en: "US",
  ar: "AR",
  bn: "IN",
  ca: "ES",
  cs: "CZ",
  da: "DK",
  de: "DE",
  el: "GR",
  es: "ES",
  "es-MX": "MX",
  fi: "FI",
  fr: "FR",
  "fr-CA": "CA",
  gu: "IN",
  he: "IL",
  hi: "IN",
  hr: "HR",
  hu: "HU",
  id: "ID",
  it: "IT",
  ja: "JP",
  kn: "IN",
  ko: "KR",
  ml: "IN",
  mr: "IN",
  ms: "MY",
  nb: "NO",
  nl: "NL",
  or: "IN",
  pa: "IN",
  pl: "PL",
  "pt-BR": "BR",
  "pt-PT": "PT",
  ro: "RO",
  ru: "RU",
  sk: "SK",
  sl: "SI",
  sv: "SE",
  ta: "IN",
  te: "IN",
  th: "TH",
  tr: "TR",
  uk: "UA",
  ur: "PK",
  vi: "VN",
  "zh-Hans": "CN",
  "zh-Hant": "TW",
}
export const bcp47 = Object.fromEntries(locales.map((l) => [l, tags[l] ?? l])) as Record<Locale, string>
export const ogLocale = Object.fromEntries(locales.map((l) => [l, `${l.split("-")[0]}_${regions[l]}`])) as Record<
  Locale,
  string
>

export const prefix = (locale: Locale) => (locale === defaultLocale ? "" : `/${locale}`)
export const localized = (rel: string): Record<Locale, string> =>
  Object.fromEntries(locales.map((l) => [l, `${prefix(l)}${rel}`])) as Record<Locale, string>
export const home = localized("/")
export const brand = localized("/brand/")
export const blog = localized("/blog/")
export const releases = localized("/releases/")
export const contact = localized("/contact/")
export const ai = localized("/ai/")
export const status = localized("/status/")
export const cloudflareEmail = localized("/cloudflare-email/")
export const templates = localized("/templates/")
export const template = (slug: string): Record<Locale, string> => localized(`/templates/${slug}/`)
export const localePaths = () => locales.filter((l) => l !== defaultLocale).map((locale) => ({ params: { locale } }))
export const APP_URL = "https://app.useyuva.com"
export const appUrl = (locale: Locale, params: Record<string, string> = {}, path = "/") => {
  const query = new URLSearchParams({ ...params, lang: locale })
  return `${APP_URL}${path}?${query}`
}
export const post = (slug: string, locale: Locale = defaultLocale) => `${prefix(locale)}/blog/${slug}/`
export const docsRoot = (locale: Locale) => `${prefix(locale)}/docs/`
export const docsPath = (rel: string): Record<Locale, string> => localized(`/docs/${rel}`)
export const brandAssets = {
  icon: "/brand/yuva-icon.svg",
  iconSquare: "/brand/yuva-icon-square.svg",
  iconPng: "/brand/yuva-icon-1024.png",
  logo: "/brand/yuva-logo.svg",
  logoWhite: "/brand/yuva-logo-white.svg",
}

export const VERSION = "0.1.0"

export const REPO = "https://github.com/productdevbook/yuva"
export const doc = (name: string) => `${REPO}/blob/main/${name}`
export const X_URL = "https://x.com/useyuva"
export const links = {
  repo: REPO,
  x: X_URL,
  docs: "/docs/",
  api: "/docs/api/",
  install: "/docs/install/",
  architecture: "/docs/architecture/",
  roadmap: "/docs/roadmap/",
  configuration: "/docs/configuration/",
  email: "/docs/email/",
  widget: "/docs/widget/",
  mobile: "/docs/mobile/",
  identity: "/docs/identity/",
  webhooks: "/docs/webhooks/",
  operations: "/docs/operations/",
  openapi: doc("openapi/openapi.yaml"),
  license: doc("LICENSE"),
  licensing: doc("LICENSING.md"),
  cla: doc("CLA.md"),
  trademark: doc("TRADEMARK.md"),
  geist: "https://vercel.com/font",
  security: doc("SECURITY.md"),
  contributing: doc("CONTRIBUTING.md"),
  templatesGuide: doc("templates/email/README.md"),
  templatesLicense: doc("templates/email/LICENSE"),
  issues: `${REPO}/issues`,
  release: `${REPO}/releases/tag/v${VERSION}`,
  changelog: doc("CHANGELOG.md"),
  releasesOnGitHub: `${REPO}/releases`,
}

export function languageName(locale: Locale) {
  return new Intl.DisplayNames([locale], { type: "language" })
    .of(locale)!
    .replace(/^./, (c) => c.toLocaleUpperCase(locale))
}

export function absolute(path: string) {
  return new URL(path, SITE).toString()
}
