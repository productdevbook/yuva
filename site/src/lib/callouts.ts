import type { HastPluginDefinition } from "satteri"

const marker = /^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*/

export const callouts: HastPluginDefinition = {
  name: "callouts",
  element: {
    filter: ["blockquote"],
    visit(node, ctx) {
      const kind = ctx.textContent(node).match(marker)?.[1]?.toLowerCase()
      if (!kind) return
      ctx.setProperty(node, "className", ["callout", `callout-${kind}`])
      ctx.setProperty(node, "dataLabel", kind.replace(/^./, (c) => c.toUpperCase()))
    },
  },
  text(node, ctx) {
    if (marker.test(node.value)) ctx.replaceNode(node, { type: "text", value: node.value.replace(marker, "") })
  },
}
