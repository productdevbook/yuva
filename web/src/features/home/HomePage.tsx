import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { AtSignIcon, CheckIcon, MessageSquareWarningIcon, PlusIcon, TriangleIcon } from "lucide-react"
import { Link, useSearchParams } from "react-router"

import { byPresence, useMemberStatus } from "@/app/TeamMenu"
import { ContactAvatar, ErrorLine, Segmented } from "@/components/common"
import { formatDateTime, formatDuration, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { entryLink } from "@/features/changelog/queries"
import { PropertyCard } from "@/features/home/PropertyCard"
import { useHome } from "@/features/home/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useMentions } from "@/features/inbox/queries"
import { share } from "@/features/reports/data"
import {
  Card,
  CardHead,
  Face,
  HeadLink,
  HeadNote,
  headButton,
  Meter,
  Page,
  PageHeader,
  ProductTag,
} from "@/features/settings/ui"
import { hubTarget, useHubPlaces, useHubText } from "@/features/setup/SetupHub"
import type { SetupState } from "@/features/setup/setup"
import { incidentLink, statusLink } from "@/features/status/queries"
import { useStatusText } from "@/features/status/text"
import { useNow } from "@/hooks/use-now"
import type { Home, HomeProperty, HomeStatusPage, HomeUpcoming, Inbox, StatusComponentStateValue } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMembers } from "@/lib/workspace"

const PERIODS = [7, 14, 30] as const
type Period = (typeof PERIODS)[number]

function usePeriod(): [Period, (p: Period) => void] {
  const [params, setParams] = useSearchParams()
  const raw = Number(params.get("days"))
  const days = (PERIODS as readonly number[]).includes(raw) ? (raw as Period) : 14
  const set = (p: Period) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (p === 14) next.delete("days")
        else next.set("days", String(p))
        return next
      },
      { replace: true }
    )
  return [days, set]
}

function PeriodBar({ h, product }: { h: Home; product?: string }) {
  const { t, i18n } = useLingui()
  const [, setDays] = usePeriod()
  const days = h.period.days
  const from = new Intl.DateTimeFormat(i18n.locale, {
    day: "numeric",
    month: "short",
    timeZone: h.period.timezone,
  }).format(new Date(h.period.since))
  const scope = product ?? t`all products`
  return (
    <div className="flex flex-wrap items-center gap-3">
      <span className="flex min-w-48 flex-1 flex-col leading-[1.3]">
        <span className="font-semibold">
          <Trans>Key figures</Trans>
        </span>
        <span className="text-caption text-faint">
          <Trans>
            {from} – today, {scope}
          </Trans>
        </span>
      </span>
      <Segmented
        value={String(days)}
        onChange={(v) => setDays(Number(v) as Period)}
        label={t`Period`}
        items={[
          { value: "7", label: t`Last 7 days`, testId: "home-period-7" },
          { value: "14", label: t`Last 14 days`, testId: "home-period-14" },
          { value: "30", label: t`Last 30 days`, testId: "home-period-30" },
        ]}
      />
    </div>
  )
}

function waitingOf(h: Home) {
  return { waiting: h.inbox.waiting_conversations, total: h.inbox.waiting }
}

function Delta({ children, good }: { children: React.ReactNode; good: boolean }) {
  return (
    <span
      className={cn(
        "rounded-full px-1.5 text-caption font-medium tabular-nums",
        good ? "bg-success/8 text-success" : "bg-warning/8 text-warning"
      )}
    >
      {children}
    </span>
  )
}

function Spark({ values, color }: { values: number[]; color: string }) {
  const max = Math.max(1, ...values)
  return (
    <span className="flex h-8 shrink-0 items-end gap-0.5" aria-hidden>
      {values.map((v, i) => (
        <span
          key={i}
          className="w-1 rounded-[1px]"
          style={{
            height: Math.max(3, Math.round((32 * v) / max)),
            backgroundColor: color,
            opacity: i === values.length - 1 ? 1 : 0.35,
          }}
        />
      ))}
    </span>
  )
}

function Kpi({
  to,
  name,
  value,
  hint,
  delta,
  spark,
  color,
  dashed,
  testId,
}: {
  to: string
  name: React.ReactNode
  value: string
  hint: React.ReactNode
  delta?: React.ReactNode
  spark?: React.ReactNode
  color?: string
  dashed?: boolean
  testId: string
}) {
  return (
    <Link
      to={to}
      className={cn(
        "flex min-w-0 flex-col gap-2.5 rounded-xl border bg-card p-4 outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring",
        dashed && "border-dashed bg-transparent"
      )}
      data-testid={testId}
    >
      <span className="flex items-center justify-between gap-2">
        <span className="truncate text-body text-muted-foreground">{name}</span>
        {delta}
      </span>
      <span className="flex items-end justify-between gap-3">
        <span className="flex min-w-0 flex-col leading-[1.2]">
          <b
            className="text-[28px] leading-[1.2] font-semibold tracking-[-0.02em] tabular-nums"
            style={color ? { color } : undefined}
          >
            {value}
          </b>
          <span className="mt-1 truncate text-caption text-faint">{hint}</span>
        </span>
        {spark}
      </span>
    </Link>
  )
}

