import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  FilterIcon,
  HistoryIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useState } from "react"
import { Link, useSearchParams } from "react-router"

import { ErrorLine, Notice, useConfirm } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useGoalText } from "@/features/analytics/Goals"
import { ConsentedOnly, offersPersistent, Panel, useCurrentProperty } from "@/features/analytics/parts"
import {
  useDeleteFunnel,
  useFunnelReport,
  useFunnels,
  useGoals,
  useSaveFunnel,
  type FunnelRange,
} from "@/features/analytics/queries"
import { todayIso, useView } from "@/features/analytics/state"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import {
  ApiError,
  type AnalyticsFunnel,
  type AnalyticsFunnelReport,
  type AnalyticsGoal,
  type AnalyticsRange,
} from "@/lib/api"
import { useSession } from "@/lib/session"

const FUNNEL_PERIODS = ["today", "yesterday", "7d", "28d", "30d", "90d", "month", "last_month"] as const

function funnelRange(r: AnalyticsRange): FunnelRange | null {
  if (r.period) {
    return (FUNNEL_PERIODS as readonly string[]).includes(r.period)
      ? { period: r.period as (typeof FUNNEL_PERIODS)[number] }
      : null
  }
  return r.from && r.to ? { from: r.from, to: r.to } : null
}

function FunnelDialog({
  funnel,
  goals,
  onClose,
  onSaved,
}: {
  funnel?: AnalyticsFunnel
  goals: AnalyticsGoal[]
  onClose: () => void
  onSaved: (f: AnalyticsFunnel) => void
}) {
  const { t } = useLingui()
  const property = useCurrentProperty()
  const describe = useGoalText()
  const save = useSaveFunnel(property.id, funnel?.id)
  const [name, setName] = useState(funnel?.name ?? "")
  const [steps, setSteps] = useState<string[]>(
    funnel?.steps ?? [goals[0]?.id ?? "", goals[1]?.id ?? ""].filter(Boolean)
  )
  const known = new Set(goals.map((g) => g.id))
  const valid = !!name.trim() && steps.length >= 2 && steps.length <= 8 && steps.every((s) => known.has(s))
  const taken = save.error instanceof ApiError && save.error.code === "funnel_taken"
  const options = goals.map((g) => [g.id, g.name] as const)
  const move = (i: number, d: -1 | 1) =>
    setSteps((s) => {
      const next = [...s]
      ;[next[i], next[i + d]] = [next[i + d], next[i]]
      return next
    })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="funnel-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (valid) save.mutate({ name: name.trim(), steps }, { onSuccess: onSaved })
          }}
        >
          <DialogHeader>
            <DialogTitle>{funnel ? <Trans>Edit funnel</Trans> : <Trans>New funnel</Trans>}</DialogTitle>
          </DialogHeader>
          <Field
            label={<Trans>Name</Trans>}
            htmlFor="funnel-name"
            error={taken ? <Trans>This property already has a funnel with this name.</Trans> : undefined}
          >
            <Input
              id="funnel-name"
              required
              maxLength={120}
              value={name}
              placeholder={t`From pricing to signup`}
              onChange={(e) => setName(e.target.value)}
              data-testid="funnel-name"
            />
          </Field>
          <Field
            label={<Trans>Steps</Trans>}
            hint={<Trans>Two to eight goals, in the order visitors reach them within one visit.</Trans>}
          >
            <ol className="flex flex-col gap-2">
              {steps.map((id, i) => {
                const g = goals.find((x) => x.id === id)
                return (
                  <li key={i} className="flex items-center gap-2" data-testid="funnel-step-edit">
                    <span className="grid size-6 shrink-0 place-items-center rounded-full bg-muted text-caption font-medium tabular-nums">
                      {i + 1}
                    </span>
                    <div className="flex min-w-0 flex-1 flex-col">
                      <ChoiceSelect
                        value={id}
                        onChange={(v) => setSteps((s) => s.map((x, j) => (j === i ? v : x)))}
                        options={options}
                        label={t`Step ${i + 1}`}
                        placeholder={t`Pick a goal`}
                        className="w-full"
                      />
                      {g && (
                        <span className="mt-0.5 truncate text-caption text-faint" dir="auto">
                          {describe(g)}
                        </span>
                      )}
                    </div>
                    <span className="flex shrink-0 items-center">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t`Move up`}
                        disabled={i === 0}
                        onClick={() => move(i, -1)}
                      >
                        <ArrowUpIcon />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t`Move down`}
                        disabled={i === steps.length - 1}
                        onClick={() => move(i, 1)}
                      >
                        <ArrowDownIcon />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t`Remove step`}
                        disabled={steps.length <= 2}
                        onClick={() => setSteps((s) => s.filter((_, j) => j !== i))}
                      >
                        <XIcon />
                      </Button>
                    </span>
                  </li>
                )
              })}
            </ol>
            {steps.length < 8 && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-fit"
                onClick={() => setSteps((s) => [...s, goals.find((g) => !s.includes(g.id))?.id ?? goals[0].id])}
                data-testid="funnel-add-step"
              >
                <PlusIcon />
                <Trans>Add a step</Trans>
              </Button>
            )}
          </Field>
          {!taken && <ErrorLine error={save.error} />}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!valid || save.isPending} data-testid="funnel-save">
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function FunnelProblem({ error, tooLong }: { error: unknown; tooLong: boolean }) {
  const { i18n } = useLingui()
  const { setRange } = useView()
  if (tooLong || (error instanceof ApiError && error.code === "range_too_large")) {
    return (
      <Notice tone="warning" icon={HistoryIcon} data-testid="funnel-range-too-large">
        <p>
          <Trans>
            A funnel follows each visit step by step, so it covers at most 90 days at once, or 31 days for a property
            with more than 10 million events a month. Pick a shorter range.
          </Trans>
        </p>
        <Button variant="outline" size="sm" className="w-fit" onClick={() => setRange({ period: "30d" })}>
          <Trans>Show the last 30 days</Trans>
        </Button>
      </Notice>
    )
  }
  if (error instanceof ApiError && error.code === "needs_raw_window") {
    const from = (error.body as { raw_window_from?: string } | undefined)?.raw_window_from
    const day = from ? formatDate(`${from}T12:00:00Z`, i18n.locale) : ""
    return (
      <Notice tone="warning" icon={HistoryIcon} data-testid="funnel-needs-raw-window">
        <p>
          {from ? (
            <Trans>
              A funnel needs each visit’s steps, which are kept since {day}. Earlier days only have totals, so they
              cannot show a funnel.
            </Trans>
          ) : (
            <Trans>A funnel needs each visit’s steps, which are no longer kept for this range.</Trans>
          )}
        </p>
        {from && (
          <Button
            variant="outline"
            size="sm"
            className="w-fit"
            onClick={() => setRange({ from, to: todayIso() })}
            data-testid="narrow-range"
          >
            <Trans>Show from {day}</Trans>
          </Button>
        )}
      </Notice>
    )
  }
  return <ErrorLine error={error} />
}

