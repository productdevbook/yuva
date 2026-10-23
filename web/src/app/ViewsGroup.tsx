import { Trans, useLingui } from "@lingui/react/macro"
import { BookmarkIcon, CheckIcon, PlusIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useLocation, useSearchParams } from "react-router"

import { inboxRoot, isChats } from "@/app/shell"
import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import {
  SidebarGroup,
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
      className="right-1.5 in-data-[mobile=true]:top-3.5! in-data-[mobile=true]:right-3 in-data-[mobile=true]:text-xs text-sidebar-muted"
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
  const { canManage } = useSession()
  const { setOpenMobile } = useSidebar()
  const to = { pathname: inboxRoot(canManage), search: viewSearch(view.id, view.filters) }
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
        className="h-8 gap-2.5 px-2.5 pe-11 text-body in-data-[mobile=true]:h-12 in-data-[mobile=true]:gap-3.5 in-data-[mobile=true]:px-3 in-data-[mobile=true]:text-base in-data-[mobile=true]:[&_svg]:size-5 text-sidebar-foreground data-active:bg-sidebar-primary data-active:font-medium data-active:text-sidebar-primary-foreground data-active:hover:bg-sidebar-primary data-active:hover:text-sidebar-primary-foreground [&_svg]:size-4.5"
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
        <BookmarkIcon />
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
  const { canManage } = useSession()
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
  const activeId = isChats(pathname, canManage) ? params.get("view") : null
  return (
    <SidebarGroup
      className="px-3 py-1 group-data-[collapsible=icon]:hidden"
      role="group"
      aria-label={t`Saved views`}
      data-testid="views-group"
    >
      <SidebarGroupLabel className="h-7 px-2.5 text-[11px] tracking-[0.06em] text-sidebar-muted uppercase">
        <Trans>Saved views</Trans>
      </SidebarGroupLabel>
      {sections.map((s, i) => (
        <SidebarMenu key={s.key} className="gap-px" data-testid="views-section">
          {(i > 0 || sections.length > 1) && (
            <li className="truncate px-2.5 pt-1.5 pb-0.5 text-caption font-medium text-sidebar-muted">{s.title}</li>
          )}
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
                  <SidebarMenuButton
                    className={cn(
                      "h-8 gap-2.5 px-2.5 text-body text-sidebar-muted in-data-[mobile=true]:h-12 in-data-[mobile=true]:text-base [&_svg]:size-4.5"
                    )}
                  />
                }
                data-rail-item
                data-testid="views-browse"
              >
                <PlusIcon />
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
    </SidebarGroup>
  )
}