function Kpis({ h, waiting }: { h: Home; waiting: ReturnType<typeof waitingOf> }) {
  const { t, i18n } = useLingui()
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const days = h.period.days
  const now = useNow(60_000)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  const oldest = h.inbox.oldest_waiting
  const oldestInbox = inboxes.find((i) => i.id === oldest?.inbox_id)?.name
  const oldestWho = oldest?.contact.name || oldest?.contact.email || ""
  const oldestFor = oldest ? formatDuration(Math.max(0, (now - Date.parse(oldest.since)) / 1000), i18n.locale) : ""
  const median = h.activity.median_first_reply_seconds
  const rated = h.activity.good + h.activity.bad
  const good = share(h.activity.good, h.activity.bad)
  const props = h.analytics?.properties ?? []
  const today = props.reduce((n, p) => n + p.visitors_today, 0)
  const perDay = Array.from({ length: 7 }, (_, i) => props.reduce((n, p) => n + (p.days[i]?.visitors ?? 0), 0))
  const yesterday = perDay[5] ?? 0
  const change = yesterday > 0 ? (today - yesterday) / yesterday : undefined
  const signed = new Intl.NumberFormat(i18n.locale, { style: "percent", signDisplay: "exceptZero" })
  return (
    <div className="grid grid-cols-4 gap-4 max-lg:grid-cols-2 phone:gap-3" data-testid="home-inbox">
      <Kpi
        to="/conversations"
        name={<Trans>Waiting for you</Trans>}
        value={fmt.format(waiting.total)}
        color={waiting.total > 0 ? "var(--brand)" : "var(--success)"}
        hint={
          oldest ? (
            inboxId && oldestWho ? (
              <Trans>
                oldest {oldestFor}, {oldestWho}
              </Trans>
            ) : !inboxId && oldestInbox && inboxes.length > 1 ? (
              <Trans>
                oldest {oldestFor}, in {oldestInbox}
              </Trans>
            ) : (
              <Trans>oldest {oldestFor}</Trans>
            )
          ) : (
            <Trans>nothing waiting</Trans>
          )
        }
        testId="home-inbox-waiting"
      />
      <Kpi
        to="/reports/today"
        name={<Trans>First reply</Trans>}
        value={median !== undefined ? formatDuration(median, i18n.locale) : "–"}
        hint={<Plural value={days} one="median, last # day" other="median, last # days" />}
        testId="home-first-reply"
      />
      <Kpi
        to="/reports/ratings"
        name={<Trans>Rating</Trans>}
        value={good !== undefined ? t`${pct.format(good)} good` : "–"}
        hint={<Plural value={rated} _0="no ratings yet" one="# rating" other="# ratings" />}
        testId="home-rating"
      />
      <Kpi
        to="/analytics"
        name={<Trans>Visitors today</Trans>}
        value={h.analytics && props.length > 0 ? fmt.format(today) : "–"}
        color={h.analytics && props.length > 0 ? undefined : "var(--faint)"}
        dashed={!h.analytics || props.length === 0}
        hint={
          props.length > 0 ? (
            props.length === 1 ? (
              props[0].name
            ) : (
              <Trans>all sites and apps</Trans>
            )
          ) : (
            <Trans>analytics not set up</Trans>
          )
        }
        delta={change !== undefined && <Delta good={change >= 0}>{signed.format(change)}</Delta>}
        spark={props.length > 0 && <Spark values={perDay} color="var(--success)" />}
        testId="home-visitors"
      />
    </div>
  )
}

function IconBox({ children, tone }: { children: React.ReactNode; tone?: "warning" }) {
  return (
    <span
      className={cn(
        "grid size-8 shrink-0 place-items-center rounded-lg text-caption font-semibold [&_svg]:size-4",
        tone === "warning" ? "bg-warning/10 text-warning" : "bg-surface text-foreground"
      )}
    >
      {children}
    </span>
  )
}