function Report({ report, consented }: { report: AnalyticsFunnelReport; consented: boolean }) {
  const { i18n } = useLingui()
  const n = new Intl.NumberFormat(i18n.locale)
  const pct = (v: number) =>
    new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 1 }).format(v / 100)
  const first = report.steps[0]?.visitors ?? 0
  const last = report.steps[report.steps.length - 1]
  const overall = pct(last?.conversion_rate ?? 0)
  const from = formatDate(`${report.from}T12:00:00Z`, i18n.locale)
  const to = formatDate(`${report.to}T12:00:00Z`, i18n.locale)
  if (first === 0) {
    return (
      <p className="px-2 py-6 text-center text-body text-faint" data-testid="funnel-empty">
        <Trans>Nobody reached the first step in this range.</Trans>
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-1" data-testid="funnel-report">
      <ol className="flex flex-col">
        {report.steps.map((s, i) => {
          const visitors = n.format(s.visitors)
          const lost = n.format(s.dropoff)
          const lostPct = pct(100 - s.step_rate)
          const consentedCount = n.format(s.consented_visitors)
          return (
            <li key={s.goal_id} className="flex flex-col" data-testid="funnel-step">
              {i > 0 && (
                <div className="flex items-center gap-2 py-1.5 ps-9 text-small text-faint" data-testid="funnel-dropoff">
                  <ArrowDownIcon className="size-3.5 shrink-0" />
                  <span className="tabular-nums">
                    <Trans>
                      {lost} left ({lostPct})
                    </Trans>
                  </span>
                </div>
              )}
              <div className="flex items-center gap-3">
                <span className="grid size-6 shrink-0 place-items-center rounded-full bg-muted text-caption font-medium tabular-nums">
                  {i + 1}
                </span>
                <div className="relative flex h-10 min-w-0 flex-1 items-center rounded-md px-2.5">
                  <span
                    aria-hidden
                    className="absolute inset-y-0 start-0 rounded-md bg-brand-wash"
                    style={{ width: `${Math.max(1.5, s.conversion_rate)}%` }}
                  />
                  <span className="relative min-w-0 flex-1 truncate text-body font-medium">{s.name}</span>
                  <span className="relative shrink-0 text-body tabular-nums">
                    {visitors}
                    <span className="ms-2 text-small text-faint">{pct(s.conversion_rate)}</span>
                  </span>
                </div>
              </div>
              {consented && (
                <p className="ps-9 pt-1 text-caption text-faint tabular-nums" data-testid="funnel-consented">
                  <Trans>{consentedCount} consented, across visits and days</Trans>
                </p>
              )}
            </li>
          )
        })}
      </ol>
      <p className="mt-3 border-t px-1 pt-3 text-small text-muted-foreground" data-testid="funnel-overall">
        <Trans>
          {overall} of the visitors who reached the first step also reached the last, {from} – {to}.
        </Trans>
      </p>
      {consented && (
        <div className="flex flex-wrap items-center gap-2 px-1 text-caption text-faint">
          <ConsentedOnly />
          <span>
            <Trans>
              The consented counts follow visitors with a persistent id from one visit and day to the next, so a step
              reached on a later day still counts.
            </Trans>
          </span>
        </div>
      )}
    </div>
  )
}

