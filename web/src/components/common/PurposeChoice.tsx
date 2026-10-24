import { Trans, useLingui } from "@lingui/react/macro"

import { Field } from "@/features/settings/ui"
import type { InboxPurpose } from "@/lib/api"
import { cn } from "@/lib/utils"

export function PurposeChoice({ value, onChange }: { value: InboxPurpose; onChange: (p: InboxPurpose) => void }) {
  const { t } = useLingui()
  const items: { value: InboxPurpose; title: React.ReactNode; hint: React.ReactNode }[] = [
    {
      value: "support",
      title: <Trans>Support</Trans>,
      hint: <Trans>Customers write by chat, app or e-mail; ratings, assignment and SLA work here.</Trans>,
    },
    {
      value: "mail",
      title: <Trans>Team mail</Trans>,
      hint: <Trans>A shared mailbox like hello@ or billing@, kept out of support views and stats.</Trans>,
    },
  ]
  return (
    <Field label={<Trans>What is this inbox for?</Trans>}>
      <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label={t`Purpose`}>
        {items.map((it) => (
          <button
            key={it.value}
            type="button"
            role="radio"
            aria-checked={value === it.value}
            onClick={() => onChange(it.value)}
            className={cn(
              "flex flex-col rounded-xl border px-3.5 py-2.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
              value === it.value ? "border-foreground bg-surface" : "hover:border-faint"
            )}
            data-testid={`purpose-${it.value}`}
          >
            <span className="font-medium">{it.title}</span>
            <span className="text-caption text-faint">{it.hint}</span>
          </button>
        ))}
      </div>
    </Field>
  )
}
