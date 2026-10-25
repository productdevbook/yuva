import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronsUpDownIcon, LoaderIcon } from "lucide-react"
import { useMemo, useState } from "react"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Switch } from "@/components/ui/switch"
import { useUpdateBroadcast } from "@/features/broadcasts/broadcastQueries"
import { Field } from "@/features/settings/ui"
import type { Broadcast, BroadcastSendWindow } from "@/lib/api"
import { timeZones } from "@/lib/utils"
import {
  browserZone,
  clock,
  clockChange,
  dayLabel,
  localParts,
  windowInstants,
  wallClock,
  windowMinutes,
  zoneCity,
  zoneLabel,
  zoneLongName,
} from "@/lib/zones"
import { cn } from "@/lib/utils"

const MIN_WINDOW = 30
const END_OF_DAY = "24:00"

export type HoursText = { on: boolean; start: string; end: string }

export function hoursOf(w: BroadcastSendWindow | undefined): HoursText {
  return { on: !!w, start: w?.start ?? "05:00", end: w?.end ?? "12:00" }
}

export function hoursInvalid(h: HoursText) {
  return h.on && windowMinutes(h.start, h.end) < MIN_WINDOW
}

export function windowOf(h: HoursText): BroadcastSendWindow | null {
  return h.on && !hoursInvalid(h) ? { start: h.start, end: h.end } : null
}

export function zoneChoices(...first: (string | undefined)[]) {
  return [...new Set([...first.filter((z): z is string => !!z), browserZone(), "UTC", ...timeZones()])]
}