function FunnelPanel({ funnel, goals }: { funnel: AnalyticsFunnel; goals: AnalyticsGoal[] }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const property = useCurrentProperty()
  const { view } = useView()
  const range = funnelRange(view.range)
  const report = useFunnelReport(property.id, funnel.id, range)
  const remove = useDeleteFunnel(property.id)
  const [ask, confirm] = useConfirm()
  const [editing, setEditing] = useState(false)
  return (
    <Panel
      title={funnel.name}
      testId="funnel-panel"
      action={
        canManage && (
          <span className="flex items-center gap-1">
            <Button variant="ghost" size="sm" onClick={() => setEditing(true)} data-testid="funnel-edit">
              <PencilIcon />
              <Trans>Edit</Trans>
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t`Delete funnel`}
              title={t`Delete funnel`}
              onClick={() =>
                ask({
                  title: <Trans>Delete the funnel “{funnel.name}”?</Trans>,
                  description: <Trans>Its goals stay.</Trans>,
                  confirm: <Trans>Delete funnel</Trans>,
                  run: () => remove.mutate(funnel.id),
                })
              }
              data-testid="funnel-delete"
            >
              <Trash2Icon />
            </Button>
          </span>
        )
      }
      footer={
        <p className="text-caption text-faint">
          <Trans>
            Counts the visitors who reached each step after the ones before it, within one visit on one day. Numbers are
            at most ten minutes old.
          </Trans>
        </p>
      }
    >
      <ErrorLine error={remove.error} className="px-1 pb-2" />
      {!range || report.error ? (
        <FunnelProblem error={report.error} tooLong={!range} />
      ) : report.isPending ? (
        <Skeleton className="h-48 w-full" />
      ) : report.data ? (
        <Report report={report.data} consented={offersPersistent(property)} />
      ) : null}
      {editing && (
        <FunnelDialog
          funnel={funnel}
          goals={goals}
          onClose={() => setEditing(false)}
          onSaved={() => setEditing(false)}
        />
      )}
      {confirm}
    </Panel>
  )
}

