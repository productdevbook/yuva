import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeftIcon, PlusIcon, RefreshCwIcon, Trash2Icon, XIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { Link, useNavigate, useParams } from "react-router"

import { ErrorLine, PersonAvatar, SecretDialog, useConfirm } from "@/components/common"
import { MODES, useEnumText, WEEKDAYS } from "@/components/common/text"
import { ChannelsSection } from "@/components/settings/ChannelsSection"
import { Field, PageTitle, Section } from "@/components/settings/SettingsLayout"
import { WebhooksSection } from "@/components/settings/WebhooksSettings"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { api, unwrap, type BusinessHours, type Inbox, type InboxMode, type InboxUpdate, type Weekday } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useInboxMembers, useMembers } from "@/lib/queries"
import { useSession } from "@/lib/session"

export function InboxSettings() {
  const { t } = useLingui()
  const { inboxId = "" } = useParams()
  const { workspaceId: ws } = useSession()
  const inbox = useQuery({
    queryKey: [...keys.inboxes(ws), inboxId],
    queryFn: () => unwrap(api.GET("/v1/inboxes/{inboxId}", { params: { path: { inboxId } } })),
  })
  return (
    <>
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon-sm" render={<Link to="/settings/inboxes" />} aria-label={t`Back`}>
          <ArrowLeftIcon />
        </Button>
        <PageTitle>{inbox.data?.name ?? <Trans>Inbox</Trans>}</PageTitle>
      </div>
      {inbox.isPending ? (
        <Skeleton className="h-64 w-full" />
      ) : inbox.data ? (
        <>
          <GeneralForm inbox={inbox.data} />
          <HoursForm inbox={inbox.data} />
          <AccessSection inbox={inbox.data} />
          <ChannelsSection inbox={inbox.data} />
          <IdentitySecretSection inbox={inbox.data} />
          <WebhooksSection inboxId={inbox.data.id} />
          <DeleteSection inbox={inbox.data} />
        </>
      ) : (
        <ErrorLine error={inbox.error} />
      )}
    </>
  )
}

function useSaveInbox(inbox: Inbox) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: InboxUpdate) =>
      unwrap(api.PATCH("/v1/inboxes/{inboxId}", { params: { path: { inboxId: inbox.id } }, body })),
    onSuccess: (data) => {
      qc.setQueryData([...keys.inboxes(ws), inbox.id], data)
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws), exact: true })
    },
  })
}

