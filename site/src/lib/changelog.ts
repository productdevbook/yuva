import fs from "node:fs"
import path from "node:path"
import { marked, type Tokens } from "marked"
import { REPO } from "@/lib/routes"

export type Kind = "Added" | "Changed" | "Deprecated" | "Removed" | "Fixed" | "Security"
export type Item = { title?: string; html: string }
export type Group = { kind: Kind; items: Item[] }
export type Release = { version: string; date: string; intro: string; groups: Group[]; compare?: string; tag: string }

const heading = /^## \[([^\]]+)\](?: - (\d{4}-\d{2}-\d{2}))?\s*$/gm
const reference = /^\[([^\]]+)\]: (\S+)\s*$/gm
const titled = /^\*\*(.+?)\*\*:?\s*/
const capital = (s: string) => s.replace(/^[a-z]/, (c) => c.toUpperCase())

function groups(md: string) {
  const out: Group[] = []
  const intro: string[] = []
  for (const token of marked.lexer(md)) {
    if (token.type === "heading" && token.depth === 3) out.push({ kind: token.text as Kind, items: [] })
    else if (token.type === "list" && out.length)
      out.at(-1)!.items.push(
        ...(token as Tokens.List).items.map((li) => {
          const text = li.text.replace(/\s*\n\s*/g, " ")
          const m = text.match(titled)
          return m ? { title: m[1], html: marked.parseInline(capital(text.slice(m[0].length)), { async: false }) } : { html: marked.parseInline(text, { async: false }) }
        })
      )
    else if (token.type === "paragraph" && !out.length) intro.push(marked.parseInline(token.text, { async: false }))
  }
  return { groups: out, intro: intro.join(" ") }
}

export function releases(): Release[] {
  const text = fs.readFileSync(path.resolve("../CHANGELOG.md"), "utf8")
  const refs = new Map([...text.matchAll(reference)].map((m) => [m[1]!, m[2]!]))
  const body = text.replace(reference, "")
  const marks = [...body.matchAll(heading)]
  return marks
    .map((m, i) => {
      const start = m.index! + m[0].length
      const end = i + 1 < marks.length ? marks[i + 1]!.index! : body.length
      return { version: m[1]!, date: m[2] ?? "", md: body.slice(start, end).trim() }
    })
    .filter((r) => r.version !== "Unreleased" && r.date)
    .map((r) => ({
      version: r.version,
      date: r.date,
      ...groups(r.md),
      compare: refs.get(r.version),
      tag: `${REPO}/releases/tag/v${r.version}`,
    }))
}
