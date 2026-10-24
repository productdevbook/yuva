import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Field } from "@/features/settings/ui"

const MINUTE = 60_000
const DAY = 24 * 60 * MINUTE
export const MIN_AHEAD = MINUTE
export const MAX_AHEAD = 366 * DAY

export function formatWhen(at: Date | string, locale: string, now = new Date()) {
  const d = typeof at === "string" ? new Date(at) : at
  return new Intl.DateTimeFormat(locale, {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: d.getFullYear() === now.getFullYear() ? undefined : "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(d)
}

function morning(base: Date, addDays: number) {
  const d = new Date(base)
  d.setDate(d.getDate() + addDays)
  d.setHours(8, 0, 0, 0)
  return d
}

export function sendTimes(now: Date) {
  const later = new Date(Math.ceil((now.getTime() + 3 * 60 * MINUTE) / (15 * MINUTE)) * 15 * MINUTE)
  const tomorrow = morning(now, 1)
  const untilMonday = (8 - now.getDay()) % 7 || 7
  const monday = morning(now, untilMonday)
  return {
    later: later.toDateString() === now.toDateString() ? later : undefined,
    tomorrow,
    monday: monday.getTime() === tomorrow.getTime() ? undefined : monday,
  }
}

function inputDate(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0")
  return {
    date: `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`,
    time: `${pad(d.getHours())}:${pad(d.getMinutes())}`,
  }
}

export function PickTimeDialog({
  open,
  onOpenChange,
  title,
  confirm,
  initial,
  onPick,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: React.ReactNode
  confirm: React.ReactNode
  initial?: Date
  onPick: (at: Date) => void
}) {
  const [bounds] = useState(() => {
    const now = new Date()
    return {
      start: inputDate(initial && initial.getTime() > now.getTime() ? initial : morning(now, 1)),
      min: inputDate(now).date,
      max: inputDate(new Date(now.getTime() + MAX_AHEAD)).date,
    }
  })
  const [date, setDate] = useState(bounds.start.date)
  const [time, setTime] = useState(bounds.start.time)
  const [problem, setProblem] = useState<"soon" | "far" | null>(null)
  const parse = (d: string, tm: string) => {
    const at = d && tm ? new Date(`${d}T${tm}`) : undefined
    return at && !Number.isNaN(at.getTime()) ? at : undefined
  }
  const check = (d: string, tm: string) => {
    const at = parse(d, tm)
    if (!at) return { at, problem: null }
    const ahead = at.getTime() - Date.now()
    return { at, problem: ahead < MIN_AHEAD ? ("soon" as const) : ahead > MAX_AHEAD ? ("far" as const) : null }
  }
  const { min, max } = bounds
  const invalid = !parse(date, time)
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm" data-testid="pick-time">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            const r = check(date, time)
            setProblem(r.problem)
            if (r.at && !r.problem) {
              onPick(r.at)
              onOpenChange(false)
            }
          }}
        >
          <div className="grid grid-cols-2 gap-3 phone:grid-cols-1">
            <Field label={<Trans>Date</Trans>} htmlFor="pick-date">
              <Input
                id="pick-date"
                type="date"
                required
                min={min}
                max={max}
                value={date}
                onChange={(e) => {
                  setDate(e.target.value)
                  setProblem(check(e.target.value, time).problem)
                }}
                data-testid="pick-date"
              />
            </Field>
            <Field label={<Trans>Time</Trans>} htmlFor="pick-time">
              <Input
                id="pick-time"
                type="time"
                required
                value={time}
                onChange={(e) => {
                  setTime(e.target.value)
                  setProblem(check(date, e.target.value).problem)
                }}
                data-testid="pick-time-input"
              />
            </Field>
          </div>
          {problem && (
            <p role="alert" className="text-caption text-destructive" data-testid="pick-time-error">
              {problem === "soon" ? (
                <Trans>Pick a time at least a minute from now.</Trans>
              ) : (
                <Trans>Pick a time within a year.</Trans>
              )}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={invalid || problem !== null} data-testid="pick-time-confirm">
              {confirm}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Times({ onPick, onCustom }: { onPick: (at: Date) => void; onCustom: () => void }) {
  const { i18n, t } = useLingui()
  const [times] = useState(() => sendTimes(new Date()))
  const hour = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const day = new Intl.DateTimeFormat(i18n.locale, { weekday: "short", hour: "2-digit", minute: "2-digit" })
  const rows: { key: string; label: string; detail: string; at: Date }[] = []
  if (times.later) rows.push({ key: "later", label: t`Later today`, detail: hour.format(times.later), at: times.later })
  rows.push({ key: "tomorrow", label: t`Tomorrow morning`, detail: day.format(times.tomorrow), at: times.tomorrow })
  if (times.monday)
    rows.push({ key: "monday", label: t`Monday morning`, detail: day.format(times.monday), at: times.monday })
  return (
    <DropdownMenuGroup>
      <DropdownMenuLabel>
        <Trans>Send later</Trans>
      </DropdownMenuLabel>
      {rows.map((r) => (
        <DropdownMenuItem key={r.key} onClick={() => onPick(r.at)} data-testid={`send-later-${r.key}`}>
          <span className="flex-1">{r.label}</span>
          <span className="text-caption text-faint">{r.detail}</span>
        </DropdownMenuItem>
      ))}
      <DropdownMenuSeparator />
      <DropdownMenuItem onClick={onCustom} data-testid="send-later-pick">
        <Trans>Pick a time…</Trans>
      </DropdownMenuItem>
    </DropdownMenuGroup>
  )
}

export function SendLaterMenu({
  trigger,
  children,
  onPick,
  open,
  onOpenChange,
  title,
  confirm,
  initial,
  align = "end",
  side = "top",
  error,
}: {
  trigger: React.ReactElement
  children?: React.ReactNode
  onPick: (at: Date) => void
  open?: boolean
  onOpenChange?: (open: boolean) => void
  title?: React.ReactNode
  confirm?: React.ReactNode
  initial?: Date
  align?: "start" | "end"
  side?: "top" | "bottom"
  error?: unknown
}) {
  const [custom, setCustom] = useState(false)
  return (
    <>
      <DropdownMenu open={open} onOpenChange={onOpenChange}>
        <DropdownMenuTrigger render={trigger}>{children}</DropdownMenuTrigger>
        <DropdownMenuContent align={align} side={side} className="w-[280px] max-w-[calc(100vw-32px)]">
          <Times onPick={onPick} onCustom={() => setCustom(true)} />
          {error ? <ErrorLine error={error} className="px-2.5 pb-1 text-caption" /> : null}
        </DropdownMenuContent>
      </DropdownMenu>
      {custom && (
        <PickTimeDialog
          open
          onOpenChange={setCustom}
          title={title ?? <Trans>Send later</Trans>}
          confirm={confirm ?? <Trans>Schedule</Trans>}
          initial={initial}
          onPick={onPick}
        />
      )}
    </>
  )
}
