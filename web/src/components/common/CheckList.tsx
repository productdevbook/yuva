import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"

export function CheckList({ labelId, label, hint, children }: { labelId: string; label: React.ReactNode; hint?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-2" role="group" aria-labelledby={labelId}>
      <span id={labelId} className="text-body font-medium">
        {label}
      </span>
      <ul className="divide-y rounded-xl border">{children}</ul>
      {hint}
    </div>
  )
}

export function CheckItem({
  checked,
  disabled,
  onChange,
  children,
  testId,
}: {
  checked: boolean
  disabled?: boolean
  onChange: (on: boolean) => void
  children: React.ReactNode
  testId?: string
}) {
  return (
    <li>
      <Label className="gap-3 px-3.5 py-2.5 font-normal has-disabled:opacity-50" data-testid={testId}>
        <Checkbox checked={checked} disabled={disabled} onCheckedChange={onChange} />
        <span className="flex min-w-0 flex-1 flex-col">{children}</span>
      </Label>
    </li>
  )
}
