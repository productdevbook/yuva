import path from "node:path"
import { fileURLToPath } from "node:url"
import type { HastPluginDefinition } from "satteri"
import { REPO } from "./routes"

const repoRoot = path.resolve("..")
const docsDir = path.join(repoRoot, "docs")

export function repoLink(href: string, from: string) {
  if (/^([a-z][a-z0-9+.-]*:|#|\/)/i.test(href)) return href
  const [target = "", hash] = href.split("#")
  const absolute = path.resolve(path.dirname(from), target)
  const relative = path.relative(repoRoot, absolute)
  if (relative.startsWith("..")) return href
  const anchor = hash ? `#${hash}` : ""
  if (path.dirname(absolute) === docsDir && absolute.endsWith(".md")) return `/docs/${path.basename(absolute, ".md")}/${anchor}`
  return `${REPO}/blob/main/${relative.split(path.sep).join("/")}${anchor}`
}

export const repoLinks: HastPluginDefinition = {
  name: "repo-links",
  element: {
    filter: ["a"],
    visit(node, ctx) {
      const href = node.properties?.href
      if (typeof href !== "string" || !ctx.fileURL) return
      const next = repoLink(href, fileURLToPath(ctx.fileURL))
      if (next !== href) ctx.setProperty(node, "href", next)
    },
  },
}
