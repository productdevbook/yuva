import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, PencilIcon, SendIcon, Trash2Icon } from "lucide-react"
import { useRef, useState } from "react"
import { Link, useSearchParams } from "react-router"

import { ErrorLine, InfoSection, KV, toast, topButton, topInk, useConfirm } from "@/components/common"
import { INCIDENT_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { formatDateTime, formatDuration, formatMs, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { look } from "@/features/inbox/PageFrame"
import { IncidentForm } from "@/features/status/IncidentForm"
import { LinkedConversations } from "@/features/status/LinkedSection"
import { NotifyCheck } from "@/features/status/NotifyCheck"
import { NoticesSection } from "@/features/status/Notices"
import { Pill, ProductTag } from "@/features/status/PageFrame"
import {
  statusLink,
  useCancelNotice,
  useIncidentMutations,
  useMonitorSummary,
  useStatusComponents,
  useStatusMonitors,
  useStatusNotices,
} from "@/features/status/queries"
import {
  INCIDENT_STATES,
  isClosed,
  MAINTENANCE_STATES,
  nextStates,
  PlainText,
  useStatusText,
} from "@/features/status/text"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import type {
  StatusComponentStateValue,
  StatusIncident,
  StatusIncidentState,
  StatusIncidentUpdate,
  StatusMonitor,
  StatusPage,
} from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes, useMemberMap } from "@/lib/workspace"
import { Section, Card } from "@/features/settings/ui"

export function incidentTone(i: Pick<StatusIncident, "state">) {
  if (isClosed(i)) return "success" as const
  if (i.state === "scheduled") return "info" as const
  return "warning" as const
}

export function componentTone(c: StatusComponentStateValue) {
  return c === "operational" ? "success" : c === "major_outage" ? "danger" : c === "maintenance" ? "info" : "warning"
}

function updateDot(state: StatusIncidentState) {
  if (state === "resolved" || state === "completed") return "bg-success"
  if (state === "scheduled") return "bg-brand"
  return "bg-warning"
}

function useStageHints(): Record<StatusIncidentState, string> {
  const { t } = useLingui()
  return {
    investigating: t`Looking into it`,
    identified: t`Cause found`,
    monitoring: t`Fix is out`,
    resolved: t`All good`,
    scheduled: t`Planned`,
    in_progress: t`Work is going on`,
    completed: t`Done`,
  }
}

function Stage({
  i,
  value,
  onPick,
}: {
  i: StatusIncident
  value: StatusIncidentState
  onPick: (s: StatusIncidentState) => void
}) {
  const { t } = useLingui()
  const text = useStatusText()
  const hint = useStageHints()
  const stages = i.kind === "incident" ? INCIDENT_STATES : MAINTENANCE_STATES
  const allowed = nextStates(i)
  const at = stages.indexOf(value)
  return (
    <div
      role="radiogroup"
      aria-label={t`Stage`}
      className="grid grid-cols-2 gap-1.5 @5xl:grid-flow-col @5xl:auto-cols-fr"
      data-testid="status-stepper"
    >
      {stages.map((s, n) => {
        const done = n < at
        const here = n === at
        const can = allowed.includes(s)
        return (
          <button
            key={s}
            type="button"
            role="radio"
            aria-checked={here}
            disabled={!can}
            onClick={() => onPick(s)}
            className={cn(
              "flex items-center gap-2.5 rounded-lg border px-3 py-2 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
              here ? "border-foreground bg-background" : done ? "text-foreground" : "text-faint",
              can && !here && "hover:border-faint",
              !can && "cursor-default opacity-60"
            )}
            data-testid={`status-stage-${s}`}
          >
            <span
              className={cn(
                "grid size-5 shrink-0 place-items-center rounded-full text-[11px] font-semibold",
                here ? "bg-foreground text-background" : done ? "bg-success text-white" : "border"
              )}
              aria-hidden
            >
              {done ? <CheckIcon className="size-3" /> : n + 1}
            </span>
            <span className="flex min-w-0 flex-col leading-tight">
              <span className="truncate text-body font-medium">{text.incident[s]}</span>
              <span className="truncate text-caption opacity-75">{hint[s]}</span>
            </span>
          </button>
        )
      })}
    </div>
  )
}

function UpdateItem({ i, u, pageId }: { i: StatusIncident; u: StatusIncidentUpdate; pageId: string }) {
  const { t, i18n } = useLingui()
  const text = useStatusText()
  const members = useMemberMap()
  const m = useIncidentMutations(pageId)
  const [editing, setEditing] = useState(false)
  const [body, setBody] = useState(u.body)
  const author = u.author_member_id
    ? members.get(u.author_member_id)?.name
    : u.author_api_key_id
      ? t`API key`
      : undefined
  return (
    <li className="group/update flex gap-3.5" data-testid="status-update">
      <span className="flex shrink-0 flex-col items-center" aria-hidden>
        <span className={cn("mt-1.5 size-2.5 rounded-full", updateDot(u.state))} />
        <span className="w-px flex-1 bg-border" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5 pb-4.5">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-caption text-faint">
          <b className="text-body font-semibold text-foreground">{text.incident[u.state]}</b>
          <time dateTime={u.created_at} title={formatDateTime(u.created_at, i18n.locale)}>
            {formatShort(u.created_at, i18n.locale)}
          </time>
          {author ? (
            <span>· {author}</span>
          ) : (
            <Pill tone="warning">
              <Trans>automatic</Trans>
            </Pill>
          )}
          {!i.published_at && (
            <Pill tone="muted">
              <Trans>not public</Trans>
            </Pill>
          )}
          {u.edited_at && (
            <span title={formatDateTime(u.edited_at, i18n.locale)} data-testid="status-update-edited">
              · <Trans>edited</Trans>
            </span>
          )}
          {!editing && u.body && (
            <Button
              variant="ghost"
              size="icon-xs"
              className="ms-auto"
              aria-label={t`Edit this update`}
              onClick={() => {
                setBody(u.body)
                setEditing(true)
              }}
              data-testid="status-update-edit"
            >
              <PencilIcon />
            </Button>
          )}
        </div>
        {editing ? (
          <form
            className="flex flex-col gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              if (body.trim())
                m.editUpdate.mutate(
                  { id: i.id, updateId: u.id, body: body.trim() },
                  { onSuccess: () => setEditing(false) }
                )
            }}
          >
            <Textarea
              value={body}
              maxLength={5000}
              rows={3}
              autoFocus
              onChange={(e) => setBody(e.target.value)}
              aria-label={t`Update text`}
              data-testid="status-update-text"
            />
            <ErrorLine error={m.editUpdate.error} className="text-caption" />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(false)}>
                <Trans>Cancel</Trans>
              </Button>
              <Button
                type="submit"
                size="sm"
                disabled={!body.trim() || m.editUpdate.isPending}
                data-testid="status-update-save"
              >
                <Trans>Save</Trans>
              </Button>
            </div>
          </form>
        ) : (
          u.body && <PlainText text={u.body} className="text-body text-muted-foreground" />
        )}
      </div>
    </li>
  )
}

