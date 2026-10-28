import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Field } from "@/features/settings/ui"
import type { Broadcast, BroadcastBasis } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export type DelayUnit = "minutes" | "hours" | "days"
export type DelayText = { value: string; unit: DelayUnit }

export const MAX_DELAY = 525_600
const PER: Record<DelayUnit, number> = { minutes: 1, hours: 60, days: 1440 }

export function delayTextOf(minutes: number): DelayText {
  if (minutes > 0 && minutes % PER.days === 0) return { value: String(minutes / PER.days), unit: "days" }
  if (minutes > 0 && minutes % PER.hours === 0) return { value: String(minutes / PER.hours), unit: "hours" }
  return { value: String(minutes), unit: "minutes" }
}

export function delayMinutes(d: DelayText) {
  if (!/^\d{1,6}$/.test(d.value.trim())) return null
  const n = Number(d.value) * PER[d.unit]
  return n <= MAX_DELAY ? n : null
}

export function sentenceCase(s: string, locale: string) {
  return s.charAt(0).toLocaleUpperCase(locale) + s.slice(1)
}

export function useTriggerSentence(b: Pick<Broadcast, "inbox_id" | "trigger">) {
  const { t } = useLingui()
  const inbox = useInboxes().data?.find((i) => i.id === b.inbox_id)?.name ?? ""
  const minutes = b.trigger?.delay_minutes ?? 0
  if (minutes === 0) return t`when someone first signs in to ${inbox}`
  if (minutes % PER.days === 0) {
    const n = minutes / PER.days
    return plural(n, {
      one: `# day after someone first signs in to ${inbox}`,
      other: `# days after someone first signs in to ${inbox}`,
    })
  }
  if (minutes % PER.hours === 0) {
    const n = minutes / PER.hours
    return plural(n, {
      one: `# hour after someone first signs in to ${inbox}`,
      other: `# hours after someone first signs in to ${inbox}`,
    })
  }
  return plural(minutes, {
    one: `# minute after someone first signs in to ${inbox}`,
    other: `# minutes after someone first signs in to ${inbox}`,
  })
}

export function DelayFields({
  value,
  onChange,
  disabled,
}: {
  value: DelayText
  onChange: (d: DelayText) => void
  disabled: boolean
}) {
  const { t } = useLingui()
  const bad = delayMinutes(value) === null
  const units: Record<DelayUnit, string> = { minutes: t`minutes`, hours: t`hours`, days: t`days` }
  return (
    <Field
      label={<Trans>Send it after</Trans>}
      htmlFor="trigger-delay"
      hint={<Trans>0 sends it as soon as the person signs in. The longest delay is 365 days.</Trans>}
      error={bad ? <Trans>Use a whole number of at most 365 days.</Trans> : undefined}
    >
      <div className="flex items-center gap-2">
        <Input
          id="trigger-delay"
          inputMode="numeric"
          value={value.value}
          disabled={disabled}
          maxLength={6}
          aria-invalid={bad || undefined}
          className="w-28"
          onChange={(e) => onChange({ ...value, value: e.target.value.trim() })}
          data-testid="trigger-delay"
        />
        <Select
          value={value.unit}
          items={units}
          disabled={disabled}
          onValueChange={(v) => v && onChange({ ...value, unit: v as DelayUnit })}
        >
          <SelectTrigger className="w-36 min-w-0" aria-label={t`Unit`} data-testid="trigger-unit">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(Object.keys(units) as DelayUnit[]).map((u) => (
              <SelectItem key={u} value={u}>
                {units[u]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </Field>
  )
}

export function BasisChoice({
  value,
  onChange,
  disabled,
  inboxName,
}: {
  value: BroadcastBasis
  onChange: (b: BroadcastBasis) => void
  disabled: boolean
  inboxName: string
}) {
  const { t } = useLingui()
  const items: { value: BroadcastBasis; title: React.ReactNode; hint: React.ReactNode }[] = [
    {
      value: "service",
      title: <Trans>Service mail</Trans>,
      hint: <Trans>About using {inboxName}, sent to everyone who matches. Never advertising.</Trans>,
    },
    {
      value: "marketing",
      title: <Trans>Marketing</Trans>,
      hint: <Trans>Only to people subscribed to the lists you pick. For news, offers and newsletters.</Trans>,
    },
  ]
  return (
    <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label={t`Why they get this mail`}>
      {items.map((it) => (
        <button
          key={it.value}
          type="button"
          role="radio"
          aria-checked={value === it.value}
          disabled={disabled}
          onClick={() => onChange(it.value)}
          className={cn(
            "flex flex-col rounded-xl border px-3.5 py-2.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default",
            value === it.value
              ? "border-foreground bg-surface"
              : "hover:border-faint disabled:opacity-60 disabled:hover:border-input"
          )}
          data-testid={`basis-${it.value}`}
        >
          <span className="font-medium">{it.title}</span>
          <span className="text-caption text-faint">{it.hint}</span>
        </button>
      ))}
    </div>
  )
}
