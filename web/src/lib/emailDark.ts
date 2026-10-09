export type Rgb = { r: number; g: number; b: number; a: number }
export type Oklch = { l: number; c: number; h: number }

const NAMED =
  "aliceblue:f0f8ff,antiquewhite:faebd7,aqua:00ffff,aquamarine:7fffd4,azure:f0ffff,beige:f5f5dc,bisque:ffe4c4,black:000000,blanchedalmond:ffebcd,blue:0000ff,blueviolet:8a2be2,brown:a52a2a,burlywood:deb887,cadetblue:5f9ea0,chartreuse:7fff00,chocolate:d2691e,coral:ff7f50,cornflowerblue:6495ed,cornsilk:fff8dc,crimson:dc143c,cyan:00ffff,darkblue:00008b,darkcyan:008b8b,darkgoldenrod:b8860b,darkgray:a9a9a9,darkgreen:006400,darkgrey:a9a9a9,darkkhaki:bdb76b,darkmagenta:8b008b,darkolivegreen:556b2f,darkorange:ff8c00,darkorchid:9932cc,darkred:8b0000,darksalmon:e9967a,darkseagreen:8fbc8f,darkslateblue:483d8b,darkslategray:2f4f4f,darkslategrey:2f4f4f,darkturquoise:00ced1,darkviolet:9400d3,deeppink:ff1493,deepskyblue:00bfff,dimgray:696969,dimgrey:696969,dodgerblue:1e90ff,firebrick:b22222,floralwhite:fffaf0,forestgreen:228b22,fuchsia:ff00ff,gainsboro:dcdcdc,ghostwhite:f8f8ff,gold:ffd700,goldenrod:daa520,gray:808080,green:008000,greenyellow:adff2f,grey:808080,honeydew:f0fff0,hotpink:ff69b4,indianred:cd5c5c,indigo:4b0082,ivory:fffff0,khaki:f0e68c,lavender:e6e6fa,lavenderblush:fff0f5,lawngreen:7cfc00,lemonchiffon:fffacd,lightblue:add8e6,lightcoral:f08080,lightcyan:e0ffff,lightgoldenrodyellow:fafad2,lightgray:d3d3d3,lightgreen:90ee90,lightgrey:d3d3d3,lightpink:ffb6c1,lightsalmon:ffa07a,lightseagreen:20b2aa,lightskyblue:87cefa,lightslategray:778899,lightslategrey:778899,lightsteelblue:b0c4de,lightyellow:ffffe0,lime:00ff00,limegreen:32cd32,linen:faf0e6,magenta:ff00ff,maroon:800000,mediumaquamarine:66cdaa,mediumblue:0000cd,mediumorchid:ba55d3,mediumpurple:9370db,mediumseagreen:3cb371,mediumslateblue:7b68ee,mediumspringgreen:00fa9a,mediumturquoise:48d1cc,mediumvioletred:c71585,midnightblue:191970,mintcream:f5fffa,mistyrose:ffe4e1,moccasin:ffe4b5,navajowhite:ffdead,navy:000080,oldlace:fdf5e6,olive:808000,olivedrab:6b8e23,orange:ffa500,orangered:ff4500,orchid:da70d6,palegoldenrod:eee8aa,palegreen:98fb98,paleturquoise:afeeee,palevioletred:db7093,papayawhip:ffefd5,peachpuff:ffdab9,peru:cd853f,pink:ffc0cb,plum:dda0dd,powderblue:b0e0e6,purple:800080,rebeccapurple:663399,red:ff0000,rosybrown:bc8f8f,royalblue:4169e1,saddlebrown:8b4513,salmon:fa8072,sandybrown:f4a460,seagreen:2e8b57,seashell:fff5ee,sienna:a0522d,silver:c0c0c0,skyblue:87ceeb,slateblue:6a5acd,slategray:708090,slategrey:708090,snow:fffafa,springgreen:00ff7f,steelblue:4682b4,tan:d2b48c,teal:008080,thistle:d8bfd8,tomato:ff6347,turquoise:40e0d0,violet:ee82ee,wheat:f5deb3,white:ffffff,whitesmoke:f5f5f5,yellow:ffff00,yellowgreen:9acd32"

let named: Map<string, string> | undefined

function namedHex(name: string) {
  named ??= new Map(NAMED.split(",").map((p) => p.split(":") as [string, string]))
  return named.get(name)
}

const clamp = (v: number, lo = 0, hi = 1) => Math.min(hi, Math.max(lo, v))

function fromHex(hex: string): Rgb | undefined {
  const h = hex.length <= 4 ? [...hex].map((d) => d + d).join("") : hex
  if (!/^[0-9a-f]{6}([0-9a-f]{2})?$/i.test(h)) return undefined
  const n = (i: number) => parseInt(h.slice(i, i + 2), 16) / 255
  return { r: n(0), g: n(2), b: n(4), a: h.length === 8 ? n(6) : 1 }
}

