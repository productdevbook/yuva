import type { APIRoute } from "astro"
import { getCollection } from "astro:content"
import { apiHome, docHref, docsHome, eventsHref, ordered, tagHref } from "@/lib/docs"
import { tags } from "@/lib/openapi"
import { absolute, blog, brand, contact, home, locales, post, releases } from "@/lib/routes"

export const GET: APIRoute = async () => {
  const posts = (await getCollection("blog")).map((p) => `<url><loc>${absolute(post(p.id))}</loc></url>`)
  const docs = [docsHome, ...(await ordered()).map((d) => docHref(d.id)), apiHome, ...tags().map((t) => tagHref(t.name)), eventsHref].map(
    (href) => `<url><loc>${absolute(href)}</loc></url>`
  )
  const urls = [home, blog, releases, brand, contact]
    .flatMap((page) => {
      const alternates = locales.map((l) => `<xhtml:link rel="alternate" hreflang="${l}" href="${absolute(page[l])}"/>`).join("")
      return locales.map((l) => `<url><loc>${absolute(page[l])}</loc>${alternates}</url>`)
    })
    .concat(posts, docs)
    .join("")
  const xml = `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">${urls}</urlset>`
  return new Response(xml, { headers: { "Content-Type": "application/xml" } })
}
