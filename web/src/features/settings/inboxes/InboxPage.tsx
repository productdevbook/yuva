import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon } from "lucide-react"
import { createContext, useContext, useMemo, useState } from "react"
import { Link, useParams } from "react-router"

import { ChannelIcon, ErrorLine, toast, useConfirm } from "@/components/common"
import { useEnumText, useErrorText, WEEKDAYS } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { ChannelDialog } from "@/features/settings/channels/ChannelDialog"
import { ChannelSummary } from "@/features/settings/channels/ChannelSummary"
import { DeleteInbox, IdentitySecret } from "@/features/settings/inboxes/InboxSecurity"
import { useInbox, useSaveInbox } from "@/features/settings/inboxes/queries"
import { Card, EmptyRow, PageHeader, Row, RowIcon, Rows, RowText, Section, SettingRow, StatusTag } from "@/features/settings/ui"
import { api, unwrap, type BusinessHours, type Channel, type ChannelKind, type Inbox, type InboxUpdate, type Weekday } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxMembers, useMembers } from "@/lib/workspace"

const InboxContext = createContext<{ inbox: Inbox } | null>(null)

export function useInboxOutlet() {
  const v = useContext(InboxContext)
  if (!v) throw new Error("useInboxOutlet outside an inbox page")
  return v
}

export const selectClass =
  "h-9 max-w-full min-w-0 cursor-pointer rounded-[10px] border bg-background px-2.5 text-sm outline-none focus:border-brand disabled:cursor-not-allowed disabled:opacity-50"
const fieldClass = "h-9 w-60 min-w-0 rounded-[10px] bg-background phone:w-40"

function useAutosave(inbox: Inbox) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const save = useSaveInbox(inbox)
  return (body: InboxUpdate) => save.mutate(body, { onSuccess: () => toast(t`Saved`), onError: (e) => toast(errorText(e)) })
}

function channelState(ch: Channel): "ok" | "receive" {
  return ch.kind === "email" && !ch.email?.smtp ? "receive" : "ok"
}

function Channels({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const text = useEnumText()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const [editing, setEditing] = useState<{ channel: Channel | null; kind?: ChannelKind } | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const channels = useQuery({
    queryKey: keys.channels(ws, inbox.id),
    queryFn: () => unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: inbox.id } } })).then((r) => r.items),
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/channels/{channelId}", { params: { path: { channelId: id } } })),
    onSuccess: () => {
      toast(t`Channel removed`)
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inbox.id) })
    },
  })
  const adds: [ChannelKind, string][] = [
    ["email", t`E-mail`],
    ["chat", t`Website chat`],
    ["app", t`Mobile app`],
  ]
  return (
    <Section title={<Trans>Where customers write from</Trans>}>
      <Card flush>
        {channels.data && channels.data.length === 0 ? (
          <EmptyRow>
            <Trans>No channels yet. Add one so customers can write to {inbox.name}.</Trans>
          </EmptyRow>
        ) : (
          <Rows>
            {(channels.data ?? []).map((ch) => (
              <Row key={ch.id} data-testid="channel-row" className="pe-2">
                <button
                  type="button"
                  className="flex min-w-0 flex-1 items-center gap-3 text-start disabled:cursor-default"
                  onClick={() => setEditing({ channel: ch })}
                  disabled={!canManage}
                >
                  <RowIcon>
                    <ChannelIcon kind={ch.kind} />
                  </RowIcon>
                  <RowText
                    title={ch.name}
                    detail={
                      <>
                        {text.channel[ch.kind]} · <ChannelSummary ch={ch} />
                      </>
                    }
                  />
                </button>
                {channelState(ch) === "ok" ? (
                  <StatusTag tone="success">
                    <Trans>Working</Trans>
                  </StatusTag>
                ) : (
                  <StatusTag tone="warning">
                    <Trans>Receives only</Trans>
                  </StatusTag>
                )}
                {canManage && (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t`Remove ${ch.name}`}
                    onClick={() =>
                      confirm({
                        title: <Trans>Remove this channel?</Trans>,
                        description: <Trans>Conversations that started on it keep their messages.</Trans>,
                        confirm: <Trans>Remove</Trans>,
                        run: () => remove.mutate(ch.id),
                      })
                    }
                  >
                    <Trash2Icon />
                  </Button>
                )}
              </Row>
            ))}
          </Rows>
        )}
        <ErrorLine error={channels.error ?? remove.error} className="px-4 pb-3" />
      </Card>
      {canManage && (
        <div className="mt-1 flex flex-wrap gap-2">
          {adds.map(([kind, label]) => (
            <Button key={kind} variant="outline" size="sm" className="bg-card" onClick={() => setEditing({ channel: null, kind })} data-testid={`add-${kind}`}>
              + {label}
            </Button>
          ))}
        </div>
      )}
      {editing && (
        <ChannelDialog
          key={editing.channel?.id ?? editing.kind}
          inboxId={inbox.id}
          channel={editing.channel}
          kind={editing.kind}
          onClose={() => setEditing(null)}
          onCreated={(ch) => {
            toast(t`Channel added`)
            setEditing(ch.kind === "chat" || ch.kind === "app" ? { channel: ch } : null)
          }}
        />
      )}
      {confirmDialog}
    </Section>
  )
}

