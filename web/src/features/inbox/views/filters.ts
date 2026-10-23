import type { SavedViewFilters } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"

const FIELD = "field."
const LISTS = new Set(["status", "kind", "channel_kind", "sla_state"])
const FLAGS = new Set(["spam", "trashed", "pinned"])

export function toViewFilters(f: ConversationFilters): SavedViewFilters {
  const out: SavedViewFilters = {}
  for (const [key, value] of Object.entries(f)) {
    if (value === undefined || value === "") continue
    if (key === "fields") {
      for (const [name, v] of Object.entries(value as Record<string, string>)) out[`${FIELD}${name}`] = v
    } else if (Array.isArray(value)) {
      if (value.length > 0) out[key] = value.join(",")
    } else if (key !== "pinned") {
      out[key] = String(value)
    }
  }
  return out
}

export function fromViewFilters(p: Record<string, string>): ConversationFilters {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(p)) {
    if (key.startsWith(FIELD)) out.fields = { ...(out.fields as object), [key.slice(FIELD.length)]: value }
    else if (LISTS.has(key)) out[key] = value.split(",").filter(Boolean)
    else if (FLAGS.has(key)) out[key] = value === "true"
    else out[key] = value
  }
  return out as ConversationFilters
}

export function viewSearch(id: string, filters: SavedViewFilters) {
  return `?${new URLSearchParams({ view: id, ...filters }).toString()}`
}
