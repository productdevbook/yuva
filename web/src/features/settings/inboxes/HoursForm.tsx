import { Trans, useLingui } from "@lingui/react/macro"
import { XIcon } from "lucide-react"
import { useState } from "react"

import { useEnumText, WEEKDAYS } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { useInboxOutlet } from "@/features/settings/inboxes/InboxLayout"
import { useSaveInbox } from "@/features/settings/inboxes/queries"
import { FormActions, FormCard, Section, ToggleRow } from "@/features/settings/ui"
import type { BusinessHours, Weekday } from "@/lib/api"
import { useSession } from "@/lib/session"

type Interval = BusinessHours["intervals"][number]

function Day({
  day,
  rows,
  editable,
  onChange,
  onRemove,
  onAdd,
}: {
  day: Weekday
  rows: { iv: Interval; i: number }[]
  editable: boolean
  onChange: (i: number, patch: Partial<Interval>) => void
  onRemove: (i: number) => void
  onAdd: () => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  return (
    <li className="flex flex-col gap-2 px-5 py-3 sm:flex-row sm:items-start">
      <span className="w-28 shrink-0 pt-2 text-sm font-medium">{text.weekday[day]}</span>
      <div className="flex flex-1 flex-col gap-2">
        {rows.length === 0 && (
          <span className="pt-2 text-sm text-faint">
            <Trans>Closed</Trans>
          </span>
        )}
        {rows.map(({ iv, i }) => (
          <div key={i} className="flex items-center gap-2">
            <Input type="time" value={iv.start} disabled={!editable} onChange={(e) => onChange(i, { start: e.target.value })} className="min-w-0 flex-1 sm:w-32 sm:flex-none" aria-label={t`Opens`} />
            <span className="text-faint">–</span>
            <Input
              type="time"
              value={iv.end === "24:00" ? "23:59" : iv.end}
              disabled={!editable}
              onChange={(e) => onChange(i, { end: e.target.value })}
              className="min-w-0 flex-1 sm:w-32 sm:flex-none"
              aria-label={t`Closes`}
            />
            {editable && (
              <Button type="button" variant="ghost" size="icon-sm" aria-label={t`Remove these hours`} onClick={() => onRemove(i)}>
                <XIcon />
              </Button>
            )}
          </div>
        ))}
      </div>
      {editable && (
        <Button type="button" variant="ghost" size="sm" className="self-start" onClick={onAdd}>
          <Trans>Add hours</Trans>
        </Button>
      )}
    </li>
  )
}

export function HoursForm() {
  const { t } = useLingui()
  const { inbox } = useInboxOutlet()
  const { canManage } = useSession()
  const save = useSaveInbox(inbox)
  const [hours, setHours] = useState<BusinessHours>(inbox.business_hours)
  const setIntervals = (fn: (all: Interval[]) => Interval[]) => setHours((h) => ({ ...h, intervals: fn(h.intervals) }))
  return (
    <Section
      title={<Trans>Business hours</Trans>}
      description={<Trans>Times are in the inbox's time zone. When off, the inbox counts as always open.</Trans>}
    >
      <FormCard
        flush
        onSubmit={() => save.mutate({ business_hours: hours })}
        footer={canManage && <FormActions pending={save.isPending} saved={save.isSuccess} error={save.error} label={<Trans>Save hours</Trans>} />}
      >
        <div className="px-5 py-4">
          <ToggleRow title={<Trans>Use business hours</Trans>}>
            <Switch
              checked={hours.enabled}
              disabled={!canManage}
              onCheckedChange={(enabled) => setHours((h) => ({ ...h, enabled }))}
              aria-label={t`Use business hours`}
            />
          </ToggleRow>
        </div>
        {hours.enabled && (
          <ul className="divide-y border-t">
            {WEEKDAYS.map((d) => (
              <Day
                key={d}
                day={d}
                rows={hours.intervals.map((iv, i) => ({ iv, i })).filter((x) => x.iv.day === d)}
                editable={canManage}
                onChange={(i, patch) => setIntervals((all) => all.map((iv, j) => (j === i ? { ...iv, ...patch } : iv)))}
                onRemove={(i) => setIntervals((all) => all.filter((_, j) => j !== i))}
                onAdd={() => setIntervals((all) => [...all, { day: d, start: "09:00", end: "17:00" }])}
              />
            ))}
          </ul>
        )}
      </FormCard>
    </Section>
  )
}
