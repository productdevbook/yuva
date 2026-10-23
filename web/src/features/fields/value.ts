import { useLingui } from "@lingui/react/macro"

import { formatDate } from "@/components/common/text"
import type { CustomField } from "@/lib/api"

export type FieldValue = string | number | boolean

export function isSet(v: unknown): v is FieldValue {
  return v !== undefined && v !== null && v !== ""
}

export function useFieldFormat() {
  const { t, i18n } = useLingui()
  return (f: CustomField, v: unknown): string => {
    if (!isSet(v)) return "—"
    if (f.type === "checkbox") return v === true ? t`Yes` : t`No`
    if (f.type === "number" && typeof v === "number") return new Intl.NumberFormat(i18n.locale).format(v)
    if (f.type === "date" && typeof v === "string") return formatDate(`${v}T12:00:00`, i18n.locale)
    return String(v)
  }
}

export function byPosition(a: CustomField, b: CustomField) {
  return a.position - b.position || a.created_at.localeCompare(b.created_at)
}

export function useFieldTypeText() {
  const { t } = useLingui()
  return {
    text: t`Text`,
    number: t`Number`,
    date: t`Date`,
    select: t`Select`,
    checkbox: t`Checkbox`,
  } satisfies Record<CustomField["type"], string>
}
