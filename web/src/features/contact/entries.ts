import { attrValue } from "@/features/contact/Section"
import { byPosition, useFieldFormat } from "@/features/fields/value"
import type { Contact } from "@/lib/api"
import { useCustomFields } from "@/lib/workspace"

function label(k: string) {
  return k.charAt(0).toUpperCase() + k.slice(1).replace(/[_-]+/g, " ")
}

export function useContactEntries(contact: Contact) {
  const defs = useCustomFields("contact").data ?? []
  const format = useFieldFormat()
  const attributes = contact.attributes ?? {}
  const typed = [...defs].sort(byPosition).filter((f) => attributes[f.key] !== undefined && attributes[f.key] !== null)
  const known = new Set(typed.map((f) => f.key))
  return [
    ...typed.map((f) => ({ key: f.key, label: f.name, value: format(f, attributes[f.key]), raw: false })),
    ...Object.entries(attributes)
      .filter(([k]) => !known.has(k))
      .map(([k, v]) => ({ key: k, label: label(k), value: attrValue(v), raw: true })),
  ]
}
