import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { ChoiceSelect, Field, ToggleRow } from "@/features/settings/ui"
import {
  api,
  unwrap,
  type ChannelKind,
  type Priority,
  type SlaPolicy,
  type SlaPolicyCreate,
  type SlaPolicyUpdate,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useLabels } from "@/lib/workspace"

const MAX_MINUTES = 525600
const CHANNELS: ChannelKind[] = ["chat", "email", "app", "api"]
const PRIORITY_LIST: Priority[] = ["urgent", "high", "normal", "low"]

type Unit = "minutes" | "hours"
type Amount = { value: string; unit: Unit }

function toAmount(minutes: number | undefined): Amount {
  if (minutes === undefined) return { value: "", unit: "minutes" }
  return minutes >= 60 && minutes % 60 === 0
    ? { value: String(minutes / 60), unit: "hours" }
    : { value: String(minutes), unit: "minutes" }
}

function toMinutes(a: Amount): number | undefined {
  const n = Number(a.value)
  if (a.value.trim() === "" || !Number.isInteger(n)) return undefined
  return a.unit === "hours" ? n * 60 : n
}

function validAmount(a: Amount) {
  if (a.value.trim() === "") return true
  const m = toMinutes(a)
  return m !== undefined && m >= 1 && m <= MAX_MINUTES
}

function sameList<T>(a: T[], b: T[]) {
  return a.length === b.length && a.every((x) => b.includes(x))
}

function toggled<T>(list: T[], v: T) {
  return list.includes(v) ? list.filter((x) => x !== v) : [...list, v]
}

function Chips<T extends string>({
  value,
  onChange,
  options,
  label,
}: {
  value: T[]
  onChange: (next: T[]) => void
  options: readonly (readonly [T, string])[]
  label: string
}) {
  return (
    <div className="flex flex-wrap gap-1.5" role="group" aria-label={label}>
      {options.map(([v, text]) => {
        const on = value.includes(v)
        return (
          <button
            key={v}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(toggled(value, v))}
            className={cn(
              "inline-flex h-[30px] max-w-full items-center rounded-lg border px-2.5 text-body outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
              on
                ? "border-foreground bg-surface font-medium text-foreground"
                : "border-input bg-background text-muted-foreground hover:border-faint hover:text-foreground"
            )}
          >
            <span className="truncate">{text}</span>
          </button>
        )
      })}
    </div>
  )
}

function AmountInput({
  id,
  value,
  onChange,
  testId,
}: {
  id: string
  value: Amount
  onChange: (next: Amount) => void
  testId: string
}) {
  const { t } = useLingui()
  return (
    <div className="flex gap-2">
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        min={1}
        max={MAX_MINUTES}
        value={value.value}
        placeholder={t`Not tracked`}
        aria-invalid={!validAmount(value) || undefined}
        onChange={(e) => onChange({ ...value, value: e.target.value })}
        data-testid={testId}
      />
      <ChoiceSelect<Unit>
        value={value.unit}
        onChange={(unit) => onChange({ ...value, unit })}
        options={[
          ["minutes", t`minutes`],
          ["hours", t`hours`],
        ]}
        label={t`Unit`}
        className="w-32 shrink-0"
      />
    </div>
  )
}