const LANGS = ["en", "tr", "de", "fr", "es", "it", "nl", "pt", "ar", "ja"]
const PROMISES = [15, 60, 240, 1440]

function languageName(tag: string, locale: string) {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(tag) ?? tag
  } catch {
    return tag
  }
}

function CustomerSees({ inbox }: { inbox: Inbox }) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const save = useAutosave(inbox)
  const [name, setName] = useState(inbox.name)
  const [greeting, setGreeting] = useState(inbox.branding.greeting ?? "")
  const [color, setColor] = useState(inbox.branding.color ?? "#d4431c")
  const langs = LANGS.includes(inbox.default_locale) ? LANGS : [inbox.default_locale, ...LANGS]
  const promise = inbox.expected_reply_minutes
  const promises = promise && !PROMISES.includes(promise) ? [...PROMISES, promise].sort((a, b) => a - b) : PROMISES
  const promiseText = (m: number) =>
    m === 15 ? t`Within minutes` : m === 60 ? t`Within an hour` : m === 240 ? t`Within 4 hours` : m === 1440 ? t`Within a day` : t`Within ${m} minutes`
  const branding = (patch: Partial<Inbox["branding"]>) => save({ branding: { ...inbox.branding, ...patch } })
  return (
    <Section title={<Trans>What the customer sees</Trans>}>
      <Card flush>
        <fieldset disabled={!canManage}>
          <Rows>
            <SettingRow title={<Trans>Name</Trans>} htmlFor="inbox-name">
              <Input
                id="inbox-name"
                className={fieldClass}
                value={name}
                maxLength={200}
                onChange={(e) => setName(e.target.value)}
                onBlur={() => name.trim() && name !== inbox.name && save({ name: name.trim() })}
              />
            </SettingRow>
            <SettingRow title={<Trans>Language</Trans>} htmlFor="inbox-locale">
              <select id="inbox-locale" className={selectClass} value={inbox.default_locale} onChange={(e) => save({ default_locale: e.target.value })}>
                {langs.map((l) => (
                  <option key={l} value={l}>
                    {languageName(l, i18n.locale)}
                  </option>
                ))}
              </select>
            </SettingRow>
            <SettingRow
              title={<Trans>Online status</Trans>}
              hint={inbox.mode === "live" ? <Trans>The chat shows who is available and typing</Trans> : <Trans>The chat shows when you usually reply</Trans>}
              htmlFor="inbox-mode"
            >
              <select id="inbox-mode" className={selectClass} value={inbox.mode} onChange={(e) => save({ mode: e.target.value as Inbox["mode"] })}>
                <option value="live">{t`Show who is online`}</option>
                <option value="async">{t`Show the reply time`}</option>
              </select>
            </SettingRow>
            <SettingRow title={<Trans>Reply time</Trans>} hint={<Trans>Shown in the chat window</Trans>} htmlFor="inbox-promise">
              <select
                id="inbox-promise"
                className={selectClass}
                value={promise ?? ""}
                onChange={(e) => save({ expected_reply_minutes: e.target.value === "" ? null : Number(e.target.value) })}
              >
                <option value="">{t`Not shown`}</option>
                {promises.map((m) => (
                  <option key={m} value={m}>
                    {promiseText(m)}
                  </option>
                ))}
              </select>
            </SettingRow>
            <SettingRow title={<Trans>Colour</Trans>} htmlFor="inbox-color">
              <input
                id="inbox-color"
                type="color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                onBlur={() => color !== inbox.branding.color && branding({ color })}
                className="h-9 w-12 cursor-pointer rounded-[10px] border bg-background p-1"
              />
            </SettingRow>
            <li className="flex flex-col gap-2 px-4 py-3">
              <label htmlFor="inbox-greeting" className="text-sm">
                <Trans>Greeting</Trans>
                <small className="mt-px block text-[13px] text-faint">
                  <Trans>The first thing the chat says</Trans>
                </small>
              </label>
              <Textarea
                id="inbox-greeting"
                rows={2}
                maxLength={500}
                value={greeting}
                placeholder={t`Hi! Ask us anything.`}
                onChange={(e) => setGreeting(e.target.value)}
                onBlur={() => greeting !== (inbox.branding.greeting ?? "") && branding({ greeting: greeting || undefined })}
                className="rounded-[10px] bg-background"
              />
            </li>
          </Rows>
        </fieldset>
      </Card>
    </Section>
  )
}

