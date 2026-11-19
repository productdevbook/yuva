import "@/features/analytics/i18n"

import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
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

import { Dot, ErrorLine, PageTop, Segmented } from "@/components/common"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import { FilterRow, KeyHints, ListBox, ListBoxEmpty, ListBoxSkeleton, PageTitle } from "@/components/common/ListBox"
import { ANALYTICS_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { keyHint } from "@/features/broadcasts/BroadcastParts"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { UsageBanner } from "@/features/analytics/AnalyticsColumn"
import {
  AnalyticsLayout,
  PropertyPicker,
  useDetailBack,
  useScopedProperties,
  useViewLedes,
  useViewNames,
  VIEWS,
  type ViewKey,
} from "@/features/analytics/AnalyticsFrame"
import { BreakdownView } from "@/features/analytics/Breakdown"
import { ComparePicker, FilterBar, RangePicker, SegmentPicker } from "@/features/analytics/Controls"
import { EventsView } from "@/features/analytics/Events"
import { FunnelsView } from "@/features/analytics/Funnels"
import { GoalsView } from "@/features/analytics/Goals"
import { hostOnly, splitLines, useZones, WaitForFirstEvent } from "@/features/analytics/NewProperty"
import { Overview } from "@/features/analytics/Overview"
import { PagesView } from "@/features/analytics/Pages"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useCreateProperty, useProperty } from "@/features/analytics/queries"
import { Realtime } from "@/features/analytics/Realtime"
import { RetentionView } from "@/features/analytics/Retention"
import { SettingsView } from "@/features/analytics/Settings"
import { SupportView } from "@/features/analytics/Support"
import { useViewSearch } from "@/features/analytics/state"
import { formatCompact } from "@/features/analytics/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useListCursor } from "@/features/inbox/listCursor"
import { Ago } from "@/features/inbox/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const NO_INBOX = "none"

type Kind = "web" | "ios" | "android"

const SNIPPET: Record<Kind, string> = { web: "html", ios: "swift", android: "kotlin" }

