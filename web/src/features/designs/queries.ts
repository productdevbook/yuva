import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useCallback, useEffect, useMemo } from "react"

import { store } from "@/features/docs/queries"
import {
  api,
  ApiError,
  unwrap,
  type EmailDesign,
  type EmailDesignBulkChange,
  type EmailDesignDocument,
  type EmailDesignIssue,
  type EmailImage,
  type EmailImageBulkChange,
  type EmailTemplateLocale,
} from "@/lib/api"
import { useSession } from "@/lib/session"

export const useShowArchivedDesigns = store(false)

export function designLink(id: string) {
  return `/broadcasts/designs/${id}`
}

const k = {
  all: (ws: string) => ["ws", ws, "designs"] as const,
  templates: (ws: string, locale: string) => ["ws", ws, "designs", "templates", locale] as const,
  pages: (ws: string, inbox: string, q: string, archived: boolean) =>
    ["ws", ws, "designs", "pages", inbox, q, archived] as const,
  one: (ws: string, id: string) => ["ws", ws, "designs", "one", id] as const,
  versions: (ws: string, id: string) => ["ws", ws, "designs", "versions", id] as const,
  version: (ws: string, id: string, v: number) => ["ws", ws, "designs", "version", id, v] as const,
  images: (ws: string) => ["ws", ws, "email-images"] as const,
  render: (ws: string, body: string) => ["ws", ws, "designs", "render", body] as const,
}

export function useTemplates(locale: EmailTemplateLocale) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: k.templates(ws, locale),
    queryFn: () => unwrap(api.GET("/v1/email-templates", { params: { query: { locale } } })).then((r) => r.items),
    staleTime: 10 * 60_000,
  })
}

export function useDesigns(inboxId: string, q: string, archived: boolean, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: k.pages(ws, inboxId, q, archived),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/email-designs", {
          params: {
            query: {
              inbox_id: inboxId || undefined,
              q: q || undefined,
              archived: archived || undefined,
              cursor: pageParam,
              limit: 50,
            },
          },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    placeholderData: keepPreviousData,
    enabled,
  })
}

export function useDesign(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: k.one(ws, id),
    queryFn: () => unwrap(api.GET("/v1/email-designs/{designId}", { params: { path: { designId: id } } })),
    enabled: !!id,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    retry: (n, err) => !(err instanceof ApiError && err.status < 500) && n < 2,
  })
}

export function useRefreshDesigns() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useCallback(
    (d?: EmailDesign) => {
      if (d) qc.setQueryData(k.one(ws, d.id), d)
      void qc.invalidateQueries({ queryKey: [...k.all(ws), "pages"] })
    },
    [qc, ws]
  )
}

export function useCreateDesign() {
  const refresh = useRefreshDesigns()
  return useMutation({
    mutationFn: (body: {
      name: string
      inbox_id?: string
      document?: EmailDesignDocument
      template?: { slug: string; locale?: EmailTemplateLocale; locales?: EmailTemplateLocale[] }
    }) => unwrap(api.POST("/v1/email-designs", { body })),
    onSuccess: (d) => refresh(d),
  })
}

export function useDuplicateDesign() {
  const refresh = useRefreshDesigns()
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(api.POST("/v1/email-designs/{designId}/duplicate", { params: { path: { designId: id } }, body: {} })),
    onSuccess: (d) => refresh(d),
  })
}

export function useArchiveDesign() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/email-designs/{designId}", { params: { path: { designId: id } } })),
    onSuccess: (_, id) => {
      void qc.invalidateQueries({ queryKey: k.one(ws, id) })
      void qc.invalidateQueries({ queryKey: [...k.all(ws), "pages"] })
    },
  })
}

export function useRestoreDesign() {
  const refresh = useRefreshDesigns()
  return useMutation({
    mutationFn: async (id: string) => {
      const d = await unwrap(api.GET("/v1/email-designs/{designId}", { params: { path: { designId: id } } }))
      return saveDesign(id, {
        name: d.name,
        inbox_id: d.inbox_id,
        for_conversations: d.for_conversations,
        document: d.document,
        base_version: d.version,
      })
    },
    onSuccess: (d) => refresh(d),
  })
}

