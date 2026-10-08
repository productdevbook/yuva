import fs from "node:fs"
import path from "node:path"
import opentype from "opentype.js"
import { iconBody } from "@/lib/icon"

const semibold = path.resolve("node_modules/geist/dist/fonts/geist-sans/Geist-SemiBold.ttf")

let font: opentype.Font | undefined

function wordmark(x: number, baseline: number, size: number) {
  if (!font) {
    const buf = fs.readFileSync(semibold)
    font = opentype.parse(buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength))
  }
  const p = font.getPath("Yuva", x, baseline, size, { kerning: true })
  const box = p.getBoundingBox()
  return { d: p.toPathData(2), right: box.x2 }
}

export function logoSvg(color: string) {
  const height = 256
  const text = wordmark(304, 184, 176)
  const width = Math.ceil(text.right + 8)
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}"><svg width="256" height="256" viewBox="0 0 1024 1024">${iconBody("yuva", "squircle")}</svg><path d="${text.d}" fill="${color}"/></svg>`
}