function NeedRow({
  icon,
  title,
  tag,
  meta,
  when,
  late,
  to,
  act,
  primary,
  testId,
}: {
  icon: React.ReactNode
  title: React.ReactNode
  tag?: Inbox
  meta: React.ReactNode
  when?: string
  late?: boolean
  to: string
  act: React.ReactNode
  primary?: boolean
  testId?: string
}) {
  return (
    <li className="flex items-center gap-3 border-b px-4 py-[11px] last:border-b-0" data-testid={testId}>
      {icon}
      <span className="flex min-w-0 flex-1 flex-col leading-[1.35]">
        <span className="truncate font-medium">{title}</span>
        <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint">
          {tag && <ProductTag name={tag.name} color={tag.branding.color} />}
          <span className="truncate">{meta}</span>
        </span>
      </span>
      {when !== undefined && (
        <span
          className={cn(
            "w-14 shrink-0 text-end text-caption tabular-nums",
            late ? "font-medium text-brand" : "text-faint"
          )}
        >
          {when}
        </span>
      )}
      <Button
        size="sm"
        variant={primary ? "default" : "outline"}
        className={cn(
          "h-[30px] rounded-lg px-2.5",
          primary ? "bg-brand px-3 hover:bg-brand/90" : "border-input font-normal"
        )}
        render={<Link to={to} />}
      >
        {act}
      </Button>
    </li>
  )
}

function NeedsYou({ h, waiting }: { h: Home; waiting: ReturnType<typeof waitingOf> }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const now = useNow(60_000)
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const scoped = inboxes.find((i) => i.id === inboxId)?.name
  const byId = new Map(inboxes.map((i) => [i.id, i]))
  const many = inboxes.length > 1 && !inboxId
  const mentions = (useMentions().data?.pages[0]?.items ?? []).filter(
    (m) => !m.seen && (!inboxId || m.conversation.inbox_id === inboxId)
  )
  const feedback = h.inbox.feedback
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const ago = (iso: string) => formatDuration(Math.max(0, (now - Date.parse(iso)) / 1000), i18n.locale)
  const shown = waiting.waiting.slice(0, 5)
  const notes = mentions.slice(0, 2)
  const count = waiting.total + mentions.length + (feedback > 0 ? 1 : 0)
  return (
    <Card flush data-testid="home-needs-card">
      <CardHead action={<HeadLink to="/conversations" testId="home-inbox-link">{t`Open inbox →`}</HeadLink>}>
        <Trans>Needs you</Trans>
        <span
          className={cn(
            "inline-flex h-[22px] min-w-[22px] items-center justify-center rounded-full px-1.5 text-caption font-semibold text-white tabular-nums dark:text-background",
            count > 0 ? "bg-brand" : "bg-success"
          )}
        >
          {count}
        </span>
      </CardHead>
      {count === 0 ? (
        <div className="flex flex-col items-center gap-1 px-4 py-9 text-center" data-testid="home-needs-empty">
          <span className="grid size-9 place-items-center rounded-full bg-success text-white dark:text-background">
            <CheckIcon className="size-4" />
          </span>
          <p className="mt-1.5 font-semibold">
            {scoped ? <Trans>Nothing needs you in {scoped}</Trans> : <Trans>Nothing needs you</Trans>}
          </p>
          <p className="text-faint">
            <Trans>New conversations and feedback show up here.</Trans>
          </p>
        </div>
      ) : (
        <ul data-testid="home-needs">
          {shown.map((c, i) => {
            const name = c.contact.name || c.contact.email || t`Unnamed contact`
            const about = c.subject || c.preview
            const at = time.format(new Date(c.since))
            const channel = c.channel_kind ? text.channel[c.channel_kind] : ""
            return (
              <NeedRow
                key={c.id}
                icon={<ContactAvatar id={c.contact.id} name={name} className="size-8 text-caption font-semibold" />}
                title={about ? `${name}: “${about}”` : name}
                tag={many ? byId.get(c.inbox_id) : undefined}
                meta={
                  channel ? (
                    <Trans>
                      {channel} · waiting since {at}
                    </Trans>
                  ) : (
                    <Trans>waiting since {at}</Trans>
                  )
                }
                when={ago(c.since)}
                late={now - Date.parse(c.since) > 3_600_000}
                to={`/conversations/${c.id}`}
                act={i === 0 ? <Trans>Reply</Trans> : <Trans>Open</Trans>}
                primary={i === 0}
                testId={i === 0 ? "home-oldest-waiting" : `home-waiting-${c.id}`}
              />
            )
          })}
          {notes.map((m) => {
            const who = m.note.author.name || t`Someone`
            return (
              <NeedRow
                key={m.note.id}
                icon={
                  <IconBox>
                    <AtSignIcon />
                  </IconBox>
                }
                title={
                  <Trans>
                    {who} mentioned you: “{m.note.text}”
                  </Trans>
                }
                tag={many ? byId.get(m.conversation.inbox_id) : undefined}
                meta={<Trans>Note</Trans>}
                when={ago(m.note.created_at)}
                to={`/conversations/${m.conversation.id}?m=${m.note.id}`}
                act={<Trans>View</Trans>}
                testId="home-mention"
              />
            )
          })}
          {feedback > 0 && (
            <NeedRow
              icon={
                <IconBox tone="warning">
                  <MessageSquareWarningIcon />
                </IconBox>
              }
              title={
                <Plural value={feedback} one="# feedback is not resolved yet" other="# feedback are not resolved yet" />
              }
              meta={<Trans>Feedback from your apps and pages</Trans>}
              to="/feedback"
              act={<Trans>Triage</Trans>}
              testId="home-feedback"
            />
          )}
        </ul>
      )}
    </Card>
  )
}

