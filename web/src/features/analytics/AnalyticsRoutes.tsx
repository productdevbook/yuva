import "@/features/analytics/i18n"

import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useRef } from "react"
import { ChartLineIcon, ChevronsUpDownIcon, PlusIcon } from "lucide-react"
import {
  Link,
  Navigate,
  Outlet,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router"

import { Dot, ErrorLine, FlatEmpty, ListRow, PageHeading, PageTop as FlatTop, topInk } from "@/components/common"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { UsageBanner } from "@/features/analytics/AnalyticsColumn"
import { BreakdownView } from "@/features/analytics/Breakdown"
import { ComparePicker, FilterBar, RangePicker, SegmentPicker } from "@/features/analytics/Controls"
import { EventsView } from "@/features/analytics/Events"
import { FunnelsView } from "@/features/analytics/Funnels"
import { GoalsView } from "@/features/analytics/Goals"
import { WaitForFirstEvent } from "@/features/analytics/NewProperty"
import { Overview } from "@/features/analytics/Overview"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useProperty } from "@/features/analytics/queries"
import { Realtime } from "@/features/analytics/Realtime"
import { RetentionView } from "@/features/analytics/Retention"
import { SettingsView } from "@/features/analytics/Settings"
import { SupportView } from "@/features/analytics/Support"
import { useViewSearch } from "@/features/analytics/state"
import {
  AnalyticsLayout,
  AnalyticsTrail,
  useDetailBack,
  useScopedProperties,
  useViewLedes,
  useViewNames,
  VIEWS,
  type ViewKey,
} from "@/features/analytics/AnalyticsFrame"
import { formatCompact } from "@/features/analytics/text"
import { useListCursor } from "@/features/inbox/listCursor"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useIsPhone } from "@/hooks/use-media-query"
import { PagesView } from "@/features/analytics/Pages"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import type { SetupState } from "@/features/setup/setup"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

function AnalyticsHome() {
  const { canManage } = useSession()
  const phone = useIsPhone()
  const [inboxId] = useInboxFilter()
  const list = useAnalyticsProperties()
  const scoped = (list.data?.items ?? []).filter((p) => !inboxId || p.inbox_id === inboxId)
  const first = scoped[0]
  if (!phone) {
    if (first && scoped.length === 1) return <Navigate to={`/analytics/${first.id}`} replace />
    return <PropertyList />
  }
  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="analytics-home">
      <PageTop title={<Trans>Analytics</Trans>} sub={<Trans>Cookieless visitors across your sites and apps</Trans>} />
      <div className="@container min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[1240px] p-6 pb-16 @max-xl:p-4">
          <div className="flex flex-col items-center gap-2 rounded-[14px] border bg-card px-6 py-12 text-center">
            <span className="mb-2 grid size-12 place-items-center rounded-xl bg-brand-wash text-brand">
              <ChartLineIcon className="size-6" />
            </span>
            <p className="text-title">
              <Trans>Count your website’s visitors</Trans>
            </p>
            <p className="max-w-md text-body text-muted-foreground">
              <Trans>
                Pages, sources, places and devices, without cookies, without storing IP addresses and without a consent
                banner where the law allows it.
              </Trans>
            </p>
            {canManage && (
              <Button
                className={cn(look.btnInk, "mt-3")}
                render={<Link to="/analytics/new" />}
                data-testid="analytics-home-add"
              >
                <PlusIcon />
                <Trans>Add a website</Trans>
              </Button>
            )}
          </div>
        </div>
      </div>
    </main>
  )
}

