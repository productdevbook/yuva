import { useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useRef, useState } from "react"

import { cn } from "@/lib/utils"

const REMOTE = /^\s*(https?:)?\/\//i
const REMOTE_IN_SRCSET = /(^|,)\s*(https?:)?\/\//i

function remoteImages(doc: Document) {
  return [...doc.querySelectorAll("img, [background]")].filter((el) => {
    const src = el.getAttribute("src") ?? el.getAttribute("background") ?? ""
    return REMOTE.test(src) || REMOTE_IN_SRCSET.test(el.getAttribute("srcset") ?? "")
  })
}

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
    for (const el of remoteImages(doc)) {
      if (el.getAttribute("src")?.startsWith(`${window.location.origin}/v1/attachments/`)) continue
      el.removeAttribute("src")
      el.removeAttribute("srcset")
      el.removeAttribute("background")
    }
  }
  return doc.body.innerHTML
}

function frameDocument(html: string, images: boolean, inline: ReadonlyMap<string, string>) {
  const own = `${window.location.origin}/v1/attachments/`
  const img = images ? "data: https: http:" : inline.size > 0 ? `data: ${own}` : "data:"
  return `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src ${img}; font-src data:">
<base target="_blank">
<style>
html,body{margin:0;padding:0;background:transparent}
body{padding:10px 12px;font:14px/1.5 ui-sans-serif,system-ui,sans-serif;color:#171717;overflow-wrap:anywhere}
img{max-width:100%;height:auto}
img:not([src]),img[src^="cid:"]{display:none}
table{max-width:100%}
pre{white-space:pre-wrap}
a{color:#2563eb}
blockquote{margin:.5em 0;padding-left:.75em;border-left:2px solid #d4d4d4;color:#525252}
p:first-child{margin-top:0}p:last-child{margin-bottom:0}
</style></head><body>${prepare(html, images, inline)}</body></html>`
}

const NO_INLINE: ReadonlyMap<string, string> = new Map()

export function EmailHtml({
  html,
  images,
  inline = NO_INLINE,
  className,
}: {
  html: string
  images: boolean
  inline?: ReadonlyMap<string, string>
  className?: string
}) {
  const { t } = useLingui()
  const frame = useRef<HTMLIFrameElement>(null)
  const [height, setHeight] = useState(40)
  const srcDoc = useMemo(() => frameDocument(html, images, inline), [html, images, inline])

  useEffect(() => {
    const el = frame.current
    if (!el) return
    let ro: ResizeObserver | undefined
    const measure = () => {
      const doc = el.contentDocument
      if (!doc?.body) return
      setHeight(Math.ceil(doc.documentElement.scrollHeight))
      ro?.disconnect()
      ro = new ResizeObserver(() => setHeight(Math.ceil(doc.documentElement.scrollHeight)))
      ro.observe(doc.body)
    }
    el.addEventListener("load", measure)
    measure()
    return () => {
      el.removeEventListener("load", measure)
      ro?.disconnect()
    }
  }, [srcDoc])

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
