import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"
import { useState } from "react"

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
import { utcLine } from "@/lib/zones"
import { ErrorLine, toLocalInput } from "@/features/changelog/text"
import { Field } from "@/features/settings/ui"

const MAX_AHEAD_MS = 365 * 86400_000

export function ScheduleDialog({
  initial,
  pending,
  error,
  onSchedule,
  onClose,
}: {
  initial?: string
  pending: boolean
  error: unknown
  onSchedule: (iso: string) => void
  onClose: () => void
}) {
  const { t, i18n } = useLingui()
  const [now] = useState(() => Date.now())
  const [at, setAt] = useState(() => toLocalInput(initial ? new Date(initial) : new Date(now + 60 * 60_000)))
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const when = at ? new Date(at) : null
  const valid = !!when && !Number.isNaN(when.getTime()) && when.getTime() > now && when.getTime() < now + MAX_AHEAD_MS
  const utc = when && valid ? utcLine(when.toISOString(), i18n.locale) : ""
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="schedule-entry-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (valid && when) onSchedule(when.toISOString())
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Schedule this entry</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                The entry goes public by itself at this time. Until then only members who may publish can change it.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <Field
            label={<Trans>Publish on</Trans>}
            htmlFor="schedule-entry-at"
            hint={
              utc ? (
                <Trans>
                  Your time zone, {zone}. That is {utc} UTC.
                </Trans>
              ) : (
                <Trans>Pick a time in the next year, in your time zone, {zone}.</Trans>
              )
            }
          >
            <Input
              id="schedule-entry-at"
              type="datetime-local"
              value={at}
              min={toLocalInput(new Date(now))}
              autoFocus
              onChange={(e) => setAt(e.target.value)}
              aria-label={t`Publish on`}
              aria-invalid={!valid || undefined}
              data-testid="schedule-entry-at"
            />
          </Field>
          <ErrorLine error={error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!valid || pending} data-testid="schedule-entry-confirm">
              {pending && <LoaderIcon className="animate-spin" />}
              <Trans>Schedule</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
