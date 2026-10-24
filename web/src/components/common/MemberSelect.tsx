import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { useMembers } from "@/lib/workspace"

export function MemberSelect({
  value,
  onChange,
  exclude = [],
  label,
  placeholder,
  className,
  testId,
}: {
  value: string
  onChange: (id: string) => void
  exclude?: string[]
  label: string
  placeholder: string
  className?: string
  testId?: string
}) {
  const members = (useMembers().data ?? []).filter((m) => !exclude.includes(m.id))
  return (
    <NativeSelect
      className={className}
      size="sm"
      value={value}
      aria-label={label}
      onChange={(e) => onChange(e.target.value)}
      data-testid={testId}
    >
      <NativeSelectOption value="">{placeholder}</NativeSelectOption>
      {members.map((m) => (
        <NativeSelectOption key={m.id} value={m.id}>
          {m.name || m.email}
        </NativeSelectOption>
      ))}
    </NativeSelect>
  )
}
