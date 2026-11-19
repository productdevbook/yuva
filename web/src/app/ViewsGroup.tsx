import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ListFilterIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useLocation, useSearchParams } from "react-router"

import { inboxRoot, isChats } from "@/app/shell"
import { toast } from "@/components/common"
import {
  PlaceCount,
  placeButtonClass,
  placeItemClass,
  placeLabelClass,
  placeMenuClass,
} from "@/components/common/SectionSidebar"
import { useErrorText } from "@/components/common/text"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"
import { viewSearch } from "@/features/inbox/views/filters"
import { byPosition, useViewCountRefresh, useViewMutations, useViews } from "@/features/inbox/views/queries"
import type { SavedView } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useMyTeams, useTeams } from "@/lib/workspace"

type Section = { key: string; title: string; views: SavedView[]; writable: boolean; queryKey: readonly unknown[] }

function CountBadge({ view }: { view: SavedView }) {
  if (view.count === undefined || view.count_unavailable) return null
  if (view.count_capped)
    return (
      <span className="ms-auto shrink-0 ps-2 text-caption font-medium text-muted-foreground" data-testid="view-count">
        1000+
      </span>
    )
  return <PlaceCount n={view.count} testId="view-count" />
}

function ViewRow({
  view,
  active,
  writable,
  reorderTitle,
  onDrag,
  onDrop,
  onMove,
}: {
  view: SavedView
  active: boolean
  writable: boolean
  reorderTitle: string
  onDrag: (id: string | null) => void
  onDrop: (id: string) => void
  onMove: (id: string, by: -1 | 1) => void
}) {
  const { apart } = useSession()
  const to = { pathname: inboxRoot(apart), search: viewSearch(view.id, view.filters) }
  return (
    <SidebarMenuItem
      draggable={writable}
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = "move"
        onDrag(view.id)
      }}
      onDragEnd={() => onDrag(null)}
      onDragOver={(e) => writable && e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault()
        onDrop(view.id)
      }}
      className={placeItemClass}
      data-testid="view-item"
    >
      <SidebarMenuButton
        render={<Link to={to} draggable={false} />}
        isActive={active}
        aria-current={active ? "page" : undefined}
        title={writable ? `${view.name} · ${reorderTitle}` : view.name}
        className={cn(placeButtonClass, "[&_svg]:size-[15px] [&_svg]:text-muted-foreground")}
        onKeyDown={(e) => {
          if (!writable || !e.altKey || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return
          e.preventDefault()
          e.stopPropagation()
          onMove(view.id, e.key === "ArrowUp" ? -1 : 1)
        }}
        data-place
        data-view-id={view.id}
        data-testid="view-link"
      >
        <ListFilterIcon />
        <span className="min-w-0 flex-1 truncate">{view.name}</span>
        <CountBadge view={view} />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

export function ViewsGroup() {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const { canManage, apart } = useSession()
  const { workspaceId: ws } = useSession()
  const teams = useTeams().data ?? []
  const mine = useMyTeams()
  const [browse, setBrowse] = useState("")
  const views = useViews()
  const other = useViews(browse || undefined, !!browse)
  const { reorder } = useViewMutations()
  const [dragging, setDragging] = useState<string | null>(null)
  const refocus = useRef<string | null>(null)
  useViewCountRefresh()
  useEffect(() => {
    const id = refocus.current
    refocus.current = null
    if (id) document.querySelector<HTMLElement>(`[data-view-id="${id}"]`)?.focus()
  })

  const all = [...(views.data ?? [])].sort(byPosition)
  const sections: Section[] = [
    {
      key: "personal",
      title: t`My views`,
      views: all.filter((v) => v.visibility === "personal"),
      writable: true,
      queryKey: keys.views(ws),
    },
    {
      key: "workspace",
      title: t`Workspace`,
      views: all.filter((v) => v.visibility === "workspace"),
      writable: canManage,
      queryKey: keys.views(ws),
    },
    ...teams
      .filter((x) => all.some((v) => v.team_id === x.id))
      .map((x) => ({
        key: `team:${x.id}`,
        title: x.name,
        views: all.filter((v) => v.team_id === x.id),
        writable: canManage,
        queryKey: keys.views(ws),
      })),
    ...(browse
      ? [
          {
            key: `team:${browse}`,
            title: teams.find((x) => x.id === browse)?.name ?? "",
            views: [...(other.data ?? [])].sort(byPosition),
            writable: canManage,
            queryKey: keys.teamViews(ws, browse),
          },
        ]
      : []),
  ].filter((s) => s.views.length > 0)

  const place = (s: Section, id: string, at: number) => {
    const ids = s.views.map((v) => v.id).filter((x) => x !== id)
    ids.splice(Math.max(0, Math.min(at, ids.length)), 0, id)
    reorder.mutate({ ids, key: s.queryKey }, { onError: (e) => toast(errorText(e)) })
  }
  const move = (s: Section, id: string, by: -1 | 1) => {
    const at = s.views.findIndex((v) => v.id === id)
    if (at + by < 0 || at + by >= s.views.length) return
    refocus.current = id
    place(s, id, at + by)
  }
  const drop = (s: Section, target: string) => {
    if (!dragging || dragging === target || !s.views.some((v) => v.id === dragging)) return
    place(
      s,
      dragging,
      s.views.findIndex((v) => v.id === target)
    )
  }

  const strangers = canManage ? teams.filter((x) => !mine.some((m) => m.id === x.id)) : []
  if (sections.length === 0 && strangers.length === 0) return null
  const activeId = isChats(pathname, apart) ? params.get("view") : null
  return (
    <div
      className="flex flex-col max-[899px]:contents"
      role="group"
      aria-label={t`Saved views`}
      data-testid="views-group"
    >
      {sections.map((s) => (
        <SidebarMenu key={s.key} className={placeMenuClass} data-testid="views-section">
          <SidebarGroupLabel render={<li />} className={cn(placeLabelClass, "block truncate")}>
            {s.title}
          </SidebarGroupLabel>
          {s.views.map((v) => (
            <ViewRow
              key={v.id}
              view={v}
              active={v.id === activeId}
              writable={s.writable}
              reorderTitle={t`Reorder with Alt ↑ ↓ or by dragging`}
              onDrag={setDragging}
              onDrop={(target) => drop(s, target)}
              onMove={(id, by) => move(s, id, by)}
            />
          ))}
        </SidebarMenu>
      ))}
      {strangers.length > 0 && (
        <SidebarMenu className={placeMenuClass}>
          <SidebarMenuItem className={placeItemClass}>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<SidebarMenuButton className={cn(placeButtonClass, "text-muted-foreground")} />}
                data-place
                data-testid="views-browse"
              >
                <span className="min-w-0 flex-1 truncate">
                  <Trans>Another team’s views</Trans>
                </span>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
                {strangers.map((x) => (
                  <DropdownMenuItem key={x.id} onClick={() => setBrowse(browse === x.id ? "" : x.id)}>
                    <span className="min-w-0 flex-1 truncate">{x.name}</span>
                    <CheckIcon className={cn("size-3.5", browse !== x.id && "invisible")} />
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      )}
    </div>
  )
}
