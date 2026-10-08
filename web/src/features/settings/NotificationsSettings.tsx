import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  BellOffIcon,
  BellRingIcon,
  MonitorIcon,
  MoonIcon,
  PlusIcon,
  SendIcon,
  SmartphoneIcon,
  Trash2Icon,
} from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Field, PageTitle, Section } from "@/features/settings/SettingsLayout"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import {
  api,
  unwrap,
  type Inbox,
  type InboxNotifications,
  type NotificationChannels,
  type NotificationEvents,
  type NotificationEventsUpdate,
  type NotificationSettings,
  type PushSubscriptionItem,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { pushKeys, pushSupport, subscribe, unsubscribe, useBrowserSubscription, useVapidKey } from "@/lib/push"
import { useInboxes } from "@/lib/workspace"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

type EventName = keyof NotificationEvents
type Channel = keyof NotificationChannels

const EVENTS: EventName[] = [
  "new_live_conversation",
  "new_async_conversation",
  "message_in_my_conversation",
  "message_in_unassigned_conversation",
  "assigned_to_me",
]

const DELAYS = [5, 15, 30, 60, 120, 240, 1440]

function useEventText(): Record<EventName, { label: string; hint: string }> {
  const { t } = useLingui()
  return {
    new_live_conversation: {
      label: t`New conversation in a live inbox`,
      hint: t`The first message of a conversation in an inbox that shows who is available.`,
    },
    new_async_conversation: {
      label: t`New conversation in an async inbox`,
      hint: t`The first message of a conversation in an inbox that shows a reply time.`,
    },
    message_in_my_conversation: {
      label: t`New message in a conversation assigned to me`,
      hint: t`A later message from the contact.`,
    },
    message_in_unassigned_conversation: {
      label: t`New message in an unassigned conversation`,
      hint: t`A later message from the contact while nobody is assigned.`,
    },
    assigned_to_me: {
      label: t`A conversation is assigned to me`,
      hint: t`When someone else assigns it to you.`,
    },
  }
}

export function NotificationsSettings() {
  const { workspaceId: ws } = useSession()
  const settings = useQuery({
    queryKey: keys.notifications(ws),
    queryFn: () => unwrap(api.GET("/v1/me/notifications")),
  })
  return (
    <>
      <PageTitle>
        <Trans>Notifications</Trans>
      </PageTitle>
      <PushSection />
      {settings.data && (
        <>
          <EventsSection settings={settings.data} />
          <InboxOverrides settings={settings.data} />
        </>
      )}
      <ErrorLine error={settings.error} />
    </>
  )
}

function PushSection() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const [confirm, confirmDialog] = useConfirm()
  const support = pushSupport()
  const vapid = useVapidKey()
  const browser = useBrowserSubscription()
  const [permission, setPermission] = useState(() => ("Notification" in window ? Notification.permission : "default"))
  const [tested, setTested] = useState<string | null>(null)
  const enabled = support === "supported" && vapid.data !== null
  const list = useQuery({
    queryKey: pushKeys.subscriptions,
    queryFn: () => unwrap(api.GET("/v1/me/push-subscriptions")).then((r) => r.items),
    enabled,
  })
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: pushKeys.subscriptions })
    void qc.invalidateQueries({ queryKey: pushKeys.browser })
  }
  const mine = list.data?.find((s) => !!browser.data && s.endpoint === browser.data)
  const turnOn = useMutation({
    mutationFn: () => subscribe(vapid.data!),
    onSettled: () => {
      setPermission(Notification.permission)
      refresh()
    },
  })
  const turnOff = useMutation({ mutationFn: () => unsubscribe(mine?.id), onSettled: refresh })
  const remove = useMutation({
    mutationFn: (s: PushSubscriptionItem) =>
      s.id === mine?.id
        ? unsubscribe(s.id)
        : unwrap(
            api.DELETE("/v1/me/push-subscriptions/{pushSubscriptionId}", {
              params: { path: { pushSubscriptionId: s.id } },
            }),
          ),
    onSettled: refresh,
  })
  const test = useMutation({
    mutationFn: (id: string) =>
      unwrap(
        api.POST("/v1/me/push-subscriptions/{pushSubscriptionId}/test", {
          params: { path: { pushSubscriptionId: id } },
        }),
      ),
    onSuccess: (_, id) => {
      setTested(id)
      setTimeout(() => void qc.invalidateQueries({ queryKey: pushKeys.subscriptions }), 4000)
    },
  })

  let status: React.ReactNode
  if (support === "needs-install") {
    status = (
      <Trans>
        On iPhone and iPad, add Yuva to your Home Screen with Share › Add to Home Screen, then open it from there to
        turn on notifications.
      </Trans>
    )
  } else if (support === "unsupported") {
    status = <Trans>This browser cannot receive push notifications.</Trans>
  } else if (vapid.data === null) {
    status = <Trans>This server is not set up to send push notifications.</Trans>
  } else if (permission === "denied") {
    status = (
      <Trans>Notifications are blocked for this site. Allow them in your browser's site settings, then reload.</Trans>
    )
  }

  return (
    <Section
      title={<Trans>Push notifications</Trans>}
      description={<Trans>Get notified on your phone or computer, even when the panel is closed.</Trans>}
    >
      {status ? (
        <p className="text-sm text-muted-foreground" data-testid="push-status">
          {status}
        </p>
      ) : (
        vapid.data &&
        list.data && (
          <div className="flex flex-wrap items-center gap-3 rounded-lg border p-3" data-testid="push-this-device">
            {mine ? (
              <BellRingIcon className="size-5 shrink-0 text-primary" />
            ) : (
              <BellOffIcon className="size-5 shrink-0 text-muted-foreground" />
            )}
            <p className="min-w-0 flex-1 text-sm">
              {mine ? <Trans>On for this device.</Trans> : <Trans>Off for this device.</Trans>}
            </p>
            {mine ? (
              <Button variant="outline" size="sm" disabled={turnOff.isPending} onClick={() => turnOff.mutate()}>
                <Trans>Turn off</Trans>
              </Button>
            ) : (
              <Button size="sm" disabled={turnOn.isPending} onClick={() => turnOn.mutate()} data-testid="push-turn-on">
                <BellRingIcon />
                <Trans>Turn on for this device</Trans>
              </Button>
            )}
          </div>
        )
      )}
      {enabled && list.data && list.data.length > 0 && (
        <ul className="flex flex-col divide-y rounded-lg border" data-testid="push-devices">
          {list.data.map((s) => {
            const Icon = /Android|iPhone|iPad/.test(s.user_agent) ? SmartphoneIcon : MonitorIcon
            const name = s.user_agent || t`Unknown device`
            const created = formatDateTime(s.created_at, i18n.locale)
            const delivered = s.last_success_at ? formatDateTime(s.last_success_at, i18n.locale) : null
            const failed =
              s.last_failure_at && (!s.last_success_at || s.last_failure_at > s.last_success_at) ? s.last_error : null
            return (
              <li key={s.id} className="flex items-center gap-3 px-3 py-2.5">
                <Icon className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                    <span className="truncate">{name}</span>
                    {s.id === mine?.id && (
                      <Badge variant="secondary">
                        <Trans>This device</Trans>
                      </Badge>
                    )}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {delivered ? (
                      <Trans>
                        Added {created}, last delivered {delivered}
                      </Trans>
                    ) : (
                      <Trans>Added {created}</Trans>
                    )}
                  </p>
                  {failed && (
                    <p className="text-xs text-destructive">
                      <Trans>Last push failed: {failed}</Trans>
                    </p>
                  )}
                  {tested === s.id && (
                    <p role="status" className="text-xs text-success">
                      <Trans>Test sent. It should arrive in a few seconds.</Trans>
                    </p>
                  )}
                </div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Send a test notification to ${name}`}
                  title={t`Send a test notification`}
                  disabled={test.isPending}
                  onClick={() => test.mutate(s.id)}
                  data-testid="push-test"
                >
                  <SendIcon />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Remove ${name}`}
                  onClick={() =>
                    confirm({
                      title: <Trans>Remove this device?</Trans>,
                      description: <Trans>It will no longer receive notifications.</Trans>,
                      confirm: <Trans>Remove</Trans>,
                      run: () => remove.mutate(s),
                    })
                  }
                >
                  <Trash2Icon />
                </Button>
              </li>
            )
          })}
        </ul>
      )}
      {turnOn.data === "denied" && permission !== "denied" && (
        <p role="alert" className="text-sm text-muted-foreground">
          <Trans>Notifications were not allowed.</Trans>
        </p>
      )}
      <ErrorLine error={turnOn.error ?? turnOff.error ?? remove.error ?? test.error ?? list.error ?? vapid.error} />
      {confirmDialog}
    </Section>
  )
}

