import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, CalendarClockIcon, PencilIcon, PlusIcon, TargetIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { useSearchParams } from "react-router"

import { ErrorLine, Notice, Segmented, useConfirm } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { RowLabel } from "@/features/analytics/Breakdown"
import { RevenueAmounts, RevenueTiles } from "@/features/analytics/Events"
import { Change, TimeChart } from "@/features/analytics/Overview"
import {
  BarList,
  Panel,
  PanelProblem,
  QueryProblem,
  useCurrentProperty,
  usePropertyInbox,
} from "@/features/analytics/parts"
import {
  useAnalytics,
  useDeleteGoal,
  useGoalTotals,
  useGoals,
  useSaveGoal,
  useShare,
} from "@/features/analytics/queries"
import { grainFor, scopeOf, useView } from "@/features/analytics/state"
import { formatCompact, formatMetric, num, OTHER, useAnalyticsText, type NumMetric } from "@/features/analytics/text"
import { Bars, Field, ProductTag } from "@/features/settings/ui"
import {
  ApiError,
  type AnalyticsFilterDimension,
  type AnalyticsGoal,
  type AnalyticsGoalKind,
  type AnalyticsShareView,
} from "@/lib/api"
import { useOptionalSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function useGoalText() {
  const { t, i18n } = useLingui()
  return (g: AnalyticsGoal) => {
    const path = g.path_pattern ?? "/"
    switch (g.kind) {
      case "pageview":
        return t`Page view of ${path}`
      case "scroll": {
        const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" }).format((g.scroll_pct ?? 0) / 100)
        return t`Scrolled to ${pct} on ${path}`
      }
      case "event": {
        const event = g.event_name ?? ""
        const key = g.prop_key
        const value = g.prop_value ?? ""
        return key ? t`Event ${event} with ${key} = ${value}` : t`Event ${event}`
      }
    }
  }
}

function useEventNames(propertyId: string, enabled: boolean) {
  const { view } = useView()
  const q = useAnalytics(
    propertyId,
    { metrics: ["events"], dimensions: ["event"], range: view.range, limit: 100 },
    enabled
  )
  return (q.data?.rows ?? []).map((r) => r.dimensions[0] ?? "").filter((v) => v && v !== OTHER)
}

function GoalDialog({ goal, onClose }: { goal?: AnalyticsGoal; onClose: () => void }) {
  const { t } = useLingui()
  const property = useCurrentProperty()
  const save = useSaveGoal(property.id, goal?.id)
  const [name, setName] = useState(goal?.name ?? "")
  const [kind, setKind] = useState<AnalyticsGoalKind>(goal?.kind ?? "pageview")
  const [path, setPath] = useState(goal?.path_pattern ?? "")
  const [event, setEvent] = useState(goal?.event_name ?? "")
  const [propKey, setPropKey] = useState(goal?.prop_key ?? "")
  const [propValue, setPropValue] = useState(goal?.prop_value ?? "")
  const [pct, setPct] = useState(String(goal?.scroll_pct ?? 75))
  const events = useEventNames(property.id, kind === "event")
  const scroll = Number(pct)
  const pathOk = path.trim().startsWith("/")
  const valid =
    !!name.trim() &&
    (kind === "event"
      ? !!event.trim()
      : pathOk && (kind === "pageview" || (Number.isInteger(scroll) && scroll >= 1 && scroll <= 100)))
  const taken = save.error instanceof ApiError && save.error.code === "goal_taken"
  const submit = () =>
    save.mutate(
      kind === "event"
        ? {
            name: name.trim(),
            kind,
            event_name: event.trim(),
            ...(propKey.trim() ? { prop_key: propKey.trim(), prop_value: propValue } : {}),
          }
        : kind === "scroll"
          ? { name: name.trim(), kind, path_pattern: path.trim(), scroll_pct: scroll }
          : { name: name.trim(), kind, path_pattern: path.trim() },
      { onSuccess: onClose }
    )
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="goal-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (valid) submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{goal ? <Trans>Edit goal</Trans> : <Trans>New goal</Trans>}</DialogTitle>
          </DialogHeader>
          <Field
            label={<Trans>Name</Trans>}
            htmlFor="goal-name"
            error={taken ? <Trans>This property already has a goal with this name.</Trans> : undefined}
          >
            <Input
              id="goal-name"
              required
              maxLength={120}
              value={name}
              placeholder={t`Signed up`}
              onChange={(e) => setName(e.target.value)}
              data-testid="goal-name"
            />
          </Field>
          <Field label={<Trans>Reached when a visitor</Trans>}>
            <Segmented
              value={kind}
              onChange={setKind}
              label={t`Kind of goal`}
              className="w-fit max-w-full"
              items={[
                { value: "pageview", label: t`Views a page`, testId: "goal-kind-pageview" },
                { value: "event", label: t`Sends an event`, testId: "goal-kind-event" },
                { value: "scroll", label: t`Scrolls`, testId: "goal-kind-scroll" },
              ]}
            />
          </Field>
          {kind === "event" ? (
            <>
              <Field
                label={<Trans>Event</Trans>}
                htmlFor="goal-event"
                hint={<Trans>The name given to track(), or outbound_click, file_download, not_found, chat_open.</Trans>}
              >
                <Input
                  id="goal-event"
                  required
                  dir="ltr"
                  list="goal-events"
                  maxLength={120}
                  value={event}
                  placeholder="Signup"
                  onChange={(e) => setEvent(e.target.value)}
                  data-testid="goal-event"
                />
                <datalist id="goal-events">
                  {events.map((n) => (
                    <option key={n} value={n} />
                  ))}
                </datalist>
              </Field>
              <div className="grid gap-3 @xl:grid-cols-2">
                <Field label={<Trans>Only with prop</Trans>} htmlFor="goal-prop-key">
                  <Input
                    id="goal-prop-key"
                    dir="ltr"
                    maxLength={300}
                    value={propKey}
                    placeholder="plan"
                    onChange={(e) => setPropKey(e.target.value)}
                    data-testid="goal-prop-key"
                  />
                </Field>
                <Field label={<Trans>equal to</Trans>} htmlFor="goal-prop-value">
                  <Input
                    id="goal-prop-value"
                    dir="auto"
                    maxLength={2000}
                    value={propValue}
                    disabled={!propKey.trim()}
                    placeholder="pro"
                    onChange={(e) => setPropValue(e.target.value)}
                    data-testid="goal-prop-value"
                  />
                </Field>
              </div>
              <p className="-mt-3 text-small text-faint">
                <Trans>The goal reports the revenue its events carry, per currency.</Trans>
              </p>
            </>
          ) : (
            <div className={cn("grid gap-3", kind === "scroll" && "grid-cols-[1fr_7rem]")}>
              <Field
                label={<Trans>Page</Trans>}
                htmlFor="goal-path"
                hint={<Trans>A path. * stands for one part of it, ** for any number of them.</Trans>}
              >
                <Input
                  id="goal-path"
                  required
                  dir="ltr"
                  maxLength={300}
                  value={path}
                  placeholder={kind === "scroll" ? "/blog/**" : "/thanks"}
                  onChange={(e) => setPath(e.target.value)}
                  aria-invalid={!!path && !pathOk}
                  data-testid="goal-path"
                />
              </Field>
              {kind === "scroll" && (
                <Field label={<Trans>Depth in %</Trans>} htmlFor="goal-pct">
                  <Input
                    id="goal-pct"
                    type="number"
                    min={1}
                    max={100}
                    required
                    value={pct}
                    onChange={(e) => setPct(e.target.value)}
                    data-testid="goal-pct"
                  />
                </Field>
              )}
            </div>
          )}
          <p className="text-small text-faint">
            {goal ? (
              <Trans>A changed goal is counted again over the days the detailed records are kept.</Trans>
            ) : (
              <Trans>A new goal also counts the days the detailed records are kept, not only from today.</Trans>
            )}
          </p>
          {!taken && <ErrorLine error={save.error} />}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!valid || save.isPending} data-testid="goal-save">
              {goal ? <Trans>Save</Trans> : <Trans>Make goal</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

const GOAL_DIMS: AnalyticsFilterDimension[] = ["channel", "source", "entry_page", "country", "device"]

const GOAL_DIM_VIEW: Record<string, AnalyticsShareView> = {
  channel: "sources",
  source: "sources",
  entry_page: "pages",
  country: "locations",
  device: "devices",
}

function GoalBreakdown({ goal }: { goal: AnalyticsGoal }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const { view } = useView()
  const share = useShare()
  const dims = GOAL_DIMS.filter((d) => !share || share.views.includes(GOAL_DIM_VIEW[d]))
  const [picked, setBy] = useState<AnalyticsFilterDimension>("channel")
  const by = dims.includes(picked) ? picked : (dims[0] ?? "channel")
  const q = useAnalytics(
    property.id,
    {
      metrics: ["conversions", "conversion_rate"],
      dimensions: [by],
      range: view.range,
      ...scopeOf(view),
      goal_id: goal.id,
      order_by: [{ metric: "conversions", dir: "desc" }],
      limit: 10,
    },
    dims.length > 0
  )
  const rows = (q.data?.rows ?? []).filter((r) => (num(r.metrics, "conversions") ?? 0) > 0)
  if (dims.length === 0) return null
  return (
    <Panel
      title={<Trans>Where conversions come from</Trans>}
      testId="goal-breakdown"
      action={
        <div className="-mx-1 max-w-[calc(100%+0.5rem)] overflow-x-auto px-1 py-1">
          <Segmented
            value={by}
            onChange={setBy}
            label={t`Breakdown`}
            itemClassName="h-6 px-2 text-small"
            items={dims.map((d) => ({ value: d, label: text.dimension[d] }))}
          />
        </div>
      }
    >
      {q.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : q.error ? (
        <PanelProblem error={q.error} />
      ) : rows.length === 0 ? (
        <p className="px-2 py-6 text-center text-body text-faint">
          <Trans>No conversions in this range.</Trans>
        </p>
      ) : (
        <BarList
          head={[text.dimension[by], text.metric.conversions]}
          items={rows.map((r, i) => {
            const value = r.dimensions[0] ?? ""
            const c = num(r.metrics, "conversions") ?? 0
            const rate = formatMetric("conversion_rate", num(r.metrics, "conversion_rate"), i18n.locale)
            return {
              key: `${value}${i}`,
              label: <RowLabel dim={by} row={r} value={value} text={text} />,
              value: c,
              display: `${formatCompact(c, i18n.locale)} · ${rate}`,
            }
          })}
        />
      )}
    </Panel>
  )
}

const GOAL_KPIS: NumMetric[] = ["conversions", "conversion_rate", "completions"]

function GoalDetail({ goal, onBack }: { goal: AnalyticsGoal; onBack: () => void }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const describe = useGoalText()
  const canManage = !!useOptionalSession()?.canManage
  const property = useCurrentProperty()
  const { view } = useView()
  const remove = useDeleteGoal(property.id)
  const [ask, confirm] = useConfirm()
  const [editing, setEditing] = useState(false)
  const q = useAnalytics(property.id, {
    metrics: [...GOAL_KPIS, "revenue"],
    range: view.range,
    ...scopeOf(view),
    compare: view.compare,
    goal_id: goal.id,
  })
  const totals = q.data?.totals
  const before = q.data?.comparison?.totals
  const from = q.data?.meta.from
  const since = formatDate(`${goal.counts_from}T12:00:00Z`, i18n.locale)
  const inUse = remove.error instanceof ApiError && remove.error.code === "goal_in_use"
  return (
    <section className="flex flex-col gap-4" data-testid="analytics-goal">
      <div className="flex flex-col items-start gap-1">
        <Button variant="ghost" size="sm" className="-ms-2" onClick={onBack}>
          <ArrowLeftIcon className="rtl:rotate-180" />
          <Trans>All goals</Trans>
        </Button>
        <div className="flex w-full flex-wrap items-start gap-2">
          <div className="min-w-0 flex-1">
            <h2 className="text-title break-words" data-testid="goal-name-title">
              {goal.name}
            </h2>
            <p className="text-body break-words text-muted-foreground" dir="auto">
              {describe(goal)}
            </p>
          </div>
          {canManage && (
            <div className="flex shrink-0 items-center gap-1">
              <Button variant="outline" size="sm" onClick={() => setEditing(true)} data-testid="goal-edit">
                <PencilIcon />
                <Trans>Edit</Trans>
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t`Delete goal`}
                title={t`Delete goal`}
                onClick={() =>
                  ask({
                    title: <Trans>Delete the goal “{goal.name}”?</Trans>,
                    description: <Trans>Its numbers are deleted too. This cannot be undone.</Trans>,
                    confirm: <Trans>Delete goal</Trans>,
                    run: () => remove.mutate(goal.id, { onSuccess: onBack }),
                  })
                }
                data-testid="goal-delete"
              >
                <Trash2Icon />
              </Button>
            </div>
          )}
        </div>
      </div>
      {inUse ? (
        <Notice tone="warning" icon={TargetIcon} data-testid="goal-in-use">
          <Trans>This goal is a step of a funnel. Take it out of the funnel, or delete the funnel, first.</Trans>
        </Notice>
      ) : (
        <ErrorLine error={remove.error} />
      )}
      {from && from < goal.counts_from && (
        <Notice icon={CalendarClockIcon} data-testid="goal-counts-from">
          <Trans>This goal has numbers from {since} on. Days before that show none.</Trans>
        </Notice>
      )}
      <QueryProblem error={q.error} />
      <div className="grid grid-cols-3 gap-2 @max-2xl:grid-cols-2" data-testid="goal-kpis">
        {GOAL_KPIS.map((m) => (
          <div
            key={m}
            className="flex min-w-0 flex-col rounded-[14px] border bg-card px-4 py-3 @max-2xl:px-3"
            title={text.metricHelp[m]}
            data-testid={`kpi-${m}`}
          >
            <span className="max-w-full truncate text-small text-faint">{text.metric[m]}</span>
            {q.isPending ? (
              <Skeleton className="mt-1 h-7 w-16" />
            ) : (
              <span className="flex flex-wrap items-baseline gap-x-2">
                <b className="text-page tabular-nums">{formatMetric(m, num(totals, m), i18n.locale)}</b>
                {view.compare && <Change now={num(totals, m)} before={num(before, m)} metric={m} />}
              </span>
            )}
          </div>
        ))}
        <RevenueTiles revenue={totals?.revenue} />
      </div>
      <TimeChart metric="conversions" goalId={goal.id} />
      <GoalBreakdown goal={goal} />
      {editing && <GoalDialog goal={goal} onClose={() => setEditing(false)} />}
      {confirm}
    </section>
  )
}

function GoalSpark({ goal, color }: { goal: AnalyticsGoal; color: string }) {
  const { t } = useLingui()
  const property = useCurrentProperty()
  const { view } = useView()
  const q = useAnalytics(property.id, {
    metrics: ["conversions"],
    dimensions: [grainFor(view.range, "conversions")],
    range: view.range,
    ...scopeOf(view),
    goal_id: goal.id,
  })
  if (!q.data) return <Skeleton className="h-10 w-[120px]" />
  const values = q.data.rows.map((r) => num(r.metrics, "conversions") ?? 0)
  return (
    <Bars
      values={values.slice(-12)}
      color={color}
      label={t`Conversions over time`}
      className="h-10 w-[120px] shrink-0 gap-[3px]"
      fade={0.55}
    />
  )
}

function GoalRate({ value }: { value: string }) {
  return <Trans>{value} of visitors</Trans>
}

function GoalList({
  goals,
  onPick,
  onNew,
}: {
  goals: AnalyticsGoal[]
  onPick: (id: string) => void
  onNew?: () => void
}) {
  const { i18n } = useLingui()
  const text = useAnalyticsText()
  const describe = useGoalText()
  const property = useCurrentProperty()
  const inbox = usePropertyInbox(property.inbox_id)
  const color = inbox?.branding.color || "var(--brand)"
  const { view } = useView()
  const totals = useGoalTotals(property.id, goals, { range: view.range, ...scopeOf(view) })
  return (
    <>
      <QueryProblem error={totals.error} />
      <ul className="grid grid-cols-3 gap-4 @max-5xl:grid-cols-2 @max-2xl:grid-cols-1" data-testid="goals-table">
        {goals.map((g) => {
          const m = totals.data?.get(g.id)
          return (
            <li key={g.id}>
              <button
                type="button"
                className="flex h-full w-full min-w-0 flex-col gap-3 rounded-[14px] border bg-card p-[18px] text-start outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => onPick(g.id)}
                data-testid="goal-row"
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0 flex-1 truncate font-semibold">{g.name}</span>
                  {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
                </span>
                <span className="truncate font-mono text-caption text-faint" dir="auto">
                  {describe(g)}
                </span>
                <span className="mt-auto flex items-end justify-between gap-3">
                  <span className="flex min-w-0 flex-col leading-[1.2]">
                    <b
                      className="text-[26px] font-semibold tracking-[-0.02em] tabular-nums"
                      title={text.metricHelp.conversions}
                    >
                      {m ? (
                        formatMetric("conversions", num(m, "conversions"), i18n.locale)
                      ) : (
                        <Dash pending={totals.isPending} />
                      )}
                    </b>
                    <span className="mt-1 truncate text-caption text-faint" title={text.metricHelp.conversion_rate}>
                      {m && (
                        <GoalRate value={formatMetric("conversion_rate", num(m, "conversion_rate"), i18n.locale)} />
                      )}
                    </span>
                    {m?.revenue?.length ? (
                      <span className="truncate text-caption text-muted-foreground tabular-nums">
                        <RevenueAmounts revenue={m.revenue} />
                      </span>
                    ) : null}
                  </span>
                  <GoalSpark goal={g} color={color} />
                </span>
              </button>
            </li>
          )
        })}
        {onNew && (
          <li>
            <button
              type="button"
              onClick={onNew}
              className="flex h-full min-h-36 w-full flex-col items-center justify-center gap-1.5 rounded-[14px] border border-dashed border-input p-[18px] text-center text-muted-foreground outline-none transition-colors hover:border-faint hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
              data-testid="goal-new"
            >
              <PlusIcon className="size-5" />
              <span>
                <Trans>New goal</Trans>
              </span>
              <span className="text-caption text-faint">
                <Trans>a page visit or an event from your site or app</Trans>
              </span>
            </button>
          </li>
        )}
      </ul>
    </>
  )
}

function Dash({ pending }: { pending: boolean }) {
  return pending ? <Skeleton className="ms-auto h-4 w-10" /> : <span className="text-faint">–</span>
}

export function GoalsView() {
  const canManage = !!useOptionalSession()?.canManage
  const share = useShare()
  const property = useCurrentProperty()
  const goals = useGoals(property.id)
  const [params, setParams] = useSearchParams()
  const creating = params.get("goal") === "new"
  const list = goals.data ?? []
  const picked = list.find((g) => g.id === params.get("goal"))
  const setCreating = (on: boolean) => pick(on ? "new" : null)
  const pick = (id: string | null) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      if (id) next.set("goal", id)
      else next.delete("goal")
      return next
    })
  if (goals.isPending) return <Skeleton className="h-64 rounded-[14px]" />
  if (goals.error) return <ErrorLine error={goals.error} />
  if (picked) return <GoalDetail key={picked.id} goal={picked} onBack={() => pick(null)} />
  return (
    <section className="@container flex flex-col gap-3" data-testid="analytics-goals">
      {list.length === 0 ? (
        <div
          className="flex flex-col items-center gap-2 rounded-[14px] border border-dashed px-4 py-10 text-center"
          data-testid="goals-empty"
        >
          <TargetIcon className="size-6 text-faint" />
          <p className="font-medium">
            <Trans>No goals yet</Trans>
          </p>
          <p className="max-w-md text-body text-faint">
            {canManage ? (
              <Trans>
                A goal counts the visitors who view a page, send an event or scroll far enough, and the revenue they
                bring.
              </Trans>
            ) : (
              !share && <Trans>Owners and admins make goals. They show up here with their conversions.</Trans>
            )}
          </p>
          {canManage && (
            <Button className="mt-2" onClick={() => setCreating(true)} data-testid="goal-new">
              <PlusIcon />
              <Trans>New goal</Trans>
            </Button>
          )}
        </div>
      ) : (
        <GoalList goals={list} onPick={pick} onNew={canManage ? () => setCreating(true) : undefined} />
      )}
      {creating && <GoalDialog onClose={() => setCreating(false)} />}
    </section>
  )
}
