import { getCollection, type CollectionEntry } from "astro:content"
import { defaultLocale, type Locale } from "./routes"

type Entry = CollectionEntry<"blog">
export type Post = Entry & { data: Entry["data"] & { date: Date } }
export type Listed = { entry: Post; slug: string; lang: Locale }

const isPublished = (entry: Entry): entry is Post => !entry.data.draft && entry.data.date !== undefined

export const localeOf = (id: string) => (id.includes("/") ? (id.split("/")[0] as Locale) : defaultLocale)
export const slugOf = (id: string) => id.split("/").pop()!

async function published() {
  return (await getCollection("blog")).filter(isPublished)
}

export async function posts() {
  return (await published())
    .filter((entry) => localeOf(entry.id) === defaultLocale)
    .sort((a, b) => b.data.date.valueOf() - a.data.date.valueOf())
}

export async function translatedPosts() {
  return (await published()).filter((entry) => localeOf(entry.id) !== defaultLocale)
}

export async function listFor(locale: Locale): Promise<Listed[]> {
  const all = await published()
  return (await posts()).map((english) => {
    const slug = slugOf(english.id)
    const own = all.find((entry) => entry.id === `${locale}/${slug}`)
    return own ? { entry: own, slug, lang: locale } : { entry: english, slug, lang: defaultLocale }
  })
}

export async function localesOf(slug: string) {
  const translated = (await translatedPosts()).filter((entry) => slugOf(entry.id) === slug)
  return [defaultLocale, ...translated.map((entry) => localeOf(entry.id))]
}

export function minutes(post: Post) {
  const words = (post.body ?? "").split(/\s+/).filter(Boolean).length
  return Math.max(1, Math.round(words / 220))
}

export const isoDay = (d: Date) => d.toISOString().slice(0, 10)