function Form({ policy, onClose }: { policy: SlaPolicy | null; onClose: () => void }) {
  const { t } = useLingui()
  const enums = useEnumText()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const [name, setName] = useState(policy?.name ?? "")
  const [enabled, setEnabled] = useState(policy?.enabled ?? true)
  const [inboxIds, setInboxIds] = useState(policy?.inbox_ids ?? [])
  const [kinds, setKinds] = useState<ChannelKind[]>(policy?.channel_kinds ?? [])
  const [labelIds, setLabelIds] = useState(policy?.label_ids ?? [])
  const [priorities, setPriorities] = useState<Priority[]>(policy?.priorities ?? [])
  const [first, setFirst] = useState(toAmount(policy?.first_reply_minutes))
  const [next, setNext] = useState(toAmount(policy?.next_reply_minutes))
  const [resolution, setResolution] = useState(toAmount(policy?.resolution_minutes))
  const [warn, setWarn] = useState(String(policy?.warn_minutes ?? 15))

  const firstMin = toMinutes(first)
  const nextMin = toMinutes(next)
  const resolutionMin = toMinutes(resolution)
  const warnMin = Number(warn)
  const warnValid = warn.trim() !== "" && Number.isInteger(warnMin) && warnMin >= 0 && warnMin <= 1440
  const valid =
    name.trim() !== "" &&
    validAmount(first) &&
    validAmount(next) &&
    validAmount(resolution) &&
    (firstMin !== undefined || nextMin !== undefined || resolutionMin !== undefined) &&
    warnValid
  const changed =
    !policy ||
    name.trim() !== policy.name ||
    enabled !== policy.enabled ||
    !sameList(inboxIds, policy.inbox_ids) ||
    !sameList(kinds, policy.channel_kinds) ||
    !sameList(labelIds, policy.label_ids) ||
    !sameList(priorities, policy.priorities) ||
    firstMin !== policy.first_reply_minutes ||
    nextMin !== policy.next_reply_minutes ||
    resolutionMin !== policy.resolution_minutes ||
    warnMin !== policy.warn_minutes

  const save = useMutation({
    mutationFn: () => {
      const common = {
        name: name.trim(),
        enabled,
        inbox_ids: inboxIds,
        channel_kinds: kinds,
        label_ids: labelIds,
        priorities,
        warn_minutes: warnMin,
      }
      if (!policy) {
        const body: SlaPolicyCreate = {
          ...common,
          first_reply_minutes: firstMin,
          next_reply_minutes: nextMin,
          resolution_minutes: resolutionMin,
        }
        return unwrap(api.POST("/v1/sla-policies", { body }))
      }
      const body: SlaPolicyUpdate = {
        ...common,
        first_reply_minutes: firstMin ?? null,
        next_reply_minutes: nextMin ?? null,
        resolution_minutes: resolutionMin ?? null,
      }
      return unwrap(api.PATCH("/v1/sla-policies/{slaPolicyId}", { params: { path: { slaPolicyId: policy.id } }, body }))
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.slaPolicies(ws) })
      void qc.invalidateQueries({ queryKey: keys.lists(ws) })
      onClose()
    },
  })

  return (
    <form
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <Field label={<Trans>Name</Trans>} htmlFor="sla-name">
        <Input
          id="sla-name"
          required
          maxLength={100}
          value={name}
          placeholder={t`Priority customers`}
          onChange={(e) => setName(e.target.value)}
          data-testid="sla-name"
        />
      </Field>
      <ToggleRow title={<Trans>Enabled</Trans>} hint={<Trans>A policy that is off applies to nothing.</Trans>}>
        <Switch checked={enabled} onCheckedChange={setEnabled} aria-label={t`Enabled`} data-testid="sla-enabled" />
      </ToggleRow>
      <fieldset className="flex flex-col gap-4 rounded-xl border p-4">
        <legend className="px-1 text-small font-medium text-muted-foreground">
          <Trans>Applies to</Trans>
        </legend>
        <p className="text-small text-faint">
          <Trans>A conversation must match every row you fill in. Leave a row empty to match anything.</Trans>
        </p>
        {inboxes.length > 0 && (
          <Field label={<Trans>Inboxes</Trans>}>
            <Chips
              value={inboxIds}
              onChange={setInboxIds}
              options={inboxes.map((i) => [i.id, i.name] as const)}
              label={t`Inboxes`}
            />
          </Field>
        )}
        <Field label={<Trans>Channels</Trans>}>
          <Chips
            value={kinds}
            onChange={setKinds}
            options={CHANNELS.map((k) => [k, enums.channel[k]] as const)}
            label={t`Channels`}
          />
        </Field>
        {labels.length > 0 && (
          <Field label={<Trans>Labels</Trans>} hint={<Trans>Any of the labels you pick.</Trans>}>
            <Chips
              value={labelIds}
              onChange={setLabelIds}
              options={labels.map((l) => [l.id, l.name] as const)}
              label={t`Labels`}
            />
          </Field>
        )}
        <Field label={<Trans>Priority</Trans>}>
          <Chips
            value={priorities}
            onChange={setPriorities}
            options={PRIORITY_LIST.map((p) => [p, enums.priority[p]] as const)}
            label={t`Priority`}
          />
        </Field>
      </fieldset>
      <fieldset className="flex flex-col gap-4 rounded-xl border p-4">
        <legend className="px-1 text-small font-medium text-muted-foreground">
          <Trans>Targets</Trans>
        </legend>
        <p className="text-small text-faint">
          <Trans>
            Targets count in the business hours of the conversation's inbox, in its time zone. Leave one empty to not
            track it; at least one is required.
          </Trans>
        </p>
        <Field
          label={<Trans>First reply</Trans>}
          htmlFor="sla-first"
          hint={<Trans>From the contact's first message to the first public reply.</Trans>}
        >
          <AmountInput id="sla-first" value={first} onChange={setFirst} testId="sla-first" />
        </Field>
        <Field
          label={<Trans>Next reply</Trans>}
          htmlFor="sla-next"
          hint={<Trans>From each later contact message to the reply.</Trans>}
        >
          <AmountInput id="sla-next" value={next} onChange={setNext} testId="sla-next" />
        </Field>
        <Field
          label={<Trans>Resolution</Trans>}
          htmlFor="sla-resolution"
          hint={<Trans>From the start of the conversation to its first close.</Trans>}
        >
          <AmountInput id="sla-resolution" value={resolution} onChange={setResolution} testId="sla-resolution" />
        </Field>
        <Field
          label={<Trans>Warn before it is due</Trans>}
          htmlFor="sla-warn"
          hint={<Trans>Minutes before a due time. 0 sends no warning.</Trans>}
        >
          <Input
            id="sla-warn"
            type="number"
            inputMode="numeric"
            min={0}
            max={1440}
            value={warn}
            aria-invalid={!warnValid || undefined}
            onChange={(e) => setWarn(e.target.value)}
            data-testid="sla-warn"
          />
        </Field>
      </fieldset>
      <ErrorLine error={save.error} />
      <DialogFooter>
        <Button type="submit" disabled={save.isPending || !valid || !changed} data-testid="sla-save">
          {policy ? <Trans>Save</Trans> : <Trans>Create policy</Trans>}
        </Button>
      </DialogFooter>
    </form>
  )
}

export function PolicyDialog({ policy, onClose }: { policy: SlaPolicy | null; onClose: () => void }) {
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{policy ? <Trans>Edit policy</Trans> : <Trans>New policy</Trans>}</DialogTitle>
        </DialogHeader>
        <Form policy={policy} onClose={onClose} />
      </DialogContent>
    </Dialog>
  )
}