function useSaveSettings() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: { events?: NotificationEventsUpdate; email_delay_minutes?: number }) =>
      unwrap(api.PATCH("/v1/me/notifications", { body })),
    onMutate: (body) => {
      const prev = qc.getQueryData<NotificationSettings>(keys.notifications(ws))
      if (prev) {
        qc.setQueryData<NotificationSettings>(keys.notifications(ws), {
          ...prev,
          events: { ...prev.events, ...body.events },
          email_delay_minutes: body.email_delay_minutes ?? prev.email_delay_minutes,
        })
      }
      return prev
    },
    onError: (_, __, prev) => prev && qc.setQueryData(keys.notifications(ws), prev),
    onSuccess: (data) => qc.setQueryData(keys.notifications(ws), data),
  })
}

function sameEvents(a: NotificationEvents, b: NotificationEvents) {
  return EVENTS.every((e) => a[e].push === b[e].push && a[e].email === b[e].email)
}

function EventsSection({ settings }: { settings: NotificationSettings }) {
  const { t, i18n } = useLingui()
  const text = useEventText()
  const save = useSaveSettings()
  const plain = usePlainChoiceText()
  const delayText = (m: number) =>
    new Intl.NumberFormat(i18n.locale, {
      style: "unit",
      unit: m % 60 === 0 ? "hour" : "minute",
      unitDisplay: "long",
    }).format(m % 60 === 0 ? m / 60 : m)
  const delays = Object.fromEntries(
    [...new Set([...DELAYS, settings.email_delay_minutes])].sort((a, b) => a - b).map((m) => [String(m), delayText(m)]),
  )
  const set = (e: EventName, ch: Channel, on: boolean) =>
    save.mutate({ events: { [e]: { ...settings.events[e], [ch]: on } } })
  return (
    <Section
      title={<Trans>When to notify me</Trans>}
      description={<Trans>For every inbox in this workspace, unless an inbox below says otherwise.</Trans>}
      action={
        !sameEvents(settings.events, settings.defaults) && (
          <Button variant="outline" size="sm" onClick={() => save.mutate({ events: settings.defaults })}>
            <Trans>Reset to defaults</Trans>
          </Button>
        )
      }
    >
      <div className="flex flex-col" data-testid="notification-events">
        <div className="flex items-center gap-3 pb-2 text-xs font-medium text-muted-foreground">
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
            <div key={e} className="flex items-center gap-3 border-t py-2.5">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{text[e].label}</p>
                <p className="text-xs text-muted-foreground">{text[e].hint}</p>
                {toChoice(settings.events[e]) !== toChoice(settings.defaults[e]) && (
                  <p className="text-xs text-muted-foreground" data-testid="event-default">
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
      <Field
        label={<Trans>Send the e-mail after</Trans>}
        hint={<Trans>An e-mail goes out only if the conversation is still unread by then.</Trans>}
      >
        <Select
          value={String(settings.email_delay_minutes)}
          onValueChange={(v) => save.mutate({ email_delay_minutes: Number(v) })}
          items={delays}
        >
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
      <p className="flex gap-2 rounded-lg bg-muted px-3 py-2.5 text-sm text-muted-foreground" data-testid="away-hint">
        <MoonIcon className="mt-0.5 size-4 shrink-0" />
        <span>
          <Trans>
            While your status is Away, only new messages in conversations assigned to you and new assignments notify
            you. A conversation you have open in the panel never notifies you.
          </Trans>
        </span>
      </p>
      <ErrorLine error={save.error} />
    </Section>
  )
}

type Choice = "inherit" | "both" | "push" | "email" | "none"

function usePlainChoiceText(): Record<Exclude<Choice, "inherit">, string> {
  const { t } = useLingui()
  return { both: t`Push and e-mail`, push: t`Push only`, email: t`E-mail only`, none: t`Nothing` }
}

function toChoice(c: NotificationChannels | undefined): Choice {
  if (!c) return "inherit"
  if (c.push && c.email) return "both"
  if (c.push) return "push"
  if (c.email) return "email"
  return "none"
}

function fromChoice(c: Choice): NotificationChannels | undefined {
  if (c === "inherit") return undefined
  return { push: c === "both" || c === "push", email: c === "both" || c === "email" }
}

function inboxEvents(inbox: Inbox): EventName[] {
  return EVENTS.filter((e) => e !== (inbox.mode === "live" ? "new_async_conversation" : "new_live_conversation"))
}

function InboxOverrides({ settings }: { settings: NotificationSettings }) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const [drafts, setDrafts] = useState<string[]>([])
  const overridden = new Map(settings.inboxes.map((o) => [o.inbox_id, o]))
  const shown = inboxes.filter((i) => overridden.has(i.id) || drafts.includes(i.id))
  const addable = inboxes.filter((i) => !overridden.has(i.id) && !drafts.includes(i.id))
  const addItems = Object.fromEntries(addable.map((i) => [i.id, i.name]))
  return (
    <Section
      title={<Trans>Per inbox</Trans>}
      description={
        <Trans>Notify differently for one inbox. Anything left on “As above” follows the settings above.</Trans>
      }
    >
      {shown.length > 0 && (
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
      )}
      {shown.length === 0 && (
        <p className="text-sm text-muted-foreground">
          <Trans>Every inbox follows the settings above.</Trans>
        </p>
      )}
      {addable.length > 0 && (
        <Select value={null} onValueChange={(v) => v && setDrafts((d) => [...d, v as string])} items={addItems}>
          <SelectTrigger className="w-full sm:w-64" aria-label={t`Add an inbox`} data-testid="add-inbox-override">
            <PlusIcon />
            <span className="flex-1 text-left">
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

function InboxOverride({
  inbox,
  override,
  settings,
  onDropDraft,
}: {
  inbox: Inbox
  override?: InboxNotifications
  settings: NotificationSettings
  onDropDraft: () => void
}) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const text = useEventText()
  const name = inbox.name
  const events = override?.events ?? {}
  const put = useMutation({
    mutationFn: (next: NotificationEventsUpdate) =>
      unwrap(
        api.PUT("/v1/me/notifications/inboxes/{inboxId}", {
          params: { path: { inboxId: inbox.id } },
          body: { events: next },
        }),
      ),
    onSuccess: (data) => {
      qc.setQueryData<NotificationSettings>(
        keys.notifications(ws),
        (prev) => prev && { ...prev, inboxes: [...prev.inboxes.filter((o) => o.inbox_id !== inbox.id), data] },
      )
      onDropDraft()
    },
  })
  const remove = useMutation({
    mutationFn: () =>
      unwrap(api.DELETE("/v1/me/notifications/inboxes/{inboxId}", { params: { path: { inboxId: inbox.id } } })),
    onSuccess: () =>
      qc.setQueryData<NotificationSettings>(
        keys.notifications(ws),
        (prev) => prev && { ...prev, inboxes: prev.inboxes.filter((o) => o.inbox_id !== inbox.id) },
      ),
  })
  const plain = usePlainChoiceText()
  const choiceText = (e: EventName): Record<Choice, string> => {
    const current = plain[toChoice(settings.events[e]) as Exclude<Choice, "inherit">]
    return { inherit: t`As above (${current})`, ...plain }
  }
  const change = (e: EventName, c: Choice) => {
    const next: NotificationEventsUpdate = { ...events }
    const channels = fromChoice(c)
    if (channels) next[e] = channels
    else delete next[e]
    put.mutate(next)
  }
  return (
    <div className="flex flex-col rounded-lg border" data-testid="inbox-override">
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <span
          className="size-2.5 shrink-0 rounded-sm"
          style={{ backgroundColor: inbox.branding.color ?? "var(--muted-foreground)" }}
        />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{inbox.name}</span>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t`Stop overriding ${name}`}
          disabled={remove.isPending}
          onClick={() => (override ? remove.mutate() : onDropDraft())}
        >
          <Trash2Icon />
        </Button>
      </div>
      {inboxEvents(inbox).map((e) => {
        const items = choiceText(e)
        const value = toChoice(events[e])
        return (
          <div
            key={e}
            className={cn(
              "flex flex-col gap-1.5 px-3 py-2 sm:flex-row sm:items-center sm:gap-3",
              "border-t first:border-t-0",
            )}
          >
            <span className="min-w-0 flex-1 text-sm">{text[e].label}</span>
            <Select value={value} onValueChange={(v) => change(e, v as Choice)} items={items}>
              <SelectTrigger
                className={cn("w-full sm:w-64", value === "inherit" && "text-muted-foreground")}
                aria-label={`${inbox.name}: ${text[e].label}`}
              >
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
      <ErrorLine error={put.error ?? remove.error} className="px-3 pb-2" />
    </div>
  )
}
