import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { formatDuration } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { useUpdateBroadcast } from "@/features/broadcasts/broadcastQueries"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import { Field } from "@/features/settings/ui"
import type { Broadcast, BroadcastPace } from "@/lib/api"

const MAX = 10_000_000
const KEYS = ["per_hour", "per_day", "per_month"] as const
const WINDOW = { per_hour: 3600, per_day: 86400, per_month: 2_592_000 }

export type PaceText = Record<(typeof KEYS)[number], string>

export function paceTextOf(p: BroadcastPace | undefined): PaceText {
  return {
    per_hour: p?.per_hour ? String(p.per_hour) : "",
    per_day: p?.per_day ? String(p.per_day) : "",
    per_month: p?.per_month ? String(p.per_month) : "",
  }
}

function valid(s: string) {
  if (!/^\d+$/.test(s)) return false
  const n = Number(s)
  return n >= 1 && n <= MAX
}

export function paceInvalid(t: PaceText) {
  return KEYS.some((k) => t[k] !== "" && !valid(t[k]))
}

export function paceOf(t: PaceText): BroadcastPace {
  const out: BroadcastPace = {}
  for (const k of KEYS) if (valid(t[k])) out[k] = Number(t[k])
  return out
}

export function hasPace(p: BroadcastPace | undefined) {
  return !!p && KEYS.some((k) => !!p[k])
}

export function paceGap(p: BroadcastPace | undefined) {
  let gap = 0
  if (p) for (const k of KEYS) if (p[k]) gap = Math.max(gap, WINDOW[k] / p[k])
  return gap
}

export function usePaceSummary() {
  const { t } = useLingui()
  const num = useNumber()
  return (p: BroadcastPace | undefined) => {
    if (!hasPace(p)) return t`As fast as allowed`
    const parts: string[] = []
    if (p?.per_hour) {
      const n = num(p.per_hour)
      parts.push(t`At most ${n} an hour`)
    }
    if (p?.per_day) {
      const n = num(p.per_day)
      parts.push(t`${n} a day`)
    }
    if (p?.per_month) {
      const n = num(p.per_month)
      parts.push(t`${n} in 30 days`)
    }
    return parts.join(" · ")
  }
}

export function PaceFields({
  value,
  onChange,
  disabled,
  rate,
  windowed,
  idPrefix = "broadcast-pace",
}: {
  value: PaceText
  onChange: (v: PaceText) => void
  disabled?: boolean
  rate?: number
  windowed?: boolean
  idPrefix?: string
}) {
  const { t, i18n } = useLingui()
  const gap = paceGap(paceOf(value))
  const every = gap > 0 ? formatDuration(gap, i18n.locale) : ""
  const fields = [
    { key: "per_hour", name: "hour", label: t`Per hour` },
    { key: "per_day", name: "day", label: t`Per day` },
    { key: "per_month", name: "month", label: t`Per month (30 days)` },
  ] as const
  return (
    <div className="flex flex-col gap-3">
      <div className="grid gap-3 @2xl:grid-cols-3">
        {fields.map((f) => {
          const bad = value[f.key] !== "" && !valid(value[f.key])
          return (
            <Field
              key={f.key}
              label={f.label}
              htmlFor={`${idPrefix}-${f.name}`}
              hint={f.key === "per_day" && windowed ? <Trans>per day, within the sending hours</Trans> : undefined}
              error={bad ? <Trans>A whole number from 1 to 10,000,000</Trans> : undefined}
            >
              <Input
                id={`${idPrefix}-${f.name}`}
                inputMode="numeric"
                dir="ltr"
                value={value[f.key]}
                disabled={disabled}
                placeholder={t`No limit`}
                aria-invalid={bad || undefined}
                onChange={(e) => onChange({ ...value, [f.key]: e.target.value.trim() })}
                data-testid={`${idPrefix}-${f.name}`}
              />
            </Field>
          )
        })}
      </div>
      <div className="flex flex-col gap-0.5 text-small text-faint">
        {every && (
          <p data-testid={`${idPrefix}-gap`}>
            <Trans>One mail every {every}</Trans>
          </p>
        )}
        {rate !== undefined && (
          <p>
            <Trans>The channel’s {rate} mails a second and the workspace’s daily limit still apply.</Trans>
          </p>
        )}
      </div>
    </div>
  )
}

export function PaceDialog({ b, rate, onClose }: { b: Broadcast; rate?: number; onClose: () => void }) {
  const [none, setNone] = useState(!hasPace(b.pace))
  const [text, setText] = useState<PaceText>(() => paceTextOf(b.pace))
  const update = useUpdateBroadcast(b.id)
  const name = b.name
  const blocked = !none && (paceInvalid(text) || !hasPace(paceOf(text)))
  const submit = () => {
    if (blocked) return
    update.mutate({ pace: none ? {} : paceOf(text) }, { onSuccess: onClose })
  }
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="broadcast-pace-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Sending pace of “{name}”</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>Applies to the next mail.</Trans>
            </DialogDescription>
          </DialogHeader>
          <label className="flex items-center justify-between gap-3 text-body">
            <Trans>No limit</Trans>
            <Switch checked={none} onCheckedChange={setNone} data-testid="broadcast-pace-dialog-none" />
          </label>
          {!none && (
            <PaceFields
              value={text}
              onChange={setText}
              rate={rate}
              windowed={!!b.send_window}
              idPrefix="broadcast-pace-dialog"
            />
          )}
          <ErrorLine error={update.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={blocked || update.isPending} data-testid="broadcast-pace-save">
              {update.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
