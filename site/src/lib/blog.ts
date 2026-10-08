import { getCollection, type CollectionEntry } from "astro:content"

export type Post = CollectionEntry<"blog">

export async function posts() {
  return (await getCollection("blog")).sort((a, b) => b.data.date.valueOf() - a.data.date.valueOf())
}

export function minutes(post: Post) {
  const words = (post.body ?? "").split(/\s+/).filter(Boolean).length
  return Math.max(1, Math.round(words / 220))
}

export const isoDay = (d: Date) => d.toISOString().slice(0, 10)