function GeneralForm({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const text = useEnumText()
  const save = useSaveInbox(inbox)
  const [name, setName] = useState(inbox.name)
  const [slug, setSlug] = useState(inbox.slug)
  const [mode, setMode] = useState<InboxMode>(inbox.mode)
  const [minutes, setMinutes] = useState(inbox.expected_reply_minutes?.toString() ?? "")
  const [timezone, setTimezone] = useState(inbox.timezone)
  const [locale, setLocale] = useState(inbox.default_locale)
  const [color, setColor] = useState(inbox.branding.color ?? "#2563eb")
  const [greeting, setGreeting] = useState(inbox.branding.greeting ?? "")
  const [logo, setLogo] = useState(inbox.branding.logo_url ?? "")
  const zones = useMemo(() => Intl.supportedValuesOf("timeZone"), [])
  return (
    <Section title={<Trans>General</Trans>}>
      <form
        className="grid gap-4 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate({
            name,
            slug,
            mode,
            timezone,
            default_locale: locale,
            expected_reply_minutes: minutes === "" ? null : Number(minutes),
            branding: { color, greeting: greeting || undefined, logo_url: logo || undefined },
          })
        }}
      >
        <fieldset disabled={!canManage} className="contents">
          <Field label={<Trans>Name</Trans>} htmlFor="name">
            <Input id="name" required maxLength={200} value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label={<Trans>Slug</Trans>} htmlFor="slug">
            <Input
              id="slug"
              required
              pattern="[a-z0-9]+(-[a-z0-9]+)*"
              maxLength={64}
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
            />
          </Field>
          <Field
            label={<Trans>Mode</Trans>}
            hint={
              mode === "live" ? (
                <Trans>Contacts see who is available, typing and read receipts.</Trans>
              ) : (
                <Trans>Contacts see the expected reply time instead of who is online.</Trans>
              )
            }
          >
            <Select value={mode} onValueChange={(v) => setMode(v as InboxMode)} items={text.mode} disabled={!canManage}>
              <SelectTrigger className="w-full" aria-label={t`Mode`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {MODES.map((m) => (
                  <SelectItem key={m} value={m}>
                    {text.mode[m]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field
            label={<Trans>Expected reply time (minutes)</Trans>}
            htmlFor="minutes"
            hint={<Trans>Shown to contacts in async mode. Leave empty to hide it.</Trans>}
          >
            <Input
              id="minutes"
              type="number"
              min={1}
              max={43200}
              value={minutes}
              onChange={(e) => setMinutes(e.target.value)}
            />
          </Field>
          <Field label={<Trans>Time zone</Trans>} htmlFor="timezone">
            <Input
              id="timezone"
              list="timezones"
              required
              maxLength={64}
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
            />
            <datalist id="timezones">
              {zones.map((z) => (
                <option key={z} value={z} />
              ))}
            </datalist>
          </Field>
          <Field
            label={<Trans>Default language</Trans>}
            htmlFor="locale"
            hint={<Trans>A language tag such as en or tr.</Trans>}
          >
            <Input
              id="locale"
              required
              pattern="[a-z]{2,3}(-[A-Za-z0-9]{2,8})*"
              value={locale}
              onChange={(e) => setLocale(e.target.value)}
            />
          </Field>
          <Field label={<Trans>Brand color</Trans>} htmlFor="color">
            <div className="flex gap-2">
              <input
                type="color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                aria-label={t`Pick a color`}
                className="h-9 w-12 shrink-0 cursor-pointer rounded-md border bg-transparent p-1"
              />
              <Input
                id="color"
                pattern="#[0-9a-fA-F]{6}"
                value={color}
                onChange={(e) => setColor(e.target.value)}
              />
            </div>
          </Field>
          <Field label={<Trans>Logo URL</Trans>} htmlFor="logo">
            <Input id="logo" type="url" value={logo} onChange={(e) => setLogo(e.target.value)} />
          </Field>
          <Field label={<Trans>Greeting</Trans>} htmlFor="greeting" className="sm:col-span-2">
            <Textarea
              id="greeting"
              maxLength={500}
              rows={2}
              value={greeting}
              placeholder={t`Hi! Ask us anything.`}
              onChange={(e) => setGreeting(e.target.value)}
            />
          </Field>
        </fieldset>
        {canManage && (
          <div className="flex items-center gap-3 sm:col-span-2">
            <Button type="submit" disabled={save.isPending}>
              <Trans>Save</Trans>
            </Button>
            {save.isSuccess && (
              <span className="text-sm text-muted-foreground">
                <Trans>Saved</Trans>
              </span>
            )}
            <ErrorLine error={save.error} />
          </div>
        )}
      </form>
    </Section>
  )
}

function HoursForm({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const text = useEnumText()
  const save = useSaveInbox(inbox)
  const [hours, setHours] = useState<BusinessHours>(inbox.business_hours)
  const byDay = (d: Weekday) => hours.intervals.map((iv, i) => ({ iv, i })).filter((x) => x.iv.day === d)
  const setInterval = (i: number, patch: Partial<{ start: string; end: string }>) =>
    setHours((h) => ({ ...h, intervals: h.intervals.map((iv, j) => (j === i ? { ...iv, ...patch } : iv)) }))
  return (
    <Section
      title={<Trans>Business hours</Trans>}
      description={<Trans>Times are in the inbox's time zone. When off, the inbox counts as always open.</Trans>}
      action={
        <label className="flex items-center gap-2 text-sm">
          <Switch
            checked={hours.enabled}
            disabled={!canManage}
            onCheckedChange={(enabled) => setHours((h) => ({ ...h, enabled }))}
            aria-label={t`Use business hours`}
          />
          {hours.enabled ? <Trans>On</Trans> : <Trans>Off</Trans>}
        </label>
      }
    >
      {hours.enabled && (
        <ul className="flex flex-col divide-y rounded-lg border">
          {WEEKDAYS.map((d) => {
            const rows = byDay(d)
            return (
              <li key={d} className="flex flex-col gap-2 px-3 py-2.5 sm:flex-row sm:items-start">
                <span className="w-28 shrink-0 pt-1.5 text-sm font-medium">{text.weekday[d]}</span>
                <div className="flex flex-1 flex-col gap-2">
                  {rows.length === 0 && (
                    <span className="pt-1.5 text-sm text-muted-foreground">
                      <Trans>Closed</Trans>
                    </span>
                  )}
                  {rows.map(({ iv, i }) => (
                    <div key={i} className="flex items-center gap-2">
                      <Input
                        type="time"
                        value={iv.start}
                        disabled={!canManage}
                        onChange={(e) => setInterval(i, { start: e.target.value })}
                        className="min-w-0 flex-1 sm:w-32 sm:flex-none"
                        aria-label={t`Opens`}
                      />
                      <span className="text-muted-foreground">–</span>
                      <Input
                        type="time"
                        value={iv.end === "24:00" ? "23:59" : iv.end}
                        disabled={!canManage}
                        onChange={(e) => setInterval(i, { end: e.target.value })}
                        className="min-w-0 flex-1 sm:w-32 sm:flex-none"
                        aria-label={t`Closes`}
                      />
                      {canManage && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          aria-label={t`Remove these hours`}
                          onClick={() => setHours((h) => ({ ...h, intervals: h.intervals.filter((_, j) => j !== i) }))}
                        >
                          <XIcon />
                        </Button>
                      )}
                    </div>
                  ))}
                </div>
                {canManage && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      setHours((h) => ({ ...h, intervals: [...h.intervals, { day: d, start: "09:00", end: "17:00" }] }))
                    }
                  >
                    <PlusIcon />
                    <Trans>Add hours</Trans>
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}
      {canManage && (
        <div className="flex items-center gap-3">
          <Button onClick={() => save.mutate({ business_hours: hours })} disabled={save.isPending}>
            <Trans>Save hours</Trans>
          </Button>
          {save.isSuccess && (
            <span className="text-sm text-muted-foreground">
              <Trans>Saved</Trans>
            </span>
          )}
          <ErrorLine error={save.error} />
        </div>
      )}
    </Section>
  )
}

function AccessSection({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const members = useMembers().data ?? []
  const granted = useInboxMembers(inbox.id)
  const ids = new Set((granted.data ?? []).map((m) => m.id))
  const toggle = useMutation({
    mutationFn: ({ memberId, on }: { memberId: string; on: boolean }) => {
      const params = { params: { path: { inboxId: inbox.id, memberId } } }
      return on
        ? unwrap(api.PUT("/v1/inboxes/{inboxId}/members/{memberId}", params))
        : unwrap(api.DELETE("/v1/inboxes/{inboxId}/members/{memberId}", params))
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.inboxMembers(ws, inbox.id) }),
  })
  const agents = members.filter((m) => m.role === "agent")
  return (
    <Section
      title={<Trans>Access</Trans>}
      description={<Trans>Agents see this inbox only when they are given access. Owners and admins always see it.</Trans>}
    >
      {agents.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Trans>There are no agents in this workspace.</Trans>
        </p>
      ) : (
        <ul className="flex flex-col divide-y rounded-lg border">
          {agents.map((m) => {
            const name = m.name || m.email
            return (
              <li key={m.id}>
                <label className="flex items-center gap-3 px-3 py-2.5">
                  <Checkbox
                    checked={ids.has(m.id)}
                    disabled={!canManage || toggle.isPending}
                    onCheckedChange={(on) => toggle.mutate({ memberId: m.id, on })}
                    aria-label={t`Access for ${name}`}
                  />
                  <PersonAvatar name={name} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{name}</span>
                    <span className="block truncate text-xs text-muted-foreground">{m.email}</span>
                  </span>
                </label>
              </li>
            )
          })}
        </ul>
      )}
      <ErrorLine error={toggle.error} />
    </Section>
  )
}

function IdentitySecretSection({ inbox }: { inbox: Inbox }) {
  const { canManage } = useSession()
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const rotate = useMutation({
    mutationFn: () =>
      unwrap(api.POST("/v1/inboxes/{inboxId}/identity-secret", { params: { path: { inboxId: inbox.id } } })),
    onSuccess: (r) => setSecret(r.identity_secret),
  })
  if (!canManage) return null
  return (
    <Section
      title={<Trans>Identity secret</Trans>}
      description={
        <Trans>
          Your backend signs identity tokens with this secret so Yuva knows who your signed-in users are. It is shown
          only when created or rotated.
        </Trans>
      }
    >
      <div className="flex items-center gap-3">
        <Button
          variant="outline"
          disabled={rotate.isPending}
          onClick={() =>
            confirm({
              title: <Trans>Rotate the identity secret?</Trans>,
              description: <Trans>Tokens signed with the current secret stop working at once.</Trans>,
              confirm: <Trans>Rotate</Trans>,
              run: () => rotate.mutate(),
            })
          }
        >
          <RefreshCwIcon />
          <Trans>Rotate secret</Trans>
        </Button>
        <ErrorLine error={rotate.error} />
      </div>
      <SecretDialog
        secret={secret}
        title={<Trans>New identity secret</Trans>}
        description={<Trans>Copy it now and update your backend; it is not shown again.</Trans>}
        onClose={() => setSecret(null)}
      />
      {confirmDialog}
    </Section>
  )
}

function DeleteSection({ inbox }: { inbox: Inbox }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { workspaceId: ws, canManage } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/inboxes/{inboxId}", { params: { path: { inboxId: inbox.id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
      navigate("/settings/inboxes")
    },
  })
  if (!canManage) return null
  const name = inbox.name
  return (
    <Section
      title={<Trans>Delete inbox</Trans>}
      description={<Trans>Deletes its channels, conversations, messages and attachments. This cannot be undone.</Trans>}
      className="border-destructive/30"
    >
      <div className="flex items-center gap-3">
        <Button
          variant="destructive"
          onClick={() =>
            confirm({
              title: <Trans>Delete {name}?</Trans>,
              description: <Trans>All of its conversations are deleted with it.</Trans>,
              confirm: <Trans>Delete</Trans>,
              run: () => remove.mutate(),
            })
          }
        >
          <Trash2Icon />
          <Trans>Delete inbox</Trans>
        </Button>
        <ErrorLine error={remove.error} />
      </div>
      {confirmDialog}
    </Section>
  )
}
