import { execFileSync } from "node:child_process"
import { getCollection, type CollectionEntry } from "astro:content"
import { tags, tagTitle } from "./openapi"
import { REPO } from "./routes"

export type Doc = CollectionEntry<"docs">
export type NavItem = { slug: string; title: string; href: string; children?: NavItem[] }
export type NavGroup = { title: string; items: NavItem[] }

const groups: { title: string; slugs: string[] }[] = [
  { title: "Get started", slugs: ["install", "configuration", "operations"] },
  { title: "Channels", slugs: ["email", "widget", "mobile"] },
  { title: "Integrate", slugs: ["identity", "webhooks", "api"] },
  { title: "Project", slugs: ["architecture", "roadmap"] },
]

export const docsHome = "/docs/"
export const apiHome = "/docs/api/"
export const eventsHref = "/docs/api/events/"
export const tagHref = (name: string) => `/docs/api/${name}/`
export const docHref = (slug: string) => (slug === "api" ? apiHome : `/docs/${slug}/`)
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

function apiItem(): NavItem {
  return {
    slug: "api",
    title: "API reference",
    href: apiHome,
    children: [
      ...tags().map((t) => ({ slug: `api/${t.name}`, title: tagTitle(t.name), href: tagHref(t.name) })),
      { slug: "api/events", title: "Webhook events", href: eventsHref },
    ],
  }
}

export async function nav(): Promise<NavGroup[]> {
  const all = await docs()
  const listed = groups.flatMap((g) => g.slugs)
  const rest = [...all.keys()].filter((slug) => !listed.includes(slug)).sort()
  const item = (slug: string): NavItem => (slug === "api" ? apiItem() : { slug, title: title(all.get(slug)!), href: docHref(slug) })
  return [
    ...groups.map((g) => ({ title: g.title, items: g.slugs.filter((slug) => slug === "api" || all.has(slug)).map(item) })),
    ...(rest.length ? [{ title: "More", items: rest.map(item) }] : []),
  ]
}

export async function neighbours(slug: string) {
  const flat = (await nav()).flatMap((g) => g.items.flatMap((i) => [i, ...(i.children ?? [])]))
  const i = flat.findIndex((item) => item.slug === slug)
  return { prev: i > 0 ? flat[i - 1] : undefined, next: i >= 0 && i < flat.length - 1 ? flat[i + 1] : undefined }
}

export async function ordered() {
  const all = await docs()
  const listed = groups.flatMap((g) => g.slugs).filter((slug) => all.has(slug))
  const rest = [...all.keys()].filter((slug) => !listed.includes(slug)).sort()
  return [...listed, ...rest].map((slug) => all.get(slug)!)
}

export const proseClass =
  "prose prose-yuva max-w-none prose-headings:scroll-mt-24 prose-headings:tracking-tight prose-h1:text-4xl prose-h1:font-semibold prose-h1:tracking-[-0.03em] prose-h2:mt-14 prose-a:font-medium prose-a:underline-offset-4 prose-code:before:content-none prose-code:after:content-none prose-li:my-1 prose-table:text-sm"
