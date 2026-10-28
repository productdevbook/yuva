import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Link } from "react-router"

import { Skeleton } from "@/components/ui/skeleton"
import { ConditionRow, useConditionText } from "@/features/broadcasts/AudienceBuilder"
import { useLists } from "@/features/broadcasts/queries"
import { Field } from "@/features/settings/ui"
import type { Broadcast, BroadcastBasis, BroadcastCondition, BroadcastTriggerEvent } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export type TriggerDraft = { event: BroadcastTriggerEvent; list_id?: string; condition?: BroadcastCondition }

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

export function useListNames(inboxId: string) {
  const { t } = useLingui()
  const open = useLists(inboxId, false)
  const archived = useLists(inboxId, true)
  const names = new Map(
    [...(open.data?.pages ?? []), ...(archived.data?.pages ?? [])].flatMap((p) => p.items).map((l) => [l.id, l.name])
  )
  const pending = open.isPending || archived.isPending
  return (id: string | undefined) => (id ? (names.get(id) ?? (pending ? "…" : t`a deleted list`)) : "…")
}

export function useTriggerSentence(b: Pick<Broadcast, "inbox_id" | "trigger">) {
  const { t } = useLingui()
  const inbox = useInboxes().data?.find((i) => i.id === b.inbox_id)?.name ?? ""
  const listName = useListNames(b.inbox_id)
  const conditionText = useConditionText()
  const minutes = b.trigger?.delay_minutes ?? 0
  const event = b.trigger?.event ?? "contact_identified"
  const list = listName(b.trigger?.list_id)
  const match = b.trigger?.condition ? conditionText(b.trigger.condition) : "…"
  if (event === "subscribed") {
    if (minutes === 0) return t`when someone confirms ${list}`
    if (minutes % PER.days === 0)
      return plural(minutes / PER.days, {
        one: `# day after someone confirms ${list}`,
        other: `# days after someone confirms ${list}`,
      })
    if (minutes % PER.hours === 0)
      return plural(minutes / PER.hours, {
        one: `# hour after someone confirms ${list}`,
        other: `# hours after someone confirms ${list}`,
      })
    return plural(minutes, {
      one: `# minute after someone confirms ${list}`,
      other: `# minutes after someone confirms ${list}`,
    })
  }
  if (event === "attribute_changed") {
    if (minutes === 0) return t`when someone starts to match ${match}`
    if (minutes % PER.days === 0)
      return plural(minutes / PER.days, {
        one: `# day after someone starts to match ${match}`,
        other: `# days after someone starts to match ${match}`,
      })
    if (minutes % PER.hours === 0)
      return plural(minutes / PER.hours, {
        one: `# hour after someone starts to match ${match}`,
        other: `# hours after someone starts to match ${match}`,
      })
    return plural(minutes, {
      one: `# minute after someone starts to match ${match}`,
      other: `# minutes after someone starts to match ${match}`,
    })
  }
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

export function useEventNames(): Record<BroadcastTriggerEvent, string> {
  const { t } = useLingui()
  return {
    contact_identified: t`Someone first signs in`,
    subscribed: t`Someone confirms a subscription to a list`,
    attribute_changed: t`Someone starts to match a condition`,
  }
}

export function TriggerEvent({
  trigger,
  inboxId,
  disabled,
  onChange,
}: {
  trigger: TriggerDraft
  inboxId: string
  disabled: boolean
  onChange: (t: TriggerDraft) => void
}) {
  const { t } = useLingui()
  const events = useEventNames()
  const lists = useLists(inboxId, false)
  const archived = useLists(inboxId, true)
  const items = lists.data?.pages.flatMap((p) => p.items) ?? []
  const gone =
    !!trigger.list_id &&
    !lists.isPending &&
    !archived.isPending &&
    !items.some((l) => l.id === trigger.list_id) &&
    !archived.data?.pages.some((p) => p.items.some((l) => l.id === trigger.list_id))
  const archivedPick = !!trigger.list_id && !items.some((l) => l.id === trigger.list_id) && !gone && !lists.isPending
  const listItems = Object.fromEntries(items.map((l) => [l.id, l.name]))
  const archivedName = archived.data?.pages.flatMap((p) => p.items).find((l) => l.id === trigger.list_id)?.name
  return (
    <>
      <Field label={<Trans>Starts when</Trans>}>
        <Select
          value={trigger.event}
          items={events}
          disabled={disabled}
          onValueChange={(v) => v && onChange({ ...trigger, event: v as BroadcastTriggerEvent })}
        >
          <SelectTrigger className="w-full min-w-0" aria-label={t`Starts when`} data-testid="trigger-event">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(Object.keys(events) as BroadcastTriggerEvent[]).map((e) => (
              <SelectItem key={e} value={e} data-testid={`trigger-event-${e}`}>
                {events[e]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      {trigger.event === "subscribed" && (
        <Field
          label={<Trans>List</Trans>}
          hint={
            <Trans>
              Counts when a subscription becomes confirmed, not when it is still waiting for the person to confirm.
              People an import added never start it.
            </Trans>
          }
          error={
            gone ? <Trans>This list was deleted. Pick another list, or stop this triggered e-mail.</Trans> : undefined
          }
        >
          {lists.isPending ? (
            <Skeleton className="h-9 w-full" />
          ) : items.length === 0 && !archivedPick ? (
            <p className="text-body text-muted-foreground" data-testid="trigger-no-lists">
              <Trans>This inbox has no mailing list yet.</Trans>{" "}
              <Link to="/broadcasts/lists" className="text-brand underline-offset-4 hover:underline">
                <Trans>Go to the lists</Trans>
              </Link>
            </p>
          ) : (
            <Select
              value={trigger.list_id ?? null}
              items={archivedPick && archivedName ? { ...listItems, [trigger.list_id!]: archivedName } : listItems}
              disabled={disabled}
              onValueChange={(v) => v && onChange({ ...trigger, list_id: v })}
            >
              <SelectTrigger className="w-full min-w-0" aria-label={t`List`} data-testid="trigger-list">
                <SelectValue placeholder={t`Pick a list`} />
              </SelectTrigger>
              <SelectContent>
                {items.map((l) => (
                  <SelectItem key={l.id} value={l.id}>
                    {l.name}
                  </SelectItem>
                ))}
                {archivedPick && archivedName && <SelectItem value={trigger.list_id!}>{archivedName}</SelectItem>}
              </SelectContent>
            </Select>
          )}
        </Field>
      )}
      {trigger.event === "attribute_changed" && (
        <Field
          label={<Trans>Condition</Trans>}
          hint={
            <Trans>
              Starts when a change to the person’s attributes makes them match, and they did not match before. People
              who already match when you start it are not included.
            </Trans>
          }
        >
          <ul className="rounded-xl border" data-testid="trigger-condition">
            <ConditionRow
              c={trigger.condition ?? { attribute: "", op: "eq", value: "" }}
              index={0}
              disabled={disabled}
              onChange={(condition) => onChange({ ...trigger, condition })}
            />
          </ul>
        </Field>
      )}
    </>
  )
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
      hint={<Trans>0 sends it as soon as it starts. The longest delay is 365 days.</Trans>}
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
