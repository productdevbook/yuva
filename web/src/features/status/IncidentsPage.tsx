import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router"

import { Dot, ErrorLine, InfoColumn, PageLayout, PageTop } from "@/components/common"
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
import { INCIDENT_KEYS, keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDuration, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useListCursor } from "@/features/inbox/listCursor"
import { IncidentDetail, incidentTone } from "@/features/status/IncidentDetail"
import { IncidentForm } from "@/features/status/IncidentForm"
import { Pill } from "@/features/status/PageFrame"
import { incidentLink } from "@/features/status/queries"
import { useScopeIncidents, useStatusScope } from "@/features/status/scope"
import { isClosed, useStatusText } from "@/features/status/text"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Inbox, StatusIncident, StatusIncidentKind, StatusPage } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export const VIEWS = ["active", "scheduled", "past"] as const
export type View = (typeof VIEWS)[number]

export function viewOf(i: StatusIncident): View {
  if (isClosed(i)) return "past"
  return i.state === "scheduled" ? "scheduled" : "active"
}

const dotTone = { success: "bg-success", info: "bg-blue-600 dark:bg-blue-400", warning: "bg-warning" }

function useMeta() {
  const { i18n } = useLingui()
  const text = useStatusText()
  return (i: StatusIncident) => {
    const parts: string[] = []
    if (i.kind === "maintenance" && i.scheduled_start && i.scheduled_end) {
      const time = new Intl.DateTimeFormat(i18n.locale, { timeStyle: "short" })
      parts.push(
        `${formatShort(i.scheduled_start, i18n.locale)} · ${time.format(new Date(i.scheduled_start))}–${time.format(new Date(i.scheduled_end))}`
      )
    } else {
      const from = Date.parse(i.started_at ?? i.published_at ?? i.created_at)
      parts.push(formatShort(new Date(from).toISOString(), i18n.locale))
      if (i.resolved_at)
        parts.push(formatDuration(Math.max(60, Math.round((Date.parse(i.resolved_at) - from) / 1000)), i18n.locale))
      else parts.push(text.incident[i.state])
    }
    return parts.join(" · ")
  }
}

function Row({
  i,
  page,
  inbox,
  meta,
  cursor,
  onOpen,
}: {
  i: StatusIncident
  page?: StatusPage
  inbox?: Inbox
  meta: string
  cursor: boolean
  onOpen: () => void
}) {
  const text = useStatusText()
  const updates = i.updates.length
  return (
    <div data-row data-row-id={i.id} className={boxRowClass({ cursor })}>
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-center gap-3 py-3 pe-3.5 text-start outline-none"
        data-testid="status-incident-row"
      >
        <span aria-hidden className={cn("size-2 shrink-0 rounded-full", dotTone[incidentTone(i)])} />
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="truncate text-body font-semibold">{i.title}</span>
          <span className="flex min-w-0 items-center gap-1.5 truncate text-small text-muted-foreground">
            {page && (
              <>
                <Dot color={inbox?.branding.color} className="size-[7px] shrink-0 rounded-[2px]" />
                <span className="shrink-0">{inbox?.name ?? page.name}</span>
                <span aria-hidden>·</span>
              </>
            )}
            <span className="truncate">
              {meta} · <Plural value={updates} one="# update" other="# updates" />
            </span>
          </span>
        </span>
        {!i.published_at && (
          <Pill tone="muted" data-testid="status-incident-draft-badge">
            <Trans>Draft</Trans>
          </Pill>
        )}
        <Pill tone={incidentTone(i)}>{text.incident[i.state]}</Pill>
      </button>
    </div>
  )
}

