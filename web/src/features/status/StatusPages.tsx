import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightIcon, ArrowUpRightIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ErrorLine, InboxMark, useConfirm } from "@/components/common"
import { keyLabel, STATUS_KEYS } from "@/components/common/ShortcutSheet"
import { formatDuration, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { ComponentsTab } from "@/features/status/ComponentsTab"
import { CreatePageDialog } from "@/features/status/CreatePageDialog"
import { Pill } from "@/features/status/PageFrame"
import { PreviewDialog } from "@/features/status/PreviewDialog"
import {
  incidentLink,
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
import { isClosed, STATE_TONE, StateDot, useStatusText } from "@/features/status/text"
import { UptimeBars, usePercent } from "@/features/status/UptimeBars"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Inbox, StatusIncident, StatusPage } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport } from "@/lib/workspace"

const TABS = ["overview", "components", "subscribers", "settings"] as const
type Tab = (typeof TABS)[number]

export function hostOf(url: string) {
  return url.replace(/^https?:\/\//, "").replace(/\/$/, "")
}

function PageCard({ p, inbox, current, tab }: { p: StatusPage; inbox?: Inbox; current: boolean; tab: Tab }) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const subscribed = new Intl.NumberFormat(i18n.locale).format(p.email_subscribers.counts.subscribed)
  return (
    <li className="phone:w-64 phone:shrink-0">
      <Link
        to={statusLink(p.id, tab === "overview" ? undefined : tab)}
        replace
        aria-current={current ? "page" : undefined}
        className={cn(
          "flex w-full flex-col items-start gap-1 rounded-xl border p-3.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
          current ? "border-foreground bg-background" : "bg-surface hover:border-faint"
        )}
        data-testid="status-home-page"
      >
        <span className="flex w-full items-center gap-2">
          <InboxMark name={inbox?.name ?? p.name} color={inbox?.branding.color} className="size-[22px] rounded-[6px]" />
          <span className="min-w-0 flex-1 truncate font-semibold">{p.name}</span>
          <StateDot state={p.state} />
        </span>
        <span className="w-full truncate text-caption text-faint" dir="ltr">
          {hostOf(p.url)}
        </span>
        <span className="flex w-full items-center justify-between gap-2 text-caption">
          <span className={cn("truncate font-medium", STATE_TONE[p.state])}>{text.component[p.state]}</span>
          <span className="shrink-0 text-faint">
            {!p.published ? (
              <Trans>Unpublished</Trans>
            ) : p.email_subscribers.enabled ? (
              <Trans>{subscribed} subscribers</Trans>
            ) : (
              <Trans>Published</Trans>
            )}
          </span>
        </span>
      </Link>
    </li>
  )
}

function Banner({ p, i }: { p: StatusPage; i: StatusIncident }) {
  const { i18n } = useLingui()
  const text = useStatusText()
  const planned = i.state === "scheduled"
  const draft = !i.published_at
  const at = planned && i.scheduled_start ? i.scheduled_start : (i.started_at ?? i.published_at ?? i.created_at)
  const since = formatShort(at, i18n.locale)
  const due = since
  const state = text.incident[i.state]
  return (
    <Link
      to={incidentLink(p.id, i.id)}
      className={cn(
        "flex items-center gap-3 rounded-xl border px-4 py-3.5 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
        planned
          ? "border-blue-600/40 bg-blue-600/5 hover:bg-blue-600/10 dark:border-blue-400/40 dark:bg-blue-400/5"
          : "border-warning bg-warning/8 hover:bg-warning/12"
      )}
      data-testid="status-incident-row"
    >
      <span className="flex min-w-0 flex-1 flex-col leading-[1.35]">
        <span className="truncate font-semibold">{i.title}</span>
        <span className="truncate text-muted-foreground">
          {planned ? (
            <Trans>Scheduled for {due}</Trans>
          ) : (
            <Trans>
              {state} since {since}
            </Trans>
          )}
          {draft && (
            <>
              {" · "}
              <Trans>not posted yet</Trans>
            </>
          )}
        </span>
      </span>
      <span
        className={cn(
          "inline-flex shrink-0 items-center gap-1 font-medium",
          planned ? "text-blue-600 dark:text-blue-400" : "text-warning"
        )}
      >
        {draft ? <Trans>Review and post</Trans> : planned ? <Trans>Open</Trans> : <Trans>Post update</Trans>}
        <ArrowRightIcon className="size-3.5 rtl:rotate-180" />
      </span>
    </Link>
  )
}

