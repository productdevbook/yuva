import fs from "node:fs"
import path from "node:path"
import { Resvg } from "@resvg/resvg-js"
import opentype from "opentype.js"

const fontDir = path.resolve("node_modules/geist/dist/fonts/geist-sans")
const FONT_FILES = [path.join(fontDir, "Geist-Bold.ttf"), path.join(fontDir, "Geist-Medium.ttf")]

const BUBBLE = '<path d="M8 10a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3v8a3 3 0 0 1-3 3h-6l-5 4v-4a2 2 0 0 1-2-2z" fill="#fff"/>'

export function markSvg(size: number, radius: number) {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 32 32"><rect width="32" height="32" rx="${radius}" fill="#1d4ed8"/>${BUBBLE}</svg>`
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
<defs><radialGradient id="g" cx="0.85" cy="0.1" r="0.9"><stop offset="0" stop-color="#3b2416"/><stop offset="1" stop-color="#17120e"/></radialGradient></defs>
<rect width="1200" height="630" fill="url(#g)"/>
<g fill="none" stroke="#f59e5b" stroke-opacity="0.18" stroke-width="2"><circle cx="1060" cy="120" r="120"/><circle cx="1060" cy="120" r="180"/><circle cx="1060" cy="120" r="240"/></g>
<g transform="translate(80 70) scale(2)"><rect width="32" height="32" rx="8" fill="#1d4ed8"/>${BUBBLE}</g>
<text x="164" y="114" font-family="Geist" font-weight="700" font-size="40" fill="#fbf4ec">Yuva</text>
<text x="80" y="${top - lineHeight}" font-family="Geist" font-weight="500" font-size="30" fill="#f59e5b">${escape(kicker)}</text>
<text font-family="Geist" font-weight="700" font-size="${size}" fill="#fbf4ec">${tspans}</text>
<text x="80" y="560" font-family="Geist" font-weight="500" font-size="26" fill="#bfae9c">${escape(footer)}</text>
</svg>`
}
