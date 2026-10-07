export const locales = ["en", "tr"] as const
export type Locale = (typeof locales)[number]
export const defaultLocale: Locale = "en"

export const SITE = process.env.SITE_URL ?? "http://localhost:4321"
export const bcp47: Record<Locale, string> = { en: "en-US", tr: "tr-TR" }
export const ogLocale: Record<Locale, string> = { en: "en_US", tr: "tr_TR" }
export const home: Record<Locale, string> = { en: "/", tr: "/tr/" }

export const REPO = "https://github.com/productdevbook/yuva"
export const doc = (name: string) => `${REPO}/blob/main/${name}`
export const links = {
  repo: REPO,
  install: doc("docs/install.md"),
  quickStart: doc("docs/install.md#quick-start-with-compose"),
  architecture: doc("docs/architecture.md"),
  roadmap: doc("docs/roadmap.md"),
  configuration: doc("docs/configuration.md"),
  email: doc("docs/email.md"),
  widget: doc("docs/widget.md"),
  mobile: doc("docs/mobile.md"),
  identity: doc("docs/identity.md"),
  webhooks: doc("docs/webhooks.md"),
  operations: doc("docs/operations.md"),
  openapi: doc("openapi/openapi.yaml"),
  license: doc("LICENSE"),
  licensing: doc("LICENSING.md"),
  cla: doc("CLA.md"),
  trademark: doc("TRADEMARK.md"),
  security: doc("SECURITY.md"),
  contributing: doc("CONTRIBUTING.md"),
  issues: `${REPO}/issues`,
  release: `${REPO}/releases/tag/v0.0.1`,
}

export function other(locale: Locale): Locale {
  return locale === "en" ? "tr" : "en"
}

export function absolute(path: string) {
  return new URL(path, SITE).toString()
}
