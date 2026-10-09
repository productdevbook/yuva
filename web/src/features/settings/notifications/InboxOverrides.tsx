import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { Dot, ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { fromChoice, inboxEvents, toChoice, useEventText, usePlainChoiceText, type Choice, type EventName } from "@/features/settings/notifications/events"
import { useSetInboxOverride } from "@/features/settings/notifications/queries"
import { Card, Section } from "@/features/settings/ui"
import type { Inbox, InboxNotifications, NotificationEventsUpdate, NotificationSettings } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

function InboxOverride({ inbox, override, settings, onDropDraft }: { inbox: Inbox; override?: InboxNotifications; settings: NotificationSettings; onDropDraft: () => void }) {
  const { t } = useLingui()
  const text = useEventText()
  const plain = usePlainChoiceText()
  const { put, remove } = useSetInboxOverride(inbox.id)
  const name = inbox.name
  const events = override?.events ?? {}
  const change = (e: EventName, c: Choice) => {
    const next: NotificationEventsUpdate = { ...events }
    const channels = fromChoice(c)
    if (channels) next[e] = channels
    else delete next[e]
    put.mutate(next, { onSuccess: onDropDraft })
  }
  return (
    <Card flush data-testid="inbox-override">
      <div className="flex items-center gap-2.5 px-5 py-3">
        <Dot color={inbox.branding.color} />
        <span className="min-w-0 flex-1 truncate text-body font-medium">{inbox.name}</span>
        <Button variant="ghost" size="icon-sm" aria-label={t`Stop overriding ${name}`} disabled={remove.isPending} onClick={() => (override ? remove.mutate() : onDropDraft())}>
          <Trash2Icon />
        </Button>
      </div>
      {inboxEvents(inbox).map((e) => {
        const current = plain[toChoice(settings.events[e]) as Exclude<Choice, "inherit">]
        const items: Record<Choice, string> = { inherit: t`As above (${current})`, ...plain }
        const value = toChoice(events[e])
        return (
          <div key={e} className="flex flex-col gap-2 border-t px-5 py-3 sm:flex-row sm:items-center sm:gap-3">
            <span className="min-w-0 flex-1 text-body">{text[e].label}</span>
            <Select value={value} onValueChange={(v) => change(e, v as Choice)} items={items}>
              <SelectTrigger size="sm" className={cn("w-full sm:w-60", value === "inherit" && "text-muted-foreground")} aria-label={`${inbox.name}: ${text[e].label}`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(Object.keys(items) as Choice[]).map((c) => (
                  <SelectItem key={c} value={c}>
                    {items[c]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )
      })}
      <ErrorLine error={put.error ?? remove.error} className="px-5 pb-3" />
    </Card>
  )
}

export function InboxOverrides({ settings }: { settings: NotificationSettings }) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const [drafts, setDrafts] = useState<string[]>([])
  const overridden = new Map(settings.inboxes.map((o) => [o.inbox_id, o]))
  const shown = inboxes.filter((i) => overridden.has(i.id) || drafts.includes(i.id))
  const addable = inboxes.filter((i) => !overridden.has(i.id) && !drafts.includes(i.id))
  const addItems = Object.fromEntries(addable.map((i) => [i.id, i.name]))
  return (
    <Section title={<Trans>Per inbox</Trans>} description={<Trans>Notify differently for one inbox. Anything left on “As above” follows the settings above.</Trans>}>
      {shown.length > 0 ? (
        <div className="flex flex-col gap-3" data-testid="inbox-overrides">
          {shown.map((inbox) => (
            <InboxOverride
              key={inbox.id}
              inbox={inbox}
              override={overridden.get(inbox.id)}
              settings={settings}
              onDropDraft={() => setDrafts((d) => d.filter((x) => x !== inbox.id))}
            />
          ))}
        </div>
      ) : (
        <p className="text-body text-muted-foreground">
          <Trans>Every inbox follows the settings above.</Trans>
        </p>
      )}
      {addable.length > 0 && (
        <Select value={null} onValueChange={(v) => v && setDrafts((d) => [...d, v as string])} items={addItems}>
          <SelectTrigger className="w-full sm:w-64" aria-label={t`Add an inbox`} data-testid="add-inbox-override">
            <PlusIcon />
            <span className="flex-1 text-start">
              <Trans>Add an inbox</Trans>
            </span>
          </SelectTrigger>
          <SelectContent>
            {addable.map((i) => (
              <SelectItem key={i.id} value={i.id}>
                {i.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </Section>
  )
}