export function IncidentsPage() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { incidentId = "" } = useParams()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const { items: pages, pages: pagesQuery, inboxOf } = useStatusScope()
  const { chosen, set } = useProductScope()
  const incidents = useScopeIncidents(pages)
  const meta = useMeta()
  const [creating, setCreating] = useState<{ kind: StatusIncidentKind } | null>(
    params.get("new") === "1" ? { kind: "incident" } : null
  )
  const [kept, setKept] = useState<{ id: string; view: View } | null>(null)
  const byPage = new Map(pages.map((p) => [p.id, p]))
  const all = incidents.data.filter((i) => byPage.has(i.page_id))
  const selected = all.find((i) => i.id === incidentId)
  const raw = params.get("view") as View | null
  const view: View =
    raw && VIEWS.includes(raw)
      ? raw
      : kept && kept.id === incidentId
        ? kept.view
        : selected
          ? viewOf(selected)
          : "active"
  const counts = { active: 0, scheduled: 0 }
  for (const i of all) {
    const v = viewOf(i)
    if (v !== "past") counts[v]++
  }
  const rows = all
    .filter((i) => !chosen || byPage.get(i.page_id)?.inbox_id === chosen)
    .filter((i) => viewOf(i) === view || (kept?.view === view && kept.id === i.id))
    .sort((a, b) =>
      view === "scheduled"
        ? (a.scheduled_start ?? a.created_at).localeCompare(b.scheduled_start ?? b.created_at)
        : (b.started_at ?? b.published_at ?? b.created_at).localeCompare(a.started_at ?? a.published_at ?? a.created_at)
    )
  const listRef = useRef<HTMLDivElement>(null)
  const loading = pagesQuery.isPending || (pages.length > 0 && incidents.isPending)
  const incidentKey = keyLabel(INCIDENT_KEYS.incident).join("")
  const maintenanceKey = keyLabel(INCIDENT_KEYS.maintenance).join("")
  const canCreate = pages.length > 0
  const to = (v: View) => (v === "active" ? "/incidents" : `/incidents?view=${v}`)
  const openRow = (id: string) => {
    const i = rows.find((x) => x.id === id)
    if (i) navigate(incidentLink(i.page_id, i.id, view))
  }
  const listing = !incidentId
  const { cursorId } = useListCursor(rows, null, openRow, listing && !creating, listRef, true, true)
  const step = (by: number) => {
    const at = selected ? rows.findIndex((i) => i.id === selected.id) : -1
    const next = rows[Math.min(rows.length - 1, Math.max(0, at + by))]
    if (next) navigate(incidentLink(next.page_id, next.id, view), { replace: true })
  }
  useHotkeys(
    {
      ...(incidentId
        ? {
            [INCIDENT_KEYS.previous]: () => step(-1),
            [INCIDENT_KEYS.next]: () => step(1),
            Escape: () => navigate(to(view)),
          }
        : {}),
      [INCIDENT_KEYS.active]: () => navigate(to("active")),
      [INCIDENT_KEYS.scheduled]: () => navigate(to("scheduled")),
      [INCIDENT_KEYS.past]: () => navigate(to("past")),
      ...(canCreate && listing
        ? {
            [INCIDENT_KEYS.incident]: () => setCreating({ kind: "incident" }),
            [INCIDENT_KEYS.maintenance]: () => setCreating({ kind: "maintenance" }),
          }
        : {}),
    },
    !creating
  )

  const viewNames: Record<View, string> = { active: t`Active`, scheduled: t`Scheduled`, past: t`Past` }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const emptyText = {
    active: t`Nothing is wrong right now. Open an incident when something breaks.`,
    scheduled: t`No maintenance is planned.`,
    past: t`Nothing has been resolved yet.`,
  }[view]

  if (incidentId) {
    const back = { label: t`Incidents`, to: to(view) }
    if (loading)
      return (
        <PageLayout testId="status-incidents" label={t`Incidents`}>
          <PageTop back={back} />
          <div className="mt-3 flex max-w-[760px] flex-col gap-4">
            <Skeleton className="h-8 w-60" />
            <Skeleton className="h-24 w-full" />
          </div>
        </PageLayout>
      )
    const page = selected && byPage.get(selected.page_id)
    if (!selected || !page)
      return (
        <PageLayout testId="status-incidents" label={t`Incidents`}>
          <PageTop back={back} />
          <ListBox>
            <ListBoxEmpty>
              <ErrorLine error={pagesQuery.error ?? incidents.error} />
              <Trans>This incident no longer exists or you cannot see it.</Trans>
            </ListBoxEmpty>
          </ListBox>
        </PageLayout>
      )
    return (
      <IncidentDetail
        key={selected.id}
        page={page}
        i={selected}
        justResolved={kept?.id === selected.id}
        onClosed={(id) => setKept({ id, view })}
        onDeleted={() => navigate(to(view), { replace: true })}
        layout={({ actions, body, aside }) => (
          <PageLayout
            testId="status-incidents"
            label={selected.title}
            aside={<InfoColumn label={t`Details`}>{aside}</InfoColumn>}
          >
            <PageTop back={back} actions={<div className="flex flex-wrap items-center gap-1.5">{actions}</div>} />
            <div className="@container max-w-[760px]">{body}</div>
          </PageLayout>
        )}
      />
    )
  }

  return (
    <PageLayout testId="status-incidents" label={t`Incidents`}>
      <section className="flex min-w-0 flex-col">
        <PageTitle
          title={<Trans>Incidents</Trans>}
          sub={<Trans>What went wrong and what you told people</Trans>}
          actions={
            canCreate && (
              <>
                <Button
                  variant="outline"
                  onClick={() => setCreating({ kind: "maintenance" })}
                  disabled={!!creating}
                  title={t`Plan maintenance (${maintenanceKey})`}
                  data-testid="status-new-maintenance"
                >
                  <Trans>Plan maintenance</Trans>
                </Button>
                <Button
                  onClick={() => setCreating({ kind: "incident" })}
                  disabled={!!creating}
                  title={t`New incident (${incidentKey})`}
                  data-testid="incidents-new"
                >
                  <PlusIcon />
                  <Trans>New incident</Trans>
                </Button>
              </>
            )
          }
          testId="incidents-head"
        />
        {creating && (
          <IncidentForm
            key={creating.kind}
            pages={chosen ? pages.filter((p) => p.inbox_id === chosen) : pages}
            kind={creating.kind}
            onClose={() => setCreating(null)}
            onDone={(i) => {
              setCreating(null)
              navigate(incidentLink(i.page_id, i.id, viewOf(i)))
            }}
          />
        )}
        {pages.length > 0 && (
          <FilterRow testId="incidents-filters">
            <FilterMenu
              value={view === "active" ? "" : view}
              onChange={(v) => navigate(to((v || "active") as View), { replace: true })}
              options={[
                { value: "scheduled", label: `${viewNames.scheduled} · ${fmt.format(counts.scheduled)}` },
                { value: "past", label: viewNames.past },
              ]}
              anyLabel={`${viewNames.active} · ${fmt.format(counts.active)}`}
              label={t`Show`}
              testId="incidents-view"
            />
            <ProductFilter value={chosen} onChange={(id) => set("inbox", id)} testId="incidents-product" />
          </FilterRow>
        )}
        <ErrorLine error={pagesQuery.error ?? incidents.error} className="mt-3" />
        <div ref={listRef}>
          <ListBox testId="status-incident-list">
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
                <Trans>Incidents live on a status page. Create one first.</Trans>
              </ListBoxEmpty>
            ) : rows.length === 0 ? (
              <ListBoxEmpty
                testId="status-incidents-empty"
                action={
                  view !== "past" &&
                  !creating && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setCreating({ kind: view === "scheduled" ? "maintenance" : "incident" })}
                    >
                      {view === "scheduled" ? <Trans>Plan maintenance</Trans> : <Trans>New incident</Trans>}
                    </Button>
                  )
                }
              >
                {emptyText}
              </ListBoxEmpty>
            ) : (
              rows.map((i) => {
                const page = byPage.get(i.page_id)
                return (
                  <Row
                    key={i.id}
                    i={i}
                    page={page}
                    inbox={inboxOf(page)}
                    meta={meta(i)}
                    cursor={i.id === cursorId}
                    onOpen={() => openRow(i.id)}
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
            [[INCIDENT_KEYS.incident], <Trans key="new">new incident</Trans>],
            [[INCIDENT_KEYS.maintenance], <Trans key="plan">plan maintenance</Trans>],
          ]}
        />
      </section>
    </PageLayout>
  )
}
