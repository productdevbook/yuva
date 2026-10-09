export function attrValue(v: unknown) {
  if (v === null || v === undefined) return "—"
  if (typeof v === "object") return JSON.stringify(v)
  return String(v)
}

export function contactName(c: { name?: string; emails: string[]; external_ids: { external_id: string }[] }, fallback: string) {
  return c.name || c.emails[0] || c.external_ids[0]?.external_id || fallback
}