export function useIncidentMonitor(pageId: string, incident: StatusIncident): StatusMonitor | undefined {
  const monitors = useStatusMonitors(pageId).data ?? []
  const touched = new Set(incident.components.map((c) => c.component_id))
  return (
    monitors.find((x) => x.incident_id === incident.id) ??
    monitors.find((x) => (x.kind === "http" || x.kind === "tcp") && touched.has(x.component_id))
  )
}

function MonitorSeen({ pageId, m }: { pageId: string; m: StatusMonitor }) {
  const { t, i18n } = useLingui()
  const summary = useMonitorSummary(pageId, m.id, "24h", m.kind === "http" || m.kind === "tcp").data
  const buckets = summary?.buckets ?? []
  const size = Math.max(1, Math.ceil(buckets.length / 24))
  const groups = Array.from({ length: Math.ceil(buckets.length / size) }, (_, n) =>
    buckets.slice(n * size, n * size + size)
  ).map((g) => ({
    ms: Math.max(0, ...g.map((b) => b.p50_ms ?? 0)),
    failed: g.some((b) => b.failures > 0),
    checks: g.reduce((n, b) => n + b.checks, 0),
  }))
  const top = Math.max(1, ...groups.map((g) => g.ms))
  const goal = m.kind === "http" ? m.http?.url : m.tcp ? `${m.tcp.host}:${m.tcp.port}` : m.name
  const median = summary?.p50_ms === undefined ? undefined : formatMs(summary.p50_ms, i18n.locale)
  const name = m.name
  return (
    <Section title={<Trans>Seen by the monitor</Trans>}>
      <Card data-testid="status-incident-monitor">
        {groups.length > 0 && (
          <span className="flex h-15 items-end gap-0.5" title={t`Response time, last 24 hours`} aria-hidden>
            {groups.map((g, n) => (
              <span
                key={n}
                className={cn(
                  "min-w-px flex-1 rounded-t-[2px]",
                  g.checks === 0 ? "bg-muted" : g.failed ? "bg-warning" : "bg-success/70",
                  n !== groups.length - 1 && "opacity-70"
                )}
                style={{ height: g.checks === 0 ? 2 : Math.max(3, Math.round((60 * g.ms) / top)) }}
              />
            ))}
          </span>
        )}
        <span className="-mt-2 text-caption break-all text-faint">
          {goal}
          {median && (
            <>
              {" · "}
              <Trans>median {median}</Trans>
            </>
          )}
        </span>
        <Link to={`/monitors/${m.id}`} className="-mt-2 w-fit text-small text-brand underline-offset-4 hover:underline">
          <Trans>Open {name} in Monitors</Trans>
        </Link>
      </Card>
    </Section>
  )
}

