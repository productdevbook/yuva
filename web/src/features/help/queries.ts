import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"

import {
  api,
  ApiError,
  unwrap,
  type HelpArticle,
  type HelpArticleCreate,
  type HelpArticleTranslation,
  type HelpArticleUpdate,
  type HelpCenterCreate,
  type HelpCenterUpdate,
  type HelpCollectionCreate,
  type HelpCollectionUpdate,
  type HelpImportCreate,
} from "@/lib/api"
import { keys, type HelpArticleFilters } from "@/lib/keys"
import { useRealtimeStatus } from "@/lib/realtime"
import { useSession } from "@/lib/session"

export const centerLink = (centerId: string, tab?: string) => `/knowledge/${centerId}${tab ? `?tab=${tab}` : ""}`

export const articleLink = (centerId: string, articleId: string, locale?: string) =>
  `/knowledge/${centerId}/articles/${articleId}${locale ? `?locale=${encodeURIComponent(locale)}` : ""}`

function useRefresh() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => qc.invalidateQueries({ queryKey: keys.help(ws) })
}

export function useHelpCenters(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.helpCenters(ws),
    queryFn: () => unwrap(api.GET("/v1/help-centers")).then((r) => r.items),
    enabled,
    retry: false,
  })
}

export function useHelpCenter(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.helpCenter(ws, id),
    queryFn: () => unwrap(api.GET("/v1/help-centers/{centerId}", { params: { path: { centerId: id } } })),
    enabled: !!id,
  })
}

export function useCreateHelpCenter() {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: HelpCenterCreate) => unwrap(api.POST("/v1/help-centers", { body })),
    onSuccess: refresh,
  })
}

export function useUpdateHelpCenter(id: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: HelpCenterUpdate) =>
      unwrap(api.PATCH("/v1/help-centers/{centerId}", { params: { path: { centerId: id } }, body })),
    onSuccess: refresh,
  })
}

export function useDeleteHelpCenter(id: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/help-centers/{centerId}", { params: { path: { centerId: id } } })),
    onSuccess: () => {
      qc.removeQueries({ predicate: (q) => q.queryKey.includes(id) })
      return qc.invalidateQueries({ queryKey: keys.helpCenters(ws) })
    },
  })
}

export function useHelpCollections(centerId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.helpCollections(ws, centerId),
    queryFn: () =>
      unwrap(api.GET("/v1/help-centers/{centerId}/collections", { params: { path: { centerId } } })).then(
        (r) => r.items
      ),
    enabled: !!centerId,
  })
}

export function useCollectionMutations(centerId: string) {
  const refresh = useRefresh()
  const path = { centerId }
  return {
    create: useMutation({
      mutationFn: (body: HelpCollectionCreate) =>
        unwrap(api.POST("/v1/help-centers/{centerId}/collections", { params: { path }, body })),
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: (v: { id: string; body: HelpCollectionUpdate }) =>
        unwrap(
          api.PATCH("/v1/help-centers/{centerId}/collections/{collectionId}", {
            params: { path: { ...path, collectionId: v.id } },
            body: v.body,
          })
        ),
      onSuccess: refresh,
    }),
    reorder: useMutation({
      mutationFn: (moves: { id: string; position: number; parent_id?: string | null }[]) =>
        Promise.all(
          moves.map((m) =>
            unwrap(
              api.PATCH("/v1/help-centers/{centerId}/collections/{collectionId}", {
                params: { path: { ...path, collectionId: m.id } },
                body: { position: m.position, ...(m.parent_id !== undefined ? { parent_id: m.parent_id } : {}) },
              })
            )
          )
        ),
      onSettled: refresh,
    }),
    remove: useMutation({
      mutationFn: (id: string) =>
        unwrap(
          api.DELETE("/v1/help-centers/{centerId}/collections/{collectionId}", {
            params: { path: { ...path, collectionId: id } },
          })
        ),
      onSuccess: refresh,
    }),
  }
}

export function useHelpArticles(centerId: string, filters: HelpArticleFilters) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.helpArticles(ws, centerId, filters),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/help-centers/{centerId}/articles", {
          params: { path: { centerId }, query: { ...filters, cursor: pageParam, limit: 50 } },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled: !!centerId,
  })
}

export function useHelpArticle(centerId: string, articleId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.helpArticle(ws, centerId, articleId),
    queryFn: () =>
      unwrap(
        api.GET("/v1/help-centers/{centerId}/articles/{articleId}", { params: { path: { centerId, articleId } } })
      ),
    enabled: !!centerId && !!articleId,
  })
}

export function useCreateArticle(centerId: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: HelpArticleCreate) =>
      unwrap(api.POST("/v1/help-centers/{centerId}/articles", { params: { path: { centerId } }, body })),
    onSuccess: refresh,
  })
}