function args(body: string) {
  const [main, slash] = body.split("/")
  const parts = main.split(/[\s,]+/).filter(Boolean)
  const alpha = slash ?? parts[3]
  return { parts: parts.slice(0, 3), alpha: alpha === undefined ? 1 : unit(alpha.trim(), 1) }
}

function unit(v: string, scale: number) {
  return v.endsWith("%") ? clamp(parseFloat(v) / 100) : clamp(parseFloat(v) / scale)
}

function hslToRgb(h: number, s: number, l: number) {
  const k = (n: number) => (n + h / 30) % 12
  const a = s * Math.min(l, 1 - l)
  const f = (n: number) => l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1))
  return { r: f(0), g: f(8), b: f(4) }
}

export function parseColor(input: string): Rgb | undefined {
  const v = input.trim().toLowerCase()
  if (!v) return undefined
  if (v.startsWith("#")) return fromHex(v.slice(1))
  const fn = /^(rgba?|hsla?|oklch)\(([^()]*)\)$/.exec(v)
  if (fn) {
    const { parts, alpha } = args(fn[2])
    if (parts.length !== 3 || parts.some((p) => Number.isNaN(parseFloat(p))) || Number.isNaN(alpha)) return undefined
    if (fn[1].startsWith("rgb")) {
      const [r, g, b] = parts.map((p) => unit(p, 255))
      return { r, g, b, a: alpha }
    }
    if (fn[1] === "oklch") {
      const l = parts[0].endsWith("%") ? parseFloat(parts[0]) / 100 : parseFloat(parts[0])
      return { ...fromOklch({ l, c: parseFloat(parts[1]), h: parseFloat(parts[2]) }), a: alpha }
    }
    const h = ((parseFloat(parts[0]) % 360) + 360) % 360
    return { ...hslToRgb(h, unit(parts[1], 100), unit(parts[2], 100)), a: alpha }
  }
  const hex = namedHex(v)
  if (hex) return fromHex(hex)
  if (/^[0-9a-f]{6}$/.test(v)) return fromHex(v)
  return undefined
}

export function formatColor(c: Rgb) {
  const byte = (v: number) => Math.round(clamp(v) * 255)
  if (c.a >= 1) return "#" + [c.r, c.g, c.b].map((v) => byte(v).toString(16).padStart(2, "0")).join("")
  return `rgba(${byte(c.r)}, ${byte(c.g)}, ${byte(c.b)}, ${Math.round(clamp(c.a) * 1000) / 1000})`
}

const toLinear = (v: number) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4)
const toGamma = (v: number) => (v <= 0.0031308 ? v * 12.92 : 1.055 * v ** (1 / 2.4) - 0.055)

export function toOklch(c: Rgb): Oklch {
  const r = toLinear(c.r)
  const g = toLinear(c.g)
  const b = toLinear(c.b)
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
  const L = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s
  const A = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s
  const B = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s
  const h = (Math.atan2(B, A) * 180) / Math.PI
  return { l: L, c: Math.hypot(A, B), h: h < 0 ? h + 360 : h }
}

function oklchToLinear({ l: L, c, h }: Oklch) {
  const A = c * Math.cos((h * Math.PI) / 180)
  const B = c * Math.sin((h * Math.PI) / 180)
  const l = (L + 0.3963377774 * A + 0.2158037573 * B) ** 3
  const m = (L - 0.1055613458 * A - 0.0638541728 * B) ** 3
  const s = (L - 0.0894841775 * A - 1.291485548 * B) ** 3
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ]
}

const inGamut = (rgb: number[]) => rgb.every((v) => v >= -1e-4 && v <= 1 + 1e-4)

export function fromOklch(o: Oklch): Rgb {
  const l = clamp(o.l)
  let c = Math.max(0, o.c)
  if (!inGamut(oklchToLinear({ l, c, h: o.h }))) {
    let lo = 0
    let hi = c
    for (let i = 0; i < 20; i++) {
      const mid = (lo + hi) / 2
      if (inGamut(oklchToLinear({ l, c: mid, h: o.h }))) lo = mid
      else hi = mid
    }
    c = lo
  }
  const [r, g, b] = oklchToLinear({ l, c, h: o.h }).map((v) => clamp(toGamma(clamp(v))))
  return { r, g, b, a: 1 }
}

export function over(top: Rgb, bottom: Rgb): Rgb {
  const a = clamp(top.a)
  return {
    r: top.r * a + bottom.r * (1 - a),
    g: top.g * a + bottom.g * (1 - a),
    b: top.b * a + bottom.b * (1 - a),
    a: 1,
  }
}