function ComponentsBlock({ i, names, open }: { i: StatusIncident; names: Map<string, string>; open: boolean }) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const window =
    i.kind === "maintenance" && i.scheduled_start && i.scheduled_end
      ? `${formatDateTime(i.scheduled_start, i18n.locale)} – ${formatDateTime(i.scheduled_end, i18n.locale)}`
      : null
  return (
    <Section title={<Trans>Components</Trans>}>
      <Card data-testid="status-incident-components">
        {i.components.length === 0 ? (
          <p className="text-body text-faint">
            <Trans>No component is named.</Trans>
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {i.components.map((c) => {
              const state = open ? c.state : "operational"
              return (
                <li key={c.component_id} className="flex items-center gap-2" data-testid="status-incident-touched">
                  <span className="min-w-0 flex-1 truncate">
                    {names.get(c.component_id) ?? <Trans>Deleted component</Trans>}
                  </span>
                  <Pill tone={componentTone(state)}>{text.component[state]}</Pill>
                </li>
              )
            })}
          </ul>
        )}
        {window && <p className="text-small text-muted-foreground">{window}</p>}
      </Card>
    </Section>
  )
}

function IncidentInfo({
  page,
  i,
  names,
  open,
}: {
  page: StatusPage
  i: StatusIncident
  names: Map<string, string>
  open: boolean
}) {
  const { t, i18n } = useLingui()
  const text = useStatusText()
  const startedAt = i.started_at ?? i.published_at ?? i.created_at
  const tone = { success: "ok", info: undefined, warning: "warn" } as const
  const stateTone = (s: StatusComponentStateValue) =>
    s === "operational" ? "text-ok" : s === "major_outage" ? "text-bad" : s === "maintenance" ? undefined : "text-warn"
  return (
    <>
      <InfoSection title={text.kind[i.kind]}>
        <KV label={<Trans>Page</Trans>} value={page.name} to={statusLink(page.id)} />
        <KV label={<Trans>State</Trans>} value={text.incident[i.state]} tone={tone[incidentTone(i)]} />
        {i.kind === "incident" && <KV label={<Trans>Impact</Trans>} value={text.impact[i.impact]} />}
        <KV
          label={<Trans>Visibility</Trans>}
          value={i.published_at ? t`Public` : t`Draft`}
          tone={i.published_at ? undefined : "warn"}
        />
        {i.kind === "maintenance" && i.scheduled_start ? (
          <KV label={<Trans>Starts</Trans>} value={formatDateTime(i.scheduled_start, i18n.locale)} />
        ) : (
          <KV label={<Trans>Started</Trans>} value={formatDateTime(startedAt, i18n.locale)} />
        )}
        {i.resolved_at && <KV label={<Trans>Ended</Trans>} value={formatDateTime(i.resolved_at, i18n.locale)} />}
        <KV label={<Trans>Updates</Trans>} value={i.updates.length} />
      </InfoSection>
      {i.components.length > 0 && (
        <InfoSection title={<Trans>Components</Trans>}>
          {i.components.map((c) => {
            const state = open ? c.state : "operational"
            const name = names.get(c.component_id) ?? t`Deleted component`
            return (
              <div key={c.component_id} className="flex min-w-0 flex-col py-1 text-small leading-5">
                <span className="truncate text-muted-foreground" title={name}>
                  {name}
                </span>
                <span className={stateTone(state)}>{text.component[state]}</span>
              </div>
            )
          })}
        </InfoSection>
      )}
    </>
  )
}

