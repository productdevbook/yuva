import type { APIRoute } from "astro"
import { getCollection } from "astro:content"
import { docRel, ordered } from "@/lib/docs"
import { absolute, blog, brand, contact, docsPath, home, locales, post, releases } from "@/lib/routes"

export const GET: APIRoute = async () => {
  const posts = (await getCollection("blog")).map((p) => `<url><loc>${absolute(post(p.id))}</loc></url>`)
  const docs = ["", ...(await ordered()).map((d) => docRel(d.id)), "api/"].map(docsPath)
  const urls = [home, blog, releases, brand, contact, ...docs]
    .flatMap((page) => {
      const alternates = locales.map((l) => `<xhtml:link rel="alternate" hreflang="${l}" href="${absolute(page[l])}"/>`).join("")
      return locales.map((l) => `<url><loc>${absolute(page[l])}</loc>${alternates}</url>`)
    })
    .concat(posts)
    .join("")
  const xml = `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">${urls}</urlset>`
  return new Response(xml, { headers: { "Content-Type": "application/xml" } })
}