export function luminance(c: Rgb) {
  return 0.2126 * toLinear(c.r) + 0.7152 * toLinear(c.g) + 0.0722 * toLinear(c.b)
}

export function contrast(a: Rgb, b: Rgb) {
  const x = luminance(a)
  const y = luminance(b)
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
}

export const DARK = {
  lightAbove: 0.62,
  backgroundFrom: 0.2,
  backgroundSpan: 0.1,
  chromaScale: 0.6,
  chromaMax: 0.08,
  textFrom: 0.93,
  textSlope: 0.45,
  minContrast: 4.5,
}

export function darkBackground(c: Rgb): Rgb {
  const o = toOklch(c)
  if (o.l <= DARK.lightAbove) return c
  const depth = Math.sqrt(clamp((1 - o.l) / (1 - DARK.lightAbove)))
  const l = DARK.backgroundFrom + DARK.backgroundSpan * depth
  return { ...fromOklch({ l, c: Math.min(o.c * DARK.chromaScale, DARK.chromaMax), h: o.h }), a: c.a }
}

export function readableOn(c: Rgb, bg: Rgb): Rgb {
  const o = toOklch(c)
  const at = (l: number) => ({ ...fromOklch({ l, c: o.c, h: o.h }), a: c.a })
  const passes = (l: number) => contrast(over(at(l), bg), bg) >= DARK.minContrast
  const start = luminance(bg) < 0.18 && o.l < 0.6 ? Math.max(o.l, DARK.textFrom - DARK.textSlope * o.l) : o.l
  if (passes(start)) return start === o.l ? c : at(start)
  if (passes(1)) {
    let lo = start
    let hi = 1
    for (let i = 0; i < 20; i++) {
      const mid = (lo + hi) / 2
      if (passes(mid)) hi = mid
      else lo = mid
    }
    return at(hi)
  }
  if (!passes(0)) return contrast(at(1), bg) >= contrast(at(0), bg) ? at(1) : at(0)
  let lo = 0
  let hi = start
  for (let i = 0; i < 20; i++) {
    const mid = (lo + hi) / 2
    if (passes(mid)) lo = mid
    else hi = mid
  }
  return at(lo)
}

export function darkBorder(c: Rgb, sentOn: Rgb, bg: Rgb): Rgb {
  const target = Math.min(DARK.minContrast, contrast(over(c, sentOn), sentOn))
  const o = toOklch(c)
  const at = (l: number) => ({ ...fromOklch({ l, c: o.c, h: o.h }), a: c.a })
  let lo = toOklch(bg).l
  let hi = 1
  if (contrast(over(at(hi), bg), bg) < target) return at(hi)
  for (let i = 0; i < 20; i++) {
    const mid = (lo + hi) / 2
    if (contrast(over(at(mid), bg), bg) >= target) hi = mid
    else lo = mid
  }
  return at(hi)
}

const BORDER_COLOURS = ["border-top-color", "border-right-color", "border-bottom-color", "border-left-color", "outline-color"]
const TEXT_COLOURS = ["color", "text-decoration-color"]

function convertStyle(el: HTMLElement, name: string, fn: (c: Rgb) => Rgb) {
  const v = el.style.getPropertyValue(name)
  const c = v ? parseColor(v) : undefined
  if (!c || c.a === 0) return undefined
  const next = fn(c)
  el.style.setProperty(name, formatColor(next), el.style.getPropertyPriority(name))
  return { sent: c, next }
}

function convertAttr(el: Element, name: string, fn: (c: Rgb) => Rgb) {
  const v = el.getAttribute(name)
  const c = v ? parseColor(v) : undefined
  if (!c || c.a === 0) return undefined
  const next = fn(c)
  el.setAttribute(name, formatColor(next))
  return { sent: c, next }
}

const WHITE: Rgb = { r: 1, g: 1, b: 1, a: 1 }

export function darkenTree(root: Element, page: Rgb) {
  const walk = (el: Element, sentOn: Rgb, bg: Rgb) => {
    const html = el as HTMLElement
    const fromAttr = convertAttr(el, "bgcolor", darkBackground)
    const fromStyle = html.style ? convertStyle(html, "background-color", darkBackground) : undefined
    const fill = fromStyle ?? fromAttr
    const ownSent = fill ? over(fill.sent, sentOn) : sentOn
    const own = fill ? over(fill.next, bg) : bg
    if (html.style) {
      for (const p of TEXT_COLOURS) convertStyle(html, p, (c) => readableOn(c, own))
      for (const p of BORDER_COLOURS) convertStyle(html, p, (c) => darkBorder(c, ownSent, own))
    }
    if (el.localName === "font") convertAttr(el, "color", (c) => readableOn(c, own))
    for (const child of el.children) walk(child, ownSent, own)
  }
  walk(root, WHITE, page)
}
