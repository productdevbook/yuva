import type { EmailBlock, EmailDesignDocument, EmailDesignTranslation } from "@/lib/api"

export type Doc = EmailDesignDocument
export type Block = EmailBlock
export type BlockType = Block["type"]
export type BlockOf<T extends BlockType> = Extract<Block, { type: T }>
export type Translation = EmailDesignTranslation
export type TextType = "heading" | "text" | "button" | "image" | "footer"

export const ROOT = "root"

export const BLOCK_TYPES: BlockType[] = [
  "heading",
  "text",
  "image",
  "button",
  "columns",
  "section",
  "divider",
  "spacer",
  "social",
  "footer",
  "message",
]

export const MAX_BLOCKS = 200

const ID_CHARS = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

export function newId() {
  const bytes = crypto.getRandomValues(new Uint8Array(10))
  return Array.from(bytes, (b) => ID_CHARS[b % ID_CHARS.length]).join("")
}

export function containerOf(kind: "section", id: string): string
export function containerOf(kind: "column", id: string, index: number): string
export function containerOf(kind: "section" | "column", id: string, index = 0) {
  return kind === "section" ? `s:${id}` : `c:${id}:${index}`
}

function parseContainer(key: string) {
  if (key === ROOT) return { kind: "root" as const }
  if (key.startsWith("s:")) return { kind: "section" as const, id: key.slice(2) }
  const [, id, index] = key.split(":")
  return { kind: "column" as const, id, index: Number(index) }
}

export function accepts(container: string, type: BlockType) {
  const c = parseContainer(container)
  if (c.kind === "root") return true
  if (c.kind === "section") return type !== "section"
  return type !== "section" && type !== "columns" && type !== "message"
}

export function hasMessageBlock(doc: Doc) {
  return walk(doc).some((e) => e.block.type === "message")
}

export function childContainers(b: Block): string[] {
  if (b.type === "section") return [containerOf("section", b.id)]
  if (b.type === "columns") return b.columns.map((_, i) => containerOf("column", b.id, i))
  return []
}

export function children(doc: Doc, container: string): Block[] {
  const c = parseContainer(container)
  if (c.kind === "root") return doc.blocks
  const owner = findBlock(doc, c.id)
  if (!owner) return []
  if (c.kind === "section") return owner.type === "section" ? (owner.blocks as Block[]) : []
  return owner.type === "columns" ? ((owner.columns[c.index]?.blocks ?? []) as Block[]) : []
}

function mapBlocks(blocks: Block[], fn: (b: Block) => Block | null): Block[] {
  const out: Block[] = []
  for (const b of blocks) {
    const next = fn(b)
    if (!next) continue
    if (next.type === "section") {
      out.push({ ...next, blocks: mapBlocks(next.blocks as Block[], fn) as typeof next.blocks })
    } else if (next.type === "columns") {
      out.push({
        ...next,
        columns: next.columns.map((col) => ({
          ...col,
          blocks: mapBlocks(col.blocks as Block[], fn) as typeof col.blocks,
        })),
      })
    } else out.push(next)
  }
  return out
}

export function setChildren(doc: Doc, container: string, blocks: Block[]): Doc {
  const c = parseContainer(container)
  if (c.kind === "root") return { ...doc, blocks }
  return {
    ...doc,
    blocks: mapBlocks(doc.blocks, (b) => {
      if (b.id !== c.id) return b
      if (c.kind === "section" && b.type === "section") return { ...b, blocks: blocks as typeof b.blocks }
      if (c.kind === "column" && b.type === "columns")
        return {
          ...b,
          columns: b.columns.map((col, i) => (i === c.index ? { ...col, blocks: blocks as typeof col.blocks } : col)),
        }
      return b
    }),
  }
}

export type Entry = { block: Block; container: string; index: number; depth: number; parent?: string }

export function walk(doc: Doc): Entry[] {
  const out: Entry[] = []
  const visit = (blocks: Block[], container: string, depth: number, parent?: string) => {
    blocks.forEach((block, index) => {
      out.push({ block, container, index, depth, parent })
      if (block.type === "section")
        visit(block.blocks as Block[], containerOf("section", block.id), depth + 1, block.id)
      if (block.type === "columns")
        block.columns.forEach((col, i) =>
          visit(col.blocks as Block[], containerOf("column", block.id, i), depth + 1, block.id)
        )
    })
  }
  visit(doc.blocks, ROOT, 0)
  return out
}

export function locate(doc: Doc, id: string) {
  return walk(doc).find((e) => e.block.id === id) ?? null
}

export function findBlock(doc: Doc, id: string): Block | null {
  return locate(doc, id)?.block ?? null
}

export function countBlocks(doc: Doc) {
  return walk(doc).length
}

export function updateBlock<T extends Block>(doc: Doc, id: string, patch: Partial<T> | ((b: T) => T)): Doc {
  return {
    ...doc,
    blocks: mapBlocks(doc.blocks, (b) => {
      if (b.id !== id) return b
      return typeof patch === "function" ? patch(b as T) : ({ ...b, ...patch } as Block)
    }),
  }
}

