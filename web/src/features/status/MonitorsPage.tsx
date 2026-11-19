import { Trans, useLingui } from "@lingui/react/macro"
import {
  KeyRoundIcon,
  MoreHorizontalIcon,
  PauseIcon,
  PlayIcon,
  PlusIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from "lucide-react"
import { useRef, useState } from "react"
import { Link, useNavigate, useParams } from "react-router"

import {
  Dot,
  ErrorLine,
  InfoColumn,
  InfoSection,
  KV,
  PageLayout,
  PageTop,
  toast,
  topButton,
  useConfirm,
} from "@/components/common"
import {
  boxRowClass,
  FilterMenu,
  FilterRow,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  PageTitle,
} from "@/components/common/ListBox"
import { ProductFilter, useProductScope } from "@/components/common/ProductFilter"
import { keyLabel, MONITOR_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDate, formatDuration, formatRelative, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { useListCursor } from "@/features/inbox/listCursor"
import { look } from "@/features/inbox/PageFrame"
import { IncidentForm, type IncidentPrefill } from "@/features/status/IncidentForm"
import { MonitorChecks } from "@/features/status/MonitorChecks"
import { MonitorForm } from "@/features/status/MonitorForm"
import { MonitorTokenDialog } from "@/features/status/MonitorToken"
import { Pill, ProductTag } from "@/features/status/PageFrame"
import {
  incidentLink,
  monitorLink,
  statusLink,
  useMonitorMutations,
  useMonitorSummary,
  usePagesComponents,
  usePagesIncidents,
  usePagesMonitors,
} from "@/features/status/queries"
import { ResponseChart } from "@/features/status/ResponseChart"
import { useStatusScope } from "@/features/status/scope"
import { isClosed, useStatusText } from "@/features/status/text"
import { usePercent } from "@/features/status/UptimeBars"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import type {
  Inbox,
  StatusComponent,
  StatusIncident,
  StatusIncidentImpact,
  StatusMonitor,
  StatusMonitorWithToken,
  StatusPage,
} from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const FILTERS = ["all", "down", "up", "paused"] as const
type Filter = (typeof FILTERS)[number]

function filterOf(m: StatusMonitor): Filter[] {
  if (m.paused) return ["all", "paused"]
  if (m.state === "down") return ["all", "down"]
  if (m.state === "up") return ["all", "up"]
  return ["all"]
}

function target(m: StatusMonitor) {
  if (m.kind === "http") return m.http?.url
  if (m.kind === "tcp" && m.tcp) return `${m.tcp.host.includes(":") ? `[${m.tcp.host}]` : m.tcp.host}:${m.tcp.port}`
}

function certDays(m: StatusMonitor) {
  if (!m.cert_expires_at) return null
  return Math.floor((Date.parse(m.cert_expires_at) - Date.now()) / 86400000)
}

function useSpark(m: StatusMonitor) {
  const summary = useMonitorSummary(m.page_id, m.id, "24h").data
  const buckets = summary?.buckets ?? []
  const size = Math.max(1, Math.ceil(buckets.length / 24))
  const groups = Array.from({ length: Math.ceil(buckets.length / size) }, (_, i) =>
    buckets.slice(i * size, i * size + size)
  ).map((g) => ({
    ms: Math.max(0, ...g.map((b) => b.p50_ms ?? 0)),
    failed: g.some((b) => b.failures > 0),
    checks: g.reduce((n, b) => n + b.checks, 0),
  }))
  return { summary, groups }
}

function Spark({ m, groups }: { m: StatusMonitor; groups: ReturnType<typeof useSpark>["groups"] }) {
  const { t } = useLingui()
  const timed = m.kind === "http" || m.kind === "tcp"
  const top = Math.max(1, ...groups.map((g) => g.ms))
  const bars = groups.length > 0 ? groups : Array.from({ length: 24 }, () => ({ ms: 0, failed: false, checks: 0 }))
  return (
    <span
      className="flex h-6 items-end gap-px"
      title={timed ? t`Response time, last 24 hours` : t`Results, last 24 hours`}
      data-testid="status-monitor-metrics"
      aria-hidden
    >
      {bars.map((g, i) => (
        <span
          key={i}
          className={cn(
            "min-w-px flex-1 rounded-t-[2px]",
            g.checks === 0 ? "bg-muted" : g.failed ? "bg-destructive/70" : timed ? "bg-success/70" : "bg-success/30",
            timed && g.checks > 0 && i !== bars.length - 1 && "opacity-70"
          )}
          style={{ height: g.checks === 0 ? 2 : timed ? Math.max(3, Math.round((24 * g.ms) / top)) : 24 }}
        />
      ))}
    </span>
  )
}

function useStateLabel() {
  const { t } = useLingui()
  const text = useStatusText()
  return (m: StatusMonitor) => {
    const key = m.paused ? "paused" : m.state
    return {
      key,
      tone: key === "up" ? ("success" as const) : key === "down" ? ("danger" as const) : ("muted" as const),
      label: key === "pending" ? t`Waiting` : text.monitorState[key],
    }
  }
}

function IncidentLine({ page, i }: { page: string; i: StatusIncident }) {
  const { t } = useLingui()
  const text = useStatusText()
  const title = i.title
  const state = i.published_at ? text.incident[i.state] : t`Draft`
  return (
    <Link
      to={incidentLink(page, i.id)}
      className="w-fit max-w-full truncate text-caption text-brand underline-offset-4 hover:underline"
      data-testid="status-monitor-incident"
    >
      <Trans>
        Incident: {title} · {state}
      </Trans>
    </Link>
  )
}

function Fact({ k, v }: { k: React.ReactNode; v: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5">
      <dt className="shrink-0 text-muted-foreground">{k}</dt>
      <dd className="min-w-0 text-end break-all">{v}</dd>
    </div>
  )
}

function Rules({ m, onEdit, canManage }: { m: StatusMonitor; onEdit: () => void; canManage: boolean }) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const goal = target(m)
  const http = m.http
  return (
    <section className="flex flex-col gap-2" data-testid="status-monitor-rules">
      <div className="flex items-center justify-between">
        <h3 className="text-body font-medium">
          <Trans>Rules</Trans>
        </h3>
        {canManage && (
          <Button variant="ghost" size="sm" onClick={onEdit}>
            <Trans>Edit</Trans>
          </Button>
        )}
      </div>
      <dl className="divide-y rounded-xl border bg-card px-4 py-1.5 text-body">
        {goal && (
          <Fact
            k={m.kind === "tcp" ? <Trans>Host</Trans> : <Trans>Address</Trans>}
            v={<span className="font-mono text-[13px]">{goal}</span>}
          />
        )}
        {m.kind === "heartbeat" && m.interval && (
          <Fact k={<Trans>Expected every</Trans>} v={formatDuration(m.interval, i18n.locale)} />
        )}
        {m.kind === "heartbeat" && m.grace !== undefined && (
          <Fact
            k={<Trans>Grace</Trans>}
            v={m.grace === 0 ? <Trans>None</Trans> : formatDuration(m.grace, i18n.locale)}
          />
        )}
        {(m.kind === "http" || m.kind === "tcp") && m.interval && (
          <Fact k={<Trans>Check every</Trans>} v={formatDuration(m.interval, i18n.locale)} />
        )}
        {http && (
          <Fact k={<Trans>Expected status</Trans>} v={`${http.expected_status_min}–${http.expected_status_max}`} />
        )}
        {m.timeout !== undefined && (m.kind === "http" || m.kind === "tcp") && (
          <Fact k={<Trans>Timeout</Trans>} v={formatDuration(m.timeout, i18n.locale)} />
        )}
        <Fact k={<Trans>Failures to go down</Trans>} v={m.failure_threshold} />
        <Fact k={<Trans>Successes to recover</Trans>} v={m.recovery_threshold} />
        <Fact k={<Trans>State while down</Trans>} v={text.component[m.failure_state]} />
        <Fact k={<Trans>When it goes down</Trans>} v={text.onFailure[m.on_failure]} />
      </dl>
    </section>
  )
}

function Address({ m, onRotate, canManage }: { m: StatusMonitor; onRotate: () => void; canManage: boolean }) {
  const { i18n } = useLingui()
  const last = m.last_result_at ? formatRelative(m.last_result_at, i18n.locale) : null
  return (
    <section className="flex flex-col gap-2" data-testid="status-monitor-address">
      <h3 className="text-body font-medium">
        {m.kind === "heartbeat" ? <Trans>Heartbeat address</Trans> : <Trans>Push address</Trans>}
      </h3>
      <div className="flex flex-col gap-2 rounded-xl border bg-card p-4">
        <code className="font-mono text-[13px] break-all">{m.token_prefix ?? ""}••••••••••••</code>
        <p className="text-small text-faint">
          <Trans>
            The full address is shown only once, when you make it. Anyone who has it can report for this monitor.
          </Trans>
        </p>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-caption text-faint">
            {last ? <Trans>Last result {last}</Trans> : <Trans>Nothing has arrived yet.</Trans>}
          </span>
          {canManage && (
            <Button variant="outline" className={look.btnSm} onClick={onRotate}>
              <KeyRoundIcon />
              <Trans>Make new address</Trans>
            </Button>
          )}
        </div>
      </div>
    </section>
  )
}

function Pane({
  m,
  page,
  inbox,
  component,
  incident,
  onEdit,
  onIncident,
  onToken,
  layout,
}: {
  m: StatusMonitor
  page?: StatusPage
  inbox?: Inbox
  component?: StatusComponent
  incident?: StatusIncident
  onEdit: () => void
  onIncident: () => void
  onToken: (m: StatusMonitorWithToken, rotated: boolean) => void
  layout?: (parts: { actions: React.ReactNode; body: React.ReactNode; aside: React.ReactNode }) => React.ReactNode
}) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const text = useStatusText()
  const stateOf = useStateLabel()
  const mut = useMonitorMutations(m.page_id)
  const navigate = useNavigate()
  const [confirm, dialog] = useConfirm()
  const s = stateOf(m)
  const name = m.name
  const every = m.interval ? formatDuration(m.interval, i18n.locale) : ""
  const down = m.state === "down" && !m.paused
  const since = formatShort(m.state_since, i18n.locale)
  const cname = component?.name ?? ""
  const downState = text.component[m.down_state ?? m.failure_state]
  const hasAddress = m.kind === "heartbeat" || m.kind === "push"
  const closed = !!incident && isClosed(incident)
  const pauseKey = keyLabel(MONITOR_KEYS.pause).join("")
  const editKey = keyLabel(MONITOR_KEYS.edit).join("")
  const incidentKey = keyLabel(MONITOR_KEYS.incident).join("")
  const removeKey = keyLabel(MONITOR_KEYS.remove).join("")

  const rotate = () =>
    confirm({
      title: <Trans>Make a new address for {name}?</Trans>,
      description: <Trans>The current address stops working at once. Update whatever calls it.</Trans>,
      confirm: <Trans>Make new address</Trans>,
      run: () => mut.rotate.mutate(m.id, { onSuccess: (r) => onToken(r, true) }),
    })
  const remove = () =>
    confirm({
      title: <Trans>Delete {name}?</Trans>,
      description: <Trans>Its checks are deleted. An incident it opened is resolved.</Trans>,
      confirm: <Trans>Delete monitor</Trans>,
      run: () =>
        mut.remove.mutate(m.id, {
          onSuccess: () => {
            toast(t`Monitor deleted`)
            navigate("/monitors", { replace: true })
          },
        }),
    })
  const pause = () => mut.update.mutate({ id: m.id, paused: !m.paused })
  useHotkeys(
    canManage
      ? {
          [MONITOR_KEYS.edit]: onEdit,
          [MONITOR_KEYS.pause]: pause,
          ...(!incident ? { [MONITOR_KEYS.incident]: onIncident } : {}),
          [MONITOR_KEYS.remove]: remove,
        }
      : {}
  )

  const btn = layout ? topButton : look.btnSm
  const actions = canManage && (
    <>
      <Button
        variant="outline"
        className={btn}
        onClick={onEdit}
        title={t`Edit (${editKey})`}
        data-testid="status-monitor-edit"
      >
        <Trans>Edit</Trans>
      </Button>
      <Button
        variant="outline"
        className={btn}
        disabled={mut.update.isPending}
        onClick={pause}
        title={(m.paused ? t`Resume` : t`Pause`) + ` (${pauseKey})`}
        data-testid="status-monitor-pause"
      >
        {m.paused ? <PlayIcon /> : <PauseIcon />}
        {m.paused ? <Trans>Resume</Trans> : <Trans>Pause</Trans>}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="ghost" size="icon-sm" className="text-faint" aria-label={t`More for ${name}`} />}
          data-testid="status-monitor-menu"
        >
          <MoreHorizontalIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-52">
          {!incident && (
            <DropdownMenuItem onClick={onIncident} title={t`Open an incident (${incidentKey})`}>
              <TriangleAlertIcon />
              <Trans>Open an incident</Trans>
            </DropdownMenuItem>
          )}
          {hasAddress && (
            <DropdownMenuItem onClick={rotate}>
              <KeyRoundIcon />
              <Trans>Make new address</Trans>
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onClick={remove} title={t`Delete (${removeKey})`}>
            <Trash2Icon />
            <Trans>Delete</Trans>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  )
  const heading = (
    <header className={cn("flex flex-wrap items-start gap-x-3 gap-y-2", layout && "mt-3")}>
      <div className="flex min-w-0 flex-1 basis-48 flex-col gap-1">
        <div className="flex min-w-0 items-center gap-2">
          <h2
            className={cn(
              "min-w-0 font-semibold",
              layout ? "m-0 text-page leading-[34px] break-words" : "truncate text-[17px] leading-[1.3]"
            )}
            title={name}
          >
            {name}
          </h2>
          <Pill tone={s.tone}>{s.label}</Pill>
        </div>
        <p className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-caption text-muted-foreground">
          {page && <ProductTag name={inbox?.name ?? page.name} color={inbox?.branding.color} />}
          <span>{cname}</span>
          <span aria-hidden>·</span>
          <span>{text.monitorKind[m.kind]}</span>
          {every && m.kind !== "push" && (
            <>
              <span aria-hidden>·</span>
              <span>{every}</span>
            </>
          )}
        </p>
      </div>
    </header>
  )
  const body = (
    <>
      <ErrorLine error={mut.update.error ?? mut.remove.error ?? mut.rotate.error} />
      {down && (
        <div
          className="flex flex-col gap-3 rounded-xl border border-destructive/40 bg-destructive/5 p-4"
          data-testid="status-monitor-down"
        >
          <div className="flex flex-col leading-[1.4]">
            <span className="font-semibold text-destructive">
              <Trans>Down since {since}</Trans>
            </span>
            {m.last_message && <span>{m.last_message}</span>}
            <span className="text-small text-muted-foreground">
              <Trans>
                Sets {cname} to {downState} while it is down
              </Trans>
            </span>
          </div>
          {incident ? (
            <div className="flex flex-wrap items-center justify-between gap-2 border-t border-destructive/20 pt-3">
              <IncidentLine page={m.page_id} i={incident} />
              <Button
                variant="outline"
                className={look.btnSm}
                render={<Link to={incidentLink(m.page_id, incident.id)} />}
              >
                <Trans>Open incident</Trans>
              </Button>
            </div>
          ) : (
            canManage && (
              <div className="flex flex-wrap items-center justify-between gap-2 border-t border-destructive/20 pt-3">
                <span className="min-w-48 flex-1 text-small text-muted-foreground">
                  <Trans>Customers cannot see this yet. Tell them with an incident.</Trans>
                </span>
                <Button className={look.btnInkSm} onClick={onIncident}>
                  <Trans>Open an incident</Trans>
                </Button>
              </div>
            )
          )}
        </div>
      )}
      {!down && !m.paused && m.state === "up" && incident && !closed && (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-success/40 bg-success/8 p-4">
          <span className="flex min-w-48 flex-1 flex-col leading-[1.4]">
            <span className="font-semibold text-success">
              <Trans>Up again</Trans>
            </span>
            <span className="text-small text-muted-foreground">
              <Trans>An incident is still open. Post the all-clear.</Trans>
            </span>
          </span>
          <Button variant="outline" className={look.btnSm} render={<Link to={incidentLink(m.page_id, incident.id)} />}>
            <Trans>Open incident</Trans>
          </Button>
        </div>
      )}
      {m.paused && (
        <p className="rounded-xl border border-dashed px-4 py-3 text-muted-foreground">
          <Trans>Paused. Results are ignored and the component goes back to how you set it.</Trans>
        </p>
      )}
      <ResponseChart pageId={m.page_id} monitorId={m.id} kind={m.kind} />
      {hasAddress && <Address m={m} onRotate={rotate} canManage={canManage} />}
      <Rules m={m} onEdit={onEdit} canManage={canManage} />
      <MonitorChecks monitor={m} />
    </>
  )
  if (layout)
    return (
      <>
        {layout({
          actions,
          body: (
            <div className="flex flex-col gap-4" data-testid="status-monitor-pane">
              {heading}
              {body}
            </div>
          ),
          aside: <MonitorInfo m={m} page={page} component={component} incident={incident} />,
        })}
        {dialog}
      </>
    )
  return (
    <div className="flex min-h-full flex-col gap-4 p-5 pb-16 phone:px-4" data-testid="status-monitor-pane">
      <header className="flex flex-wrap items-start gap-x-3 gap-y-2">
        <div className="flex min-w-0 flex-1 basis-48 flex-col gap-1">
          <div className="flex min-w-0 items-center gap-2">
            <h2 className="min-w-0 truncate text-[17px] leading-[1.3] font-semibold" title={name}>
              {name}
            </h2>
            <Pill tone={s.tone}>{s.label}</Pill>
          </div>
          <p className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-caption text-muted-foreground">
            {page && <ProductTag name={inbox?.name ?? page.name} color={inbox?.branding.color} />}
            <span>{cname}</span>
            <span aria-hidden>·</span>
            <span>{text.monitorKind[m.kind]}</span>
            {every && m.kind !== "push" && (
              <>
                <span aria-hidden>·</span>
                <span>{every}</span>
              </>
            )}
          </p>
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </header>
      {body}
      {dialog}
    </div>
  )
}

function MonitorInfo({
  m,
  page,
  component,
  incident,
}: {
  m: StatusMonitor
  page?: StatusPage
  component?: StatusComponent
  incident?: StatusIncident
}) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const stateOf = useStateLabel()
  const percent = usePercent()
  const summary = useMonitorSummary(m.page_id, m.id, "24h").data
  const s = stateOf(m)
  const tone = { success: "ok", danger: "bad", muted: undefined } as const
  return (
    <>
      <InfoSection title={<Trans>Monitor</Trans>}>
        <KV label={<Trans>State</Trans>} value={s.label} tone={tone[s.tone]} />
        <KV label={<Trans>Since</Trans>} value={formatShort(m.state_since, i18n.locale)} />
        <KV label={<Trans>Kind</Trans>} value={text.monitorKind[m.kind]} />
        {summary?.uptime !== undefined && (
          <KV label={<Trans>Uptime, 24 hours</Trans>} value={percent(summary.uptime)} />
        )}
        {m.last_result_at && (
          <KV label={<Trans>Last result</Trans>} value={formatRelative(m.last_result_at, i18n.locale)} />
        )}
      </InfoSection>
      <InfoSection title={<Trans>Status page</Trans>}>
        {page && <KV label={<Trans>Page</Trans>} value={page.name} to={statusLink(page.id)} />}
        <KV label={<Trans>Component</Trans>} value={component?.name ?? "–"} />
        <KV label={<Trans>State while down</Trans>} value={text.component[m.failure_state]} />
        {incident && (
          <KV label={<Trans>Incident</Trans>} value={incident.title} to={incidentLink(m.page_id, incident.id)} />
        )}
      </InfoSection>
    </>
  )
}

