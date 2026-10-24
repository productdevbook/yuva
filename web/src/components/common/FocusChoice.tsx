import { Trans, useLingui } from "@lingui/react/macro"

import type { WorkspaceFocus } from "@/lib/api"
import { cn } from "@/lib/utils"

export function FocusChoice({
  value,
  onChange,
  disabled,
}: {
  value: WorkspaceFocus | undefined
  onChange: (f: WorkspaceFocus) => void
  disabled?: boolean
}) {
  const { t } = useLingui()
  const items: { value: WorkspaceFocus; title: React.ReactNode; hint: React.ReactNode }[] = [
    {
      value: "support",
      title: <Trans>Support</Trans>,
      hint: <Trans>Chat, e-mail and in-app support for customers</Trans>,
    },
    {
      value: "mail",
      title: <Trans>Team mail</Trans>,
      hint: <Trans>Shared mailboxes for your team</Trans>,
    },
    {
      value: "both",
      title: <Trans>Both</Trans>,
      hint: <Trans>Support for customers and shared mailboxes for your team</Trans>,
    },
  ]
  return (
    <div className="flex flex-col gap-2" role="radiogroup" aria-label={t`What Yuva is for`}>
      {items.map((it) => (
        <button
          key={it.value}
          type="button"
          role="radio"
          aria-checked={value === it.value}
          disabled={disabled}
          onClick={() => onChange(it.value)}
          className={cn(
            "flex flex-col rounded-xl border px-3.5 py-2.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60",
            value === it.value ? "border-foreground bg-surface" : "hover:border-faint"
          )}
          data-testid={`focus-${it.value}`}
        >
          <span className="font-medium">{it.title}</span>
          <span className="text-caption text-faint">{it.hint}</span>
        </button>
      ))}
    </div>
  )
}
