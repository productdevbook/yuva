import { Trans, useLingui } from "@lingui/react/macro"
import { MoonIcon } from "lucide-react"

import { ErrorLine, Notice } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { DELAYS, EVENTS, sameEvents, toChoice, useEventText, usePlainChoiceText, type Choice, type EventName } from "@/features/settings/notifications/events"
import { useSaveSettings } from "@/features/settings/notifications/queries"
import { Card, Field, Section } from "@/features/settings/ui"
import type { NotificationChannels, NotificationSettings } from "@/lib/api"

export function EventsSection({ settings }: { settings: NotificationSettings }) {
  const { t, i18n } = useLingui()
  const text = useEventText()
  const plain = usePlainChoiceText()
  const save = useSaveSettings()
  const delayText = (m: number) =>
    new Intl.NumberFormat(i18n.locale, { style: "unit", unit: m % 60 === 0 ? "hour" : "minute", unitDisplay: "long" }).format(m % 60 === 0 ? m / 60 : m)
  const delays = Object.fromEntries([...new Set([...DELAYS, settings.email_delay_minutes])].sort((a, b) => a - b).map((m) => [String(m), delayText(m)]))
  const set = (e: EventName, ch: keyof NotificationChannels, on: boolean) => save.mutate({ events: { [e]: { ...settings.events[e], [ch]: on } } })
  return (
    <Section
      title={<Trans>When to notify me</Trans>}
      description={<Trans>For every inbox in this workspace, unless an inbox below says otherwise.</Trans>}
      action={
        !sameEvents(settings.events, settings.defaults) && (
          <Button variant="ghost" size="sm" onClick={() => save.mutate({ events: settings.defaults })}>
            <Trans>Reset to defaults</Trans>
          </Button>
        )
      }
    >
      <Card
        flush
        footer={
          <Field label={<Trans>Send the e-mail after</Trans>} hint={<Trans>An e-mail goes out only if the conversation is still unread by then.</Trans>} className="w-full">
            <Select value={String(settings.email_delay_minutes)} onValueChange={(v) => save.mutate({ email_delay_minutes: Number(v) })} items={delays}>
              <SelectTrigger className="w-full sm:w-56" aria-label={t`Send the e-mail after`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(delays).map(([k, v]) => (
                  <SelectItem key={k} value={k}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        }
      >
        <div className="flex flex-col" data-testid="notification-events">
          <div className="flex items-center gap-3 px-5 pt-4 pb-2 text-caption font-medium text-faint">
            <span className="flex-1" />
            <span className="w-14 text-center">
              <Trans>Push</Trans>
            </span>
            <span className="w-14 text-center">
              <Trans>E-mail</Trans>
            </span>
          </div>
          {EVENTS.map((e) => {
            const label = text[e].label
            const fallback = plain[toChoice(settings.defaults[e]) as Exclude<Choice, "inherit">]
            return (
              <div key={e} className="flex items-center gap-3 border-t px-5 py-3">
                <div className="min-w-0 flex-1">
                  <p className="text-body font-medium">{label}</p>
                  <p className="text-caption text-muted-foreground">{text[e].hint}</p>
                  {toChoice(settings.events[e]) !== toChoice(settings.defaults[e]) && (
                    <p className="text-caption text-faint" data-testid="event-default">
                      <Trans>Default: {fallback}</Trans>
                    </p>
                  )}
                </div>
                {(["push", "email"] as const).map((ch) => (
                  <span key={ch} className="flex w-14 justify-center">
                    <Switch
                      checked={settings.events[e][ch]}
                      onCheckedChange={(on) => set(e, ch, on)}
                      aria-label={ch === "push" ? t`Push: ${label}` : t`E-mail: ${label}`}
                      data-testid={`notify-${e}-${ch}`}
                    />
                  </span>
                ))}
              </div>
            )
          })}
        </div>
        <ErrorLine error={save.error} className="px-5 pb-4" />
      </Card>
      <Notice icon={MoonIcon} data-testid="away-hint">
        <p>
          <Trans>
            While your status is Away, only new messages in conversations assigned to you and new assignments notify you.
            A conversation you have open in the panel never notifies you.
          </Trans>
        </p>
      </Notice>
    </Section>
  )
}
