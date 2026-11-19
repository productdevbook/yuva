import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightIcon, ArrowUpRightIcon, PlusIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import {
  ErrorLine,
  InboxMark,
  InfoColumn,
  InfoSection,
  KV,
  ListRow,
  MoreLink,
  PageLayout,
  PageTop,
  SectionHeading,
  Segmented,
  useConfirm,
} from "@/components/common"
import { boxRowClass, FilterRow, ListBox, ListBoxEmpty, ListBoxSkeleton, PageTitle } from "@/components/common/ListBox"
import { keyLabel, STATUS_KEYS } from "@/components/common/ShortcutSheet"
import { formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useListCursor } from "@/features/inbox/listCursor"
import { Overline } from "@/features/inbox/PageFrame"
import { ComponentsTab } from "@/features/status/ComponentsTab"
import { CreatePageForm } from "@/features/status/CreatePageForm"
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
import { isClosed, StateDot, useStatusText } from "@/features/status/text"
import { UptimeBars, usePercent } from "@/features/status/UptimeBars"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Inbox, StatusComponentStateValue, StatusIncident, StatusPage } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport } from "@/lib/workspace"

const TABS = ["overview", "components", "subscribers", "settings"] as const
type Tab = (typeof TABS)[number]

export function hostOf(url: string) {
  return url.replace(/^https?:\/\//, "").replace(/\/$/, "")
}

const card = "overflow-hidden rounded-[14px] border bg-card"
const stateWash: Record<StatusComponentStateValue, string> = {
  operational: "bg-success/12 text-success",
  degraded: "bg-warning/12 text-warning",
  partial_outage: "bg-warning/12 text-warning",
  major_outage: "bg-destructive/10 text-destructive",
  maintenance: "bg-blue-600/10 text-blue-600 dark:bg-blue-400/12 dark:text-blue-400",
}

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

function ComponentRow({
  p,
  name,
  state,
  uptime,
  flat,
}: {
  p: StatusPage
  name: string
  state: StatusComponentStateValue
  uptime?: { uptime?: number; days?: Parameters<typeof UptimeBars>[0]["days"] }
  flat?: boolean
}) {
  const percent = usePercent()
  return (
    <li
      className={cn(
        "grid grid-cols-[minmax(150px,1fr)_minmax(0,2.2fr)_64px_132px] items-center gap-x-3.5 gap-y-1.5 border-t py-2.5 text-small @max-3xl:grid-cols-[minmax(0,1fr)_auto]",
        flat ? "border-line-soft px-1" : "px-4 first:border-t-0"
      )}
    >
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

function ComponentsCard({ p, flat }: { p: StatusPage; flat?: boolean }) {
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
  const pad = flat ? "px-1" : "px-4"
  return (
    <section className={flat ? "flex flex-col" : card} data-testid="status-components-card">
      <div className={cn("flex flex-wrap items-start gap-x-3 gap-y-1 pt-3.5 pb-2", pad)}>
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
        <p className={cn("py-6 text-faint", flat ? "border-t border-line-soft px-1" : "px-4 text-center")}>
          <Trans>
            No components yet. Add the parts of your product that customers depend on, in the Components tab.
          </Trans>
        </p>
      ) : (
        <>
          <div className={cn("flex flex-wrap gap-x-3 gap-y-1 pb-3 text-caption text-faint", pad)} aria-hidden>
            {legend.map((s) => (
              <span key={s} className="inline-flex items-center gap-1.5">
                <StateDot state={s} className="size-2.5 rounded-[2px]" />
                {text.component[s]}
              </span>
            ))}
          </div>
          {sections.map((s) => (
            <div key={s.key || "none"} className={flat ? undefined : "border-t"}>
              {s.name && <Overline className={cn("pt-2.5 pb-1", pad)}>{s.name}</Overline>}
              <ul>
                {s.rows.map((c) => (
                  <ComponentRow key={c.id} p={p} name={c.name} state={c.state} uptime={uptime.get(c.id)} flat={flat} />
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

function OpenIncidentRow({ p, i }: { p: StatusPage; i: StatusIncident }) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const draft = !i.published_at
  const maintenance = i.kind === "maintenance"
  const since = formatShort(i.started_at ?? i.published_at ?? i.created_at, i18n.locale)
  const state = text.incident[i.state]
  const latest = i.updates[0]
  return (
    <ListRow to={incidentLink(p.id, i.id)} className="items-start" data-testid="status-incident-row">
      <span
        aria-hidden
        className={cn(
          "mt-1.5 size-[7px] shrink-0 rounded-full",
          maintenance ? "bg-blue-600 dark:bg-blue-400" : i.impact === "minor" ? "bg-warn" : "bg-bad"
        )}
      />
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="font-medium break-words text-foreground">{i.title}</span>
        <span className="text-small text-muted-foreground">
          {maintenance ? text.kind.maintenance : text.impact[i.impact]}
          {" · "}
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
        {latest?.body && <span className="line-clamp-2 text-small text-muted-foreground">{latest.body}</span>}
      </span>
      <span className="inline-flex shrink-0 items-center gap-1 text-small font-medium text-brand">
        {draft ? <Trans>Review and post</Trans> : maintenance ? <Trans>Open</Trans> : <Trans>Post update</Trans>}
        <ArrowRightIcon className="size-3.5 rtl:rotate-180" />
      </span>
    </ListRow>
  )
}

function PageInfo({ p, incidents }: { p: StatusPage; incidents: StatusIncident[] }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const subs = p.email_subscribers
  const past = incidents
    .filter((i) => i.kind === "incident" && !!i.published_at)
    .sort((a, b) => (b.started_at ?? b.created_at).localeCompare(a.started_at ?? a.created_at))
  const last = past[0]
  return (
    <InfoSection title={<Trans>Page</Trans>} testId="status-facts">
      <KV label={<Trans>Address</Trans>} value={<span dir="ltr">{hostOf(p.url)}</span>} />
      <KV
        label={<Trans>Visibility</Trans>}
        value={p.published ? t`Published` : t`Unpublished`}
        tone={p.published ? "ok" : undefined}
      />
      <KV
        label={<Trans>Subscribers</Trans>}
        value={subs.enabled ? fmt.format(subs.counts.subscribed) : t`Off`}
        to={statusLink(p.id, "subscribers")}
      />
      <KV
        label={<Trans>Last incident</Trans>}
        value={
          !last
            ? t`None yet`
            : !last.resolved_at
              ? t`Now`
              : formatShort(last.started_at ?? last.created_at, i18n.locale)
        }
        tone={last && !last.resolved_at ? "warn" : undefined}
        to="/incidents"
      />
    </InfoSection>
  )
}

function MonitorsInfo({ p }: { p: StatusPage }) {
  const text = useStatusText()
  const monitors = useStatusMonitors(p.id).data ?? []
  const shown = [...monitors]
    .sort((a, b) => Number(b.state === "down" && !b.paused) - Number(a.state === "down" && !a.paused))
    .slice(0, MONITOR_ROWS)
  return (
    <InfoSection
      title={<Trans>Monitors</Trans>}
      action={
        <MoreLink to="/monitors">
          <Trans>All</Trans>
        </MoreLink>
      }
      testId="status-monitors-card"
    >
      {shown.length === 0 ? (
        <p className="m-0 text-small text-muted-foreground">
          <Trans>No monitor watches this page yet</Trans>
        </p>
      ) : (
        shown.map((m) => (
          <KV
            key={m.id}
            label={m.name}
            value={text.monitorState[m.paused ? "paused" : m.state]}
            tone={m.paused ? undefined : m.state === "down" ? "bad" : m.state === "up" ? "ok" : undefined}
            to={monitorLink(m.id)}
          />
        ))
      )}
    </InfoSection>
  )
}

function ComingUpInfo({ p, planned }: { p: StatusPage; planned: StatusIncident[] }) {
  const { i18n } = useLingui()
  return (
    <InfoSection title={<Trans>Coming up</Trans>} testId="status-coming-up">
      {planned.length === 0 ? (
        <p className="m-0 text-small text-muted-foreground">
          <Trans>No maintenance is planned.</Trans>
        </p>
      ) : (
        planned.map((i) => (
          <KV
            key={i.id}
            label={i.title}
            value={formatShort(i.scheduled_start ?? i.created_at, i18n.locale)}
            to={incidentLink(p.id, i.id)}
          />
        ))
      )}
    </InfoSection>
  )
}

function FlatOverview({ p }: { p: StatusPage }) {
  const { t } = useLingui()
  const text = useStatusText()
  const incidents = usePagesIncidents([p.id], false)
  const monitors = useStatusMonitors(p.id).data
  const open = incidents.data.filter((i) => !isClosed(i) && i.state !== "scheduled")
  const headline =
    p.state === "operational"
      ? t`All systems operational`
      : p.state === "degraded" || p.state === "partial_outage"
        ? t`Partly degraded`
        : text.component[p.state]
  const watching = monitors?.length ?? 0
  return (
    <div className="flex flex-col" data-testid="status-overview">
      <div className="mt-6 flex items-center gap-3 border-y border-line-soft px-1 py-3.5" role="status">
        <StateDot state={p.state} className="size-2.5" />
        <span className="min-w-0 flex-1 text-body">
          <b className="font-semibold text-foreground" data-testid="status-overall">
            {headline}
          </b>
          <span className="text-muted-foreground">
            {" · "}
            {watching > 0 ? (
              <Plural value={watching} one="Watched by # monitor" other="Watched by # monitors" />
            ) : (
              <Trans>No monitor watches this page yet</Trans>
            )}
          </span>
        </span>
      </div>
      {open.length > 0 && (
        <>
          <SectionHeading
            title={<Trans>Open incidents</Trans>}
            more={
              <MoreLink to="/incidents">
                <Trans>All incidents</Trans>
              </MoreLink>
            }
          />
          <div>
            {open.map((i) => (
              <OpenIncidentRow key={i.id} p={p} i={i} />
            ))}
          </div>
        </>
      )}
      <div className="mt-6">
        <ComponentsCard p={p} flat />
      </div>
    </div>
  )
}

function OverviewAside({ p }: { p: StatusPage }) {
  const incidents = usePagesIncidents([p.id], false)
  const planned = incidents.data.filter((i) => !isClosed(i) && i.state === "scheduled")
  return (
    <>
      <PageInfo p={p} incidents={incidents.data} />
      <MonitorsInfo p={p} />
      <ComingUpInfo p={p} planned={planned} />
    </>
  )
}

function PagesList({
  items,
  inboxOf,
  active,
}: {
  items: StatusPage[]
  inboxOf: (p?: StatusPage) => Inbox | undefined
  active: boolean
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const text = useStatusText()
  const listRef = useRef<HTMLDivElement>(null)
  const { cursorId } = useListCursor(items, null, (id) => navigate(statusLink(id)), active, listRef, true, true)
  return (
    <div ref={listRef} className="@container/list">
      <ListBox testId="list">
        {items.map((p) => {
          const inbox = inboxOf(p)
          return (
            <div key={p.id} data-row data-row-id={p.id} className={boxRowClass({ cursor: p.id === cursorId })}>
              <button
                type="button"
                onClick={() => navigate(statusLink(p.id))}
                className="flex min-w-0 flex-1 items-center gap-3 py-3 pe-3.5 text-start outline-none"
                aria-label={t`Open ${p.name}`}
                data-testid="status-home-page"
              >
                <InboxMark
                  name={inbox?.name ?? p.name}
                  color={inbox?.branding.color}
                  className="size-[22px] rounded-[6px] text-caption"
                />
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="truncate text-body font-semibold">{p.name}</span>
                  <span className="truncate text-small text-muted-foreground" dir="ltr">
                    {hostOf(p.url)}
                  </span>
                </span>
                <span className="inline-flex shrink-0 items-center gap-2 text-small">
                  <StateDot state={p.state} />
                  <span className="hidden @md/list:inline">{text.component[p.state]}</span>
                </span>
                <span
                  className={cn(
                    "hidden w-24 shrink-0 text-end text-small @xl/list:block",
                    p.published ? "text-ok" : "text-muted-foreground"
                  )}
                >
                  {p.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
                </span>
              </button>
            </div>
          )
        })}
      </ListBox>
    </div>
  )
}

export function StatusHome() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const { items, pages, inboxOf, inboxes } = useStatusScope()
  const withPage = new Set(items.map((p) => p.inbox_id))
  const missing = canManage ? inboxes.filter((i) => isSupport(i) && !withPage.has(i.id)) : []
  const [creating, setCreating] = useState<string | null>(null)
  const [preview, setPreview] = useState(false)
  const [confirm, confirmDialog] = useConfirm()
  const pageId = params.get("page")
  const current = pageId ? items.find((p) => p.id === pageId) : undefined
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
  const busy = creating !== null || preview

  useHotkeys(
    {
      ...(missing.length > 0 && !current ? { [STATUS_KEYS.newPage]: () => setCreating("") } : {}),
      ...(current
        ? {
            [STATUS_KEYS.previousPage]: () => pick(at - 1),
            [STATUS_KEYS.nextPage]: () => pick(at + 1),
            [STATUS_KEYS.overview]: () => go(current.id),
            [STATUS_KEYS.components]: () => go(current.id, "components"),
            [STATUS_KEYS.subscribers]: () => go(current.id, "subscribers"),
            ...(canManage ? { [STATUS_KEYS.settings]: () => go(current.id, "settings") } : {}),
            [STATUS_KEYS.preview]: () => setPreview(true),
            ...(current.published
              ? { [STATUS_KEYS.openPage]: () => window.open(current.url, "_blank", "noopener,noreferrer") }
              : {}),
            ...(canManage ? { [STATUS_KEYS.publishPage]: togglePublish } : {}),
            Escape: () => navigate("/status"),
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
  const createForm = creating !== null && (
    <CreatePageForm
      inboxes={missing}
      inboxId={creating || undefined}
      onClose={() => setCreating(null)}
      onCreated={(p) => {
        setCreating(null)
        navigate(statusLink(p.id, "settings"))
      }}
    />
  )
  const dialogs = (
    <>
      {preview && current && <PreviewDialog page={current} onClose={() => setPreview(false)} />}
      {confirmDialog}
    </>
  )

  if (current) {
    return (
      <PageLayout
        testId="status-home"
        label={current.name}
        aside={
          <InfoColumn label={t`Details`}>
            <OverviewAside p={current} />
          </InfoColumn>
        }
      >
        <PageTop back={{ label: t`Status pages`, to: "/status" }} />
        <section className="@container flex min-w-0 flex-col" data-testid="status-page">
          <PageTitle
            title={<span data-testid="status-page-name">{current.name}</span>}
            sub={
              <span data-testid="status-published">
                <span dir="ltr">{hostOf(current.url)}</span>
                {" · "}
                {current.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
              </span>
            }
            actions={
              <>
                <Button
                  variant="outline"
                  onClick={() => setPreview(true)}
                  title={t`Preview (${previewKey})`}
                  data-testid="status-preview"
                >
                  <Trans>Preview</Trans>
                </Button>
                {current.published && (
                  <Button
                    variant="outline"
                    render={<a href={current.url} target="_blank" rel="noreferrer noopener" />}
                    title={t`View page (${openKey})`}
                    data-testid="status-open-public"
                  >
                    <Trans>View page</Trans>
                    <ArrowUpRightIcon className="rtl:-scale-x-100" />
                  </Button>
                )}
                {canManage && (
                  <Button
                    variant={current.published ? "outline" : "default"}
                    disabled={patch.isPending}
                    onClick={togglePublish}
                    title={current.published ? t`Unpublish (${publishKey})` : t`Publish (${publishKey})`}
                    data-testid="status-publish"
                  >
                    {current.published ? <Trans>Unpublish</Trans> : <Trans>Publish</Trans>}
                  </Button>
                )}
              </>
            }
            testId="status-head"
          />
          <FilterRow testId="status-sections">
            <div className="-my-1 max-w-full overflow-x-auto py-1">
              <Segmented
                value={tab}
                onChange={(v) => go(current.id, v)}
                label={t`Status page`}
                items={TABS.filter((v) => v !== "settings" || canManage).map((v) => ({
                  value: v,
                  label: names[v],
                  testId: `status-tab-${v}`,
                }))}
              />
            </div>
          </FilterRow>
          <ErrorLine error={patch.error} className="mt-3" />
          <div key={current.id} className={tab === "overview" ? undefined : "mt-4 flex flex-col gap-4"}>
            {tab === "overview" && <FlatOverview p={current} />}
            {tab === "components" && <ComponentsTab page={current} />}
            {tab === "subscribers" && <SubscribersTab page={current} />}
            {tab === "settings" && canManage && <SettingsTab page={current} />}
          </div>
        </section>
        {dialogs}
      </PageLayout>
    )
  }

  const fmtCount = items.length
  return (
    <PageLayout testId="status-home" label={t`Status pages`}>
      <section className="flex min-w-0 flex-col">
        <PageTitle
          title={<Trans>Status pages</Trans>}
          sub={
            items.length > 0 ? (
              <Plural value={fmtCount} one="# page" other="# pages" />
            ) : (
              <Trans>Public pages that say whether your products work</Trans>
            )
          }
          actions={
            missing.length > 0 && (
              <Button
                onClick={() => setCreating("")}
                disabled={creating !== null}
                title={t`Create status page (${newKey})`}
                data-testid="status-home-create"
              >
                <PlusIcon />
                <Trans>New status page</Trans>
              </Button>
            )
          }
          testId="status-head"
        />
        {createForm}
        {pageId && !pages.isPending && (
          <p className="m-0 mt-3 text-small text-muted-foreground" data-testid="status-missing">
            <Trans>This status page no longer exists or you cannot see it.</Trans>
          </p>
        )}
        {pages.isPending ? (
          <ListBox>
            <ListBoxSkeleton rows={2} />
          </ListBox>
        ) : pages.error ? (
          <ListBox testId="status-error">
            <ListBoxEmpty
              action={
                <Button variant="outline" size="sm" onClick={() => void pages.refetch()}>
                  <Trans>Try again</Trans>
                </Button>
              }
            >
              <ErrorLine error={pages.error} />
            </ListBoxEmpty>
          </ListBox>
        ) : items.length === 0 ? (
          <ListBox testId="status-home-empty">
            <ListBoxEmpty
              action={
                missing.length > 0
                  ? creating === null && (
                      <Button variant="outline" size="sm" onClick={() => setCreating("")}>
                        <PlusIcon />
                        <Trans>New status page</Trans>
                      </Button>
                    )
                  : !canManage && (
                      <span className="text-small" data-testid="status-none">
                        <Trans>No status page yet. Owners and admins create it.</Trans>
                      </span>
                    )
              }
            >
              <Trans>
                Tell your customers whether your product works. Each product gets one page, where you open incidents,
                plan maintenance and set the state of each component.
              </Trans>
            </ListBoxEmpty>
          </ListBox>
        ) : (
          <PagesList items={items} inboxOf={inboxOf} active={!busy} />
        )}
      </section>
      {dialogs}
    </PageLayout>
  )
}