export function FunnelsView() {
  const { t } = useLingui()
  const { canManage } = useSession()
  const property = useCurrentProperty()
  const funnels = useFunnels(property.id)
  const goals = useGoals(property.id)
  const [params, setParams] = useSearchParams()
  const [creating, setCreating] = useState(false)
  const list = funnels.data ?? []
  const goalList = goals.data ?? []
  const current = list.find((f) => f.id === params.get("funnel")) ?? list[0]
  const pick = (id: string) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.set("funnel", id)
        return next
      },
      { replace: true }
    )
  if (funnels.isPending || goals.isPending) return <Skeleton className="h-64 rounded-[14px]" />
  if (funnels.error || goals.error) return <ErrorLine error={funnels.error ?? goals.error} />
  const canCreate = canManage && goalList.length >= 2
  const newButton = canManage && (
    <Button
      size="sm"
      onClick={() => setCreating(true)}
      disabled={!canCreate}
      title={canCreate ? undefined : t`Make at least two goals first.`}
      data-testid="funnel-new"
    >
      <PlusIcon />
      <Trans>New funnel</Trans>
    </Button>
  )
  return (
    <section className="flex flex-col gap-3" data-testid="analytics-funnels">
      {list.length === 0 ? (
        <div
          className="flex flex-col items-center gap-2 rounded-[14px] border border-dashed px-4 py-10 text-center"
          data-testid="funnels-empty"
        >
          <FilterIcon className="size-6 text-faint" />
          <p className="font-medium">
            <Trans>No funnels yet</Trans>
          </p>
          <p className="max-w-md text-body text-faint">
            {!canManage ? (
              <Trans>Owners and admins make funnels from goals. They show up here with each step’s drop-off.</Trans>
            ) : goalList.length < 2 ? (
              <Trans>A funnel is made of two to eight goals in a row. Make at least two goals first.</Trans>
            ) : (
              <Trans>
                A funnel shows how many visitors go from one goal to the next within a visit, and where they leave.
              </Trans>
            )}
          </p>
          {canManage &&
            (goalList.length < 2 ? (
              <Button
                className="mt-2"
                variant="outline"
                render={<Link to={`/analytics/${property.id}/goals`} />}
                data-testid="funnel-to-goals"
              >
                <Trans>Go to goals</Trans>
              </Button>
            ) : (
              <Button className="mt-2" onClick={() => setCreating(true)} data-testid="funnel-new">
                <PlusIcon />
                <Trans>New funnel</Trans>
              </Button>
            ))}
        </div>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            {list.length > 1 && current && (
              <ChoiceSelect
                value={current.id}
                onChange={pick}
                options={list.map((f) => [f.id, f.name] as const)}
                label={t`Funnel`}
                className="min-w-48 @max-2xl:flex-1"
              />
            )}
            <span className="flex-1" />
            {newButton}
          </div>
          {current && <FunnelPanel key={current.id} funnel={current} goals={goalList} />}
        </>
      )}
      {creating && (
        <FunnelDialog
          goals={goalList}
          onClose={() => setCreating(false)}
          onSaved={(f) => {
            setCreating(false)
            pick(f.id)
          }}
        />
      )}
    </section>
  )
}
