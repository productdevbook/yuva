import { Trans, useLingui } from "@lingui/react/macro"
import { LayersIcon, MailIcon, MessageSquareIcon } from "lucide-react"

import type { WorkspaceFocus } from "@/lib/api"
import { cn } from "@/lib/utils"

export function FocusChoice({
  value,
  onChange,
  disabled,
  cards,
}: {
  value: WorkspaceFocus | undefined
  onChange: (f: WorkspaceFocus) => void
  disabled?: boolean
  cards?: boolean
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
  if (cards) {
    const Glyph = { support: MessageSquareIcon, mail: MailIcon, both: LayersIcon }
    const rail = {
      support: [t`Inbox`, t`Mail`, t`Mentions`],
      mail: [t`Mail`, t`Inbox`, t`Mentions`],
      both: [t`Inbox`, t`Mail`, t`Mentions`],
    }
    return (
      <div
        className="grid grid-cols-3 gap-2.5 @max-[760px]:grid-cols-1"
        role="radiogroup"
        aria-label={t`What Yuva is for`}
      >
        {items.map((it) => {
          const on = value === it.value
          const Icon = Glyph[it.value]
          return (
            <button
              key={it.value}
              type="button"
              role="radio"
              aria-checked={on}
              disabled={disabled}
              onClick={() => onChange(it.value)}
              className={cn(
                "relative flex flex-col gap-2.5 rounded-xl border p-3.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 @max-[760px]:flex-row @max-[760px]:items-start",
                on ? "border-brand bg-brand-wash ring-1 ring-brand" : "border-input bg-card hover:border-faint"
              )}
              data-testid={`focus-${it.value}`}
            >
              <span
                aria-hidden
                className={cn(
                  "absolute end-3.5 top-3.5 size-4 rounded-full border-[1.5px] bg-card",
                  on ? "border-[5px] border-brand" : "border-input"
                )}
              />
              <span
                aria-hidden
                className={cn(
                  "grid size-[34px] shrink-0 place-items-center rounded-[9px]",
                  on ? "bg-brand/15 text-brand" : "bg-muted text-muted-foreground"
                )}
              >
                <Icon className="size-[18px]" />
              </span>
              <span className="flex min-w-0 flex-col gap-0.5 pe-6">
                <span className="font-semibold">{it.title}</span>
                <span className="text-small text-muted-foreground">{it.hint}</span>
              </span>
              <span
                aria-hidden
                className="mt-auto flex flex-col gap-0.5 rounded-[9px] border bg-surface p-2 @max-[760px]:hidden"
              >
                {[t`Home`, ...rail[it.value]].map((name, i) => (
                  <span
                    key={name}
                    className={cn(
                      "flex h-5 items-center gap-1.5 rounded-[5px] px-1.5 text-[11.5px]",
                      i === 0 ? "bg-card font-semibold text-foreground shadow-[0_0_0_1px_var(--border)]" : "text-faint"
                    )}
                  >
                    <i className="size-1.5 rounded-[2px] bg-current opacity-50" />
                    {name}
                  </span>
                ))}
              </span>
            </button>
          )
        })}
      </div>
    )
  }
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
