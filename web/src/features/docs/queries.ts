import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useSyncExternalStore } from "react"

import { api, unwrap, type DocsPageSort } from "@/lib/api"
import { keys, type DocsDays, type PageAnswerFilters } from "@/lib/keys"
import { useSession } from "@/lib/session"

export const WINDOWS: DocsDays[] = [7, 30, 90]

export const SORTS: DocsPageSort[] = ["down", "up", "helpful", "activity"]

function store<T>(initial: T) {
  let value = initial
  const listeners = new Set<() => void>()
  const subscribe = (l: () => void) => {
    listeners.add(l)
    return () => listeners.delete(l)
  }
  const set = (v: T) => {
    value = v
    listeners.forEach((l) => l())
  }
  return () => [useSyncExternalStore(subscribe, () => value), set] as const
}

export const useDocsDays = store<DocsDays>(30)
export const useDocsSort = store<DocsPageSort>("down")

export function docsPageLink(inboxId: string, page: string) {
  return `/docs/page?${new URLSearchParams({ inbox: inboxId, url: page })}`
}

export function useDocsPages(inboxId: string, d: DocsDays, sort: DocsPageSort) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.docsPages(ws, inboxId, d, sort),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/docs/pages", { params: { query: { inbox_id: inboxId || undefined, days: d, sort, cursor: pageParam, limit: 100 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useDocsSummary(inboxId: string, d: DocsDays) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.docsSummary(ws, inboxId, d),
    queryFn: () => unwrap(api.GET("/v1/docs/summary", { params: { query: { inbox_id: inboxId || undefined, days: d } } })),
  })
}

export function useDocsPage(inboxId: string, page: string, d: DocsDays) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.docsPage(ws, inboxId, page, d),
    queryFn: () => unwrap(api.GET("/v1/docs/page", { params: { query: { inbox_id: inboxId, page, days: d } } })),
    enabled: !!inboxId && !!page,
  })
}

export function usePageAnswers(f: PageAnswerFilters, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.pageAnswers(ws, f),
    queryFn: ({ pageParam }) => unwrap(api.GET("/v1/page-answers", { params: { query: { ...f, cursor: pageParam, limit: 100 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled,
  })
}

function useRefreshDocs() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => qc.invalidateQueries({ queryKey: keys.docs(ws) })
}

export type AnswerText = { question: string; answer: string }

export function usePublish(conversationId: string) {
  const refresh = useRefreshDocs()
  return useMutation({
    mutationFn: (body: AnswerText) => unwrap(api.POST("/v1/conversations/{conversationId}/publish", { params: { path: { conversationId } }, body })),
    onSuccess: refresh,
  })
}

export function useUpdateAnswer(id: string) {
  const refresh = useRefreshDocs()
  return useMutation({
    mutationFn: (body: AnswerText) => unwrap(api.PATCH("/v1/page-answers/{pageAnswerId}", { params: { path: { pageAnswerId: id } }, body })),
    onSuccess: refresh,
  })
}

export function useUnpublish() {
  const refresh = useRefreshDocs()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/page-answers/{pageAnswerId}", { params: { path: { pageAnswerId: id } } })),
    onSuccess: refresh,
  })
}