export function saveDesign(
  id: string,
  body: {
    name: string
    inbox_id?: string
    for_conversations?: boolean
    document: EmailDesignDocument
    base_version: number
    snapshot?: boolean
  }
) {
  return unwrap(api.PUT("/v1/email-designs/{designId}", { params: { path: { designId: id } }, body }))
}

export function useVersions(id: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: k.versions(ws, id),
    queryFn: () =>
      unwrap(api.GET("/v1/email-designs/{designId}/versions", { params: { path: { designId: id } } })).then(
        (r) => r.items
      ),
    enabled,
  })
}

export function useVersionDetail(id: string, version: number | null) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: k.version(ws, id, version ?? 0),
    queryFn: () =>
      unwrap(
        api.GET("/v1/email-designs/{designId}/versions/{version}", {
          params: { path: { designId: id, version: version! } },
        })
      ),
    enabled: version !== null,
    staleTime: Infinity,
  })
}

export function useInvalidateVersions(id: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => void qc.invalidateQueries({ queryKey: k.versions(ws, id) })
}

export function restoreVersion(id: string, version: number, base: number) {
  return unwrap(
    api.POST("/v1/email-designs/{designId}/versions/{version}/restore", {
      params: { path: { designId: id, version } },
      body: { base_version: base },
    })
  )
}

export function useImages(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: k.images(ws),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/email-images", { params: { query: { cursor: pageParam, limit: 100 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled,
  })
}

export function useImageMap(wanted: Set<string>) {
  const images = useImages()
  const items = useMemo(() => images.data?.pages.flatMap((p) => p.items) ?? [], [images.data])
  const map = useMemo(() => new Map(items.map((i) => [i.id, i])), [items])
  const missing = [...wanted].some((id) => !map.has(id))
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = images
  useEffect(() => {
    if (missing && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [missing, hasNextPage, isFetchingNextPage, fetchNextPage])
  return map
}

export function useUploadImage() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (file: File) =>
      unwrap(
        api.POST("/v1/email-images", {
          body: { file: file as unknown as string },
          bodySerializer: () => {
            const form = new FormData()
            form.append("file", file)
            return form
          },
        })
      ) as Promise<EmailImage>,
    onSuccess: () => void qc.invalidateQueries({ queryKey: k.images(ws) }),
  })
}

export function useDeleteImage() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: ({ id, force }: { id: string; force?: boolean }) =>
      unwrap(
        api.DELETE("/v1/email-images/{imageId}", {
          params: { path: { imageId: id }, query: { force: force || undefined } },
        })
      ),
    onSuccess: () => void qc.invalidateQueries({ queryKey: k.images(ws) }),
  })
}

export function useBulkDesigns() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: EmailDesignBulkChange) => unwrap(api.POST("/v1/email-designs/bulk", { body })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: k.all(ws) }),
  })
}

export function useBulkImages() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: EmailImageBulkChange) => unwrap(api.POST("/v1/email-images/bulk", { body })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: k.images(ws) }),
  })
}

export type RenderInput = { document: EmailDesignDocument; inbox_id?: string; dark?: boolean; locale?: string }

export function useRender(input: RenderInput | null) {
  const { workspaceId: ws } = useSession()
  const body = input ? JSON.stringify(input) : ""
  return useQuery({
    queryKey: k.render(ws, body),
    queryFn: ({ signal }) =>
      unwrap(api.POST("/v1/email-designs/render", { body: JSON.parse(body) as RenderInput, signal })),
    enabled: !!input,
    placeholderData: keepPreviousData,
    staleTime: Infinity,
    gcTime: 60_000,
    retry: (n, err) => n < 3 && (!(err instanceof ApiError) || err.status === 429 || err.status >= 500),
    retryDelay: (n, err) => (err instanceof ApiError && err.status === 429 ? 15_000 : 1000 * 2 ** n),
  })
}

export function issuesOf(err: unknown): EmailDesignIssue[] {
  if (!(err instanceof ApiError)) return []
  const body = err.body as { errors?: EmailDesignIssue[] } | undefined
  return body?.errors ?? []
}

export function conflictOf(err: unknown): EmailDesign | null {
  if (!(err instanceof ApiError) || err.status !== 409) return null
  return (err.body as { design?: EmailDesign } | undefined)?.design ?? null
}
