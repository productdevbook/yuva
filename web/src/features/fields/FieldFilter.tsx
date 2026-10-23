import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { Separator } from "@/components/ui/separator"
import type { FieldFilterValue } from "@/features/inbox/SourceFilter"
import type { CustomField } from "@/lib/api"

export function fieldFilterText(f: CustomField, value: string, yes: string, no: string) {
  return `${f.name}: ${f.type === "checkbox" ? (value === "true" ? yes : no) : value}`
}

export function FieldFilterSection({
  fields,
  value,
  onChange,
}: {
  fields: CustomField[]
  value: FieldFilterValue | undefined
  onChange: (next: FieldFilterValue | undefined) => void
}) {
  const { t } = useLingui()
  const [key, setKey] = useState(value?.key ?? "")
  const [draft, setDraft] = useState(value?.value ?? "")
  const field = fields.find((f) => f.key === key)
  if (fields.length === 0) return null

  const apply = (raw: string) => {
    const v = raw.trim()
    if (field && v !== "") onChange({ key: field.key, value: v })
    else if (value) onChange(undefined)
  }
  const control = () => {
    if (!field) return null
    if (field.type === "select" || field.type === "checkbox") {
      const options: [string, string][] =
        field.type === "checkbox"
          ? [
              ["true", t`Yes`],
              ["false", t`No`],
            ]
          : field.options.map((o) => [o, o])
      return (
        <NativeSelect
          size="sm"
          className="w-full"
          aria-label={field.name}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value)
            apply(e.target.value)
          }}
          data-testid="field-filter-value"
        >
          <NativeSelectOption value="">{t`Choose a value`}</NativeSelectOption>
          {options.map(([v, l]) => (
            <NativeSelectOption key={v} value={v}>
              {l}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      )
    }
    return (
      <Input
        className="h-8"
        aria-label={field.name}
        type={field.type === "number" ? "number" : field.type === "date" ? "date" : "text"}
        step={field.type === "number" ? "any" : undefined}
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value)
          if (field.type === "date") apply(e.target.value)
        }}
        onBlur={() => apply(draft)}
        onKeyDown={(e) => e.key === "Enter" && apply(draft)}
        data-testid="field-filter-value"
      />
    )
  }
  return (
    <>
      <Separator className="mx-1.5 my-1 w-auto" />
      <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
        <Trans>Custom field</Trans>
      </p>
      <div className="flex flex-col gap-1.5 px-2.5 pb-2">
        <NativeSelect
          size="sm"
          className="w-full"
          aria-label={t`Custom field`}
          value={key}
          onChange={(e) => {
            setKey(e.target.value)
            setDraft("")
            if (value) onChange(undefined)
          }}
          data-testid="field-filter-key"
        >
          <NativeSelectOption value="">{t`Any field`}</NativeSelectOption>
          {fields.map((f) => (
            <NativeSelectOption key={f.id} value={f.key}>
              {f.name}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        {control()}
      </div>
    </>
  )
}
