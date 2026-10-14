const sheets = new Map<string, CSSStyleSheet>()

// Constructed sheets are not inline styles, so a strict style-src CSP accepts them.
function sheetFor(css: string): CSSStyleSheet | null {
  if (typeof CSSStyleSheet === "undefined" || !("replaceSync" in CSSStyleSheet.prototype)) return null
  let sheet = sheets.get(css)
  if (!sheet) {
    sheet = new CSSStyleSheet()
    sheet.replaceSync(css)
    sheets.set(css, sheet)
  }
  return sheet
}

function fallback(css: string): HTMLStyleElement {
  const style = document.createElement("style")
  style.textContent = css
  return style
}

export function addStyles(root: Node, css: string): HTMLStyleElement | null {
  const sheet = sheetFor(css)
  if (!sheet || !(root instanceof ShadowRoot)) return fallback(css)
  if (!root.adoptedStyleSheets.includes(sheet)) root.adoptedStyleSheets = [...root.adoptedStyleSheets, sheet]
  return null
}

export function setStyles(root: ShadowRoot, css: string): HTMLStyleElement | null {
  const sheet = sheetFor(css)
  if (!sheet) return fallback(css)
  root.adoptedStyleSheets = [sheet]
  return null
}
