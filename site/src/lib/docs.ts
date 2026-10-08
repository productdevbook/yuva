import { execFileSync } from "node:child_process"
import { getCollection, type CollectionEntry } from "astro:content"
import { docsCopy } from "@/copy/docs"
import { i18nFor } from "@/i18n"
import { tags, tagTitle } from "./openapi"
import { docsRoot, REPO, type Locale } from "./routes"

export type Doc = CollectionEntry<"docs">
export type NavItem = { slug: string; title: string; rel: string; children?: NavItem[] }
export type NavGroup = { key: string; title: string; items: NavItem[] }

const groups = [
  { key: "start", slugs: ["install", "configuration", "operations"] },
  { key: "channels", slugs: ["email", "widget", "mobile"] },
  { key: "integrate", slugs: ["identity", "webhooks", "api"] },
  { key: "project", slugs: ["architecture", "roadmap"] },
] as const

export const docRel = (slug: string) => (slug === "api" ? "api/" : `${slug}/`)
export const tagRel = (name: string) => `api/${name}/`
export const eventsRel = "api/events/"
export const href = (locale: Locale, rel = "") => `${docsRoot(locale)}${rel}`
export const editHref = (slug: string) => `${REPO}/edit/main/docs/${slug}.md`

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

export function updated(slug: string) {
  try {
    const out = execFileSync("git", ["log", "-1", "--format=%cs", "--", `../docs/${slug}.md`], { encoding: "utf8" }).trim()
    return out || undefined
  } catch {
    return undefined
  }
}

export async function docs() {
  const all = await getCollection("docs")
  return new Map(all.map((d) => [d.id, d]))
}

export async function nav(locale: Locale): Promise<NavGroup[]> {
  const t = docsCopy(i18nFor(locale))
  const all = await docs()
  const listed: string[] = groups.flatMap((g) => g.slugs)
  const rest = [...all.keys()].filter((slug) => !listed.includes(slug)).sort()
  const item = (slug: string): NavItem =>
    slug === "api"
      ? {
          slug,
          title: t.apiReference,
          rel: docRel(slug),
          children: [
            ...tags().map((tag) => ({ slug: `api/${tag.name}`, title: tagTitle(tag.name), rel: tagRel(tag.name) })),
            { slug: "api/events", title: t.webhookEvents, rel: eventsRel },
          ],
        }
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

export async function ordered() {
  const all = await docs()
  const listed: string[] = groups.flatMap((g) => g.slugs).filter((slug) => all.has(slug))
  const rest = [...all.keys()].filter((slug) => !listed.includes(slug)).sort()
  return [...listed, ...rest].map((slug) => all.get(slug)!)
}

export const proseClass =
  "prose prose-yuva max-w-none prose-headings:scroll-mt-24 prose-headings:tracking-tight prose-h1:text-4xl prose-h1:font-semibold prose-h1:tracking-[-0.03em] prose-h2:mt-14 prose-a:font-medium prose-a:underline-offset-4 prose-code:before:content-none prose-code:after:content-none prose-li:my-1 prose-table:text-sm"