const stateTone: Record<StatusComponentStateValue, string> = {
  operational: "var(--success)",
  degraded: "var(--warning)",
  partial_outage: "var(--warning)",
  major_outage: "var(--destructive)",
  maintenance: "#2563eb",
}

type Day = { date: string; state: StatusComponentStateValue }

function Strip({ days, length }: { days: Day[]; length: number }) {
  const slots = [...days].sort((a, b) => a.date.localeCompare(b.date)).slice(-length)
  return (
    <span className="flex h-4 w-full gap-0.5" aria-hidden>
      {Array.from({ length }, (_, i) => {
        const s = slots[i - (length - slots.length)]
        return (
          <span
            key={i}
            className={cn("flex-1 rounded-[2px]", !s && "bg-muted")}
            style={
              s ? { backgroundColor: stateTone[s.state], opacity: s.state === "operational" ? 0.55 : 1 } : undefined
            }
          />
        )
      })}
    </span>
  )
}

function Uptime({ page, length }: { page?: HomeStatusPage; length: number }) {
  return <Strip days={page?.days ?? []} length={length} />
}

function StatusComponents({ page, length }: { page: HomeStatusPage; length: number }) {
  const { t } = useLingui()
  const statusText = useStatusText()
  const components = page.components
  return (
    <Card flush data-testid="home-status">
      <CardHead action={<HeadLink to={statusLink(page.id)}>{`${page.name} →`}</HeadLink>}>
        <Trans>Status</Trans>
      </CardHead>
      <ul>
        {components.map((c) => (
          <li key={c.id} className="flex flex-col gap-2 border-b px-4 py-3 last:border-b-0">
            <span className="flex items-center gap-2">
              <span className="min-w-0 flex-1 truncate font-medium">{c.name}</span>
              <span className="shrink-0 text-caption font-medium" style={{ color: stateTone[c.state] }}>
                {statusText.component[c.state]}
              </span>
            </span>
            <Strip days={c.days} length={length} />
          </li>
        ))}
      </ul>
      {components.length === 0 && <p className="px-4 py-5 text-faint">{t`No components yet.`}</p>}
    </Card>
  )
}