export function removeBlock(doc: Doc, id: string): Doc {
  return { ...doc, blocks: mapBlocks(doc.blocks, (b) => (b.id === id ? null : b)) }
}

export function insertBlock(doc: Doc, container: string, index: number, block: Block): Doc {
  const list = [...children(doc, container)]
  list.splice(Math.max(0, Math.min(index, list.length)), 0, block)
  return setChildren(doc, container, list)
}

function contains(block: Block, container: string) {
  return walk({ ...EMPTY_DOC, blocks: [block] }).some((e) => childContainers(e.block).includes(container))
}

export function canMove(doc: Doc, id: string, container: string) {
  const b = findBlock(doc, id)
  if (!b || !accepts(container, b.type)) return false
  return !contains(b, container)
}

export function moveBlock(doc: Doc, id: string, container: string, index: number): Doc {
  const from = locate(doc, id)
  if (!from || !canMove(doc, id, container)) return doc
  const target = from.container === container && from.index < index ? index - 1 : index
  if (from.container === container && from.index === target) return doc
  return insertBlock(removeBlock(doc, id), container, target, from.block)
}

export function nudge(doc: Doc, id: string, by: -1 | 1): Doc {
  const at = locate(doc, id)
  if (!at) return doc
  const list = children(doc, at.container)
  const to = at.index + by
  if (to < 0 || to >= list.length) return doc
  return moveBlock(doc, id, at.container, by > 0 ? to + 1 : to)
}

export function cloneBlock(b: Block): Block {
  const copy = structuredClone(b)
  for (const e of walk({ ...EMPTY_DOC, blocks: [copy] })) e.block.id = newId()
  return copy
}

export function duplicateBlock(doc: Doc, id: string): { doc: Doc; id: string } {
  const at = locate(doc, id)
  if (!at) return { doc, id }
  const copy = cloneBlock(at.block)
  return { doc: insertBlock(doc, at.container, at.index + 1, copy), id: copy.id }
}

export function insertNear(doc: Doc, selected: string | null, block: Block): Doc {
  if (selected) {
    let at = locate(doc, selected)
    while (at) {
      if (accepts(at.container, block.type)) return insertBlock(doc, at.container, at.index + 1, block)
      at = at.parent ? locate(doc, at.parent) : null
    }
  }
  return insertBlock(doc, ROOT, doc.blocks.length, block)
}

export function resolveDrop(
  doc: Doc,
  type: BlockType,
  target: { container: string; index: number },
  moving?: string
): { container: string; index: number } | null {
  let container = target.container
  let index = target.index
  for (;;) {
    const ok = accepts(container, type) && (!moving || canMove(doc, moving, container))
    if (ok) return { container, index }
    const c = parseContainer(container)
    if (c.kind === "root") return null
    const owner = locate(doc, c.id)
    if (!owner) return null
    container = owner.container
    index = owner.index + 1
  }
}

export function setColumnCount(doc: Doc, id: string, count: 2 | 3): Doc {
  return updateBlock<BlockOf<"columns">>(doc, id, (b) => {
    if (b.columns.length === count) return b
    const widths = count === 2 ? [50, 50] : [33, 34, 33]
    if (count > b.columns.length) {
      return { ...b, columns: [...b.columns, { width: 33, blocks: [] }].map((c, i) => ({ ...c, width: widths[i] })) }
    }
    const kept = b.columns.slice(0, count)
    const extra = b.columns.slice(count).flatMap((c) => c.blocks)
    kept[count - 1] = { ...kept[count - 1], blocks: [...kept[count - 1].blocks, ...extra] }
    return { ...b, columns: kept.map((c, i) => ({ ...c, width: widths[i] })) }
  })
}

export const EMPTY_DOC: Doc = {
  schema: 1,
  theme: {
    width: 600,
    direction: "auto",
    font: "system",
    radius: 8,
    colors: {
      page: "#f4f4f5",
      card: "#ffffff",
      text: "#18181b",
      muted: "#71717a",
      accent: "#2563eb",
      link: "#2563eb",
      border: "#e4e4e7",
    },
  },
  blocks: [],
}

export type Defaults = {
  heading: string
  text: string
  button: string
  footer: string
  columnText: string
}

export function makeBlock(type: BlockType, d: Defaults): Block {
  const id = newId()
  const p = (s: string) => `<p>${s}</p>`
  switch (type) {
    case "section":
      return { id, type, blocks: [], padding: [24, 24, 24, 24] }
    case "columns":
      return {
        id,
        type,
        stack: true,
        gap: 16,
        columns: [
          { width: 50, blocks: [{ id: newId(), type: "text", html: p(d.columnText) }] },
          { width: 50, blocks: [{ id: newId(), type: "text", html: p(d.columnText) }] },
        ],
      }
    case "heading":
      return { id, type, level: 2, text: d.heading }
    case "text":
      return { id, type, html: p(d.text) }
    case "image":
      return { id, type, alt: "" }
    case "button":
      return { id, type, label: d.button, url: "https://example.com" }
    case "divider":
      return { id, type, thickness: 1 }
    case "spacer":
      return { id, type, height: 24 }
    case "social":
      return { id, type, items: [{ network: "website", url: "https://example.com" }], style: "icons", color: "dark" }
    case "footer":
      return { id, type, html: p(d.footer) }
    case "message":
      return { id, type }
  }
}

