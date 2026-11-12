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
  ErrorLine,
  FlatEmpty,
  FlatRow,
  FlatTag,
  FlatWho,
  InfoSection,
  KV,
  PageHeading,
  PageTop,
  toast,
  topButton,
  topInk,
  useConfirm,
} from "@/components/common"
import { PaneBack } from "@/components/common/Column"
import { keyLabel, MONITOR_KEYS } from "@/components/common/ShortcutSheet"
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
import { look, PageTop as StackTop } from "@/features/inbox/PageFrame"
import { IncidentDialog, type IncidentPrefill } from "@/features/status/IncidentDialog"
import { MonitorChecks } from "@/features/status/MonitorChecks"
import { MonitorDialog } from "@/features/status/MonitorDialog"
import { MonitorTokenDialog } from "@/features/status/MonitorToken"
import { Chip, Pill, ProductTag } from "@/features/status/PageFrame"
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
import { StatusLayout, StatusTrail } from "@/features/status/StatusFrame"
import { isClosed, useStatusText } from "@/features/status/text"
import { usePercent } from "@/features/status/UptimeBars"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone, useMediaQuery } from "@/hooks/use-media-query"
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

function Row({
  m,
  page,
  inbox,
  component,
  incident,
  current,
  canManage,
  onIncident,
}: {
  m: StatusMonitor
  page?: StatusPage
  inbox?: Inbox
  component?: StatusComponent
  incident?: StatusIncident
  current: boolean
  canManage: boolean
  onIncident: () => void
}) {
  const { i18n } = useLingui()
  const percent = usePercent()
  const stateOf = useStateLabel()
  const { summary, groups } = useSpark(m)
  const s = stateOf(m)
  const days = certDays(m)
  const warnDays = Math.max(m.http?.cert_warn_days ?? 0, 14)
  const expires = m.cert_expires_at ? formatDate(m.cert_expires_at, i18n.locale) : null
  const every = m.interval ? formatDuration(m.interval, i18n.locale) : ""
  const text = useStatusText()
  const down = m.state === "down" && !m.paused
  return (
    <li className={cn("border-b", current && "bg-muted")} data-testid="status-monitor">
      <Link
        to={monitorLink(m.id)}
        replace
        aria-current={current ? "true" : undefined}
        className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 px-4 py-3 outline-none transition-colors hover:bg-muted/60 focus-visible:bg-muted @2xl:grid-cols-[minmax(0,1fr)_auto_96px_56px]"
      >
        <span className="flex min-w-0 flex-col gap-0.5 leading-[1.3]">
          <span className="flex min-w-0 items-center gap-2">
            <span className="min-w-0 truncate font-medium" title={m.name}>
              {m.name}
            </span>
            <Chip>{text.monitorKind[m.kind]}</Chip>
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-caption text-muted-foreground">
            {page && <ProductTag name={inbox?.name ?? page.name} color={inbox?.branding.color} />}
            <span className="min-w-0 truncate">
              {component?.name ?? "–"}
              {every && m.kind !== "push" ? ` · ${every}` : ""}
            </span>
          </span>
          {down && m.last_message && <span className="truncate text-caption text-destructive">{m.last_message}</span>}
          {expires && days !== null && days <= warnDays && (
            <span
              className={cn("text-caption", days < 0 ? "text-destructive" : "text-warning")}
              data-testid="status-monitor-cert"
            >
              {days < 0 ? <Trans>Certificate expired {expires}</Trans> : <Trans>Certificate expires {expires}</Trans>}
            </span>
          )}
        </span>
        <Pill tone={s.tone} data-testid="status-monitor-state">
          {s.label}
        </Pill>
        <span className="hidden @2xl:block">
          <Spark m={m} groups={groups} />
        </span>
        <span className="hidden text-muted-foreground tabular-nums @2xl:block">
          {summary?.uptime === undefined ? "–" : percent(summary.uptime)}
        </span>
      </Link>
      {(incident || (down && !incident && canManage)) && (
        <div className="flex flex-wrap items-center gap-3 px-4 pb-3">
          {incident ? (
            <IncidentLine page={m.page_id} i={incident} />
          ) : (
            <Button
              variant="outline"
              className={look.btnSm}
              onClick={onIncident}
              data-testid="status-monitor-open-incident"
            >
              <TriangleAlertIcon />
              <Trans>Open an incident</Trans>
            </Button>
          )}
        </div>
      )}
    </li>
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

function FlatMonitorRow({
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
  const tone = { success: "text-ok", danger: "text-bad font-medium", muted: "text-muted-foreground" }
  return (
    <div data-testid="status-monitor" role="listitem">
      <FlatRow
        color={inbox?.branding.color}
        cursor={cursor}
        data-row-id={m.id}
        cols="grid-cols-[7px_minmax(0,1fr)_minmax(0,140px)_auto]"
        button={{ onClick: onOpen }}
        who={
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="flex min-w-0 items-center gap-2">
              <FlatWho strong>{m.name}</FlatWho>
              <FlatTag>{text.monitorKind[m.kind]}</FlatTag>
            </span>
            <span className="truncate text-small text-muted-foreground">
              {page ? `${inbox?.name ?? page.name} · ` : ""}
              {component?.name ?? "–"}
              {every && m.kind !== "push" ? ` · ${every}` : ""}
            </span>
            {down && m.last_message && <span className="truncate text-small text-bad">{m.last_message}</span>}
            {expires && days !== null && days <= warnDays && (
              <span className={cn("text-small", days < 0 ? "text-bad" : "text-warn")} data-testid="status-monitor-cert">
                {days < 0 ? <Trans>Certificate expired {expires}</Trans> : <Trans>Certificate expires {expires}</Trans>}
              </span>
            )}
          </span>
        }
        end={
          <>
            <span className={cn("text-small whitespace-nowrap", tone[s.tone])} data-testid="status-monitor-state">
              {s.label}
            </span>
            <span className="w-16 text-end font-mono text-xs text-muted-foreground tabular-nums">
              {summary?.uptime === undefined ? "–" : percent(summary.uptime)}
            </span>
          </>
        }
      >
        <Spark m={m} groups={groups} />
      </FlatRow>
      {(incident || (down && canManage)) && (
        <div className="flex flex-wrap items-center gap-3 ps-[26px] pb-3">
          {incident ? (
            <IncidentLine page={m.page_id} i={incident} />
          ) : (
            <Button
              variant="outline"
              className={look.btnSm}
              onClick={onIncident}
              data-testid="status-monitor-open-incident"
            >
              <TriangleAlertIcon />
              <Trans>Open an incident</Trans>
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

export function MonitorsPage() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { monitorId = "" } = useParams()
  const { canManage } = useSession()
  const wide = useMediaQuery("(min-width: 1180px)")
  const phone = useIsPhone()
  const { items: pages, pages: pagesQuery, inboxOf } = useStatusScope()
  const ids = pages.map((p) => p.id)
  const monitors = usePagesMonitors(ids)
  const components = usePagesComponents(ids)
  const open = usePagesIncidents(ids, true)
  const byComponent = new Map(components.map((c) => [c.id, c]))
  const byPage = new Map(pages.map((p) => [p.id, p]))
  const now = useNow(60_000)
  const [filter, setFilter] = useState<Filter>("all")
  const [dialog, setDialog] = useState<{ edit?: StatusMonitor } | null>(null)
  const [token, setToken] = useState<{ monitor: StatusMonitorWithToken; rotated: boolean } | null>(null)
  const [incidentFor, setIncidentFor] = useState<{ monitor: StatusMonitor; prefill: IncidentPrefill } | null>(null)
  const items = [...monitors.data].sort(
    (a, b) =>
      Number(b.state === "down" && !b.paused) - Number(a.state === "down" && !a.paused) ||
      ids.indexOf(a.page_id) - ids.indexOf(b.page_id) ||
      a.created_at.localeCompare(b.created_at)
  )
  const counts = Object.fromEntries(
    FILTERS.map((f) => [f, items.filter((m) => filterOf(m).includes(f)).length])
  ) as Record<Filter, number>
  const rows = items.filter((m) => filterOf(m).includes(filter))
  const selected = items.find((m) => m.id === monitorId)
  const flat = !phone
  const shown = selected ?? (wide && !flat ? rows[0] : undefined)
  const listRef = useRef<HTMLDivElement>(null)
  const openIncidents = open.data.filter((i) => !isClosed(i))
  const incidentOf = (m: StatusMonitor) =>
    openIncidents.find((i) => i.id === m.incident_id) ??
    openIncidents.find((i) => i.kind === "incident" && i.components.some((c) => c.component_id === m.component_id))
  const loading = pagesQuery.isPending || (pages.length > 0 && (monitors.isPending || open.isPending))
  const showPane = wide || !!monitorId
  const showList = wide || !monitorId
  const addKey = keyLabel(MONITOR_KEYS.add).join("")

  const step = (by: number) => {
    const at = shown ? rows.findIndex((m) => m.id === shown.id) : -1
    const next = rows[Math.min(rows.length - 1, Math.max(0, at + by))]
    if (next) navigate(monitorLink(next.id), { replace: true })
  }
  const flatList = flat && !monitorId
  const { cursorId } = useListCursor(
    rows,
    null,
    (id) => navigate(monitorLink(id)),
    flatList && !dialog && !incidentFor,
    listRef,
    true,
    true
  )
  useHotkeys(
    {
      ...(flatList
        ? {}
        : {
            j: () => step(1),
            k: () => step(-1),
            ArrowDown: () => step(1),
            ArrowUp: () => step(-1),
          }),
      ...(flat && monitorId ? { Escape: () => navigate("/monitors") } : {}),
      [MONITOR_KEYS.all]: () => setFilter("all"),
      [MONITOR_KEYS.down]: () => setFilter("down"),
      [MONITOR_KEYS.up]: () => setFilter("up"),
      [MONITOR_KEYS.paused]: () => setFilter("paused"),
      ...(canManage && pages.length > 0 ? { [MONITOR_KEYS.add]: () => setDialog({}) } : {}),
    },
    !dialog && !incidentFor
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

  const startIncident = (m: StatusMonitor) => setIncidentFor({ monitor: m, prefill: prefill(m) })

  const filters = FILTERS.map((f) => ({
    key: f,
    label: { all: t`All`, down: t`Down`, up: t`Up`, paused: t`Paused` }[f],
    count: counts[f],
  }))

  const dialogs = (
    <>
      {dialog && (
        <MonitorDialog
          pages={pages}
          pageId={shown?.page_id}
          edit={dialog.edit}
          onClose={() => setDialog(null)}
          onCreated={(monitor) => {
            setDialog(null)
            if (monitor.url) setToken({ monitor, rotated: false })
          }}
        />
      )}
      {incidentFor && (
        <IncidentDialog
          pages={pages}
          pageId={incidentFor.monitor.page_id}
          kind="incident"
          prefill={incidentFor.prefill}
          onClose={() => setIncidentFor(null)}
          onDone={() => {
            setIncidentFor(null)
            toast(t`Incident opened`)
          }}
        />
      )}
      {token && <MonitorTokenDialog monitor={token.monitor} rotated={token.rotated} onClose={() => setToken(null)} />}
    </>
  )

  if (flat) {
    const back = { label: t`Monitors`, to: "/monitors" }
    if (monitorId)
      return (
        <>
          {selected ? (
            <Pane
              key={selected.id}
              m={selected}
              page={byPage.get(selected.page_id)}
              inbox={inboxOf(byPage.get(selected.page_id))}
              component={byComponent.get(selected.component_id)}
              incident={incidentOf(selected)}
              onEdit={() => setDialog({ edit: selected })}
              onIncident={() => startIncident(selected)}
              onToken={(monitor, rotated) => setToken({ monitor, rotated })}
              layout={({ actions, body, aside }) => (
                <StatusLayout place="monitors" testId="status-monitors" aside={aside}>
                  <PageTop back={back} actions={actions && <div className="flex items-center gap-1.5">{actions}</div>}>
                    <StatusTrail place="monitors" name={byPage.get(selected.page_id)?.name} />
                  </PageTop>
                  <div className="@container max-w-[760px]">{body}</div>
                </StatusLayout>
              )}
            />
          ) : (
            <StatusLayout place="monitors" testId="status-monitors">
              <PageTop back={back} />
              {loading ? (
                <div className="mt-3 flex max-w-[760px] flex-col gap-4">
                  <Skeleton className="h-8 w-60" />
                  <Skeleton className="h-24 w-full" />
                </div>
              ) : (
                <FlatEmpty>
                  <ErrorLine error={pagesQuery.error ?? monitors.error} />
                  <Trans>This monitor no longer exists or you cannot see it.</Trans>
                </FlatEmpty>
              )}
            </StatusLayout>
          )}
          {dialogs}
        </>
      )
    return (
      <StatusLayout
        place="monitors"
        testId="status-monitors"
        aside={
          items.length > 0 && (
            <InfoSection title={<Trans>Monitors</Trans>}>
              {filters.map((f) => (
                <KV
                  key={f.key}
                  label={f.label}
                  value={f.count}
                  tone={f.key === "down" && f.count > 0 ? "bad" : f.key === "up" && f.count > 0 ? "ok" : undefined}
                />
              ))}
            </InfoSection>
          )
        }
      >
        <PageTop
          actions={
            canManage &&
            pages.length > 0 && (
              <Button
                className={topInk}
                onClick={() => setDialog({})}
                title={t`Add monitor (${addKey})`}
                data-testid="status-add-monitor"
              >
                <PlusIcon className="size-3.5" aria-hidden />
                <Trans>New monitor</Trans>
              </Button>
            )
          }
        >
          <StatusTrail place="monitors" />
        </PageTop>
        <section className="@container flex max-w-[980px] flex-col">
          <PageHeading
            title={t`Monitors`}
            lede={
              <>
                <Trans>Checks that watch your products and set component states</Trans>
                {". "}
                <Trans>Each monitor sets the state of one component while it is down.</Trans>
              </>
            }
          />
          <ErrorLine error={pagesQuery.error ?? monitors.error} className="mt-3" />
          {items.length > 0 && (
            <div className="mt-5">
              <FrameTabsAsChips label={t`Show`} value={filter} onChange={setFilter} items={filters} />
            </div>
          )}
          {loading ? (
            <div className="mt-7 flex flex-col gap-3">
              {[0, 1, 2].map((n) => (
                <Skeleton key={n} className="h-5 w-full" />
              ))}
            </div>
          ) : pages.length === 0 ? (
            <FlatEmpty>
              <p className="m-0">
                <Trans>Monitors set the state of a status page component. Create a status page first.</Trans>
              </p>
              {canManage && (
                <Button variant="outline" className={topButton} render={<Link to="/status" />}>
                  <Trans>Status pages</Trans>
                </Button>
              )}
            </FlatEmpty>
          ) : rows.length === 0 ? (
            <FlatEmpty>
              <p className="m-0">
                {items.length === 0 ? (
                  <Trans>
                    No monitors yet. Add one to let a check, a job or your monitoring set a component’s state.
                  </Trans>
                ) : (
                  <Trans>No monitor matches this filter.</Trans>
                )}
              </p>
              {canManage && items.length === 0 && (
                <Button variant="outline" className={topButton} onClick={() => setDialog({})}>
                  <Trans>New monitor</Trans>
                </Button>
              )}
            </FlatEmpty>
          ) : (
            <div ref={listRef} role="list" aria-label={t`Monitors`} className="mt-5">
              {rows.map((m) => {
                const page = byPage.get(m.page_id)
                return (
                  <FlatMonitorRow
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
              })}
            </div>
          )}
        </section>
        {dialogs}
      </StatusLayout>
    )
  }

  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="status-monitors">
      {!wide && monitorId ? (
        <div className="px-4 pt-3 sm:px-6">
          <PaneBack to="/monitors" label={t`Monitors`} always />
        </div>
      ) : (
        <StackTop
          title={<Trans>Monitors</Trans>}
          sub={<Trans>Checks that watch your products and set component states</Trans>}
          actions={
            canManage &&
            pages.length > 0 && (
              <Button
                className={look.btnInk}
                onClick={() => setDialog({})}
                title={t`Add monitor (${addKey})`}
                data-testid="status-add-monitor"
              >
                <Trans>New monitor</Trans>
              </Button>
            )
          }
        />
      )}
      <ErrorLine error={pagesQuery.error ?? monitors.error} className="m-6" />
      <div className="@container flex min-h-0 flex-1 overflow-hidden">
        {showList && (
          <div className="@container flex min-w-0 flex-1 flex-col overflow-y-auto">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-3 sm:px-6">
              <FrameTabsAsChips label={t`Show`} value={filter} onChange={setFilter} items={filters} />
              <span className="min-w-0 flex-1 basis-56 text-end text-caption text-faint phone:text-start">
                <Trans>Each monitor sets the state of one component while it is down.</Trans>
              </span>
            </div>
            {loading ? (
              <div className="flex flex-col gap-2 p-4">
                <Skeleton className="h-16 w-full rounded-xl" />
                <Skeleton className="h-16 w-full rounded-xl" />
              </div>
            ) : pages.length === 0 ? (
              <div className="m-6 flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-10 text-center">
                <p className="text-body text-muted-foreground">
                  <Trans>Monitors set the state of a status page component. Create a status page first.</Trans>
                </p>
                {canManage && (
                  <Button variant="outline" className={look.btnSm} render={<Link to="/status" />}>
                    <Trans>Status pages</Trans>
                  </Button>
                )}
              </div>
            ) : rows.length === 0 ? (
              <div className="m-6 flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-10 text-center">
                <p className="text-body text-muted-foreground">
                  {items.length === 0 ? (
                    <Trans>
                      No monitors yet. Add one to let a check, a job or your monitoring set a component’s state.
                    </Trans>
                  ) : (
                    <Trans>No monitor matches this filter.</Trans>
                  )}
                </p>
                {canManage && items.length === 0 && (
                  <Button className={look.btnInkSm} onClick={() => setDialog({})}>
                    <Trans>New monitor</Trans>
                  </Button>
                )}
              </div>
            ) : (
              <ul aria-label={t`Monitors`} className="border-t-0">
                {rows.map((m) => {
                  const page = byPage.get(m.page_id)
                  return (
                    <Row
                      key={m.id}
                      m={m}
                      page={page}
                      inbox={inboxOf(page)}
                      component={byComponent.get(m.component_id)}
                      incident={incidentOf(m)}
                      current={m.id === shown?.id}
                      canManage={canManage}
                      onIncident={() => startIncident(m)}
                    />
                  )
                })}
              </ul>
            )}
          </div>
        )}
        {showPane && (
          <div className={cn("min-w-0 overflow-y-auto bg-background", wide ? "w-[480px] shrink-0 border-s" : "flex-1")}>
            {shown ? (
              <Pane
                key={shown.id}
                m={shown}
                page={byPage.get(shown.page_id)}
                inbox={inboxOf(byPage.get(shown.page_id))}
                component={byComponent.get(shown.component_id)}
                incident={incidentOf(shown)}
                onEdit={() => setDialog({ edit: shown })}
                onIncident={() => startIncident(shown)}
                onToken={(monitor, rotated) => setToken({ monitor, rotated })}
              />
            ) : loading ? (
              <div className="p-5">
                <Skeleton className="h-24 w-full rounded-xl" />
              </div>
            ) : (
              <p className="p-10 text-center text-faint">
                <Trans>Pick a monitor to see what happened.</Trans>
              </p>
            )}
          </div>
        )}
      </div>
      {dialogs}
    </main>
  )
}

function FrameTabsAsChips<T extends string>({
  label,
  value,
  onChange,
  items,
}: {
  label: string
  value: T
  onChange: (v: T) => void
  items: { key: T; label: string; count: number }[]
}) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-1.5">
      {items.map((it) => (
        <button
          key={it.key}
          type="button"
          aria-pressed={it.key === value}
          onClick={() => onChange(it.key)}
          className={it.key === value ? look.chipOn : look.chip}
          data-testid={`status-monitors-${it.key}`}
        >
          {it.label}
          <span className="text-caption tabular-nums opacity-70">{it.count}</span>
        </button>
      ))}
    </div>
  )
}