function Products({ h }: { h: Home }) {
  const { i18n } = useLingui()
  const days = h.period.days
  const byInbox = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const rows = h.products
    .flatMap((p) => {
      const inbox = byInbox.get(p.inbox_id)
      return inbox ? [{ p, inbox }] : []
    })
    .sort((a, b) => b.p.open - a.p.open)
  const statusText = useStatusText()
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shown = fmt.format(days)
  return (
    <Card flush data-testid="home-products">
      <CardHead
        action={
          <HeadNote>
            <Trans>uptime · {shown} days</Trans>
          </HeadNote>
        }
      >
        <Trans>Products</Trans>
      </CardHead>
      <ul>
        {rows.map(({ p, inbox }) => {
          const page = h.status_pages.find((x) => x.inbox_id === inbox.id)
          const open = fmt.format(p.open)
          const good = share(p.good, p.bad)
          const rate = good !== undefined ? pct.format(good) : "–"
          return (
            <li key={inbox.id} className="border-b last:border-b-0">
              <Link
                to={`/settings/inboxes/${inbox.id}`}
                className="flex w-full flex-col gap-2 px-4 py-3 text-start outline-none transition-colors hover:bg-surface focus-visible:bg-surface"
                data-testid={`home-product-${inbox.id}`}
              >
                <span className="flex w-full items-center gap-2">
                  <span
                    className="grid size-[22px] shrink-0 place-items-center rounded-md text-[11px] font-semibold text-white"
                    style={{ backgroundColor: inbox.branding.color || "var(--faint)" }}
                  >
                    {Array.from(inbox.name.trim())[0]?.toLocaleUpperCase()}
                  </span>
                  <span className="min-w-0 flex-1 truncate font-medium">{inbox.name}</span>
                  <span className="shrink-0 text-caption text-faint tabular-nums">
                    <Trans>
                      {open} open · {rate} good
                    </Trans>
                  </span>
                  <span
                    className="w-[76px] shrink-0 truncate text-end text-caption font-medium"
                    style={page ? { color: stateTone[page.state] } : undefined}
                  >
                    {page && statusText.component[page.state]}
                  </span>
                </span>
                <Uptime page={page} length={days} />
              </Link>
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

function ProductsOrStatus({ h }: { h: Home }) {
  const [inboxId] = useInboxFilter()
  const page = h.status_pages.find((p) => p.inbox_id === inboxId)
  if (inboxId && page && page.components.length > 0) return <StatusComponents page={page} length={h.period.days} />
  return <Products h={h} />
}

function ConversationsChart({ h }: { h: Home }) {
  const { i18n } = useLingui()
  const [inboxId] = useInboxFilter()
  const all = useInboxes().data ?? []
  const days = h.period.days
  const slots = h.activity.days
  const volume = (id: string) =>
    slots.reduce((n, d) => n + (d.inboxes.find((x) => x.inbox_id === id)?.conversations ?? 0), 0)
  const inboxes = (inboxId ? all.filter((i) => i.id === inboxId) : all).sort((a, b) => volume(b.id) - volume(a.id))
  const series = slots.map((d) =>
    inboxes.map((inbox) => d.inboxes.find((x) => x.inbox_id === inbox.id)?.conversations ?? 0)
  )
  const top = Math.max(1, ...series.map((s) => s.reduce((a, b) => a + b, 0)))
  const total = h.activity.conversations
  const label = new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "short", timeZone: "UTC" })
  const at = (i: number) => (slots[i] ? label.format(new Date(`${slots[i].day}T00:00:00Z`)) : "")
  const first = at(0)
  const middle = at(Math.floor((slots.length - 1) / 2))
  const shown = new Intl.NumberFormat(i18n.locale).format(total)
  return (
    <Card flush data-testid="home-chart">
      <div className="flex flex-col gap-3.5 p-4">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <span className="flex min-w-0 flex-1 flex-col leading-[1.3]">
            <span className="font-semibold">
              <Trans>Conversations</Trans>
            </span>
            <span className="text-caption text-faint">
              <Plural value={days} one={`${shown} in # day · by product`} other={`${shown} in # days · by product`} />
            </span>
          </span>
          {inboxes.map((inbox) => (
            <span key={inbox.id} className="flex items-center gap-1.5 text-caption text-muted-foreground">
              <span
                className="size-2 rounded-[2px]"
                style={{ backgroundColor: inbox.branding.color || "var(--faint)" }}
              />
              {inbox.name}
            </span>
          ))}
        </div>
        <div className="flex h-[150px] items-end gap-2 border-b" role="img" aria-label={shown}>
          {series.map((segs, di) => (
            <span key={slots[di].day} className="flex flex-1 flex-col-reverse gap-px">
              {segs.map((v, i) =>
                v === 0 ? null : (
                  <span
                    key={inboxes[i].id}
                    className="last:rounded-t-[3px]"
                    style={{
                      height: Math.round((146 * v) / top),
                      backgroundColor: inboxes[i].branding.color || "var(--faint)",
                      opacity: di === series.length - 1 ? 1 : 0.85,
                    }}
                  />
                )
              )}
            </span>
          ))}
        </div>
        <div className="flex justify-between text-caption text-faint">
          <span>{first}</span>
          <span>{middle}</span>
          <span>
            <Trans>Today</Trans>
          </span>
        </div>
      </div>
    </Card>
  )
}

function TeamCard({ h }: { h: Home }) {
  const { t, i18n } = useLingui()
  const members = useMembers().data ?? []
  const status = useMemberStatus()
  const load = new Map(h.team.members.map((m) => [m.member_id, m.open]))
  const order = [...members].sort(byPresence).slice(0, 6)
  const online = members.filter((m) => m.online && m.availability === "auto").length
  const most = Math.max(6, ...load.values())
  const fmt = new Intl.NumberFormat(i18n.locale)
  const on = fmt.format(online)
  const of = fmt.format(members.length)
  return (
    <Card flush data-testid="home-team">
      <CardHead action={<HeadNote>{t`${on} of ${of} online`}</HeadNote>}>
        <Link to="/team" className="hover:underline">
          <Trans>Team</Trans>
        </Link>
      </CardHead>
      <ul>
        {order.map((m) => {
          const name = m.name || m.email
          const open = load.get(m.id) ?? 0
          return (
            <li key={m.id} className="border-b last:border-b-0">
              <Link
                to={`/team/${m.id}`}
                className="flex items-center gap-2.5 px-4 py-2.5 transition-colors hover:bg-surface"
                data-testid={`home-member-${m.id}`}
              >
                <Face
                  id={m.id}
                  name={name}
                  online={m.online}
                  away={m.availability === "away"}
                  className="size-[30px]"
                />
                <span className="flex min-w-0 flex-1 flex-col leading-[1.3]">
                  <span className="truncate font-medium">{name}</span>
                  <span className="truncate text-caption text-faint">{status(m).text}</span>
                </span>
                <span className="flex w-16 shrink-0 flex-col items-end gap-1">
                  <span className="text-caption whitespace-nowrap text-muted-foreground tabular-nums">
                    <Plural value={open} one="# open" other="# open" />
                  </span>
                  <Meter
                    value={open}
                    max={most}
                    className="h-1 w-16"
                    color={open >= 4 ? "var(--warning)" : "var(--success)"}
                  />
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

function SmallBox({ top, bottom, brand }: { top: React.ReactNode; bottom: React.ReactNode; brand?: boolean }) {
  return (
    <span className="flex w-10 shrink-0 flex-col items-center rounded-lg border bg-card py-1 leading-[1.1]">
      <span className={cn("text-[10px]", brand ? "font-semibold text-brand uppercase" : "text-faint")}>{top}</span>
      <span className="font-semibold tabular-nums">{bottom}</span>
    </span>
  )
}

function Asks({ ideas }: { ideas: NonNullable<Home["ideas"]> }) {
  const { t } = useLingui()
  const text = useEnumText()
  const inboxes = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const top = ideas
  return (
    <Card flush data-testid="home-asks">
      <CardHead action={<HeadLink to="/board">{t`Roadmap →`}</HeadLink>}>
        <Trans>Most asked for</Trans>
      </CardHead>
      {top.length === 0 ? (
        <p className="px-4 py-5 text-faint">
          <Trans>No ideas yet. Customers’ votes show up here.</Trans>
        </p>
      ) : (
        <ul>
          {top.slice(0, 3).map((idea) => {
            const product = inboxes.get(idea.inbox_id)?.name
            const stage = text.stage[idea.stage]
            return (
              <li key={idea.id} className="flex items-center gap-2.5 border-b px-4 py-2.5 last:border-b-0">
                <SmallBox top={<TriangleIcon className="size-2 fill-current" />} bottom={idea.votes} />
                <span className="flex min-w-0 flex-1 flex-col leading-[1.3]">
                  <span className="truncate font-medium">{idea.title}</span>
                  <span className="truncate text-caption text-faint">
                    {[product, stage].filter(Boolean).join(" · ")}
                  </span>
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

function upcomingLink(u: HomeUpcoming) {
  if (u.kind === "broadcast") return `/broadcasts/${u.id}`
  if (u.kind === "whats_new") return entryLink(u.changelog_id ?? "", u.id, u.locale)
  return incidentLink(u.status_page_id ?? "", u.id)
}

function ComingUp({ upcoming }: { upcoming: HomeUpcoming[] }) {
  const { t, i18n } = useLingui()
  const mon = new Intl.DateTimeFormat(i18n.locale, { month: "short" })
  const day = new Intl.DateTimeFormat(i18n.locale, { day: "2-digit" })
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const fmt = new Intl.NumberFormat(i18n.locale)
  const meta = (u: HomeUpcoming, at: Date) => {
    const when = time.format(at)
    if (u.kind === "broadcast") {
      const people = fmt.format(u.recipients ?? 0)
      return (
        <Trans>
          Broadcast · {people} people · {when}
        </Trans>
      )
    }
    const label = u.kind === "maintenance" ? t`Maintenance` : u.kind === "incident" ? t`Incident` : t`What’s new`
    return [label, when].join(" · ")
  }
  return (
    <Card flush data-testid="home-upcoming">
      <CardHead>
        <Trans>Coming up</Trans>
      </CardHead>
      {upcoming.length === 0 ? (
        <p className="px-4 py-5 text-faint">
          <Trans>Nothing is scheduled.</Trans>
        </p>
      ) : (
        <ul>
          {upcoming.map((u) => {
            const at = new Date(u.at)
            return (
              <li key={u.id} className="border-b last:border-b-0">
                <Link
                  to={upcomingLink(u)}
                  className="flex items-center gap-3 px-4 py-2.5 transition-colors hover:bg-surface"
                  data-testid={`home-upcoming-${u.id}`}
                >
                  <SmallBox brand top={mon.format(at)} bottom={day.format(at)} />
                  <span className="flex min-w-0 flex-1 flex-col leading-[1.3]">
                    <span className="truncate font-medium">{u.title || "–"}</span>
                    <span className="truncate text-caption text-faint">{meta(u, at)}</span>
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

function Sites({ properties }: { properties: HomeProperty[] }) {
  const { t } = useLingui()
  return (
    <section className="flex flex-col gap-3" data-testid="home-analytics">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="min-w-48 flex-1 font-semibold">
          <Trans>Websites and apps</Trans>
        </span>
        {properties.length > 0 && <HeadLink to="/analytics">{t`Open analytics →`}</HeadLink>}
      </div>
      {properties.length === 0 ? (
        <div
          className="flex flex-col items-start gap-3 rounded-xl border border-dashed p-4"
          data-testid="home-analytics-empty"
        >
          <p className="text-muted-foreground">
            <Trans>
              No website or app is counted yet. Add one to see its visitors here every day, without cookies.
            </Trans>
          </p>
          <Button
            size="sm"
            variant="outline"
            render={<Link to="/setup/analytics" state={{ back: "/" } satisfies SetupState} />}
          >
            <PlusIcon />
            <Trans>Add a website or app</Trans>
          </Button>
        </div>
      ) : (
        <div className="grid grid-cols-3 gap-4 max-lg:grid-cols-2 phone:grid-cols-1 phone:gap-3">
          {properties.map((p) => (
            <Link
              key={p.id}
              to={`/analytics/${p.id}`}
              className="flex min-w-0 rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring [&>div]:transition-colors [&>div]:hover:border-faint"
              data-testid={`home-property-${p.id}`}
            >
              <PropertyCard property={p} className="rounded-xl" />
            </Link>
          ))}
        </div>
      )}
    </section>
  )
}

function LastBroadcast({ broadcasts }: { broadcasts: NonNullable<Home["broadcasts"]> }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const b = broadcasts.last_sent
  const sentAt = b ? formatDateTime(b.sent_at, i18n.locale) : ""
  return (
    <Card flush data-testid="home-broadcast-card">
      <CardHead action={<HeadLink to="/broadcasts">{t`Open broadcasts →`}</HeadLink>}>
        <Trans>Last broadcast</Trans>
      </CardHead>
      {!b ? (
        <div className="flex flex-col items-start gap-3 p-4" data-testid="home-broadcast-empty">
          <p className="text-muted-foreground">
            <Trans>No broadcast has gone out yet. When one has, its recipients, clicks and replies show up here.</Trans>
          </p>
          <Button size="sm" variant="outline" render={<Link to="/broadcasts" />}>
            <Trans>Write a broadcast</Trans>
          </Button>
        </div>
      ) : (
        <Link
          to={`/broadcasts/${b.id}`}
          className="flex flex-col gap-4 p-4 outline-none transition-colors hover:bg-surface focus-visible:bg-surface"
          data-testid="home-broadcast"
        >
          <div className="min-w-0">
            <p className="truncate font-medium">{b.name || b.subject}</p>
            <p className="truncate text-caption text-faint">
              {b.name && b.subject ? <>{b.subject} · </> : null}
              <Trans>Sent {sentAt}</Trans>
            </p>
          </div>
          <div className="grid grid-cols-4 gap-x-4 gap-y-3 phone:grid-cols-2">
            {(
              [
                [fmt.format(b.recipients), t`recipients`],
                [fmt.format(b.sent), t`sent`],
                [b.click_tracking ? fmt.format(b.clicked) : "–", b.click_tracking ? t`clicked` : t`clicks not tracked`],
                [fmt.format(b.replied), t`replied`],
              ] as const
            ).map(([value, label]) => (
              <div key={label} className="min-w-0">
                <b className="block text-[22px] leading-[1.2] font-semibold tracking-[-0.02em] tabular-nums">{value}</b>
                <span className="block truncate text-caption text-faint">{label}</span>
              </div>
            ))}
          </div>
        </Link>
      )}
    </Card>
  )
}

function Setup({ setup }: { setup: NonNullable<Home["setup"]> }) {
  const { t, i18n } = useLingui()
  const text = useHubText()
  const places = useHubPlaces()
  const count = setup.items.length
  const done = setup.items.filter((i) => i.done).length
  const fmt = new Intl.NumberFormat(i18n.locale)
  const doneText = fmt.format(done)
  const total = fmt.format(count)
  return (
    <Card flush className="pb-1" data-testid="home-setup-card">
      <CardHead
        action={
          <Link to="/setup" className="text-caption text-faint hover:text-foreground" data-testid="home-setup-open">
            {t`${doneText} of ${total}`}
          </Link>
        }
      >
        <Trans>Finish setup</Trans>
      </CardHead>
      <span
        className="mx-4 mt-3 mb-1.5 flex h-1.5 overflow-hidden rounded-full bg-border"
        role="progressbar"
        aria-label={t`Setup`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={count ? Math.round((done / count) * 100) : 0}
        data-testid="home-setup-progress"
      >
        <span className="rounded-full bg-success" style={{ width: `${count ? (done / count) * 100 : 0}%` }} />
      </span>
      <ul data-testid="home-setup">
        {setup.items.map((item) => (
          <li key={item.id}>
            <Link
              to={hubTarget(item, places)}
              state={item.done ? undefined : ({ back: "/" } satisfies SetupState)}
              className="flex items-center gap-2.5 px-4 py-[7px] transition-colors hover:bg-surface"
              data-testid={`home-setup-${item.id}`}
            >
              <span
                className={cn(
                  "grid size-[18px] shrink-0 place-items-center rounded-full",
                  item.done ? "bg-success text-white dark:text-background" : "border-[1.5px] border-input"
                )}
              >
                {item.done && <CheckIcon className="size-3" strokeWidth={3} />}
              </span>
              <span className={cn("truncate", item.done ? "text-faint line-through" : "font-medium")}>
                {text.title[item.id]}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </Card>
  )
}

export function HomePage() {
  const { t } = useLingui()
  const { me } = useSession()
  const [days] = usePeriod()
  const [inboxId] = useInboxFilter()
  const home = useHome(true, { days, inboxId })
  const h = home.data
  const inboxes = useInboxes().data ?? []
  const product = inboxes.find((i) => i.id === inboxId)?.name
  const mentions = (useMentions().data?.pages[0]?.items ?? []).filter((m) => !m.seen).length
  const first = (me.person.name || me.person.email).split(/[\s@]/)[0]
  const hour = new Date(useNow(60_000)).getHours()
  const greeting =
    hour < 12 ? t`Good morning, ${first}` : hour < 18 ? t`Good afternoon, ${first}` : t`Good evening, ${first}`
  const needs = h ? h.inbox.waiting + mentions : 0
  return (
    <Page testId="home">
      <PageHeader
        back={false}
        title={product ?? greeting}
        description={
          !h ? undefined : needs > 0 ? (
            <Plural value={needs} one="# thing needs you today." other="# things need you today." />
          ) : (
            <Trans>Nothing needs you today.</Trans>
          )
        }
        action={
          h &&
          h.status_pages.length > 0 && (
            <Button className={headButton.primary} render={<Link to="/status" />} data-testid="home-status-update">
              <Trans>Post status update</Trans>
            </Button>
          )
        }
      />
      <ErrorLine error={home.error} />
      {!h ? (
        !home.error && (
          <div className="flex flex-col gap-4">
            <div className="grid grid-cols-4 gap-4 max-lg:grid-cols-2">
              {[0, 1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-28 rounded-xl" />
              ))}
            </div>
            <Skeleton className="h-64 rounded-xl" />
          </div>
        )
      ) : (
        <HomeBlocks h={h} product={product} />
      )}
    </Page>
  )
}

function HomeBlocks({ h, product }: { h: Home; product?: string }) {
  const waiting = waitingOf(h)
  return (
    <>
      <PeriodBar h={h} product={product} />
      <Kpis h={h} waiting={waiting} />
      <div className="grid grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)] items-start gap-4 max-lg:grid-cols-1">
        <NeedsYou h={h} waiting={waiting} />
        <ProductsOrStatus h={h} />
      </div>
      <div className="grid grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)] items-start gap-4 max-lg:grid-cols-1">
        <ConversationsChart h={h} />
        <TeamCard h={h} />
      </div>
      {h.analytics && <Sites properties={h.analytics.properties} />}
      <div className="grid grid-cols-3 items-start gap-4 max-lg:grid-cols-1">
        {h.ideas && <Asks ideas={h.ideas.slice(0, 3)} />}
        <ComingUp upcoming={h.upcoming} />
        {(h.broadcasts || h.setup) && (
          <div className="flex min-w-0 flex-col gap-4">
            {h.broadcasts && <LastBroadcast broadcasts={h.broadcasts} />}
            {h.setup && <Setup setup={h.setup} />}
          </div>
        )}
      </div>
    </>
  )
}