function AddPropertyForm({ onClose }: { onClose: () => void }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [global] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const zones = useZones()
  const create = useCreateProperty()
  const [kind, setKind] = useState<Kind>("web")
  const [name, setName] = useState("")
  const [domains, setDomains] = useState("")
  const [inbox, setInbox] = useState(global || NO_INBOX)
  const hosts = splitLines(domains).map(hostOnly).filter(Boolean)
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const products = {
    [NO_INBOX]: t`None, owners and admins only`,
    ...Object.fromEntries(inboxes.map((i) => [i.id, i.name])),
  }
  return (
    <FormCard
      title={<Trans>Add a website or app</Trans>}
      description={
        <Trans>
          Count visitors, pages, sources, places and devices without cookies and without storing IP addresses.
        </Trans>
      }
      submit={<Trans>Add and install</Trans>}
      ready={!!name.trim() && hosts.length > 0}
      busy={create.isPending}
      error={create.error}
      onCancel={onClose}
      onSubmit={() =>
        create.mutate(
          {
            name: name.trim(),
            domains: hosts,
            platforms: [kind],
            inbox_id: inbox === NO_INBOX ? undefined : inbox,
            timezone: zones.includes(zone) ? zone : "UTC",
          },
          {
            onSuccess: (p) => navigate(`/analytics/${p.id}/settings?tab=install&snippet=${SNIPPET[kind]}`),
          }
        )
      }
      testId="analytics-new"
    >
      <Field>
        <FieldLabel>
          <Trans>Platform</Trans>
        </FieldLabel>
        <div className="-mx-1 max-w-[calc(100%+0.5rem)] overflow-x-auto px-1 py-1">
          <Segmented
            value={kind}
            onChange={setKind}
            label={t`Platform`}
            className="w-fit"
            items={[
              { value: "web", label: t`Website`, testId: "analytics-kind-web" },
              { value: "ios", label: t`iOS app`, testId: "analytics-kind-ios" },
              { value: "android", label: t`Android app`, testId: "analytics-kind-android" },
            ]}
          />
        </div>
      </Field>
      <FormGrid>
        <Field>
          <FieldLabel htmlFor="analytics-name">
            <Trans>Name</Trans>
          </FieldLabel>
          <Input
            id="analytics-name"
            autoFocus
            value={name}
            maxLength={100}
            onChange={(e) => setName(e.target.value)}
            placeholder={kind === "web" ? t`For example Fieldnote website` : t`For example Fieldnote for iPhone`}
            className="h-9"
            data-testid="analytics-name"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="analytics-domains">
            {kind === "web" ? <Trans>Domains</Trans> : <Trans>Website</Trans>}
          </FieldLabel>
          <Input
            id="analytics-domains"
            dir="ltr"
            value={domains}
            onChange={(e) => setDomains(e.target.value)}
            placeholder="www.example.com"
            className="h-9"
            data-testid="analytics-domains"
          />
          <FieldDescription>
            {kind === "web" ? (
              <Trans>Separate several with spaces. *.example.com covers every subdomain.</Trans>
            ) : (
              <Trans>The app’s own website. Events from the app are not checked against it.</Trans>
            )}
          </FieldDescription>
        </Field>
        {inboxes.length > 0 && (
          <Field>
            <FieldLabel>
              <Trans>Product</Trans>
            </FieldLabel>
            <Select value={inbox} items={products} onValueChange={(v) => v && setInbox(String(v))}>
              <SelectTrigger className="h-9 w-full min-w-0" aria-label={t`Product`} data-testid="analytics-inbox">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_INBOX}>{products[NO_INBOX]}</SelectItem>
                {inboxes.map((i) => (
                  <SelectItem key={i.id} value={i.id}>
                    <Dot color={i.branding.color} className="mt-1.5 size-2 rounded-[3px]" />
                    {i.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription>
              <Trans>Agents of this product can see the numbers.</Trans>
            </FieldDescription>
          </Field>
        )}
      </FormGrid>
    </FormCard>
  )
}

function AnalyticsHome({ adding = false }: { adding?: boolean }) {
  const { list, items } = useScopedProperties()
  const first = items[0]
  if (!adding && first && items.length === 1 && !list.isPending)
    return <Navigate to={`/analytics/${first.id}`} replace />
  return <PropertyList adding={adding} />
}

const head = "h-9 px-3 text-start text-caption font-medium text-muted-foreground"
const cell = "px-3 py-2.5 text-body"

function PropertyList({ adding }: { adding: boolean }) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const { list, items } = useScopedProperties()
  const inboxes = useInboxes().data ?? []
  const colors = new Map(inboxes.map((i) => [i.id, i.branding.color]))
  const now = useNow(60_000)
  const listRef = useRef<HTMLDivElement>(null)
  const open = (id: string) => navigate(`/analytics/${id}`)
  const { cursorId } = useListCursor(items, null, open, !adding, listRef, true, true)
  const add = () => navigate("/analytics/new")
  useHotkeys({ [ANALYTICS_KEYS.add]: add }, canManage && !adding)
  return (
    <AnalyticsLayout testId="analytics-home">
      <section className="@container/list min-w-0">
        <PageTitle
          title={t`Analytics`}
          sub={<Trans>Cookieless visitors across your sites and apps</Trans>}
          actions={
            canManage && (
              <Button
                onClick={add}
                disabled={adding}
                title={keyHint(t`Add a website or app`, ANALYTICS_KEYS.add)}
                data-testid="analytics-home-add"
              >
                <PlusIcon />
                <Trans>Add a website or app</Trans>
              </Button>
            )
          }
          testId="analytics-head"
        />
        {adding && canManage && <AddPropertyForm onClose={() => navigate("/analytics", { replace: true })} />}
        {list.data && <UsageBanner usage={list.data.usage} className="mt-4" />}
        <div ref={listRef} className="mt-4">
          <ListBox testId="analytics-properties">
            {list.isPending ? (
              <ListBoxSkeleton rows={3} />
            ) : list.error ? (
              <ErrorLine error={list.error} className="px-4 py-4" />
            ) : items.length === 0 ? (
              <ListBoxEmpty
                testId="analytics-empty"
                action={
                  canManage &&
                  !adding && (
                    <Button variant="outline" size="sm" onClick={add}>
                      <Trans>Add a website or app</Trans>
                    </Button>
                  )
                }
              >
                <span className="flex max-w-xl flex-col gap-2">
                  <span className="font-semibold text-foreground">
                    <Trans>Count your website’s visitors</Trans>
                  </span>
                  <Trans>
                    Pages, sources, places and devices, without cookies, without storing IP addresses and without a
                    consent banner where the law allows it.
                  </Trans>
                </span>
              </ListBoxEmpty>
            ) : (
              <Table className="table-fixed text-body">
                <TableHeader>
                  <TableRow className="border-line-soft hover:bg-transparent">
                    <TableHead className={cn(head, "ps-4")}>
                      <Trans>Website or app</Trans>
                    </TableHead>
                    <TableHead className={cn(head, "w-36 @max-xl/list:hidden")}>
                      <Trans>Last event</Trans>
                    </TableHead>
                    <TableHead className={cn(head, "w-32 pe-4 text-end")}>
                      <Trans>Visitors today</Trans>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((p) => {
                    const quiet = !p.last_event_at || now - Date.parse(p.last_event_at) > 86_400_000
                    const color = inboxes.length > 1 && p.inbox_id ? colors.get(p.inbox_id) : undefined
                    return (
                      <TableRow
                        key={p.id}
                        onClick={() => open(p.id)}
                        className={cn(
                          "cursor-pointer border-line-soft hover:bg-subtle",
                          cursorId === p.id && "bg-subtle [&>td:first-child]:shadow-[inset_2px_0_0_var(--brand)]"
                        )}
                        data-row-id={p.id}
                        data-testid="analytics-property"
                      >
                        <TableCell className={cn(cell, "min-w-0 overflow-hidden ps-4")}>
                          <Link
                            to={`/analytics/${p.id}`}
                            onClick={(e) => e.stopPropagation()}
                            className="flex min-w-0 flex-col rounded-sm leading-5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          >
                            <span className="flex min-w-0 items-center gap-2">
                              {color && <Dot color={color} className="size-[7px] shrink-0 rounded-[2px]" />}
                              <span className="truncate font-medium text-foreground">{p.name}</span>
                            </span>
                            <span className="truncate text-small text-muted-foreground" dir="ltr">
                              {p.domains.join(", ")}
                            </span>
                          </Link>
                        </TableCell>
                        <TableCell className={cn(cell, "w-36 text-small @max-xl/list:hidden")}>
                          <span className="inline-flex items-center gap-1.5 text-muted-foreground">
                            <span
                              aria-hidden
                              className={cn("size-1.5 shrink-0 rounded-full", quiet ? "bg-warning" : "bg-success")}
                            />
                            {p.last_event_at ? (
                              <Ago iso={p.last_event_at} />
                            ) : (
                              <span data-testid="analytics-quiet">
                                <Trans>No events yet</Trans>
                              </span>
                            )}
                          </span>
                        </TableCell>
                        <TableCell className={cn(cell, "w-32 pe-4 text-end font-medium tabular-nums")}>
                          {formatCompact(p.visitors_today, i18n.locale)}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
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
          ]}
        />
      </section>
    </AnalyticsLayout>
  )
}

const HEADED: ViewKey[] = ["overview", "pages", "goals"]

const pickerClass = "h-8 rounded-md bg-card"

function PropertyLayout() {
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
  const last = pathname.split("/")[3] as ViewKey | undefined
  const current: ViewKey = last && VIEWS.includes(last) ? last : "overview"
  const names = useViewNames()
  const ledes = useViewLedes()
  const p = property.data
  const seenInbox = useRef(inboxId)
  useEffect(() => {
    if (seenInbox.current === inboxId || !p || !list) return
    seenInbox.current = inboxId
    if (!inboxId || p.inbox_id === inboxId) return
    const next = list.items.find((o) => o.inbox_id === inboxId)
    const view = current === "settings" || current === "overview" ? "" : `/${current}`
    navigate(next ? `/analytics/${next.id}${view}${search}` : "/analytics")
  }, [inboxId, p, list, current, search, navigate])
  const back = useDetailBack(current)
  useHotkeys({ Escape: () => back && navigate(back.to) }, !!back)
  if (!p) {
    return (
      <AnalyticsLayout testId="analytics-property">
        <PageTitle title={names[current]} />
        <FilterRow>
          <PropertyPicker current={current} />
        </FilterRow>
        <div className="mt-6">
          {property.error ? <ErrorLine error={property.error} /> : <Skeleton className="h-64 rounded-[10px]" />}
        </div>
      </AnalyticsLayout>
    )
  }
  const controls = current !== "realtime" && current !== "settings" && current !== "retention"
  const headed = HEADED.includes(current)
  const picked = params.get("goal")
  const newGoal = new URLSearchParams(search)
  newGoal.set("goal", "new")
  const action =
    current === "goals"
      ? canManage &&
        picked !== "new" &&
        !back && (
          <Button render={<Link to={`?${newGoal.toString()}`} />} data-testid="analytics-goal-new">
            <PlusIcon />
            <Trans>New goal</Trans>
          </Button>
        )
      : canManage &&
        current !== "pages" &&
        current !== "settings" && (
          <Button
            variant="outline"
            render={<Link to={`/analytics/${p.id}/settings?tab=sharing`} />}
            data-testid="analytics-share"
          >
            <Trans>Share</Trans>
          </Button>
        )
  return (
    <AnalyticsLayout testId="analytics-property">
      <div key={p.id} className="@container flex min-w-0 flex-col">
        {back ? (
          <PageTop back={back} actions={action || undefined} />
        ) : (
          <PageTitle
            title={names[current]}
            sub={ledes[current]}
            actions={action || undefined}
            testId="analytics-head"
          />
        )}
        <FilterRow testId="analytics-toolbar">
          <PropertyPicker property={p} current={current} />
          {controls && (
            <>
              {current !== "goals" && <RangePicker plain className={pickerClass} />}
              {!headed && current !== "events" && current !== "funnels" && <ComparePicker className={pickerClass} />}
              {!headed && current !== "funnels" && <SegmentPicker propertyId={p.id} className={pickerClass} />}
              {headed ? current !== "pages" && <FilterBar /> : current !== "funnels" && <FilterBar />}
            </>
          )}
        </FilterRow>
        <div className="mt-6 flex flex-col gap-4">
          {usage && <UsageBanner usage={usage} />}
          {!p.first_event_at && current !== "settings" && <WaitForFirstEvent propertyId={p.id} />}
          <Outlet context={{ property: p }} />
        </div>
      </div>
    </AnalyticsLayout>
  )
}

function ToInstall() {
  const { propertyId } = useParams()
  return <Navigate to={`/analytics/${propertyId}/settings?tab=install`} replace />
}

export default function AnalyticsRoutes() {
  return (
    <Routes>
      <Route index element={<AnalyticsHome />} />
      <Route path="new" element={<AnalyticsHome adding />} />
      <Route path="new/:propertyId" element={<ToInstall />} />
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
