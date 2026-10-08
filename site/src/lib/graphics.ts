import fs from "node:fs"
import path from "node:path"
import { Resvg } from "@resvg/resvg-js"
import opentype from "opentype.js"
import { iconBody, iconSvg } from "@/lib/icon"

const fontDir = path.resolve("node_modules/geist/dist/fonts/geist-sans")
const FONT_FILES = [path.join(fontDir, "Geist-Bold.ttf"), path.join(fontDir, "Geist-Medium.ttf")]

export function markSvg(size: number) {
  return iconSvg(size, "square")
}

export function renderPng(svg: string, width: number) {
  const resvg = new Resvg(svg, {
    fitTo: { mode: "width", value: width },
    font: { loadSystemFonts: false, fontFiles: FONT_FILES, defaultFontFamily: "Geist" },
  })
  return new Uint8Array(resvg.render().asPng())
}

let boldFont: opentype.Font | undefined

function bold() {
  if (!boldFont) {
    const buf = fs.readFileSync(FONT_FILES[0]!)
    boldFont = opentype.parse(buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength))
  }
  return boldFont
}

function textWidth(text: string, size: number) {
  const font = bold()
  let width = 0
  for (const ch of text) width += (font.charToGlyph(ch).advanceWidth ?? 0) * (size / font.unitsPerEm)
  return width
}

function wrap(text: string, size: number, maxWidth: number) {
  const lines: string[] = []
  let line = ""
  for (const word of text.split(/\s+/)) {
    const next = line ? `${line} ${word}` : word
    if (line && textWidth(next, size) > maxWidth) {
      lines.push(line)
      line = word
    } else {
      line = next
    }
  }
  if (line) lines.push(line)
  return lines
}

function escape(text: string) {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
}

export function ogSvg(title: string, kicker: string, footer: string) {
  let size = 76
  let lines = wrap(title, size, 1000)
  while (lines.length > 3 && size > 48) {
    size -= 6
    lines = wrap(title, size, 1000)
  }
  const lineHeight = Math.round(size * 1.12)
  const top = 450 - lineHeight * (lines.length - 1)
  const tspans = lines.map((l, i) => `<tspan x="80" y="${top + i * lineHeight}">${escape(l)}</tspan>`).join("")
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
<defs><radialGradient id="g" cx="0.85" cy="0.1" r="0.9"><stop offset="0" stop-color="#3a1a10"/><stop offset="1" stop-color="#09090b"/></radialGradient></defs>
<rect width="1200" height="630" fill="url(#g)"/>
<svg x="80" y="70" width="64" height="64" viewBox="0 0 1024 1024">${iconBody("og", "squircle")}</svg>
<text x="164" y="114" font-family="Geist" font-weight="700" font-size="40" fill="#fafafa">Yuva</text>
<text x="80" y="${top - lineHeight}" font-family="Geist" font-weight="500" font-size="30" fill="#ffb08f">${escape(kicker)}</text>
<text font-family="Geist" font-weight="700" font-size="${size}" fill="#fafafa">${tspans}</text>
<text x="80" y="560" font-family="Geist" font-weight="500" font-size="26" fill="#a1a1aa">${escape(footer)}</text>
</svg>`
}