function MonitorRow({
  m,
  page,
  inbox,
  component,
  incident,
  cursor,
  canManage,
  onOpen,
  onIncident,
}: {
  m: StatusMonitor
  page?: StatusPage
  inbox?: Inbox
  component?: StatusComponent
  incident?: StatusIncident
  cursor: boolean
  canManage: boolean
  onOpen: () => void
  onIncident: () => void
}) {
  const { i18n } = useLingui()
  const percent = usePercent()
  const stateOf = useStateLabel()
  const text = useStatusText()
  const { summary, groups } = useSpark(m)
  const s = stateOf(m)
  const days = certDays(m)
  const warnDays = Math.max(m.http?.cert_warn_days ?? 0, 14)
  const expires = m.cert_expires_at ? formatDate(m.cert_expires_at, i18n.locale) : null
  const every = m.interval ? formatDuration(m.interval, i18n.locale) : ""
  const down = m.state === "down" && !m.paused
  return (
    <div data-row data-row-id={m.id} className={boxRowClass({ cursor })} data-testid="status-monitor">
      <div className="flex min-w-0 flex-1 flex-col py-3 pe-3.5">
        <button
          type="button"
          onClick={onOpen}
          className="flex min-w-0 items-center gap-3 text-start outline-none"
          data-testid="status-monitor-open"
        >
          <span className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate text-body font-semibold">{m.name}</span>
              <span className="shrink-0 text-small text-muted-foreground">{text.monitorKind[m.kind]}</span>
            </span>
            <span className="flex min-w-0 items-center gap-1.5 truncate text-small text-muted-foreground">
              {page && (
                <>
                  <Dot color={inbox?.branding.color} className="size-[7px] shrink-0 rounded-[2px]" />
                  <span className="shrink-0">{inbox?.name ?? page.name}</span>
                  <span aria-hidden>·</span>
                </>
              )}
              <span className="truncate">
                {component?.name ?? "–"}
                {every && m.kind !== "push" ? ` · ${every}` : ""}
              </span>
            </span>
            {down && m.last_message && <span className="truncate text-small text-bad">{m.last_message}</span>}
            {expires && days !== null && days <= warnDays && (
              <span className={cn("text-small", days < 0 ? "text-bad" : "text-warn")} data-testid="status-monitor-cert">
                {days < 0 ? <Trans>Certificate expired {expires}</Trans> : <Trans>Certificate expires {expires}</Trans>}
              </span>
            )}
          </span>
          <span className="w-28 shrink-0 max-sm:hidden">
            <Spark m={m} groups={groups} />
          </span>
          <span className="w-14 shrink-0 text-end font-mono text-xs text-muted-foreground tabular-nums max-sm:hidden">
            {summary?.uptime === undefined ? "–" : percent(summary.uptime)}
          </span>
          <Pill tone={s.tone} data-testid="status-monitor-state">
            {s.label}
          </Pill>
        </button>
        {(incident || (down && canManage)) && (
          <div className="mt-2 flex flex-wrap items-center gap-3">
            {incident ? (
              <IncidentLine page={m.page_id} i={incident} />
            ) : (
              <Button variant="outline" size="sm" onClick={onIncident} data-testid="status-monitor-open-incident">
                <TriangleAlertIcon />
                <Trans>Open an incident</Trans>
              </Button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

export function MonitorsPage() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { monitorId = "" } = useParams()
  const { canManage } = useSession()
  const { items: pages, pages: pagesQuery, inboxOf } = useStatusScope()
  const { chosen, set } = useProductScope()
  const ids = pages.map((p) => p.id)
  const monitors = usePagesMonitors(ids)
  const components = usePagesComponents(ids)
  const open = usePagesIncidents(ids, true)
  const byComponent = new Map(components.map((c) => [c.id, c]))
  const byPage = new Map(pages.map((p) => [p.id, p]))
  const now = useNow(60_000)
  const [filter, setFilter] = useState<Filter>("all")
  const [form, setForm] = useState<{ edit?: StatusMonitor } | null>(null)
  const [token, setToken] = useState<{ monitor: StatusMonitorWithToken; rotated: boolean } | null>(null)
  const [incidentFor, setIncidentFor] = useState<{ monitor: StatusMonitor; prefill: IncidentPrefill } | null>(null)
  const items = [...monitors.data]
    .filter((m) => !chosen || byPage.get(m.page_id)?.inbox_id === chosen)
    .sort(
      (a, b) =>
        Number(b.state === "down" && !b.paused) - Number(a.state === "down" && !a.paused) ||
        ids.indexOf(a.page_id) - ids.indexOf(b.page_id) ||
        a.created_at.localeCompare(b.created_at)
    )
  const counts = Object.fromEntries(
    FILTERS.map((f) => [f, items.filter((m) => filterOf(m).includes(f)).length])
  ) as Record<Filter, number>
  const rows = items.filter((m) => filterOf(m).includes(filter))
  const selected = monitors.data.find((m) => m.id === monitorId)
  const listRef = useRef<HTMLDivElement>(null)
  const openIncidents = open.data.filter((i) => !isClosed(i))
  const incidentOf = (m: StatusMonitor) =>
    openIncidents.find((i) => i.id === m.incident_id) ??
    openIncidents.find((i) => i.kind === "incident" && i.components.some((c) => c.component_id === m.component_id))
  const loading = pagesQuery.isPending || (pages.length > 0 && (monitors.isPending || open.isPending))
  const addKey = keyLabel(MONITOR_KEYS.add).join("")
  const busy = !!form || !!incidentFor
  const listing = !monitorId
  const { cursorId } = useListCursor(
    rows,
    null,
    (id) => navigate(monitorLink(id)),
    listing && !busy,
    listRef,
    true,
    true
  )
  const step = (by: number) => {
    const at = selected ? rows.findIndex((m) => m.id === selected.id) : -1
    const next = rows[Math.min(rows.length - 1, Math.max(0, at + by))]
    if (next) navigate(monitorLink(next.id), { replace: true })
  }
  useHotkeys(
    monitorId
      ? {
          [MONITOR_KEYS.previous]: () => step(-1),
          [MONITOR_KEYS.next]: () => step(1),
          Escape: () => navigate("/monitors"),
        }
      : {
          [MONITOR_KEYS.all]: () => setFilter("all"),
          [MONITOR_KEYS.down]: () => setFilter("down"),
          [MONITOR_KEYS.up]: () => setFilter("up"),
          [MONITOR_KEYS.paused]: () => setFilter("paused"),
          ...(canManage && pages.length > 0 ? { [MONITOR_KEYS.add]: () => setForm({}) } : {}),
        },
    !busy
  )

  const prefill = (m: StatusMonitor): IncidentPrefill => {
    const comp = byComponent.get(m.component_id)?.name ?? m.name
    const state = m.down_state ?? m.failure_state
    const impact: StatusIncidentImpact =
      state === "degraded" ? "minor" : state === "partial_outage" ? "major" : "critical"
    const monitor = m.name
    const at = formatShort(m.state === "down" ? m.state_since : new Date(now).toISOString(), i18n.locale)
    return {
      title: t`${comp} is not working`,
      impact,
      components: { [m.component_id]: state },
      message: t`Our monitor ${monitor} has seen a problem since ${at}. We are looking into it.`,
    }
  }
  const startIncident = (m: StatusMonitor) => {
    setForm(null)
    setIncidentFor({ monitor: m, prefill: prefill(m) })
  }

  const names: Record<Filter, string> = { all: t`All`, down: t`Down`, up: t`Up`, paused: t`Paused` }
  const fmt = new Intl.NumberFormat(i18n.locale)

  const forms = (
    <>
      {form && (
        <MonitorForm
          key={form.edit?.id ?? "new"}
          pages={chosen ? pages.filter((p) => p.inbox_id === chosen) : pages}
          pageId={selected?.page_id}
          edit={form.edit}
          className={monitorId ? "mt-0" : undefined}
          onClose={() => setForm(null)}
          onCreated={(monitor) => {
            setForm(null)
            if (monitor.url) setToken({ monitor, rotated: false })
          }}
        />
      )}
      {incidentFor && (
        <IncidentForm
          pages={pages}
          pageId={incidentFor.monitor.page_id}
          kind="incident"
          prefill={incidentFor.prefill}
          className={monitorId ? "mt-0" : undefined}
          onClose={() => setIncidentFor(null)}
          onDone={() => {
            setIncidentFor(null)
            toast(t`Incident opened`)
          }}
        />
      )}
    </>
  )
  const tokenDialog = token && (
    <MonitorTokenDialog monitor={token.monitor} rotated={token.rotated} onClose={() => setToken(null)} />
  )

  if (monitorId) {
    const back = { label: t`Monitors`, to: "/monitors" }
    if (!selected)
      return (
        <PageLayout testId="status-monitors" label={t`Monitors`}>
          <PageTop back={back} />
          {loading ? (
            <div className="mt-3 flex max-w-[760px] flex-col gap-4">
              <Skeleton className="h-8 w-60" />
              <Skeleton className="h-24 w-full" />
            </div>
          ) : (
            <ListBox>
              <ListBoxEmpty>
                <ErrorLine error={pagesQuery.error ?? monitors.error} />
                <Trans>This monitor no longer exists or you cannot see it.</Trans>
              </ListBoxEmpty>
            </ListBox>
          )}
        </PageLayout>
      )
    return (
      <>
        <Pane
          key={selected.id}
          m={selected}
          page={byPage.get(selected.page_id)}
          inbox={inboxOf(byPage.get(selected.page_id))}
          component={byComponent.get(selected.component_id)}
          incident={incidentOf(selected)}
          onEdit={() => {
            setIncidentFor(null)
            setForm({ edit: selected })
          }}
          onIncident={() => startIncident(selected)}
          onToken={(monitor, rotated) => setToken({ monitor, rotated })}
          layout={({ actions, body, aside }) => (
            <PageLayout
              testId="status-monitors"
              label={selected.name}
              aside={<InfoColumn label={t`Details`}>{aside}</InfoColumn>}
            >
              <PageTop back={back} actions={actions && <div className="flex items-center gap-1.5">{actions}</div>} />
              <div className="@container flex max-w-[760px] flex-col gap-4">
                {forms}
                {body}
              </div>
            </PageLayout>
          )}
        />
        {tokenDialog}
      </>
    )
  }

  return (
    <PageLayout testId="status-monitors" label={t`Monitors`}>
      <section className="@container flex min-w-0 flex-col">
        <PageTitle
          title={<Trans>Monitors</Trans>}
          sub={<Trans>Each monitor sets the state of one component while it is down.</Trans>}
          actions={
            canManage &&
            pages.length > 0 && (
              <Button
                onClick={() => {
                  setIncidentFor(null)
                  setForm({})
                }}
                disabled={!!form}
                title={t`Add monitor (${addKey})`}
                data-testid="status-add-monitor"
              >
                <PlusIcon />
                <Trans>New monitor</Trans>
              </Button>
            )
          }
          testId="monitors-head"
        />
        {forms}
        {pages.length > 0 && (
          <FilterRow testId="monitors-filters">
            <FilterMenu
              value={filter === "all" ? "" : filter}
              onChange={(v) => setFilter((v || "all") as Filter)}
              options={(["down", "up", "paused"] as const).map((f) => ({
                value: f,
                label: `${names[f]} · ${fmt.format(counts[f])}`,
              }))}
              anyLabel={`${names.all} · ${fmt.format(counts.all)}`}
              label={t`Show`}
              testId="monitors-state"
            />
            <ProductFilter value={chosen} onChange={(id) => set("inbox", id)} testId="monitors-product" />
          </FilterRow>
        )}
        <ErrorLine error={pagesQuery.error ?? monitors.error} className="mt-3" />
        <div ref={listRef}>
          <ListBox testId="status-monitor-list">
            {loading ? (
              <ListBoxSkeleton rows={3} />
            ) : pages.length === 0 ? (
              <ListBoxEmpty
                action={
                  canManage && (
                    <Button variant="outline" size="sm" render={<Link to="/status" />}>
                      <Trans>Status pages</Trans>
                    </Button>
                  )
                }
              >
                <Trans>Monitors set the state of a status page component. Create a status page first.</Trans>
              </ListBoxEmpty>
            ) : rows.length === 0 ? (
              <ListBoxEmpty
                action={
                  canManage &&
                  items.length === 0 &&
                  !form && (
                    <Button variant="outline" size="sm" onClick={() => setForm({})}>
                      <Trans>New monitor</Trans>
                    </Button>
                  )
                }
              >
                {items.length === 0 ? (
                  <Trans>
                    No monitors yet. Add one to let a check, a job or your monitoring set a component’s state.
                  </Trans>
                ) : (
                  <Trans>No monitor matches this filter.</Trans>
                )}
              </ListBoxEmpty>
            ) : (
              rows.map((m) => {
                const page = byPage.get(m.page_id)
                return (
                  <MonitorRow
                    key={m.id}
                    m={m}
                    page={page}
                    inbox={inboxOf(page)}
                    component={byComponent.get(m.component_id)}
                    incident={incidentOf(m)}
                    cursor={m.id === cursorId}
                    canManage={canManage}
                    onOpen={() => navigate(monitorLink(m.id))}
                    onIncident={() => startIncident(m)}
                  />
                )
              })
            )}
          </ListBox>
        </div>
        <KeyHints
          items={[
            [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
            [
              ["↵"],
              <Trans key="open" context="keyboard hint">
                open
              </Trans>,
            ],
            [[MONITOR_KEYS.add], <Trans key="new">new monitor</Trans>],
          ]}
        />
      </section>
      {tokenDialog}
    </PageLayout>
  )
}
