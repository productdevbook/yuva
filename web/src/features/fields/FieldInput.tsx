import { useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { isSet, type FieldValue } from "@/features/fields/value"
import type { CustomField } from "@/lib/api"

function show(v: unknown) {
  return isSet(v) && typeof v !== "boolean" ? String(v) : ""
}

export function FieldInput({
  field,
  value,
  onValue,
  immediate,
  id,
  invalid,
  autoFocus,
  describedBy,
}: {
  field: CustomField
  value: unknown
  onValue: (v: FieldValue | null) => void
  immediate?: boolean
  id: string
  invalid?: boolean
  autoFocus?: boolean
  describedBy?: string
}) {
  const { t } = useLingui()
  const [draft, setDraft] = useState(show(value))
  const [seen, setSeen] = useState(value)
  if (seen !== value) {
    setSeen(value)
    setDraft(show(value))
  }
  const common = { id, "aria-invalid": invalid || undefined, "aria-describedby": describedBy, autoFocus }

  if (field.type === "checkbox") {
    return (
      <Checkbox
        {...common}
        data-field-input
        checked={value === true}
        onCheckedChange={(on) => onValue(on)}
        aria-label={field.name}
      />
    )
  }
  if (field.type === "select") {
    return (
      <NativeSelect
        {...common}
        data-field-input
        className="w-full"
        value={typeof value === "string" ? value : ""}
        onChange={(e) => onValue(e.target.value === "" ? null : e.target.value)}
      >
        <NativeSelectOption value="">{t`Not set`}</NativeSelectOption>
        {field.options.map((o) => (
          <NativeSelectOption key={o} value={o}>
            {o}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    )
  }
  const parse = (raw: string): FieldValue | null => {
    const s = raw.trim()
    if (s === "") return null
    if (field.type === "number") {
      const n = Number(s)
      return Number.isFinite(n) ? n : null
    }
    return s
  }
  const send = (raw: string) => {
    const next = parse(raw)
    if (next === (isSet(value) ? value : null)) return
    onValue(next)
  }
  return (
    <Input
      {...common}
      data-field-input
      type={field.type === "number" ? "number" : field.type === "date" ? "date" : "text"}
      step={field.type === "number" ? "any" : undefined}
      maxLength={field.type === "text" ? 1000 : undefined}
      className="h-9"
      value={draft}
      onChange={(e) => {
        setDraft(e.target.value)
        if (immediate || field.type === "date") send(e.target.value)
      }}
      onBlur={() => !immediate && field.type !== "date" && send(draft)}
      onKeyDown={(e) => {
        if (e.key === "Enter" && !immediate) {
          e.preventDefault()
          send(draft)
        }
      }}
    />
  )
}
