import { execFileSync } from "node:child_process"
import { getCollection, type CollectionEntry } from "astro:content"
import { docsCopy } from "@/copy/docs"
import { i18nFor } from "@/i18n"
import { defaultLocale, docsRoot, locales, REPO, type Locale } from "./routes"

export type Doc = CollectionEntry<"docs">
export type NavItem = { slug: string; title: string; rel: string; children?: NavItem[] }
export type NavGroup = { key: string; title: string; items: NavItem[] }

const groups = [
  { key: "start", slugs: ["install", "configuration", "operations"] },
  { key: "channels", slugs: ["email", "widget", "documentation-pages", "mobile"] },
  { key: "integrate", slugs: ["identity", "webhooks", "headless", "mcp", "api"] },
  { key: "project", slugs: ["architecture", "roadmap"] },
] as const

export const docRel = (slug: string) => (slug === "api" ? "api/" : `${slug}/`)
export const href = (locale: Locale, rel = "") => `${docsRoot(locale)}${rel}`
export const englishOnly = ["architecture", "roadmap"]

export const slugOf = (doc: Doc) => doc.id.split("/").at(-1)!
export const localeOf = (doc: Doc): Locale => locales.find((l) => l !== defaultLocale && doc.id.startsWith(`${l}/`)) ?? defaultLocale
const file = (doc: Doc) => doc.filePath ?? `../docs/${doc.id}.md`
export const editHref = (doc: Doc) => `${REPO}/edit/main/docs/${file(doc).replace(/^(\.\.\/)?docs\//, "")}`

export function title(doc: Doc) {
  return doc.body?.match(/^#\s+(.+)$/m)?.[1]?.trim() ?? doc.id
}

export function plain(markdown: string) {
  return markdown
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/^#+\s+/gm, "")
    .replace(/^\s*[|>-].*$/gm, " ")
    .replace(/\*+/g, "")
    .replace(/\s+/g, " ")
    .trim()
}

export function description(doc: Doc) {
  const body = doc.body ?? ""
  const afterTitle = body.replace(/^#\s+.+$/m, "").trim()
  const paragraph = afterTitle.split(/\n\s*\n/).find((p) => p.trim() && !/^([#|>-]|```)/.test(p.trim())) ?? ""
  return plain(paragraph)
}

export function minutes(doc: Doc) {
  const words = (doc.body ?? "").replace(/```[\s\S]*?```/g, " ").match(/[\p{L}\p{N}]+/gu)?.length ?? 0
  return Math.max(1, Math.round(words / 220))
}

function committed(doc: Doc) {
  try {
    const out = execFileSync("git", ["log", "-1", "--format=%ct %cs", "--", file(doc)], { encoding: "utf8" }).trim()
    if (!out) return undefined
    const [time, day] = out.split(" ")
    return { time: Number(time), day: day! }
  } catch {
    return undefined
  }
}

export function updated(doc: Doc) {
  return committed(doc)?.day
}

export async function docs(locale: Locale = defaultLocale) {
  const all = await getCollection("docs")
  const english = all.filter((d) => localeOf(d) === defaultLocale)
  const local = new Map(all.filter((d) => localeOf(d) === locale && locale !== defaultLocale).map((d) => [slugOf(d), d]))
  return new Map(english.map((d) => [d.id, (!englishOnly.includes(d.id) && local.get(d.id)) || d]))
}

export type Translation = { status: "source" | "translated" | "missing" | "english-only"; english: Doc; newer: boolean }

export async function translation(entry: Doc, locale: Locale): Promise<Translation> {
  const slug = slugOf(entry)
  const english = (await docs()).get(slug) ?? entry
  if (locale === defaultLocale) return { status: "source", english, newer: false }
  if (englishOnly.includes(slug)) return { status: "english-only", english, newer: false }
  if (localeOf(entry) !== locale) return { status: "missing", english, newer: false }
  const a = committed(english)?.time
  const b = committed(entry)?.time
  return { status: "translated", english, newer: !!a && !!b && a > b }
}

export async function translatedCount(locale: Locale) {
  if (locale === defaultLocale) return 0
  return [...(await docs(locale)).values()].filter((d) => localeOf(d) === locale).length
}

export async function nav(locale: Locale): Promise<NavGroup[]> {
  const t = docsCopy(i18nFor(locale))
  const all = await docs(locale)
  const listed: string[] = groups.flatMap((g) => g.slugs)
  const rest = [...all.keys()].filter((slug) => !listed.includes(slug)).sort()
  const item = (slug: string): NavItem =>
    slug === "api"
      ? { slug, title: t.apiReference, rel: docRel(slug) }
      : { slug, title: title(all.get(slug)!), rel: docRel(slug) }
  return [
    ...groups.map((g) => ({ key: g.key, title: t.groups[g.key], items: g.slugs.filter((slug) => slug === "api" || all.has(slug)).map(item) })),
    ...(rest.length ? [{ key: "more", title: t.groups.more, items: rest.map(item) }] : []),
  ]
}

export async function neighbours(slug: string, locale: Locale) {
  const flat = (await nav(locale)).flatMap((g) => g.items.flatMap((i) => [i, ...(i.children ?? [])]))
  const i = flat.findIndex((item) => item.slug === slug)
  return { prev: i > 0 ? flat[i - 1] : undefined, next: i >= 0 && i < flat.length - 1 ? flat[i + 1] : undefined }
}

export async function ordered(locale: Locale = defaultLocale) {
  const all = await docs(locale)
  const listed: string[] = groups.flatMap((g) => g.slugs).filter((slug) => all.has(slug))
  const rest = [...all.keys()].filter((slug) => !listed.includes(slug)).sort()
  return [...listed, ...rest].map((slug) => all.get(slug)!)
}

export const proseClass =
  "prose prose-yuva max-w-none text-base prose-p:leading-[1.75] prose-li:leading-[1.7] prose-li:my-1 prose-headings:scroll-mt-24 prose-headings:tracking-tight prose-a:font-medium prose-a:no-underline hover:prose-a:underline prose-code:before:content-none prose-code:after:content-none prose-strong:font-semibold prose-table:my-6 [&_:not(pre)>code]:rounded-md [&_:not(pre)>code]:border [&_:not(pre)>code]:border-rule [&_:not(pre)>code]:bg-paper-deep [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:text-[0.84em] [&_:not(pre)>code]:font-normal [&_:not(pre)>code]:text-ink-soft"
