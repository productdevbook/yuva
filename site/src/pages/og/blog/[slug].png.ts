import type { APIRoute, GetStaticPaths } from "astro"
import { posts, type Post } from "@/lib/blog"
import { ogSvg, renderPng } from "@/lib/graphics"

export const getStaticPaths: GetStaticPaths = async () => (await posts()).map((entry) => ({ params: { slug: entry.id }, props: { entry } }))

export const GET: APIRoute = ({ props }) => {
  const { data } = (props as { entry: Post }).entry
  const day = new Intl.DateTimeFormat("en-US", { dateStyle: "long", timeZone: "UTC" }).format(data.date)
  return new Response(renderPng(ogSvg(data.title, "Yuva blog", `${day} · ${data.author}`), 1200), { headers: { "Content-Type": "image/png" } })
}