export function useArticleMutations(centerId: string, articleId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = useRefresh()
  const path = { centerId, articleId }
  const put = (a: HelpArticle) => qc.setQueryData(keys.helpArticle(ws, centerId, articleId), a)
  const withTranslation = (tr: HelpArticleTranslation) => {
    qc.setQueryData<HelpArticle>(keys.helpArticle(ws, centerId, articleId), (old) =>
      old
        ? {
            ...old,
            translations: old.translations.some((x) => x.locale === tr.locale)
              ? old.translations.map((x) => (x.locale === tr.locale ? tr : x))
              : [...old.translations, tr],
          }
        : old
    )
    void qc.invalidateQueries({ queryKey: ["ws", ws, "help", "articles", centerId] })
  }
  return {
    update: useMutation({
      mutationFn: (body: HelpArticleUpdate) =>
        unwrap(api.PATCH("/v1/help-centers/{centerId}/articles/{articleId}", { params: { path }, body })),
      onSuccess: (a) => {
        put(a)
        void refresh()
      },
    }),
    remove: useMutation({
      mutationFn: () => unwrap(api.DELETE("/v1/help-centers/{centerId}/articles/{articleId}", { params: { path } })),
      onSuccess: () => {
        qc.removeQueries({ queryKey: keys.helpArticle(ws, centerId, articleId) })
        return refresh()
      },
    }),
    save: useMutation({
      mutationFn: (v: {
        locale: string
        base_version: number
        title: string
        body: string
        keywords: string
        slug?: string
      }) => {
        const { locale, ...body } = v
        return unwrap(
          api.PUT("/v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}", {
            params: { path: { ...path, locale } },
            body,
          })
        )
      },
      onSuccess: withTranslation,
    }),
    publish: useMutation({
      mutationFn: (v: { locale: string; base_version?: number }) =>
        unwrap(
          api.POST("/v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}/publish", {
            params: { path: { ...path, locale: v.locale } },
            body: { base_version: v.base_version },
          })
        ),
      onSuccess: (tr) => {
        withTranslation(tr)
        void refresh()
      },
    }),
    unpublish: useMutation({
      mutationFn: (locale: string) =>
        unwrap(
          api.POST("/v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}/unpublish", {
            params: { path: { ...path, locale } },
          })
        ),
      onSuccess: (tr) => {
        withTranslation(tr)
        void refresh()
      },
    }),
    removeTranslation: useMutation({
      mutationFn: (locale: string) =>
        unwrap(
          api.DELETE("/v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}", {
            params: { path: { ...path, locale } },
          })
        ),
      onSuccess: refresh,
    }),
    restore: useMutation({
      mutationFn: (v: { locale: string; revisionId: string; base_version: number }) =>
        unwrap(
          api.POST(
            "/v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}/revisions/{revisionId}/restore",
            {
              params: { path: { ...path, locale: v.locale, revisionId: v.revisionId } },
              body: { base_version: v.base_version },
            }
          )
        ),
      onSuccess: withTranslation,
    }),
  }
}

export function useHelpRevisions(centerId: string, articleId: string, locale: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.helpRevisions(ws, centerId, articleId, locale),
    queryFn: () =>
      unwrap(
        api.GET("/v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}/revisions", {
          params: { path: { centerId, articleId, locale } },
        })
      ).then((r) => r.items),
    enabled,
    gcTime: 0,
  })
}

export function useRenderedMarkdown(centerId: string, body: string) {
  return useQuery({
    queryKey: ["help-render", centerId, body],
    queryFn: () =>
      unwrap(api.POST("/v1/help-centers/{centerId}/render", { params: { path: { centerId } }, body: { body } })).then(
        (r) => r.html
      ),
    enabled: !!centerId,
    staleTime: Infinity,
    gcTime: 60_000,
    placeholderData: (previous) => previous,
    retry: (count, err) => !(err instanceof ApiError) && count < 2,
  })
}

export function useUploadHelpImage(centerId: string) {
  return useMutation({
    mutationFn: (file: File) =>
      unwrap(
        api.POST("/v1/help-centers/{centerId}/images", {
          params: { path: { centerId } },
          body: { file: file as unknown as string },
          bodySerializer: () => {
            const form = new FormData()
            form.append("file", file, file.name)
            return form
          },
        })
      ),
  })
}

export function useHelpSearch(centerId: string, q: string, locale: string, drafts: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.helpSearch(ws, centerId, q, locale), drafts],
    queryFn: () =>
      unwrap(
        api.GET("/v1/help-centers/{centerId}/search", {
          params: {
            path: { centerId },
            query: { q, locale: locale || undefined, drafts: drafts || undefined, limit: 8 },
          },
        })
      ),
    enabled: !!centerId && q.trim().length > 0,
    placeholderData: (previous) => previous,
  })
}

export function useHelpReport(centerId: string, days: 7 | 30 | 90) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.helpReport(ws, centerId, days),
    queryFn: () =>
      unwrap(api.GET("/v1/help-centers/{centerId}/report", { params: { path: { centerId }, query: { days } } })),
    enabled: !!centerId,
  })
}

export function useHelpPreview(centerId: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.helpCenter(ws, centerId), "preview"],
    queryFn: async () => {
      const { data, response } = await api.GET("/v1/help-centers/{centerId}/preview", {
        params: { path: { centerId }, query: { view: "home" } },
        parseAs: "text",
      })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      return data as string
    },
    enabled,
    gcTime: 0,
  })
}

export function useHelpImports(centerId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const live = useRealtimeStatus() === "live"
  useEffect(() => {
    if (live) void qc.invalidateQueries({ queryKey: keys.helpImports(ws, centerId) })
  }, [live, ws, centerId, qc])
  return useInfiniteQuery({
    queryKey: keys.helpImports(ws, centerId),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/help-centers/{centerId}/imports", {
          params: { path: { centerId }, query: { cursor: pageParam, limit: 20 } },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useStartHelpImport(centerId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: HelpImportCreate) =>
      unwrap(api.POST("/v1/help-centers/{centerId}/imports", { params: { path: { centerId } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.helpImports(ws, centerId) }),
  })
}

export function useCancelHelpImport(centerId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (importId: string) =>
      unwrap(
        api.DELETE("/v1/help-centers/{centerId}/imports/{importId}", { params: { path: { centerId, importId } } })
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.helpImports(ws, centerId) }),
  })
}

export function useHelpImportRedirects(centerId: string, importId: string) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.helpImportRedirects(ws, centerId, importId),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/help-centers/{centerId}/imports/{importId}/redirects", {
          params: { path: { centerId, importId }, query: { cursor: pageParam, limit: 50 } },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}