export function imageIds(doc: Doc) {
  const ids = new Set<string>()
  for (const e of walk(doc)) if (e.block.type === "image" && e.block.image_id) ids.add(e.block.image_id)
  return ids
}

const URL_RE = /^(https?:\/\/[^\s]+|mailto:[^\s]+|tel:[^\s]+)$/i

export function validUrl(v: string) {
  return v.length <= 2048 && URL_RE.test(v.trim())
}

export function fileName(name: string) {
  const base = name
    .normalize("NFKD")
    .replace(/[^\w\s-]/g, "")
    .trim()
    .replace(/\s+/g, "-")
    .toLowerCase()
  return `${base || "design"}.json`
}

export const MAX_VERSIONS = 20

const TEXT_TYPES: BlockType[] = ["heading", "text", "button", "image", "footer"]

export function hasText(type: BlockType): type is TextType {
  return TEXT_TYPES.includes(type)
}

export function textOf(b: Block): string {
  switch (b.type) {
    case "heading":
      return b.text
    case "text":
    case "footer":
      return b.html
    case "button":
      return b.label
    case "image":
      return b.alt ?? ""
    default:
      return ""
  }
}

function baseText(b: Block) {
  return b.type === "image" && b.decorative ? "" : textOf(b)
}

function withText(b: Block, s: string): Block {
  switch (b.type) {
    case "heading":
      return { ...b, text: s }
    case "text":
    case "footer":
      return { ...b, html: s }
    case "button":
      return { ...b, label: s }
    case "image":
      return { ...b, alt: s }
    default:
      return b
  }
}

export const sameTag = (a: string | undefined, b: string | undefined) =>
  !!a && !!b && a.toLowerCase() === b.toLowerCase()

export function versions(doc: Doc): string[] {
  const keys = Object.keys(doc.translations ?? {})
  if (!doc.locale || keys.length === 0) return []
  return [doc.locale, ...keys.sort()]
}

export function fallbackOf(doc: Doc) {
  return doc.fallback_locale ?? doc.locale
}

export function translationOf(doc: Doc, tag: string | null): Translation | undefined {
  return tag ? doc.translations?.[tag] : undefined
}

export function localized(doc: Doc, tag: string | null): Doc {
  const tr = translationOf(doc, tag)
  if (!tr) return doc
  return {
    ...doc,
    preheader: tr.preheader || doc.preheader,
    theme: tr.direction ? { ...doc.theme, direction: tr.direction } : doc.theme,
    blocks: mapBlocks(doc.blocks, (b) => (b.id in tr.blocks ? withText(b, tr.blocks[b.id]) : b)),
  }
}

export function missing(doc: Doc, tag: string): string[] {
  const tr = doc.translations?.[tag]
  if (!tr) return []
  return walk(doc)
    .filter((e) => hasText(e.block.type) && baseText(e.block) !== "" && !(e.block.id in tr.blocks))
    .map((e) => e.block.id)
}

export function setTranslation(doc: Doc, tag: string, fn: (t: Translation) => Translation): Doc {
  const cur = doc.translations?.[tag]
  if (!cur) return doc
  return { ...doc, translations: { ...doc.translations, [tag]: fn(cur) } }
}

export function setTranslatedText(doc: Doc, tag: string, id: string, s: string): Doc {
  return setTranslation(doc, tag, (t) => ({ ...t, blocks: { ...t.blocks, [id]: s } }))
}

export function addVersion(doc: Doc, tag: string, base?: string): Doc {
  return {
    ...doc,
    locale: doc.locale ?? base,
    translations: { ...doc.translations, [tag]: { blocks: {} } },
  }
}

export function removeVersion(doc: Doc, tag: string): Doc {
  const rest = { ...doc.translations }
  delete rest[tag]
  const fallback = sameTag(doc.fallback_locale, tag) || Object.keys(rest).length === 0 ? undefined : doc.fallback_locale
  return { ...doc, translations: rest, fallback_locale: fallback }
}

export function pruneTranslations(doc: Doc): Doc {
  if (!doc.translations) return doc
  const ids = new Set(
    walk(doc)
      .filter((e) => hasText(e.block.type))
      .map((e) => e.block.id)
  )
  let changed = false
  const out: Record<string, Translation> = {}
  for (const [tag, t] of Object.entries(doc.translations)) {
    const blocks = Object.fromEntries(Object.entries(t.blocks).filter(([id]) => ids.has(id)))
    if (Object.keys(blocks).length !== Object.keys(t.blocks).length) changed = true
    out[tag] = { ...t, blocks }
  }
  return changed ? { ...doc, translations: out } : doc
}

const FOOTER_LANGUAGES = ["en", "tr", "de"]

export function footerInEnglish(tag: string) {
  return !FOOTER_LANGUAGES.includes(tag.split("-")[0].toLowerCase())
}