type Interval = BusinessHours["intervals"][number]

function uniform(h: BusinessHours) {
  const first = h.intervals[0]
  const days = new Set(h.intervals.map((i) => i.day))
  const same = h.intervals.every((i) => i.start === first?.start && i.end === first?.end) && days.size === h.intervals.length
  return { same, days, start: first?.start ?? "09:00", end: first?.end ?? "18:00" }
}

function Hours({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const text = useEnumText()
  const { canManage } = useSession()
  const save = useAutosave(inbox)
  const h = inbox.business_hours
  const u = uniform(h)
  const [start, setStart] = useState(u.start)
  const [end, setEnd] = useState(u.end === "24:00" ? "23:59" : u.end)
  const [zone, setZone] = useState(inbox.timezone)
  const zones = useMemo(() => Intl.supportedValuesOf("timeZone"), [])
  const days: Set<Weekday> = h.intervals.length ? u.days : new Set(["mon", "tue", "wed", "thu", "fri"])
  const write = (next: Set<Weekday>, s = start, e = end, enabled = h.enabled) => {
    const intervals: Interval[] = WEEKDAYS.filter((d) => next.has(d)).map((day) => ({ day, start: s, end: e }))
    save({ business_hours: { enabled, intervals } })
  }
  const short = (d: Weekday) => text.weekday[d].slice(0, 2)
  return (
    <Section
      title={<Trans>Business hours</Trans>}
      description={!u.same && h.intervals.length > 0 ? <Trans>The hours differ by day; changing them here sets the same hours for every chosen day.</Trans> : undefined}
    >
      <Card flush>
        <fieldset disabled={!canManage}>
          <Rows>
            <SettingRow title={<Trans>Appear online only in business hours</Trans>} hint={<Trans>Outside them the inbox counts as closed</Trans>}>
              <Switch checked={h.enabled} onCheckedChange={(on) => write(days, start, end, on)} aria-label={t`Appear online only in business hours`} />
            </SettingRow>
            {h.enabled && (
              <li className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
                <span className="flex flex-wrap gap-1" role="group" aria-label={t`Days`}>
                  {WEEKDAYS.map((d) => {
                    const on = days.has(d)
                    return (
                      <button
                        key={d}
                        type="button"
                        aria-pressed={on}
                        title={text.weekday[d]}
                        onClick={() => {
                          const next = new Set(days)
                          if (on) next.delete(d)
                          else next.add(d)
                          write(next)
                        }}
                        className={
                          on
                            ? "grid size-8 place-items-center rounded-full border border-primary bg-primary text-xs text-white"
                            : "grid size-8 place-items-center rounded-full border text-xs hover:border-faint"
                        }
                      >
                        {short(d)}
                      </button>
                    )
                  })}
                </span>
                <span className="flex items-center gap-1.5">
                  <Input type="time" value={start} onChange={(e) => setStart(e.target.value)} onBlur={() => start !== u.start && write(days)} className="h-9 w-auto rounded-[10px] bg-background" aria-label={t`Opens`} />
                  –
                  <Input type="time" value={end} onChange={(e) => setEnd(e.target.value)} onBlur={() => end !== u.end && write(days)} className="h-9 w-auto rounded-[10px] bg-background" aria-label={t`Closes`} />
                </span>
              </li>
            )}
            <SettingRow title={<Trans>Time zone</Trans>} htmlFor="inbox-zone">
              <Input
                id="inbox-zone"
                list="inbox-zones"
                className={fieldClass}
                value={zone}
                onChange={(e) => setZone(e.target.value)}
                onBlur={() => zone !== inbox.timezone && zones.includes(zone) && save({ timezone: zone })}
              />
              <datalist id="inbox-zones">
                {zones.map((z) => (
                  <option key={z} value={z} />
                ))}
              </datalist>
            </SettingRow>
          </Rows>
        </fieldset>
      </Card>
    </Section>
  )
}

function WhoCanReply({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const members = useMembers().data ?? []
  const granted = new Set((useInboxMembers(inbox.id).data ?? []).map((m) => m.id))
  const toggle = useMutation({
    mutationFn: ({ memberId, on }: { memberId: string; on: boolean }) => {
      const params = { params: { path: { inboxId: inbox.id, memberId } } }
      return on
        ? unwrap(api.PUT("/v1/inboxes/{inboxId}/members/{memberId}", params))
        : unwrap(api.DELETE("/v1/inboxes/{inboxId}/members/{memberId}", params))
    },
    onSuccess: () => {
      toast(t`Saved`)
      void qc.invalidateQueries({ queryKey: keys.inboxMembers(ws, inbox.id) })
    },
    onError: (e) => toast(errorText(e)),
  })
  return (
    <Section title={<Trans>Who can reply</Trans>} description={<Trans>Owners and admins see every inbox; members see the inboxes they are given.</Trans>}>
      <Card flush>
        <Rows>
          {members.map((m) => {
            const name = m.name || m.email
            const always = m.role !== "agent"
            return (
              <SettingRow key={m.id} title={name} hint={text.role[m.role]}>
                <Switch
                  checked={always || granted.has(m.id)}
                  disabled={always || !canManage || toggle.isPending}
                  onCheckedChange={(on) => toggle.mutate({ memberId: m.id, on })}
                  aria-label={t`${name} can reply`}
                />
              </SettingRow>
            )
          })}
        </Rows>
      </Card>
    </Section>
  )
}

function Developer({ inbox }: { inbox: Inbox }) {
  const save = useAutosave(inbox)
  const [slug, setSlug] = useState(inbox.slug)
  return (
    <Section title={<Trans>For developers</Trans>}>
      <Card flush>
        <Rows>
          <SettingRow title={<Trans>Slug</Trans>} hint={<Trans>Used in addresses and the widget</Trans>} htmlFor="inbox-slug">
            <Input
              id="inbox-slug"
              className={fieldClass}
              value={slug}
              pattern="[a-z0-9]+(-[a-z0-9]+)*"
              maxLength={64}
              onChange={(e) => setSlug(e.target.value)}
              onBlur={() => slug !== inbox.slug && /^[a-z0-9]+(-[a-z0-9]+)*$/.test(slug) && save({ slug })}
            />
          </SettingRow>
          <SettingRow title={<Trans>Webhooks for this inbox</Trans>}>
            <Button variant="outline" size="sm" render={<Link to="webhooks" />}>
              <Trans>Manage</Trans>
            </Button>
          </SettingRow>
        </Rows>
      </Card>
    </Section>
  )
}

export function InboxProvider({ children }: { children: (inbox: Inbox) => React.ReactNode }) {
  const { inboxId = "" } = useParams()
  const inbox = useInbox(inboxId)
  if (inbox.isPending) return <Skeleton className="h-64 w-full rounded-2xl" />
  if (!inbox.data) return <ErrorLine error={inbox.error} />
  return <InboxContext.Provider value={{ inbox: inbox.data }}>{children(inbox.data)}</InboxContext.Provider>
}

export function InboxPage() {
  const text = useEnumText()
  const { canManage } = useSession()
  return (
    <InboxProvider>
      {(inbox) => (
        <>
          <PageHeader title={inbox.name} description={`${text.mode[inbox.mode]} · ${inbox.timezone}`} />
          <Channels inbox={inbox} />
          <CustomerSees key={`see:${inbox.id}`} inbox={inbox} />
          <Hours key={`hours:${inbox.id}`} inbox={inbox} />
          <WhoCanReply inbox={inbox} />
          {canManage && (
            <>
              <Developer key={`dev:${inbox.id}`} inbox={inbox} />
              <IdentitySecret />
              <DeleteInbox />
            </>
          )}
        </>
      )}
    </InboxProvider>
  )
}

export function InboxWebhooks({ children }: { children: React.ReactNode }) {
  return (
    <InboxProvider>
      {(inbox) => (
        <>
          <PageHeader title={<Trans>Webhooks for {inbox.name}</Trans>} back={{ to: `/settings/inboxes/${inbox.id}`, label: inbox.name }} />
          {children}
        </>
      )}
    </InboxProvider>
  )
}