export function IncidentDetail({
  page,
  i,
  justResolved,
  onClosed,
  onDeleted,
  layout,
}: {
  page: StatusPage
  i: StatusIncident
  justResolved: boolean
  onClosed: (id: string) => void
  onDeleted: () => void
  layout?: (parts: { actions: React.ReactNode; body: React.ReactNode; aside: React.ReactNode }) => React.ReactNode
}) {
  const { t, i18n } = useLingui()
  const pageId = page.id
  const inbox = (useInboxes().data ?? []).find((x) => x.id === page.inbox_id)
  const components = useStatusComponents(pageId).data ?? []
  const text = useStatusText()
  const m = useIncidentMutations(pageId)
  const [confirm, confirmDialog] = useConfirm()
  const [params, setParams] = useSearchParams()
  const replyId = params.get("reply") ?? ""
  const [editing, setEditing] = useState(false)
  const [state, setState] = useState<StatusIncidentState | null>(null)
  const [body, setBody] = useState("")
  const box = useRef<HTMLTextAreaElement>(null)
  const [notifyChoice, setNotifyChoice] = useState<boolean | null>(null)
  const [publishNotify, setPublishNotify] = useState<boolean | null>(null)
  const subscribers = !!page.email_subscribers.enabled
  const notices = useStatusNotices(pageId, i.id, subscribers).data ?? []
  const cancelNotice = useCancelNotice(pageId)
  const waiting = notices.find((n) => n.state === "pending")
  const monitor = useIncidentMonitor(pageId, i)
  const now = useNow(60_000)

  const maintenance = i.kind === "maintenance"
  const closing: StatusIncidentState = maintenance ? "completed" : "resolved"
  const open = !isClosed(i)
  const draft = !i.published_at
  const names = new Map(components.map((c) => [c.id, c.name]))
  const canResolve = open && !draft && !(maintenance && i.state === "scheduled")
  const recovered = open && !!monitor && monitor.incident_id === i.id && monitor.state === "up" && !monitor.paused

  const remove = () =>
    confirm({
      title: <Trans>Delete {i.title}?</Trans>,
      description: <Trans>Only a draft can be deleted. It was never shown on the page.</Trans>,
      confirm: <Trans>Delete draft</Trans>,
      run: () =>
        m.remove.mutate(i.id, {
          onSuccess: () => {
            toast(t`Draft deleted`)
            onDeleted()
          },
        }),
    })
  const focus = () => {
    box.current?.focus()
  }
  const startResolve = () => {
    setState(closing)
    setBody((b) =>
      b.trim()
        ? b
        : maintenance
          ? t`The maintenance is complete. Everything works as before.`
          : t`The problem is fixed. Sorry for the trouble.`
    )
    focus()
  }
  const notifyOnPublish = publishNotify ?? i.notify ?? true
  const publish = () => m.publish.mutate({ id: i.id, notify: subscribers ? notifyOnPublish : undefined })

  useHotkeys(
    {
      ...(open ? { [INCIDENT_KEYS.update]: focus } : {}),
      ...(canResolve ? { [INCIDENT_KEYS.resolve]: startResolve } : {}),
      ...(open ? { [INCIDENT_KEYS.edit]: () => setEditing(true) } : {}),
      ...(draft ? { [INCIDENT_KEYS.publish]: publish } : {}),
      ...(waiting ? { [INCIDENT_KEYS.undoNotice]: () => cancelNotice.mutate(waiting.id) } : {}),
      ...(draft ? { [INCIDENT_KEYS.remove]: remove } : {}),
    },
    !editing
  )

  const touchedIds = i.components.map((c) => c.component_id)
  const current = state && nextStates(i).includes(state) ? state : i.state
  const notifyUpdate = !draft && (notifyChoice ?? current !== i.state)
  const touched = i.components.map((c) => ({ ...c, name: names.get(c.component_id) }))
  const updateKey = keyLabel(INCIDENT_KEYS.update).join("")
  const resolveKey = keyLabel(INCIDENT_KEYS.resolve).join("")
  const publishKey = keyLabel(INCIDENT_KEYS.publish).join("")
  const editKey = keyLabel(INCIDENT_KEYS.edit).join("")
  const removeKey = keyLabel(INCIDENT_KEYS.remove).join("")
  const startedAt = i.started_at ?? i.published_at ?? i.created_at
  const started = new Intl.DateTimeFormat(i18n.locale, { timeStyle: "short" }).format(new Date(startedAt))
  const lasted = formatDuration(
    Math.max(60, Math.round(((i.resolved_at ? Date.parse(i.resolved_at) : now) - Date.parse(startedAt)) / 1000)),
    i18n.locale
  )
  const stage = text.incident[current]
  const closingNow = current === closing && open
  const endedAt = new Intl.DateTimeFormat(i18n.locale, { timeStyle: "short" }).format(
    new Date(i.resolved_at ?? i.updated_at)
  )
  const upSince = monitor
    ? new Intl.DateTimeFormat(i18n.locale, { timeStyle: "short" }).format(new Date(monitor.state_since))
    : ""
  const monitorName = monitor?.name ?? ""
  const updates = [...i.updates].sort((a, b) => b.created_at.localeCompare(a.created_at))
  const resolveLabel = maintenance ? t`Complete` : t`Resolve`
  const btn = layout ? topButton : look.btnSm
  const ink = layout ? topInk : look.btnInkSm
  const actions = (
    <>
      {open && (
        <Button
          variant="outline"
          className={btn}
          onClick={() => setEditing(true)}
          title={t`Edit details (${editKey})`}
          data-testid="status-incident-edit"
        >
          <Trans>Edit details</Trans>
        </Button>
      )}
      {draft && (
        <Button
          variant="outline"
          className={cn(btn, "text-destructive hover:text-destructive")}
          disabled={m.remove.isPending}
          onClick={remove}
          title={t`Delete draft (${removeKey})`}
          data-testid="status-incident-delete"
        >
          <Trash2Icon />
          <Trans>Delete draft</Trans>
        </Button>
      )}
      {canResolve && (
        <Button
          className={ink}
          onClick={startResolve}
          title={`${resolveLabel} (${resolveKey})`}
          data-testid="status-incident-resolve"
        >
          {resolveLabel}
        </Button>
      )}
    </>
  )
  const heading = (
    <header className={cn("flex flex-wrap items-start gap-x-3 gap-y-2", layout && "mt-3")}>
      <div className="flex min-w-0 flex-1 basis-64 flex-col gap-1.5">
        <div className="flex flex-wrap items-center gap-2">
          <ProductTag name={inbox?.name ?? page.name} color={inbox?.branding.color} />
          <span className="text-caption text-faint">{text.kind[i.kind]}</span>
          <Pill tone={incidentTone(i)} data-testid="status-incident-state">
            {text.incident[i.state]}
          </Pill>
          {!maintenance && i.impact !== "none" && (
            <Pill tone={i.impact === "minor" ? "warning" : "danger"}>{text.impact[i.impact]}</Pill>
          )}
          {draft && (
            <Pill tone="muted" data-testid="status-incident-draft-badge">
              <Trans>Draft</Trans>
            </Pill>
          )}
        </div>
        <h2
          className={cn(
            "font-semibold break-words",
            layout ? "m-0 text-page leading-[34px] text-balance" : "text-[19px] leading-[1.3] tracking-[-0.01em]"
          )}
          data-testid="status-incident-title-heading"
        >
          {i.title}
        </h2>
        <p className="text-caption text-faint">
          {maintenance && i.scheduled_start && i.scheduled_end ? (
            `${formatDateTime(i.scheduled_start, i18n.locale)} – ${formatDateTime(i.scheduled_end, i18n.locale)}`
          ) : (
            <Trans>
              started {started} · {lasted}
            </Trans>
          )}
        </p>
      </div>
    </header>
  )
  const main = (
    <>
      {editing && (
        <IncidentForm
          pages={[page]}
          pageId={pageId}
          kind={i.kind}
          edit={i}
          className="mt-0"
          onClose={() => setEditing(false)}
          onDone={() => setEditing(false)}
        />
      )}
      {justResolved && !open && (
        <div
          role="status"
          className="flex flex-col rounded-xl border border-success/40 bg-success/8 px-4 py-3 leading-[1.4]"
          data-testid="status-incident-resolved-note"
        >
          <span className="font-semibold text-success">
            {maintenance ? <Trans>Completed at {endedAt}</Trans> : <Trans>Resolved at {endedAt}</Trans>}
          </span>
          <span className="text-muted-foreground">
            <Trans>The components are back to operational. It moved to Past.</Trans>
          </span>
        </div>
      )}
      {recovered && monitor && (
        <div
          className="flex flex-wrap items-center gap-3 rounded-xl border border-success/40 bg-success/8 px-4 py-3"
          data-testid="status-incident-recovered"
        >
          <span className="flex min-w-48 flex-1 flex-col leading-[1.4]">
            <span className="font-semibold">
              <Trans>{monitorName} answers normally again</Trans>
            </span>
            <span className="text-muted-foreground">
              <Trans>Up since {upSince}. Post the all-clear when you are sure.</Trans>
            </span>
          </span>
          {canResolve && (
            <Button variant="outline" className={look.btnSm} onClick={startResolve}>
              {resolveLabel}
            </Button>
          )}
        </div>
      )}
      {draft && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-dashed px-4 py-3">
          <p className="min-w-60 flex-1 text-muted-foreground" data-testid="status-draft-note">
            <Trans>This is a draft. Only your team sees it, and it changes no component until you publish it.</Trans>
          </p>
          <NotifyCheck
            page={page}
            components={touchedIds}
            checked={notifyOnPublish}
            onChange={setPublishNotify}
            testId="status-incident-publish-notify"
          />
          <Button
            className={look.btnInkSm}
            disabled={m.publish.isPending}
            onClick={publish}
            title={t`Publish (${publishKey})`}
            data-testid="status-incident-publish-draft"
          >
            <SendIcon />
            <Trans>Publish</Trans>
          </Button>
        </div>
      )}
      <ErrorLine error={m.publish.error ?? m.remove.error} />
      {open && (
        <form
          className="flex flex-col gap-3 rounded-xl border bg-card p-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!body.trim()) return
            m.postUpdate.mutate(
              {
                id: i.id,
                body: body.trim(),
                state: current,
                ...(subscribers && !draft ? { notify: notifyUpdate } : {}),
              },
              {
                onSuccess: () => {
                  setBody("")
                  setState(null)
                  setNotifyChoice(null)
                  if (closingNow) onClosed(i.id)
                },
              }
            )
          }}
        >
          <h3 className="font-semibold">
            <Trans>New update · {stage}</Trans>
          </h3>
          <Stage i={i} value={current} onPick={setState} />
          <Textarea
            ref={box}
            value={body}
            maxLength={5000}
            rows={3}
            className="rounded-lg"
            onChange={(e) => setBody(e.target.value)}
            placeholder={t`What do customers need to know?`}
            aria-label={t`Update text`}
            data-testid="status-update-body"
          />
          {touched.length > 0 && (
            <p className="flex flex-wrap items-center gap-1.5">
              <span className="text-caption text-faint">
                <Trans>Affects</Trans>
              </span>
              {touched.map((c) => {
                const shown = closingNow ? "operational" : c.state
                return (
                  <Pill key={c.component_id} tone={componentTone(shown)}>
                    {c.name ?? <Trans>Deleted component</Trans>} · {text.component[shown]}
                  </Pill>
                )
              })}
              {closingNow && (
                <span className="text-caption text-faint">
                  <Trans>go back to operational when you post this</Trans>
                </span>
              )}
            </p>
          )}
          <div className="flex flex-wrap items-center gap-4 border-t pt-2.5">
            {!draft && (
              <NotifyCheck
                page={page}
                components={touchedIds}
                checked={notifyUpdate}
                onChange={setNotifyChoice}
                testId="status-update-notify"
              />
            )}
            <ErrorLine error={m.postUpdate.error} className="min-w-0 flex-1 text-caption" />
            <Button
              type="submit"
              className={cn(
                "ms-auto h-[34px] rounded-lg px-3.5 text-white disabled:text-white disabled:opacity-60 dark:text-black dark:disabled:text-black",
                closingNow
                  ? "bg-success hover:bg-success/90 disabled:bg-success"
                  : "bg-brand hover:bg-brand/90 disabled:bg-brand"
              )}
              disabled={!body.trim() || m.postUpdate.isPending}
              title={t`Post an update (${updateKey})`}
              data-testid="status-update-post"
            >
              {closingNow ? (
                maintenance ? (
                  <Trans>Complete maintenance</Trans>
                ) : (
                  <Trans>Resolve incident</Trans>
                )
              ) : (
                <Trans>Publish update</Trans>
              )}
            </Button>
          </div>
        </form>
      )}
      <section className="flex flex-col gap-3">
        <h3 className={look.over}>
          <Trans>Updates</Trans>
        </h3>
        <ul className="flex flex-col" data-testid="status-updates">
          {updates.map((u) => (
            <UpdateItem key={u.id} i={i} u={u} pageId={pageId} />
          ))}
        </ul>
      </section>
    </>
  )
  const blocks = (
    <>
      <ComponentsBlock i={i} names={names} open={open} />
      <LinkedConversations
        pageId={pageId}
        incident={i}
        replyId={replyId}
        onReply={(id) => setParams({ reply: id }, { replace: true })}
        onDismissReply={() => setParams({}, { replace: true })}
      />
      {notices.length > 0 && <NoticesSection notices={notices} pageId={pageId} />}
      {monitor && <MonitorSeen pageId={pageId} m={monitor} />}
    </>
  )
  const dialogs = confirmDialog
  if (layout)
    return (
      <>
        {layout({
          actions,
          body: (
            <div className="flex flex-col gap-4" data-testid="status-incident" key={i.id}>
              {heading}
              {main}
              {blocks}
            </div>
          ),
          aside: <IncidentInfo page={page} i={i} names={names} open={open} />,
        })}
        {dialogs}
      </>
    )
  return (
    <div className="flex min-h-full flex-col @3xl:flex-row" data-testid="status-incident" key={i.id}>
      <div className="flex min-w-0 flex-1 flex-col gap-4 p-6 pb-16 phone:px-4">
        <header className="flex flex-wrap items-start gap-x-3 gap-y-2">
          <div className="flex min-w-0 flex-1 basis-64 flex-col gap-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <ProductTag name={inbox?.name ?? page.name} color={inbox?.branding.color} />
              <span className="text-caption text-faint">{text.kind[i.kind]}</span>
              <Pill tone={incidentTone(i)} data-testid="status-incident-state">
                {text.incident[i.state]}
              </Pill>
              {!maintenance && i.impact !== "none" && (
                <Pill tone={i.impact === "minor" ? "warning" : "danger"}>{text.impact[i.impact]}</Pill>
              )}
              {draft && (
                <Pill tone="muted" data-testid="status-incident-draft-badge">
                  <Trans>Draft</Trans>
                </Pill>
              )}
            </div>
            <h2
              className={cn(
                "font-semibold break-words",
                layout ? "m-0 text-page leading-[34px] text-balance" : "text-[19px] leading-[1.3] tracking-[-0.01em]"
              )}
              data-testid="status-incident-title-heading"
            >
              {i.title}
            </h2>
            <p className="text-caption text-faint">
              {maintenance && i.scheduled_start && i.scheduled_end ? (
                `${formatDateTime(i.scheduled_start, i18n.locale)} – ${formatDateTime(i.scheduled_end, i18n.locale)}`
              ) : (
                <Trans>
                  started {started} · {lasted}
                </Trans>
              )}
            </p>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>
        </header>
        {main}
      </div>
      <aside className="flex w-full shrink-0 flex-col gap-4 border-t p-4 @3xl:w-[320px] @3xl:border-s @3xl:border-t-0">
        {blocks}
      </aside>
      {dialogs}
    </div>
  )
}
