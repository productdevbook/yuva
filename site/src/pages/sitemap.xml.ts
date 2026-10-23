import type { APIRoute } from "astro"
import { posts as published, localeOf, slugOf, translatedPosts } from "@/lib/blog"
import { docRel, ordered } from "@/lib/docs"
import { templates as gallery } from "@/lib/templates"
import {
  absolute,
  blog,
  brand,
  contact,
  ai,
  cloudflareEmail,
  status,
  docsPath,
  home,
  locales,
  post,
  releases,
  template,
  templates,
} from "@/lib/routes"

export const GET: APIRoute = async () => {
  const posts = [...(await published()), ...(await translatedPosts())].map(
    (p) => `<url><loc>${absolute(post(slugOf(p.id), localeOf(p.id)))}</loc></url>`
  )
  const docs = ["", ...(await ordered()).map((d) => docRel(d.id)), "api/"].map(docsPath)
  const urls = [
    home,
    ai,
    cloudflareEmail,
    status,
    blog,
    releases,
    brand,
    contact,
    templates,
    ...gallery().map((g) => template(g.slug)),
    ...docs,
  ]
    .flatMap((page) => {
      const alternates = locales
        .map((l) => `<xhtml:link rel="alternate" hreflang="${l}" href="${absolute(page[l])}"/>`)
        .join("")
      return locales.map((l) => `<url><loc>${absolute(page[l])}</loc>${alternates}</url>`)
    })
    .concat(posts)
    .join("")
  const xml = `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">${urls}</urlset>`
  return new Response(xml, { headers: { "Content-Type": "application/xml" } })
}
