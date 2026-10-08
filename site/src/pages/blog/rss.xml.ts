import rss from "@astrojs/rss"
import type { APIRoute } from "astro"
import { posts as all } from "@/lib/blog"
import { post, SITE } from "@/lib/routes"

export const GET: APIRoute = async () => {
  const posts = await all()
  return rss({
    title: "Yuva blog",
    description: "News, release stories and notes from building Yuva.",
    site: SITE,
    items: posts.map((p) => ({ title: p.data.title, description: p.data.description, pubDate: p.data.date, link: post(p.id) })),
  })
}
