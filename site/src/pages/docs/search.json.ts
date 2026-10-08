import type { APIRoute } from "astro"
import { render } from "astro:content"
import { docRel, ordered, plain, title } from "@/lib/docs"
import { tags, tagTitle } from "@/lib/openapi"

type Entry = { page: string; heading: string; href: string; text: string }

const strip = (html: string) => html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim()

export const GET: APIRoute = async () => {
  const entries: Entry[] = []

  for (const doc of await ordered()) {
    const page = title(doc)
    const href = docRel(doc.id)
    const { headings } = await render(doc)
    const sections: { heading: string; slug?: string; lines: string[] }[] = [{ heading: page, lines: [] }]
    let fenced = false
    let next = 0
    for (const line of (doc.body ?? "").split("\n")) {
      if (/^\s*```/.test(line)) fenced = !fenced
      const match = !fenced && line.match(/^(#{1,6})\s+(.+)$/)
      if (match) {
        const h = headings[next++]
        if (match[1]!.length === 1) continue
        sections.push({ heading: h?.text ?? match[2]!, slug: h?.slug, lines: [] })
      } else {
        sections.at(-1)!.lines.push(line)
      }
    }
    for (const s of sections) {
      entries.push({ page, heading: s.heading, href: s.slug ? `${href}#${s.slug}` : href, text: plain(s.lines.join("\n")).slice(0, 600) })
    }
  }

  for (const tag of tags()) {
    for (const op of tag.operations) {
      entries.push({
        page: `API · ${tagTitle(tag.name)}`,
        heading: `${op.method} ${op.path}`,
        href: `api/#tag/${tag.name}/${op.method}${op.path}`,
        text: `${op.summary} ${strip(op.description)}`.slice(0, 600),
      })
    }
  }


  return new Response(JSON.stringify(entries), { headers: { "Content-Type": "application/json" } })
}
