import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ListFilterIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useLocation, useSearchParams } from "react-router"

import { groupLabelClass, rowClass } from "@/app/Sidebar"
import { inboxRoot, isChats } from "@/app/shell"
import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import {
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"
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
  return (
    <SidebarMenuBadge
      className="top-1.5 right-auto end-2.5 font-mono text-caption font-normal text-sidebar-muted in-data-[mobile=true]:top-3"
      data-testid="view-count"
    >
      {view.count_capped ? "1000+" : view.count}
    </SidebarMenuBadge>
  )
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
  const { setOpenMobile } = useSidebar()
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
      data-testid="view-item"
    >
      <SidebarMenuButton
        render={<Link to={to} draggable={false} />}
        isActive={active}
        aria-current={active ? "page" : undefined}
        title={writable ? `${view.name} · ${reorderTitle}` : view.name}
        className={cn(rowClass, "h-8 pe-14 [&_svg]:size-[15px] [&_svg]:text-sidebar-muted")}
        onClick={() => setOpenMobile(false)}
        onKeyDown={(e) => {
          if (!writable || !e.altKey || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return
          e.preventDefault()
          e.stopPropagation()
          onMove(view.id, e.key === "ArrowUp" ? -1 : 1)
        }}
        data-rail-item
        data-view-id={view.id}
        data-testid="view-link"
      >
        <ListFilterIcon />
        <span className="min-w-0 flex-1 truncate">{view.name}</span>
      </SidebarMenuButton>
      <CountBadge view={view} />
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
      className="mt-2 flex flex-col gap-px group-data-[collapsible=icon]:hidden"
      role="group"
      aria-label={t`Saved views`}
      data-testid="views-group"
    >
      {sections.map((s) => (
        <SidebarMenu key={s.key} className="gap-px" data-testid="views-section">
          <SidebarGroupLabel render={<li />} className={cn(groupLabelClass, "block truncate pt-1.5")}>
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
        <SidebarMenu className="gap-px">
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <SidebarMenuButton className="h-[30px] rounded-lg px-2.5 text-small text-sidebar-muted in-data-[mobile=true]:h-11 in-data-[mobile=true]:text-base" />
                }
                data-rail-item
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
