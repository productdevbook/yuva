import type { APIRoute, GetStaticPaths } from "astro"
import { localeOf, posts, translatedPosts, type Post } from "@/lib/blog"
import { ogSvg, renderPng } from "@/lib/graphics"
import { bcp47, type Locale } from "@/lib/routes"

export const getStaticPaths: GetStaticPaths = async () =>
  [...(await posts()), ...(await translatedPosts())].map((entry) => ({ params: { slug: entry.id }, props: { entry } }))

export const GET: APIRoute = ({ props }) => {
  const { entry } = props as { entry: Post }
  const { data } = entry
  const locale: Locale = localeOf(entry.id)
  const day = new Intl.DateTimeFormat(bcp47[locale], { dateStyle: "long", timeZone: "UTC" }).format(data.date)
  return new Response(renderPng(ogSvg(data.title, "Yuva blog", `${day} · ${data.author}`), 1200), {
    headers: { "Content-Type": "image/png" },
  })
}
