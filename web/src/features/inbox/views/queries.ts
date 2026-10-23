import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"

import { api, unwrap, type SavedView, type SavedViewCreate, type SavedViewUpdate } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

const REFRESH_MS = 4000

export function useViews(teamId?: string, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: teamId ? keys.teamViews(ws, teamId) : keys.views(ws),
    queryFn: () => unwrap(api.GET("/v1/views", { params: { query: { team_id: teamId } } })).then((r) => r.items),
    staleTime: 15_000,
    enabled,
  })
}

export function useView(id: string | null) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.view(ws, id ?? ""),
    queryFn: () => unwrap(api.GET("/v1/views/{viewId}", { params: { path: { viewId: id! } } })),
    enabled: !!id,
    staleTime: 15_000,
    retry: false,
  })
}

export function useViewCountRefresh() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  useEffect(() => {
    let timer: number | undefined
    const stop = qc.getQueryCache().subscribe((event) => {
      if (event.type !== "updated" || event.action.type !== "success" || timer !== undefined) return
      const [scope, id, kind] = event.query.queryKey
      if (scope !== "ws" || id !== ws || (kind !== "conversations" && kind !== "counts")) return
      timer = window.setTimeout(() => {
        timer = undefined
        void qc.invalidateQueries({ queryKey: keys.views(ws) })
      }, REFRESH_MS)
    })
    return () => {
      stop()
      window.clearTimeout(timer)
    }
  }, [qc, ws])
}

export function useViewMutations() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = () => qc.invalidateQueries({ queryKey: keys.views(ws) })
  const create = useMutation({
    mutationFn: (body: SavedViewCreate) => unwrap(api.POST("/v1/views", { body })),
    onSuccess: refresh,
  })
  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: SavedViewUpdate }) =>
      unwrap(api.PATCH("/v1/views/{viewId}", { params: { path: { viewId: id } }, body })),
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/views/{viewId}", { params: { path: { viewId: id } } })),
    onSuccess: refresh,
  })
  const reorder = useMutation({
    mutationFn: ({ ids }: { ids: string[]; key: readonly unknown[] }) =>
      unwrap(api.PUT("/v1/views/order", { body: { ids } })),
    onMutate: ({ ids, key }) => {
      qc.setQueryData<SavedView[]>(key, (old) =>
        old?.map((v) => (ids.includes(v.id) ? { ...v, position: ids.indexOf(v.id) } : v))
      )
    },
    onSettled: refresh,
  })
  return { create, update, remove, reorder }
}

export function canEditView(view: SavedView, memberId: string, manager: boolean) {
  return view.visibility === "personal" ? view.owner_member_id === memberId : manager
}

export function byPosition(a: SavedView, b: SavedView) {
  return a.position - b.position || a.name.localeCompare(b.name)
}
