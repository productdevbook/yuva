import geistLatinExt from "@fontsource-variable/geist/files/geist-latin-ext-wght-normal.woff2?url"
import geistLatin from "@fontsource-variable/geist/files/geist-latin-wght-normal.woff2?url"
import { useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"

import { cn } from "@/lib/utils"

function cidOf(src: string) {
  const m = /^\s*cid:(.+)$/i.exec(src)
  if (!m) return undefined
  try {
    return decodeURIComponent(m[1].trim()).replace(/^<|>$/g, "")
  } catch {
    return m[1].trim()
  }
}

export function placedContentIds(html: string) {
  const doc = new DOMParser().parseFromString(html, "text/html")
  const ids = new Set<string>()
  for (const img of doc.querySelectorAll("img[src]")) {
    const id = cidOf(img.getAttribute("src") ?? "")
    if (id) ids.add(id)
  }
  return ids
}

function prepare(html: string, images: boolean, inline: ReadonlyMap<string, string>) {
  const doc = new DOMParser().parseFromString(html, "text/html")
  for (const img of doc.querySelectorAll("img[src]")) {
    const id = cidOf(img.getAttribute("src") ?? "")
    const url = id ? inline.get(id) : undefined
    if (url) img.setAttribute("src", new URL(url, window.location.origin).href)
  }
  if (!images) {
    const own = `${window.location.origin}/v1/attachments/`
    const kept = (v: string | null) => !v || v.startsWith(own) || /^\s*(data|cid):/i.test(v)
    for (const el of doc.querySelectorAll("img, [background]")) {
      if (!kept(el.getAttribute("src"))) el.removeAttribute("src")
      if (!kept(el.getAttribute("background"))) el.removeAttribute("background")
      el.removeAttribute("srcset")
    }
  }
  return doc.body.innerHTML
}

type Look = {
  dark: boolean
  font: string
  size: string
  line: string
  fg: string
  muted: string
  border: string
  link: string
  code: string
}

function lookOf(dark: boolean): Look {
  const root = getComputedStyle(document.documentElement)
  const v = (name: string, fallback: string) => root.getPropertyValue(name).trim() || fallback
  return {
    dark,
    font: getComputedStyle(document.body).fontFamily || "ui-sans-serif, system-ui, sans-serif",
    size: v("--text-reading", "0.9375rem"),
    line: v("--text-reading--line-height", "1.5rem"),
    fg: v("--foreground", dark ? "#fafafa" : "#0b0b0f"),
    muted: v("--muted-foreground", dark ? "#a1a1aa" : "#3f3f46"),
    border: v("--border", dark ? "#232327" : "#e4e4e7"),
    link: v("--brand", dark ? "#ff8a5c" : "#c2410c"),
    code: v("--muted", dark ? "#1c1c21" : "#f1f1f3"),
  }
}

function readingStyle(l: Look) {
  return `:root{color-scheme:${l.dark ? "dark" : "light"}}
html,body{margin:0;padding:0;background:transparent;overflow:hidden}
body{font-family:${l.font};font-size:${l.size};line-height:${l.line};color:${l.fg};overflow-wrap:anywhere;word-break:break-word}
#y{display:flow-root}
#y *,#y *::before,#y *::after{box-sizing:border-box;max-width:100%}
img{height:auto!important}
img:not([src]),img[src^="cid:"],img[width="0"],img[width="1"],img[height="0"],img[height="1"]{display:none}
${l.dark ? "img[src]{background:#ededf0;padding:2px;border-radius:4px}" : ""}
table{width:auto!important;table-layout:auto;border-collapse:collapse}
td,th{width:auto!important;min-width:0;vertical-align:top;text-align:start}
pre{white-space:pre-wrap;font-family:ui-monospace,"SF Mono",Menlo,monospace;font-size:.9em;background:${l.code};padding:.5em .75em;border-radius:6px}
code{font-family:ui-monospace,"SF Mono",Menlo,monospace;font-size:.9em}
a{color:${l.link};text-decoration:underline;text-underline-offset:3px}
h1,h2,h3,h4,h5,h6{font-size:1em;font-weight:600;margin:1em 0 .4em}
h1{font-size:1.25em}h2{font-size:1.125em}
hr{border:0;border-top:1px solid ${l.border};margin:1em 0}
blockquote{margin:.5em 0;padding-left:.75em;border-left:2px solid ${l.border};color:${l.muted}}
#y>:first-child,p:first-child{margin-top:0}#y>:last-child,p:last-child{margin-bottom:0}`
}

function originalStyle(l: Look) {
  return `:root{color-scheme:light}
html{margin:0;padding:0;background:#fff;overflow-x:auto;overflow-y:hidden}
body{margin:0;padding:0;font-family:${l.font};font-size:14px;line-height:1.5;color:#171717;overflow-wrap:anywhere}
#y{display:flow-root;box-sizing:border-box;padding:12px;transform-origin:0 0}
img{height:auto}
img:not([src]),img[src^="cid:"]{display:none}`
}

function frameDocument(html: string, images: boolean, inline: ReadonlyMap<string, string>, style: string) {
  const own = `${window.location.origin}/v1/attachments/`
  const img = images ? "data: https: http:" : inline.size > 0 ? `data: ${own}` : "data:"
  return `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src ${img}; font-src data:">
<base target="_blank">
<style>${style}</style></head><body><div id="y">${prepare(html, images, inline)}</div></body></html>`
}

const GEIST = [
  { url: geistLatinExt, range: "U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF" },
  { url: geistLatin, range: "U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD" },
]

let geist: Promise<{ data: ArrayBuffer; range: string }[]> | undefined

function geistFaces() {
  geist ??= Promise.all(GEIST.map(async (f) => ({ data: await (await fetch(f.url)).arrayBuffer(), range: f.range }))).catch(() => [])
  return geist
}

// The frame's CSP allows fonts only from data:, so the panel's font goes in as bytes, which no fetch from the frame needs.
function addFonts(win: Window & typeof globalThis, doc: Document) {
  void geistFaces().then((faces) => {
    for (const f of faces) {
      const face = new win.FontFace("Geist Variable", f.data.slice(0), { weight: "100 900", unicodeRange: f.range })
      doc.fonts.add(face)
      void face.load().catch(() => undefined)
    }
  })
}

function subscribeDark(change: () => void) {
  const mo = new MutationObserver(change)
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] })
  return () => mo.disconnect()
}

