import { ROLES, useEnumText } from "@/components/common/text"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { Role } from "@/lib/api"

export function RoleSelect({
  value,
  onChange,
  disabled,
  allowOwner,
  label,
}: {
  value: Role
  onChange: (r: Role) => void
  disabled?: boolean
  allowOwner: boolean
  label: string
}) {
  const text = useEnumText()
  return (
    <Select value={value} onValueChange={(v) => onChange(v as Role)} items={text.role} disabled={disabled}>
      <SelectTrigger size="sm" className="w-32" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {ROLES.filter((r) => allowOwner || r !== "owner" || r === value).map((r) => (
          <SelectItem key={r} value={r} disabled={!allowOwner && r === "owner"}>
            {text.role[r]}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