export function ZonePicker({
  value,
  onChange,
  disabled,
  inboxZone,
  id,
  className,
}: {
  value: string
  onChange: (zone: string) => void
  disabled?: boolean
  inboxZone?: string
  id?: string
  className?: string
}) {
  const { t, i18n } = useLingui()
  const [open, setOpen] = useState(false)
  const [now] = useState(() => new Date())
  const zones = useMemo(() => zoneChoices(value, inboxZone), [value, inboxZone])
  const rows = useMemo(
    () =>
      zones.map((z) => ({
        zone: z,
        label: zoneLabel(z, now, i18n.locale),
        long: zoneLongName(z, now, i18n.locale),
      })),
    [zones, now, i18n.locale]
  )
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            id={id}
            type="button"
            variant="outline"
            disabled={disabled}
            className={cn("h-9 w-full justify-between rounded-xl px-3 font-normal", className)}
            data-testid="broadcast-zone"
          />
        }
      >
        <span className="truncate">{zoneLabel(value, now, i18n.locale)}</span>
        <ChevronsUpDownIcon className="size-3.5 shrink-0 text-faint" />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--anchor-width) min-w-72 gap-0 p-0">
        <Command label={t`Time zone`}>
          <CommandInput placeholder={t`Search time zones`} />
          <CommandList>
            <CommandEmpty>
              <Trans>No zone matches</Trans>
            </CommandEmpty>
            {rows.map((r) => (
              <CommandItem
                key={r.zone}
                value={`${r.label} ${r.zone} ${r.long}`}
                onSelect={() => {
                  onChange(r.zone)
                  setOpen(false)
                }}
                className="flex items-start gap-2"
              >
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">{r.label}</span>
                  <small className="truncate text-caption text-faint" dir="ltr">
                    {r.long ? `${r.long} · ${r.zone}` : r.zone}
                  </small>
                </span>
                {r.zone === value && <CheckIcon className="mt-0.5 size-4 shrink-0" />}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function HoursSummary({ zone, hours, days }: { zone: string; hours: HoursText; days: number }) {
  const { i18n } = useLingui()
  const [now] = useState(() => new Date())
  const city = zoneCity(zone)
  const day = localParts(zone, now).date
  const inst = windowInstants(zone, day, hours.start, hours.end)
  const start = wallClock(hours.start, i18n.locale)
  const end = hours.end === END_OF_DAY ? wallClock("00:00", i18n.locale) : wallClock(hours.end, i18n.locale)
  const utcStart = inst ? clock(inst.from, "UTC", i18n.locale) : ""
  const utcEnd = inst ? clock(inst.to, "UTC", i18n.locale) : ""
  const mine = browserZone()
  const mineStart = inst ? clock(inst.from, mine, i18n.locale) : ""
  const mineEnd = inst ? clock(inst.to, mine, i18n.locale) : ""
  const showMine = !!inst && mine !== zone && (mineStart !== start || mineEnd !== end)
  const mineName = zoneLabel(mine, now, i18n.locale)
  const change = clockChange(zone, now, new Date(now.getTime() + days * 86_400_000))
  const date = change ? dayLabel(change, i18n.locale) : ""
  return (
    <div className="flex flex-col gap-1 text-small text-muted-foreground" data-testid="broadcast-hours-summary">
      <p>
        <Trans>
          Mails go out between {start} and {end} {city} time ({utcStart}–{utcEnd} UTC) and continue the next day.
        </Trans>
      </p>
      {showMine && (
        <p>
          <Trans>
            That is {mineStart}–{mineEnd} your time ({mineName}).
          </Trans>
        </p>
      )}
      {change && (
        <p>
          <Trans>
            Clocks change on {date}; the hours stay {start}–{end} local time.
          </Trans>
        </p>
      )}
    </div>
  )
}

export function SendTimeFields({
  zone,
  onZone,
  hours,
  onHours,
  disabled,
  inboxZone,
  days = 14,
  idPrefix = "broadcast-hours",
}: {
  zone: string
  onZone: (zone: string) => void
  hours: HoursText
  onHours: (h: HoursText) => void
  disabled?: boolean
  inboxZone?: string
  days?: number
  idPrefix?: string
}) {
  const { t } = useLingui()
  const bad = hoursInvalid(hours)
  const allDay = hours.end === END_OF_DAY
  return (
    <div className="flex flex-col gap-4">
      <Field label={<Trans>Time zone</Trans>} htmlFor={`${idPrefix}-zone`}>
        <ZonePicker id={`${idPrefix}-zone`} value={zone} onChange={onZone} disabled={disabled} inboxZone={inboxZone} />
      </Field>
      <label className="flex items-start justify-between gap-4 text-body">
        <span className="flex flex-col gap-0.5">
          <span className="font-medium">
            <Trans>Only send during set hours</Trans>
          </span>
          <span className="text-small text-faint">
            <Trans>Outside them sending waits and goes on when they open again.</Trans>
          </span>
        </span>
        <Switch
          checked={hours.on}
          disabled={disabled}
          onCheckedChange={(on) => onHours({ ...hours, on })}
          aria-label={t`Only send during set hours`}
          data-testid={`${idPrefix}-on`}
        />
      </label>
      {hours.on && (
        <>
          <div className="grid gap-3 @2xl:grid-cols-2">
            <Field label={<Trans>From</Trans>} htmlFor={`${idPrefix}-start`}>
              <Input
                id={`${idPrefix}-start`}
                type="time"
                step={900}
                dir="ltr"
                value={hours.start}
                disabled={disabled}
                aria-invalid={bad || undefined}
                onChange={(e) => onHours({ ...hours, start: e.target.value })}
                data-testid={`${idPrefix}-start`}
              />
            </Field>
            <Field
              label={<Trans>Until</Trans>}
              htmlFor={`${idPrefix}-end`}
              hint={
                <button
                  type="button"
                  disabled={disabled}
                  aria-pressed={allDay}
                  onClick={() => onHours({ ...hours, end: allDay ? "23:45" : END_OF_DAY })}
                  className="text-brand underline-offset-4 hover:underline disabled:opacity-60"
                  data-testid={`${idPrefix}-allday`}
                >
                  {allDay ? <Trans>Pick a time</Trans> : <Trans>End of day (24:00)</Trans>}
                </button>
              }
              error={bad ? <Trans>Pick at least 30 minutes</Trans> : undefined}
            >
              {allDay ? (
                <Input
                  id={`${idPrefix}-end`}
                  dir="ltr"
                  value={END_OF_DAY}
                  readOnly
                  disabled={disabled}
                  data-testid={`${idPrefix}-end`}
                />
              ) : (
                <Input
                  id={`${idPrefix}-end`}
                  type="time"
                  step={900}
                  dir="ltr"
                  value={hours.end}
                  disabled={disabled}
                  aria-invalid={bad || undefined}
                  onChange={(e) => onHours({ ...hours, end: e.target.value })}
                  data-testid={`${idPrefix}-end`}
                />
              )}
            </Field>
          </div>
          {!bad && <HoursSummary zone={zone} hours={hours} days={days} />}
        </>
      )}
    </div>
  )
}

export function SendTimeDialog({ b, inboxZone, onClose }: { b: Broadcast; inboxZone?: string; onClose: () => void }) {
  const [zone, setZone] = useState(b.timezone)
  const [hours, setHours] = useState<HoursText>(() => hoursOf(b.send_window))
  const update = useUpdateBroadcast(b.id)
  const name = b.name
  const blocked = hoursInvalid(hours)
  const submit = () => {
    if (blocked) return
    update.mutate({ timezone: zone, send_window: windowOf(hours) }, { onSuccess: onClose })
  }
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="broadcast-hours-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Sending hours of “{name}”</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>Applies to the next mail.</Trans>
            </DialogDescription>
          </DialogHeader>
          <SendTimeFields
            zone={zone}
            onZone={setZone}
            hours={hours}
            onHours={setHours}
            inboxZone={inboxZone}
            idPrefix="broadcast-hours-dialog"
          />
          <ErrorLine error={update.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={blocked || update.isPending} data-testid="broadcast-hours-save">
              {update.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