function useDark() {
  return useSyncExternalStore(subscribeDark, () => document.documentElement.classList.contains("dark"))
}

const NO_INLINE: ReadonlyMap<string, string> = new Map()
const MIN_SCALE = 0.6

export function EmailHtml({
  html,
  images,
  inline = NO_INLINE,
  original = false,
  className,
}: {
  html: string
  images: boolean
  inline?: ReadonlyMap<string, string>
  original?: boolean
  className?: string
}) {
  const { t } = useLingui()
  const frame = useRef<HTMLIFrameElement>(null)
  const [height, setHeight] = useState(40)
  const dark = useDark()
  const style = useMemo(() => {
    const look = lookOf(dark)
    return original ? originalStyle(look) : readingStyle(look)
  }, [dark, original])
  const srcDoc = useMemo(() => frameDocument(html, images, inline, style), [html, images, inline, style])

  useEffect(() => {
    const el = frame.current
    if (!el) return
    let ro: ResizeObserver | undefined
    let ready: Document | undefined
    const fit = (doc: Document, box: HTMLElement) => {
      const root = doc.documentElement
      let scale = 1
      if (original) {
        const width = root.clientWidth
        box.style.transform = ""
        box.style.width = `${width}px`
        const natural = box.scrollWidth
        scale = natural > width ? Math.max(MIN_SCALE, width / natural) : 1
        box.style.width = scale < 1 ? `${width / scale}px` : ""
        box.style.transform = scale < 1 ? `scale(${scale})` : ""
      }
      const bar = original ? Math.max(0, (doc.defaultView?.innerHeight ?? 0) - root.clientHeight) : 0
      setHeight(Math.ceil(box.offsetHeight * scale) + bar)
    }
    const start = () => {
      const doc = el.contentDocument
      const win = el.contentWindow as (Window & typeof globalThis) | null
      const box = doc?.getElementById("y")
      if (!doc || !win || !box || ready === doc) return
      ready = doc
      addFonts(win, doc)
      ro?.disconnect()
      ro = new ResizeObserver(() => fit(doc, box))
      ro.observe(doc.documentElement)
      ro.observe(box)
      fit(doc, box)
    }
    el.addEventListener("load", start)
    start()
    return () => {
      el.removeEventListener("load", start)
      ro?.disconnect()
    }
  }, [srcDoc, original])

  return (
    <iframe
      ref={frame}
      title={t`E-mail content`}
      srcDoc={srcDoc}
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      referrerPolicy="no-referrer"
      className={cn("block w-full border-0", className)}
      style={{ height }}
      data-testid="email-html"
    />
  )
}