function ComponentsCard({ p }: { p: StatusPage }) {
  const text = useStatusText()
  const percent = usePercent()
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
  return (
    <section className="overflow-hidden rounded-xl border bg-card" data-testid="status-components-card">
      <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
        <Link to={statusLink(p.id, "components")} className="font-semibold hover:underline">
          <Trans>Components</Trans>
        </Link>
        <span className="text-caption text-faint">
          <Plural value={days} one="# day" other="# days" />
        </span>
      </div>
      {components.isPending ? (
        <div className="p-4">
          <Skeleton className="h-20 w-full" />
        </div>
      ) : items.length === 0 ? (
        <p className="px-4 py-6 text-center text-faint">
          <Trans>
            No components yet. Add the parts of your product that customers depend on, in the Components tab.
          </Trans>
        </p>
      ) : (
        <ul>
          {items.map((c) => {
            const u = uptime.get(c.id)
            return (
              <li key={c.id} className="flex flex-col gap-2 border-b px-4 py-3 last:border-b-0">
                <span className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate font-medium">{c.name}</span>
                  {p.show_uptime && u?.uptime !== undefined && (
                    <span className="text-caption text-faint">
                      <Trans>{percent(u.uptime)} uptime</Trans>
                    </span>
                  )}
                  <span className={cn("w-auto min-w-21 text-end text-caption font-medium", STATE_TONE[c.state])}>
                    {text.component[c.state]}
                  </span>
                </span>
                {u?.days && u.days.length > 0 ? (
                  <UptimeBars days={u.days} count={days} className="h-[22px] gap-0.5" />
                ) : (
                  <span className="flex h-[22px] gap-0.5" aria-hidden>
                    {Array.from({ length: 45 }, (_, n) => (
                      <span key={n} className="flex-1 rounded-[2px] bg-muted" />
                    ))}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

function Tile({
  label,
  value,
  hint,
  to,
  testId,
}: {
  label: React.ReactNode
  value: string
  hint?: React.ReactNode
  to?: string
  testId?: string
}) {
  const body = (
    <>
      <span className="truncate text-muted-foreground">{label}</span>
      <b className="text-title tabular-nums">{value}</b>
      <span className="truncate text-caption text-faint">{hint ?? " "}</span>
    </>
  )
  const cls = "flex min-w-0 flex-col gap-1 rounded-xl border bg-card p-4"
  return to ? (
    <Link
      to={to}
      className={cn(
        cls,
        "outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring"
      )}
      data-testid={testId}
    >
      {body}
    </Link>
  ) : (
    <div className={cls} data-testid={testId}>
      {body}
    </div>
  )
}

function lasted(i: StatusIncident) {
  const from = Date.parse(i.started_at ?? i.published_at ?? i.created_at)
  const to = i.resolved_at ? Date.parse(i.resolved_at) : Date.now()
  return Math.max(60, Math.round((to - from) / 1000))
}

function Tiles({ p, incidents }: { p: StatusPage; incidents: StatusIncident[] }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const monitors = useStatusMonitors(p.id).data
  const subs = p.email_subscribers
  const down = (monitors ?? []).filter((m) => m.state === "down" && !m.paused).length
  const up = (monitors ?? []).filter((m) => m.state === "up" && !m.paused).length
  const past = incidents
    .filter((i) => i.kind === "incident" && !!i.published_at)
    .sort((a, b) => (b.started_at ?? b.created_at).localeCompare(a.started_at ?? a.created_at))
  const [last, before] = past
  const date = (i: StatusIncident) => formatShort(i.started_at ?? i.created_at, i18n.locale)
  const took = (i: StatusIncident) => formatDuration(lasted(i), i18n.locale)
  const open = !!last && !last.resolved_at
  return (
    <div className="grid grid-cols-1 gap-4 @xl:grid-cols-3" data-testid="status-facts">
      <Tile
        label={<Trans>Subscribers</Trans>}
        value={subs.enabled ? fmt.format(subs.counts.subscribed) : "–"}
        hint={subs.enabled ? <Trans>by e-mail</Trans> : <Trans>E-mail subscriptions are off</Trans>}
        to={statusLink(p.id, "subscribers")}
      />
      <Tile
        label={<Trans>Monitors</Trans>}
        value={monitors ? fmt.format(monitors.length) : "–"}
        to="/monitors"
        hint={
          monitors &&
          monitors.length > 0 &&
          (down > 0 ? (
            <>
              <Plural value={down} one="# down" other="# down" />
              {", "}
              <Plural value={up} one="# up" other="# up" />
            </>
          ) : up === monitors.length ? (
            <Trans>all up</Trans>
          ) : (
            <Plural value={up} one="# up" other="# up" />
          ))
        }
      />
      <Tile
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
    </div>
  )
}

function Overview({ p }: { p: StatusPage }) {
  const { t } = useLingui()
  const text = useStatusText()
  const incidents = usePagesIncidents([p.id], false).data
  const monitors = useStatusMonitors(p.id).data
  const live = incidents.filter((i) => !isClosed(i))
  const headline =
    p.state === "operational"
      ? t`All systems operational`
      : p.state === "degraded" || p.state === "partial_outage"
        ? t`Partly degraded`
        : text.component[p.state]
  const watching = monitors?.length ?? 0
  return (
    <div className="flex flex-col gap-4" data-testid="status-overview">
      <div className="flex items-center gap-3">
        <StateDot state={p.state} className="size-3.5" />
        <span className="flex min-w-0 flex-1 flex-col leading-tight">
          <span className="text-title" data-testid="status-overall">
            {headline}
          </span>
          <span className="truncate text-caption text-faint">
            {watching > 0 ? (
              <Plural value={watching} one="Watched by # monitor" other="Watched by # monitors" />
            ) : (
              <Trans>No monitor watches this page yet</Trans>
            )}
          </span>
        </span>
      </div>
      {live.map((i) => (
        <Banner key={i.id} p={p} i={i} />
      ))}
      <ComponentsCard p={p} />
      <Tiles p={p} incidents={incidents} />
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
  const inbox = current && inboxOf(current)

  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="status-home">
      <PageTop
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
      <div className="flex min-h-0 flex-1 overflow-hidden phone:flex-col phone:overflow-y-auto">
        {pages.isPending ? (
          <div className="w-full p-6">
            <Skeleton className="h-24 w-72 rounded-xl" />
          </div>
        ) : pages.error ? (
          <div className="w-full p-6">
            <ErrorLine error={pages.error} />
          </div>
        ) : items.length === 0 ? (
          <div className="w-full p-6 phone:p-4">
            <div
              className="flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-12 text-center"
              data-testid="status-home-empty"
            >
              <p className="max-w-md text-muted-foreground">
                <Trans>
                  Tell your customers whether your product works. Each product gets one page, where you open incidents,
                  plan maintenance and set the state of each component.
                </Trans>
              </p>
              {missing.length > 0 ? (
                <Button variant="outline" className={look.btnSm} onClick={() => setCreating("")}>
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
          <>
            <div className="flex w-[300px] shrink-0 flex-col gap-2.5 overflow-y-auto border-e p-4 phone:w-full phone:border-e-0 phone:border-b phone:py-3">
              <ul
                className="flex flex-col gap-2.5 phone:flex-row phone:overflow-x-auto phone:pb-1"
                aria-label={t`Status pages`}
              >
                {items.map((p) => (
                  <PageCard key={p.id} p={p} inbox={inboxOf(p)} current={p.id === current?.id} tab={tab} />
                ))}
              </ul>
              {!phone &&
                missing.map((i) => {
                  const product = i.name
                  return (
                    <Button
                      key={i.id}
                      variant="outline"
                      className="h-auto justify-start gap-2 border-dashed px-3.5 py-3 font-normal text-muted-foreground"
                      onClick={() => setCreating(i.id)}
                      data-testid="create-status-page-open"
                    >
                      <PlusIcon />
                      <span className="min-w-0 truncate">
                        <Trans>Create status page for {product}</Trans>
                      </span>
                    </Button>
                  )
                })}
            </div>
            {current && (
              <section
                key={current.id}
                className="@container flex min-w-0 flex-1 flex-col overflow-y-auto"
                data-testid="status-page"
              >
                <header className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b px-6 py-3 phone:px-4">
                  <span className="flex min-w-0 flex-1 basis-56 items-center gap-2.5">
                    <InboxMark
                      name={inbox?.name ?? current.name}
                      color={inbox?.branding.color}
                      className="size-6 text-caption"
                    />
                    <span className="flex min-w-0 flex-col leading-[1.3]">
                      <span className="flex items-center gap-2">
                        <span className="truncate font-semibold" data-testid="status-page-name">
                          {current.name}
                        </span>
                        <Pill tone={current.published ? "success" : "muted"} data-testid="status-published">
                          {current.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
                        </Pill>
                      </span>
                      <span className={cn(look.faint, "truncate")} dir="ltr">
                        {hostOf(current.url)}
                        {!current.published && (
                          <>
                            {" · "}
                            <Trans>only your team can preview it</Trans>
                          </>
                        )}
                      </span>
                    </span>
                  </span>
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
                </header>
                <ErrorLine error={patch.error} className="mx-6 mt-3" />
                <div className="flex max-w-[1040px] flex-col gap-4 p-6 pb-16 phone:px-4 phone:pt-4">
                  {tab === "overview" && <Overview p={current} />}
                  {tab === "components" && <ComponentsTab page={current} />}
                  {tab === "subscribers" && <SubscribersTab page={current} />}
                  {tab === "settings" && canManage && <SettingsTab page={current} />}
                </div>
              </section>
            )}
          </>
        )}
      </div>
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
