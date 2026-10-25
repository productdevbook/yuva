import { Plural, Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowRightIcon,
  ArrowUpRightIcon,
  CheckIcon,
  ChevronDownIcon,
  InfoIcon,
  PlusIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ErrorLine, InboxMark, useConfirm } from "@/components/common"
import { keyLabel, STATUS_KEYS } from "@/components/common/ShortcutSheet"
import { formatDuration, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { FrameTabs, look, Overline, PageTop } from "@/features/inbox/PageFrame"
import { ComponentsTab } from "@/features/status/ComponentsTab"
import { CreatePageDialog } from "@/features/status/CreatePageDialog"
import { Pill } from "@/features/status/PageFrame"
import { PreviewDialog } from "@/features/status/PreviewDialog"
import {
  incidentLink,
  monitorLink,
  statusLink,
  usePagesIncidents,
  usePatchStatusPage,
  usePublicStatus,
  useStatusComponents,
  useStatusGroups,
  useStatusMonitors,
} from "@/features/status/queries"
import { useStatusScope } from "@/features/status/scope"
import { SettingsTab } from "@/features/status/SettingsTab"
import { SubscribersTab } from "@/features/status/SubscribersTab"
import { INCIDENT_STATES, isClosed, MAINTENANCE_STATES, StateDot, useStatusText } from "@/features/status/text"
import { UptimeBars, usePercent } from "@/features/status/UptimeBars"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type {
  Inbox,
  StatusComponentStateValue,
  StatusIncident,
  StatusIncidentImpact,
  StatusMonitor,
  StatusPage,
} from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport } from "@/lib/workspace"

const TABS = ["overview", "components", "subscribers", "settings"] as const
type Tab = (typeof TABS)[number]