function PropertyList() {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const { list, items } = useScopedProperties()
  const inboxes = useInboxes().data ?? []
  const colors = new Map(inboxes.map((i) => [i.id, i.branding.color]))
  const now = useNow(60_000)
  const listRef = useRef<HTMLDivElement>(null)
  const open = (id: string) => navigate(`/analytics/${id}`)
  const { cursorId } = useListCursor(items, null, open, true, listRef, true, true)
  return (
    <AnalyticsLayout testId="analytics-home">
      <FlatTop
        actions={
          canManage && (
            <Button className={topInk} render={<Link to="/analytics/new" />} data-testid="analytics-home-add">
              <PlusIcon className="size-3.5" aria-hidden />
              <Trans>Add a website</Trans>
            </Button>
          )
        }
      >
        <AnalyticsTrail />
      </FlatTop>
      <div className="mt-3 flex max-w-[980px] flex-col">
        <PageHeading title={t`Analytics`} lede={<Trans>Cookieless visitors across your sites and apps</Trans>} />
        {list.data && <UsageBanner usage={list.data.usage} className="mt-5" />}
        <ErrorLine error={list.error} className="mt-3" />
        {list.isPending ? (
          <div className="mt-7 flex flex-col gap-3">
            {[0, 1, 2].map((n) => (
              <Skeleton key={n} className="h-5 w-full" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <FlatEmpty testId="analytics-empty">
            <p className="m-0 font-medium text-foreground">
              <Trans>Count your website’s visitors</Trans>
            </p>
            <p className="m-0 max-w-[560px]">
              <Trans>
                Pages, sources, places and devices, without cookies, without storing IP addresses and without a consent
                banner where the law allows it.
              </Trans>
            </p>
          </FlatEmpty>
        ) : (
          <div ref={listRef} className="mt-6 flex flex-col border-b border-line-soft">
            {items.map((p) => {
              const quiet = !p.last_event_at || now - Date.parse(p.last_event_at) > 86_400_000
              const color = inboxes.length > 1 && p.inbox_id ? colors.get(p.inbox_id) : undefined
              return (
                <ListRow
                  key={p.id}
                  to={`/analytics/${p.id}`}
                  className={cn(cursorId === p.id && "bg-subtle shadow-[inset_2px_0_0_var(--brand)]")}
                  data-row-id={p.id}
                  data-testid="analytics-property"
                >
                  <span
                    aria-hidden
                    className="size-[7px] shrink-0 rounded-full bg-muted2"
                    style={color ? { backgroundColor: color } : undefined}
                  />
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="truncate font-medium text-foreground">{p.name}</span>
                      {quiet && (
                        <span
                          className="size-1.5 shrink-0 rounded-full bg-warn"
                          title={p.first_event_at ? t`No events in the last 24 hours` : t`No events yet`}
                          data-testid="analytics-quiet"
                        />
                      )}
                    </span>
                    <span className="truncate font-mono text-caption text-muted2" dir="ltr">
                      {p.domains.join(", ")}
                    </span>
                  </span>
                  <span className="flex shrink-0 flex-col items-end">
                    <span className="font-mono text-body font-medium tabular-nums">
                      {formatCompact(p.visitors_today, i18n.locale)}
                    </span>
                    <span className="text-caption text-muted-foreground">
                      <Plural value={p.visitors_today} one="visitor today" other="visitors today" />
                    </span>
                  </span>
                </ListRow>
              )
            })}
          </div>
        )}
      </div>
    </AnalyticsLayout>
  )
}

const HEADED: ViewKey[] = ["overview", "pages", "goals"]

const pickerClass = "h-[30px] rounded-lg"

function PropertyLayout() {
  const { t } = useLingui()
  const { propertyId = "" } = useParams()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const search = useViewSearch(true)
  const [params] = useSearchParams()
  const [inboxId] = useInboxFilter()
  const property = useProperty(propertyId)
  const list = useAnalyticsProperties().data
  const usage = list?.usage
  const others = (list?.items ?? []).filter((o) => !inboxId || o.inbox_id === inboxId)
  const inbox = (useInboxes().data ?? []).find((i) => i.id === property.data?.inbox_id)
  const last = pathname.split("/")[3] as ViewKey | undefined
  const current: ViewKey = last && VIEWS.includes(last) ? last : "overview"
  const names = useViewNames()
  const p = property.data
  const seenInbox = useRef(inboxId)
  useEffect(() => {
    if (seenInbox.current === inboxId || !p || !list) return
    seenInbox.current = inboxId
    if (!inboxId || p.inbox_id === inboxId) return
    const next = list.items.find((o) => o.inbox_id === inboxId)
    const view = current === "settings" ? "" : current === "overview" ? "" : `/${current}`
    navigate(next ? `/analytics/${next.id}${view}${search}` : "/analytics")
  }, [inboxId, p, list, current, search, navigate])
  const phone = useIsPhone()
  const ledes = useViewLedes()
  const back = useDetailBack(current)
  useHotkeys({ Escape: () => back && navigate(back.to) }, !phone && !!back)
  if (!p && !phone) {
    return (
      <AnalyticsLayout current={current} testId="analytics-property">
        <FlatTop>
          <AnalyticsTrail current={current} />
        </FlatTop>
        <div className="mt-3">
          {property.error ? <ErrorLine error={property.error} /> : <Skeleton className="h-64 rounded-[10px]" />}
        </div>
      </AnalyticsLayout>
    )
  }
  if (!p) {
    return (
      <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface">
        <PageTop title={<Trans>Analytics</Trans>} />
        <div className="mx-auto w-full max-w-[1240px] p-6 @max-xl:p-4">
          {property.error ? <ErrorLine error={property.error} /> : <Skeleton className="h-64 rounded-[14px]" />}
        </div>
      </main>
    )
  }
  const controls = current !== "realtime" && current !== "settings" && current !== "retention"
  const headed = HEADED.includes(current)
  const to = (v: ViewKey, id = p.id) => `/analytics/${id}${v === "overview" ? "" : `/${v}`}${search}`
  const where = (
    <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
      {inbox && (
        <span className="inline-flex items-center gap-1.5">
          <Dot color={inbox.branding.color} className="size-[7px] rounded-[2px]" />
          {inbox.name}
        </span>
      )}
      <span className="min-w-0 truncate" dir="ltr">
        {p.domains.join(", ")}
      </span>
      <span aria-hidden>·</span>
      <span title={t`Days are counted in this time zone`}>{p.timezone}</span>
    </span>
  )
  const head: Partial<Record<ViewKey, { title: React.ReactNode; description: React.ReactNode }>> = {
    overview: {
      title: <Trans>Analytics</Trans>,
      description: <Trans>Cookieless visitors across your sites and apps</Trans>,
    },
    pages: { title: names.pages, description: <Trans>Which pages people visit, and which lead to questions</Trans> },
    goals: { title: names.goals, description: <Trans>Visits and events that count as success</Trans> },
  }
  const picked = params.get("goal")
  const newGoal = new URLSearchParams(search)
  newGoal.set("goal", "new")
  const tabs = VIEWS.map((v) => ({
    key: v,
    label: names[v],
    to: to(v),
    testId: `analytics-tab-${v}`,
  }))
  const toolbar = controls ? (
    <div
      className="flex flex-wrap items-center gap-2 border-b px-6 py-3 empty:hidden phone:px-4"
      data-testid="analytics-toolbar"
    >
      {current !== "goals" && <RangePicker plain className={pickerClass} />}
      {!headed && current !== "events" && current !== "funnels" && <ComparePicker className={pickerClass} />}
      {!headed && current !== "funnels" && <SegmentPicker propertyId={p.id} className={pickerClass} />}
      {headed ? current !== "pages" && <FilterBar /> : current !== "funnels" && <FilterBar />}
    </div>
  ) : null
  if (!phone) {
    const action =
      current === "goals"
        ? canManage &&
          picked !== "new" &&
          !back && (
            <Button className={topInk} render={<Link to={`?${newGoal.toString()}`} />} data-testid="analytics-goal-new">
              <PlusIcon className="size-3.5" aria-hidden />
              <Trans>New goal</Trans>
            </Button>
          )
        : canManage &&
          current !== "pages" &&
          current !== "settings" && (
            <Button
              className={topInk}
              render={<Link to={`/analytics/${p.id}/settings?tab=sharing`} />}
              data-testid="analytics-share"
            >
              <Trans>Share</Trans>
            </Button>
          )
    const goalOpen = current === "goals" && !!back
    return (
      <AnalyticsLayout property={p} current={current} testId="analytics-property">
        <div key={p.id} className="@container flex min-w-0 flex-col">
          <FlatTop back={back} actions={action || undefined}>
            <AnalyticsTrail property={p} current={current} />
          </FlatTop>
          {!goalOpen && (
            <div className="mt-3">
              <PageHeading title={names[current]} lede={ledes[current]} />
            </div>
          )}
          {toolbar && (
            <div className="mt-5 flex flex-wrap items-center gap-2 empty:hidden" data-testid="analytics-toolbar">
              {current !== "goals" && <RangePicker plain className={pickerClass} />}
              {!headed && current !== "events" && current !== "funnels" && <ComparePicker className={pickerClass} />}
              {!headed && current !== "funnels" && <SegmentPicker propertyId={p.id} className={pickerClass} />}
              {headed ? current !== "pages" && <FilterBar /> : current !== "funnels" && <FilterBar />}
            </div>
          )}
          <div className="mt-6 flex flex-col gap-4">
            {usage && <UsageBanner usage={usage} />}
            {!p.first_event_at && current !== "settings" && <WaitForFirstEvent propertyId={p.id} />}
            <Outlet context={{ property: p }} />
          </div>
        </div>
      </AnalyticsLayout>
    )
  }
  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="analytics-property" key={p.id}>
      <PageTop
        title={head[current]?.title ?? names[current]}
        sub={head[current]?.description ?? where}
        actions={
          <>
            {others.length > 1 ? (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={<Button variant="outline" className={cn(look.btn, "max-w-56 bg-card")} />}
                  data-testid="analytics-property-name"
                >
                  <span className="truncate">{p.name}</span>
                  <ChevronsUpDownIcon className="text-faint" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  {others.map((o) => (
                    <DropdownMenuItem
                      key={o.id}
                      onClick={() => navigate(to(current === "settings" ? "overview" : current, o.id))}
                      aria-current={o.id === p.id ? "page" : undefined}
                      className="aria-[current=page]:font-medium"
                    >
                      <span className="truncate">{o.name}</span>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <span className="sr-only" data-testid="analytics-property-name">
                {p.name}
              </span>
            )}
            {current === "goals"
              ? canManage &&
                picked !== "new" && (
                  <Button
                    className={look.btnInk}
                    render={<Link to={`?${newGoal.toString()}`} />}
                    data-testid="analytics-goal-new"
                  >
                    <Trans>New goal</Trans>
                  </Button>
                )
              : canManage &&
                current !== "pages" && (
                  <Button
                    className={look.btnInk}
                    render={<Link to={`/analytics/${p.id}/settings?tab=sharing`} />}
                    data-testid="analytics-share"
                  >
                    <Trans>Share</Trans>
                  </Button>
                )}
          </>
        }
        tabs={<FrameTabs label={t`Analytics`} value={current} items={tabs} moreTestId="analytics-tab-more" />}
      />
      {toolbar}
      <div className="@container min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-[1240px] flex-col gap-4 p-6 pb-16 @max-xl:p-4 @max-xl:pb-16">
          {usage && <UsageBanner usage={usage} />}
          {!p.first_event_at && current !== "settings" && <WaitForFirstEvent propertyId={p.id} />}
          <Outlet context={{ property: p }} />
        </div>
      </div>
    </main>
  )
}

function ToSetup() {
  const { propertyId } = useParams()
  return (
    <Navigate
      to={propertyId ? `/setup/analytics/${propertyId}` : "/setup/analytics"}
      state={{ back: "/analytics" } satisfies SetupState}
      replace
    />
  )
}

export default function AnalyticsRoutes() {
  return (
    <Routes>
      <Route index element={<AnalyticsHome />} />
      <Route path="new" element={<ToSetup />} />
      <Route path="new/:propertyId" element={<ToSetup />} />
      <Route path=":propertyId" element={<PropertyLayout />}>
        <Route index element={<Overview />} />
        <Route path="realtime" element={<Realtime />} />
        <Route path="pages" element={<PagesView />} />
        <Route path="sources" element={<BreakdownView group="sources" />} />
        <Route path="locations" element={<BreakdownView group="locations" />} />
        <Route path="devices" element={<BreakdownView group="devices" />} />
        <Route path="events" element={<EventsView />} />
        <Route path="goals" element={<GoalsView />} />
        <Route path="funnels" element={<FunnelsView />} />
        <Route path="retention" element={<RetentionView />} />
        <Route path="support" element={<SupportView />} />
        <Route path="settings" element={<SettingsView />} />
        <Route path="*" element={<Navigate to=".." replace />} />
      </Route>
    </Routes>
  )
}
