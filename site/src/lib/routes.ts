export const locales = ["en", "tr", "de"] as const
export type Locale = (typeof locales)[number]
export const defaultLocale: Locale = "en"

export const SITE = process.env.SITE_URL ?? "http://localhost:4321"
export const bcp47: Record<Locale, string> = { en: "en-US", tr: "tr-TR", de: "de-DE" }
export const ogLocale: Record<Locale, string> = { en: "en_US", tr: "tr_TR", de: "de_DE" }
export const home: Record<Locale, string> = { en: "/", tr: "/tr/", de: "/de/" }
export const brand: Record<Locale, string> = { en: "/brand/", tr: "/tr/brand/", de: "/de/brand/" }
export const blog: Record<Locale, string> = { en: "/blog/", tr: "/tr/blog/", de: "/de/blog/" }
export const releases: Record<Locale, string> = { en: "/releases/", tr: "/tr/releases/", de: "/de/releases/" }
export const contact: Record<Locale, string> = { en: "/contact/", tr: "/tr/contact/", de: "/de/contact/" }
export const post = (slug: string) => `/blog/${slug}/`
export const docsRoot = (locale: Locale) => (locale === "en" ? "/docs/" : `/${locale}/docs/`)
export const docsPath = (rel: string): Record<Locale, string> => ({ en: `/docs/${rel}`, tr: `/tr/docs/${rel}`, de: `/de/docs/${rel}` })
export const brandAssets = {
  icon: "/brand/yuva-icon.svg",
  iconSquare: "/brand/yuva-icon-square.svg",
  iconPng: "/brand/yuva-icon-1024.png",
  logo: "/brand/yuva-logo.svg",
  logoWhite: "/brand/yuva-logo-white.svg",
}

export const VERSION = "0.0.6"

export const REPO = "https://github.com/productdevbook/yuva"
export const doc = (name: string) => `${REPO}/blob/main/${name}`
export const links = {
  repo: REPO,
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
  issues: `${REPO}/issues`,
  release: `${REPO}/releases/tag/v${VERSION}`,
  changelog: doc("CHANGELOG.md"),
  releasesOnGitHub: `${REPO}/releases`,
}

export function languageName(locale: Locale) {
  return new Intl.DisplayNames([locale], { type: "language" }).of(locale)!.replace(/^./, (c) => c.toLocaleUpperCase(locale))
}

export function absolute(path: string) {
  return new URL(path, SITE).toString()
}