export function hostOf(url: string) {
  return url.replace(/^https?:\/\//, "").replace(/\/$/, "")
}

const card = "overflow-hidden rounded-[14px] border bg-card"
const rowDot = "inline-block size-2 shrink-0 rounded-full"

const stateWash: Record<StatusComponentStateValue, string> = {
  operational: "bg-success/12 text-success",
  degraded: "bg-warning/12 text-warning",
  partial_outage: "bg-warning/12 text-warning",
  major_outage: "bg-destructive/10 text-destructive",
  maintenance: "bg-blue-600/10 text-blue-600 dark:bg-blue-400/12 dark:text-blue-400",
}

const impactWash: Record<StatusIncidentImpact, string> = {
  none: "bg-muted text-muted-foreground",
  minor: "bg-warning/12 text-warning",
  major: "bg-destructive/10 text-destructive",
  critical: "bg-destructive/10 text-destructive",
}

const maintenanceWash = stateWash.maintenance

function StatePill({ state, className }: { state: StatusComponentStateValue; className?: string }) {
  const text = useStatusText()
  return (
    <span
      className={cn(
        "inline-flex h-[22px] w-fit max-w-full shrink-0 items-center gap-1.5 rounded-full px-2 text-caption font-medium whitespace-nowrap",
        stateWash[state],
        className
      )}
    >
      <StateDot state={state} className="size-1.5 bg-current" />
      <span className="truncate">{text.component[state]}</span>
    </span>
  )
}

function PagePicker({
  items,
  current,
  inboxOf,
  missing,
  onPick,
  onCreate,
}: {
  items: StatusPage[]
  current: StatusPage
  inboxOf: (p?: StatusPage) => Inbox | undefined
  missing: Inbox[]
  onPick: (id: string) => void
  onCreate: (inboxId: string) => void
}) {
  const { t } = useLingui()
  const inbox = inboxOf(current)
  const mark = (
    <InboxMark
      name={inbox?.name ?? current.name}
      color={inbox?.branding.color}
      className="size-[22px] rounded-[6px] text-caption"
    />
  )
  const name = (
    <span className="min-w-0 truncate font-semibold" data-testid="status-page-name">
      {current.name}
    </span>
  )
  if (items.length < 2 && missing.length === 0) {
    return (
      <span className="flex h-9 min-w-0 items-center gap-2.5">
        {mark}
        {name}
      </span>
    )
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            className="h-9 max-w-full min-w-0 gap-2.5 rounded-[9px] border-input bg-card px-2.5"
            aria-label={t`Status pages`}
            data-testid="status-page-picker"
          />
        }
      >
        {mark}
        {name}
        <ChevronDownIcon className="text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-[min(320px,calc(100vw-2rem))]">
        <DropdownMenuGroup>
          <DropdownMenuRadioGroup
            value={current.id}
            onValueChange={(id) => onPick(String(id))}
            aria-label={t`Status pages`}
          >
            {items.map((p) => (
              <DropdownMenuRadioItem
                key={p.id}
                value={p.id}
                className={cn(
                  "relative min-h-10 py-1.5",
                  p.id === current.id &&
                    "before:absolute before:inset-y-2.5 before:start-0 before:w-[3px] before:rounded-[3px] before:bg-brand"
                )}
                data-testid="status-home-page"
              >
                <StateDot state={p.state} className="size-2.5" />
                <span className="flex min-w-0 flex-1 flex-col leading-[1.3]">
                  <span className="truncate font-medium">{p.name}</span>
                  <span className="truncate text-caption text-faint" dir="ltr">
                    {hostOf(p.url)}
                  </span>
                </span>
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
        {missing.length > 0 && <DropdownMenuSeparator />}
        {missing.map((i) => {
          const product = i.name
          return (
            <DropdownMenuItem key={i.id} onClick={() => onCreate(i.id)} data-testid="create-status-page-open">
              <PlusIcon />
              <span className="min-w-0 truncate">
                <Trans>Create status page for {product}</Trans>
              </span>
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function IncidentCard({ p, i, names }: { p: StatusPage; i: StatusIncident; names: Map<string, string> }) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const draft = !i.published_at
  const maintenance = i.kind === "maintenance"
  const since = formatShort(i.started_at ?? i.published_at ?? i.created_at, i18n.locale)
  const state = text.incident[i.state]
  const steps = maintenance ? MAINTENANCE_STATES : INCIDENT_STATES
  const at = steps.indexOf(i.state)
  const latest = i.updates[0]
  const affected = i.components.map((c) => names.get(c.component_id)).filter((n): n is string => !!n)
  return (
    <Link
      to={incidentLink(p.id, i.id)}
      className={cn(
        card,
        "flex flex-col gap-3 p-4 outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring"
      )}
      data-testid="status-incident-row"
    >
      <span className="flex flex-wrap items-start gap-x-3 gap-y-2">
        <span className="flex min-w-0 flex-1 basis-56 flex-col gap-1.5">
          <span className="flex flex-wrap items-center gap-2">
            <span
              className={cn(
                "inline-flex h-[22px] items-center gap-1.5 rounded-full px-2 text-caption font-semibold whitespace-nowrap before:size-1.5 before:rounded-full before:bg-current before:content-['']",
                maintenance ? maintenanceWash : impactWash[i.impact]
              )}
            >
              {maintenance ? text.kind.maintenance : text.impact[i.impact]}
            </span>
            <span className="text-caption text-faint">
              <Trans>
                {state} since {since}
              </Trans>
              {draft && (
                <>
                  {" · "}
                  <Trans>not posted yet</Trans>
                </>
              )}
            </span>
          </span>
          <span className="text-reading leading-snug font-semibold break-words">{i.title}</span>
        </span>
        <span className="inline-flex shrink-0 items-center gap-1 text-small font-medium text-brand">
          {draft ? <Trans>Review and post</Trans> : maintenance ? <Trans>Open</Trans> : <Trans>Post update</Trans>}
          <ArrowRightIcon className="size-3.5 rtl:rotate-180" />
        </span>
      </span>
      {at >= 0 && (
        <ol aria-label={state} className="flex gap-0.5">
          {steps.map((s, n) => (
            <li
              key={s}
              aria-current={n === at ? "step" : undefined}
              className={cn(
                "flex min-w-0 flex-1 flex-col gap-1.5 text-caption text-faint before:h-1 before:rounded-[2px] before:bg-muted before:content-['']",
                n < at && "before:bg-muted-foreground",
                n === at && "font-semibold text-foreground before:bg-brand"
              )}
            >
              <span className={cn("truncate", n !== at && "max-sm:hidden")}>{text.incident[s]}</span>
            </li>
          ))}
        </ol>
      )}
      {latest?.body && (
        <span className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3.5 text-small leading-[1.45]">
          <time className="pt-px text-caption text-faint" dateTime={latest.created_at}>
            {formatShort(latest.created_at, i18n.locale)}
          </time>
          <span className="line-clamp-2 text-muted-foreground">
            <b className="font-semibold text-foreground">{text.incident[latest.state]}</b>
            {" · "}
            {latest.body}
          </span>
        </span>
      )}
      {affected.length > 0 && (
        <span className="flex flex-wrap items-center gap-1.5 text-small text-faint">
          <Trans>Affects</Trans>
          {affected.map((n, k) => (
            <span
              key={k}
              className="inline-flex h-[22px] max-w-48 items-center rounded-full bg-muted px-2 text-caption text-muted-foreground"
            >
              <span className="truncate">{n}</span>
            </span>
          ))}
        </span>
      )}
    </Link>
  )
}

function ComponentRow({
  p,
  name,
  state,
  uptime,
}: {
  p: StatusPage
  name: string
  state: StatusComponentStateValue
  uptime?: { uptime?: number; days?: Parameters<typeof UptimeBars>[0]["days"] }
}) {
  const percent = usePercent()
  return (
    <li className="grid grid-cols-[minmax(150px,1fr)_minmax(0,2.2fr)_64px_132px] items-center gap-x-3.5 gap-y-1.5 border-t px-4 py-2.5 text-small first:border-t-0 @max-3xl:grid-cols-[minmax(0,1fr)_auto]">
      <span className="truncate font-semibold" title={name}>
        {name}
      </span>
      {uptime?.days && uptime.days.length > 0 ? (
        <UptimeBars
          days={uptime.days}
          count={p.history_days}
          className="h-7 gap-0.5 @max-3xl:order-last @max-3xl:col-span-2 @max-3xl:h-[22px]"
        />
      ) : (
        <span className="flex h-7 gap-0.5 @max-3xl:order-last @max-3xl:col-span-2 @max-3xl:h-[22px]" aria-hidden>
          {Array.from({ length: 45 }, (_, n) => (
            <span key={n} className="flex-1 rounded-[2px] bg-muted" />
          ))}
        </span>
      )}
      <span className="text-end font-semibold tabular-nums @max-3xl:hidden">
        {p.show_uptime && uptime?.uptime !== undefined ? percent(uptime.uptime) : ""}
      </span>
      <span className="flex justify-end @max-3xl:order-2">
        <StatePill state={state} />
      </span>
    </li>
  )
}

function ComponentsCard({ p }: { p: StatusPage }) {
  const text = useStatusText()
  const components = useStatusComponents(p.id)
  const pub = usePublicStatus(p)
  const uptime = new Map((pub.data?.components ?? []).map((c) => [c.id, c]))
  const groups = useStatusGroups(p.id).data ?? []
  const order = new Map(groups.map((g, n) => [g.id, n]))
  const rank = (c: { group_id?: string }) => (c.group_id ? (order.get(c.group_id) ?? groups.length) : groups.length + 1)
  const items = (components.data ?? [])
    .filter((c) => !c.hidden)
    .sort((a, b) => rank(a) - rank(b) || a.position - b.position)
  const days = p.history_days
  const sections: { key: string; name?: string; rows: typeof items }[] = []
  for (const c of items) {
    const key = c.group_id && order.has(c.group_id) ? c.group_id : ""
    const last = sections[sections.length - 1]
    if (last && last.key === key) last.rows.push(c)
    else sections.push({ key, name: groups.find((g) => g.id === key)?.name, rows: [c] })
  }
  const legend: StatusComponentStateValue[] = ["operational", "degraded", "major_outage", "maintenance"]
  return (
    <section className={card} data-testid="status-components-card">
      <div className="flex flex-wrap items-start gap-x-3 gap-y-1 px-4 pt-3.5 pb-2">
        <div className="flex min-w-0 flex-1 flex-col leading-[1.35]">
          <Link
            to={statusLink(p.id, "components")}
            className="w-fit rounded-sm font-semibold outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Trans>Components</Trans>
          </Link>
          <span className="text-caption text-faint">
            <Plural value={days} one="# day" other="# days" />
          </span>
        </div>
      </div>
      {components.isPending ? (
        <div className="flex flex-col gap-2 p-4" aria-busy="true">
          <Skeleton className="h-12 w-full rounded-[10px]" />
          <Skeleton className="h-12 w-full rounded-[10px]" />
          <Skeleton className="h-12 w-full rounded-[10px]" />
        </div>
      ) : items.length === 0 ? (
        <p className="px-4 py-6 text-center text-faint">
          <Trans>
            No components yet. Add the parts of your product that customers depend on, in the Components tab.
          </Trans>
        </p>
      ) : (
        <>
          <div className="flex flex-wrap gap-x-3 gap-y-1 px-4 pb-3 text-caption text-faint" aria-hidden>
            {legend.map((s) => (
              <span key={s} className="inline-flex items-center gap-1.5">
                <StateDot state={s} className="size-2.5 rounded-[2px]" />
                {text.component[s]}
              </span>
            ))}
          </div>
          {sections.map((s) => (
            <div key={s.key || "none"} className="border-t">
              {s.name && <Overline className="px-4 pt-2.5 pb-1">{s.name}</Overline>}
              <ul>
                {s.rows.map((c) => (
                  <ComponentRow key={c.id} p={p} name={c.name} state={c.state} uptime={uptime.get(c.id)} />
                ))}
              </ul>
            </div>
          ))}
        </>
      )}
    </section>
  )
}

const MONITOR_ROWS = 6

function MonitorsCard({ p }: { p: StatusPage }) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const q = useStatusMonitors(p.id)
  const monitors = q.data
  const live = (monitors ?? []).filter((m) => !m.paused)
  const down = live.filter((m) => m.state === "down").length
  const up = live.filter((m) => m.state === "up").length
  const shown = [...(monitors ?? [])]
    .sort((a, b) => Number(b.state === "down" && !b.paused) - Number(a.state === "down" && !a.paused))
    .slice(0, MONITOR_ROWS)
  const dot = (m: StatusMonitor) =>
    m.paused
      ? "bg-faint/50"
      : m.state === "down"
        ? "bg-destructive shadow-[0_0_0_3px] shadow-destructive/20"
        : m.state === "up"
          ? "bg-success"
          : "bg-faint"
  const line = (m: StatusMonitor) => {
    const state = text.monitorState[m.paused ? "paused" : m.state]
    if (m.paused || m.state === "pending") return state
    const since = formatShort(m.state_since, i18n.locale)
    return (
      <Trans>
        {state} since {since}
      </Trans>
    )
  }
  return (
    <section className={card} data-testid="status-monitors-card">
      <div className="flex items-start gap-3 px-4 pt-3.5 pb-2.5">
        <div className="flex min-w-0 flex-1 flex-col leading-[1.35]">
          <span className="font-semibold">
            <Trans>Monitors</Trans>
          </span>
          <span className="truncate text-caption text-faint">
            {monitors && monitors.length > 0 ? (
              down > 0 ? (
                <>
                  <Plural value={down} one="# down" other="# down" />
                  {", "}
                  <Plural value={up} one="# up" other="# up" />
                </>
              ) : up === monitors.length ? (
                <Trans>all up</Trans>
              ) : (
                <Plural value={up} one="# up" other="# up" />
              )
            ) : (
              " "
            )}
          </span>
        </div>
        <Link
          to="/monitors"
          className="rounded-md px-2 py-1 text-small font-medium text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Trans>All monitors</Trans>
        </Link>
      </div>
      {q.isPending ? (
        <div className="p-4 pt-1">
          <Skeleton className="h-24 w-full rounded-[10px]" />
        </div>
      ) : shown.length === 0 ? (
        <p className="border-t px-4 py-4 text-small text-faint">
          <Trans>No monitor watches this page yet</Trans>
        </p>
      ) : (
        <ul>
          {shown.map((m) => (
            <li key={m.id} className="border-t">
              <Link
                to={monitorLink(m.id)}
                className="grid grid-cols-[10px_minmax(0,1fr)] items-center gap-x-2.5 gap-y-0.5 px-4 py-2.5 text-small outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
              >
                <span aria-hidden className={cn(rowDot, "size-2", dot(m))} />
                <span className="truncate font-medium">{m.name}</span>
                <span className="col-start-2 truncate text-caption text-faint">
                  {text.monitorKind[m.kind]}
                  {" · "}
                  {line(m)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function lasted(i: StatusIncident) {
  const from = Date.parse(i.started_at ?? i.published_at ?? i.created_at)
  const to = i.resolved_at ? Date.parse(i.resolved_at) : Date.now()
  return Math.max(60, Math.round((to - from) / 1000))
}

function Fact({
  label,
  value,
  hint,
  to,
}: {
  label: React.ReactNode
  value: string
  hint?: React.ReactNode
  to: string
}) {
  return (
    <Link
      to={to}
      className="flex items-center gap-3 border-t px-4 py-3 text-small outline-none first:border-t-0 hover:bg-surface focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
    >
      <span className="flex min-w-0 flex-1 flex-col leading-[1.35]">
        <span className="truncate text-muted-foreground">{label}</span>
        <span className="truncate text-caption text-faint">{hint ?? " "}</span>
      </span>
      <b className="shrink-0 font-semibold tabular-nums">{value}</b>
    </Link>
  )
}

function ComingUp({ p, planned }: { p: StatusPage; planned: StatusIncident[] }) {
  const { i18n } = useLingui()
  return (
    <section className={card} data-testid="status-coming-up">
      <div className="flex flex-col px-4 pt-3.5 pb-2.5 leading-[1.35]">
        <span className="font-semibold">
          <Trans>Coming up</Trans>
        </span>
        <span className="text-caption text-faint">
          <Trans>Maintenance</Trans>
        </span>
      </div>
      {planned.length === 0 ? (
        <p className="border-t px-4 py-4 text-small text-faint">
          <Trans>No maintenance is planned.</Trans>
        </p>
      ) : (
        <ul>
          {planned.map((i) => {
            const due = formatShort(i.scheduled_start ?? i.created_at, i18n.locale)
            return (
              <li key={i.id} className="border-t">
                <Link
                  to={incidentLink(p.id, i.id)}
                  className="grid grid-cols-[10px_minmax(0,1fr)] items-start gap-x-2.5 gap-y-0.5 px-4 py-2.5 text-small outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
                  data-testid="status-incident-row"
                >
                  <span aria-hidden className={cn(rowDot, "mt-1.5 size-2 bg-blue-600 dark:bg-blue-400")} />
                  <span className="font-medium break-words">{i.title}</span>
                  <span className="col-start-2 text-caption text-faint">
                    <Trans>Scheduled for {due}</Trans>
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

function Facts({ p, incidents }: { p: StatusPage; incidents: StatusIncident[] }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const subs = p.email_subscribers
  const past = incidents
    .filter((i) => i.kind === "incident" && !!i.published_at)
    .sort((a, b) => (b.started_at ?? b.created_at).localeCompare(a.started_at ?? a.created_at))
  const [last, before] = past
  const date = (i: StatusIncident) => formatShort(i.started_at ?? i.created_at, i18n.locale)
  const took = (i: StatusIncident) => formatDuration(lasted(i), i18n.locale)
  const open = !!last && !last.resolved_at
  return (
    <section className={card} data-testid="status-facts">
      <Fact
        label={<Trans>Subscribers</Trans>}
        value={subs.enabled ? fmt.format(subs.counts.subscribed) : "–"}
        hint={subs.enabled ? <Trans>by e-mail</Trans> : <Trans>E-mail subscriptions are off</Trans>}
        to={statusLink(p.id, "subscribers")}
      />
      <Fact
        label={<Trans>Last incident</Trans>}
        value={!last ? "–" : open ? t`Now` : date(last)}
        to="/incidents"
        hint={
          !last ? (
            <Trans>None yet</Trans>
          ) : open ? (
            before ? (
              <Trans>
                before that: {date(before)}, {took(before)}
              </Trans>
            ) : (
              <Trans>the first one</Trans>
            )
          ) : (
            <Trans>resolved in {took(last)}</Trans>
          )
        }
      />
    </section>
  )
}

const heroTone = {
  ok: "border-success/30 bg-success/7",
  warn: "border-warning/35 bg-warning/10",
  bad: "border-destructive/35 bg-destructive/8",
  info: "border-blue-600/30 bg-blue-600/7 dark:border-blue-400/30 dark:bg-blue-400/8",
}

const heroMark = {
  ok: "bg-success",
  warn: "bg-warning",
  bad: "bg-destructive",
  info: "bg-blue-600 dark:bg-blue-400",
}

function Overview({ p }: { p: StatusPage }) {
  const { t } = useLingui()
  const text = useStatusText()
  const incidents = usePagesIncidents([p.id], false)
  const monitors = useStatusMonitors(p.id).data
  const components = useStatusComponents(p.id).data
  const names = new Map((components ?? []).map((c) => [c.id, c.name]))
  const live = incidents.data.filter((i) => !isClosed(i))
  const planned = live.filter((i) => i.state === "scheduled")
  const open = live.filter((i) => i.state !== "scheduled")
  const headline =
    p.state === "operational"
      ? t`All systems operational`
      : p.state === "degraded" || p.state === "partial_outage"
        ? t`Partly degraded`
        : text.component[p.state]
  const tone =
    p.state === "operational" ? "ok" : p.state === "maintenance" ? "info" : p.state === "major_outage" ? "bad" : "warn"
  const Mark = tone === "ok" ? CheckIcon : tone === "info" ? InfoIcon : TriangleAlertIcon
  const watching = monitors?.length ?? 0
  return (
    <div
      className="grid grid-cols-[minmax(0,1fr)_300px] items-start gap-4 @max-[1100px]:grid-cols-1"
      data-testid="status-overview"
    >
      <div className="flex min-w-0 flex-col gap-4">
        <div className={cn("flex items-center gap-3 rounded-[14px] border px-4 py-3.5", heroTone[tone])} role="status">
          <span className={cn("grid size-7 shrink-0 place-items-center rounded-full text-white", heroMark[tone])}>
            <Mark className="size-4" aria-hidden />
          </span>
          <span className="flex min-w-0 flex-1 flex-col leading-tight">
            <span className="text-reading font-semibold" data-testid="status-overall">
              {headline}
            </span>
            <span className="truncate text-small text-muted-foreground">
              {watching > 0 ? (
                <Plural value={watching} one="Watched by # monitor" other="Watched by # monitors" />
              ) : (
                <Trans>No monitor watches this page yet</Trans>
              )}
            </span>
          </span>
        </div>
        {open.map((i) => (
          <IncidentCard key={i.id} p={p} i={i} names={names} />
        ))}
        <ComponentsCard p={p} />
      </div>
      <aside className="flex flex-col gap-4 @max-[1100px]:grid @max-[1100px]:grid-cols-2 @max-3xl:grid-cols-1 @max-[1100px]:items-start">
        <MonitorsCard p={p} />
        <div className="flex flex-col gap-4">
          <ComingUp p={p} planned={planned} />
          <Facts p={p} incidents={incidents.data} />
        </div>
      </aside>
    </div>
  )
}

function OverviewSkeleton() {
  return (
    <div
      className="grid grid-cols-[minmax(0,1fr)_300px] items-start gap-4 @max-[1100px]:grid-cols-1"
      aria-busy="true"
      data-testid="status-loading"
    >
      <div className="flex flex-col gap-4">
        <Skeleton className="h-[58px] rounded-[14px]" />
        <Skeleton className="h-60 rounded-[14px]" />
        <Skeleton className="h-[360px] rounded-[14px]" />
      </div>
      <Skeleton className="h-64 rounded-[14px]" />
    </div>
  )
}

export function StatusHome() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const { items, pages, inboxes, inboxOf } = useStatusScope()
  const withPage = new Set(items.map((p) => p.inbox_id))
  const missing = canManage ? inboxes.filter((i) => isSupport(i) && !withPage.has(i.id)) : []
  const [creating, setCreating] = useState<string | null>(null)
  const [preview, setPreview] = useState(false)
  const [confirm, confirmDialog] = useConfirm()
  const current = items.find((p) => p.id === params.get("page")) ?? items[0]
  const raw = params.get("tab") as Tab | null
  const tab: Tab = raw && TABS.includes(raw) && (raw !== "settings" || canManage) ? raw : "overview"
  const at = current ? items.indexOf(current) : -1
  const patch = usePatchStatusPage(current?.id ?? "")
  const newKey = keyLabel(STATUS_KEYS.newPage).join("")
  const previewKey = keyLabel(STATUS_KEYS.preview).join("")
  const publishKey = keyLabel(STATUS_KEYS.publishPage).join("")
  const openKey = keyLabel(STATUS_KEYS.openPage).join("")
  const go = (id: string, t?: Tab) => navigate(statusLink(id, t && t !== "overview" ? t : undefined), { replace: true })
  const pick = (n: number) => items[n] && go(items[n].id, tab)
  const togglePublish = () => {
    if (!current) return
    if (!current.published) return patch.mutate({ published: true })
    const pageName = current.name
    confirm({
      title: <Trans>Unpublish {pageName}?</Trans>,
      description: <Trans>The page answers not found until you publish it again.</Trans>,
      confirm: <Trans>Unpublish</Trans>,
      run: () => patch.mutate({ published: false }),
    })
  }
  const busy = !!creating || preview

  useHotkeys(
    {
      j: () => pick(at + 1),
      k: () => pick(at - 1),
      ArrowDown: () => pick(at + 1),
      ArrowUp: () => pick(at - 1),
      ...(missing.length > 0 ? { [STATUS_KEYS.newPage]: () => setCreating("") } : {}),
      ...(current
        ? {
            [STATUS_KEYS.overview]: () => go(current.id),
            [STATUS_KEYS.components]: () => go(current.id, "components"),
            [STATUS_KEYS.subscribers]: () => go(current.id, "subscribers"),
            ...(canManage ? { [STATUS_KEYS.settings]: () => go(current.id, "settings") } : {}),
            [STATUS_KEYS.preview]: () => setPreview(true),
            ...(current.published
              ? { [STATUS_KEYS.openPage]: () => window.open(current.url, "_blank", "noopener,noreferrer") }
              : {}),
            ...(canManage ? { [STATUS_KEYS.publishPage]: togglePublish } : {}),
          }
        : {}),
    },
    !busy
  )

  const names: Record<Tab, string> = {
    overview: t`Overview`,
    components: t`Components`,
    subscribers: t`Subscribers`,
    settings: t`Settings`,
  }
  const tabs = TABS.filter((v) => v !== "settings" || canManage).map((v) => ({
    key: v,
    label: names[v],
    to: current ? statusLink(current.id, v === "overview" ? undefined : v) : "/status",
    testId: `status-tab-${v}`,
  }))

  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="status-home">
      <PageTop
        className="bg-surface"
        title={<Trans>Status pages</Trans>}
        sub={<Trans>Public pages that say whether your products work</Trans>}
        actions={
          missing.length > 0 && (
            <Button
              className={look.btnInk}
              onClick={() => setCreating("")}
              title={t`Create status page (${newKey})`}
              data-testid="status-home-create"
            >
              <Trans>New page</Trans>
            </Button>
          )
        }
        tabs={current ? <FrameTabs label={t`Status page`} value={tab} items={tabs} /> : undefined}
      />
      {pages.isPending ? (
        <div className="@container min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto max-w-[1360px] px-6 py-5 phone:px-4">
            <OverviewSkeleton />
          </div>
        </div>
      ) : pages.error ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-12 phone:px-4">
          <div className="mx-auto flex max-w-sm flex-col items-center gap-2.5 text-center" data-testid="status-error">
            <span className="grid size-8 place-items-center rounded-full bg-destructive text-white">
              <TriangleAlertIcon className="size-4" aria-hidden />
            </span>
            <ErrorLine error={pages.error} />
            <Button variant="outline" className={look.btnSm} onClick={() => void pages.refetch()}>
              <Trans>Try again</Trans>
            </Button>
          </div>
        </div>
      ) : items.length === 0 ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-8 phone:px-4">
          <div
            className={cn(card, "mx-auto flex max-w-[560px] flex-col items-start gap-3.5 p-6")}
            data-testid="status-home-empty"
          >
            <p className="text-muted-foreground">
              <Trans>
                Tell your customers whether your product works. Each product gets one page, where you open incidents,
                plan maintenance and set the state of each component.
              </Trans>
            </p>
            {missing.length > 0 ? (
              <Button className={look.btnInk} onClick={() => setCreating("")}>
                <PlusIcon />
                <Trans>New page</Trans>
              </Button>
            ) : (
              !canManage && (
                <p className="text-small text-faint" data-testid="status-none">
                  <Trans>No status page yet. Owners and admins create it.</Trans>
                </p>
              )
            )}
          </div>
        </div>
      ) : (
        current && (
          <section
            key={current.id}
            className="@container flex min-h-0 min-w-0 flex-1 flex-col"
            data-testid="status-page"
          >
            <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b bg-surface px-6 py-3 phone:px-4">
              <div className="flex min-w-0 flex-1 basis-56 flex-wrap items-center gap-x-3 gap-y-1.5">
                <PagePicker
                  items={items}
                  current={current}
                  inboxOf={inboxOf}
                  missing={phone ? [] : missing}
                  onPick={(id) => go(id, tab)}
                  onCreate={setCreating}
                />
                <Pill tone={current.published ? "success" : "muted"} data-testid="status-published">
                  {current.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
                </Pill>
                <span className={cn(look.faint, "min-w-0 truncate")} dir="ltr">
                  {hostOf(current.url)}
                  {!current.published && (
                    <>
                      {" · "}
                      <Trans>only your team can preview it</Trans>
                    </>
                  )}
                </span>
              </div>
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  className={look.btnSm}
                  onClick={() => setPreview(true)}
                  title={t`Preview (${previewKey})`}
                  data-testid="status-preview"
                >
                  <Trans>Preview</Trans>
                </Button>
                {current.published && (
                  <Button
                    variant="outline"
                    className={look.btnSm}
                    render={<a href={current.url} target="_blank" rel="noreferrer noopener" />}
                    title={t`View page (${openKey})`}
                    data-testid="status-open-public"
                  >
                    <Trans>View page</Trans>
                    <ArrowUpRightIcon className="text-faint rtl:-scale-x-100" />
                  </Button>
                )}
                {canManage &&
                  (current.published ? (
                    <Button
                      variant="outline"
                      className={look.btnSm}
                      disabled={patch.isPending}
                      onClick={togglePublish}
                      title={t`Unpublish (${publishKey})`}
                      data-testid="status-publish"
                    >
                      <Trans>Unpublish</Trans>
                    </Button>
                  ) : (
                    <Button
                      className={look.btnInkSm}
                      disabled={patch.isPending}
                      onClick={togglePublish}
                      title={t`Publish (${publishKey})`}
                      data-testid="status-publish"
                    >
                      <Trans>Publish</Trans>
                    </Button>
                  ))}
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto">
              <ErrorLine error={patch.error} className="mx-auto max-w-[1360px] px-6 pt-3 phone:px-4" />
              <div
                className={cn(
                  "flex flex-col gap-4 px-6 py-5 pb-16 phone:px-4",
                  tab === "overview" ? "mx-auto max-w-[1360px]" : "max-w-[1040px]"
                )}
              >
                {tab === "overview" && <Overview p={current} />}
                {tab === "components" && <ComponentsTab page={current} />}
                {tab === "subscribers" && <SubscribersTab page={current} />}
                {tab === "settings" && canManage && <SettingsTab page={current} />}
              </div>
            </div>
          </section>
        )
      )}
      {creating !== null && (
        <CreatePageDialog
          inboxes={missing}
          inboxId={creating || undefined}
          onClose={() => setCreating(null)}
          onCreated={(p) => {
            setCreating(null)
            navigate(statusLink(p.id, "settings"))
          }}
        />
      )}
      {preview && current && <PreviewDialog page={current} onClose={() => setPreview(false)} />}
      {confirmDialog}
    </main>
  )
}
