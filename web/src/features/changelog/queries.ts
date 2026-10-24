import { useInfiniteQuery, useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  api,
  ApiError,
  unwrap,
  type Changelog,
  type ChangelogBroadcast,
  type ChangelogCreate,
  type ChangelogEntry,
  type ChangelogEntryBulkChange,
  type ChangelogEntryCreate,
  type ChangelogEntryTranslation,
  type ChangelogEntryUpdate,
  type ChangelogUpdate,
} from "@/lib/api"
import { keys, type ChangelogEntryFilters } from "@/lib/keys"
import { useSession } from "@/lib/session"

export const changelogLink = (id: string, tab?: string) => `/updates/${id}${tab ? `?tab=${tab}` : ""}`

export const entryLink = (id: string, entryId: string, locale?: string) =>
  `/updates/${id}/entries/${entryId}${locale ? `?locale=${encodeURIComponent(locale)}` : ""}`

function useRefresh() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => qc.invalidateQueries({ queryKey: keys.changelog(ws) })
}

export function useChangelogs(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.changelogs(ws),
    queryFn: () => unwrap(api.GET("/v1/changelogs")).then((r) => r.items),
    enabled,
    retry: false,
  })
}

export function useChangelog(id: string) {
  const list = useChangelogs(!!id)
  return { ...list, data: list.data?.find((c) => c.id === id) }
}

export function changelogOfInbox(items: Changelog[] | undefined, inboxId: string) {
  return items?.find((c) => c.inbox_id === inboxId)
}

export function useCreateChangelog() {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: ChangelogCreate) => unwrap(api.POST("/v1/changelogs", { body })),
    onSuccess: refresh,
  })
}

export function useUpdateChangelog(id: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: ChangelogUpdate) =>
      unwrap(api.PATCH("/v1/changelogs/{changelogId}", { params: { path: { changelogId: id } }, body })),
    onSuccess: refresh,
  })
}

export function useDeleteChangelog(id: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/changelogs/{changelogId}", { params: { path: { changelogId: id } } })),
    onSuccess: () => {
      qc.removeQueries({ predicate: (q) => q.queryKey.includes(id) })
      return qc.invalidateQueries({ queryKey: keys.changelogs(ws) })
    },
  })
}

export function useEntries(changelogId: string, filters: ChangelogEntryFilters, locale?: string) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: [...keys.changelogEntries(ws, changelogId, filters), locale ?? ""],
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/changelogs/{changelogId}/entries", {
          params: { path: { changelogId }, query: { ...filters, locale, cursor: pageParam, limit: 50 } },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled: !!changelogId,
  })
}

export function useAllEntries(changelogIds: string[], filters: ChangelogEntryFilters) {
  const { workspaceId: ws } = useSession()
  return useQueries({
    queries: changelogIds.map((changelogId) => ({
      queryKey: [...keys.changelogEntries(ws, changelogId, filters), "all"],
      queryFn: () =>
        unwrap(
          api.GET("/v1/changelogs/{changelogId}/entries", {
            params: { path: { changelogId }, query: { ...filters, limit: 50 } },
          })
        ),
    })),
    combine: (rs) => ({
      data: rs.flatMap((r) => r.data?.items ?? []),
      isPending: rs.some((r) => r.isPending),
      error: rs.find((r) => r.error)?.error ?? null,
    }),
  })
}

export function useEntry(changelogId: string, entryId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.changelogEntry(ws, changelogId, entryId),
    queryFn: () =>
      unwrap(api.GET("/v1/changelogs/{changelogId}/entries/{entryId}", { params: { path: { changelogId, entryId } } })),
    enabled: !!changelogId && !!entryId,
  })
}

export function useCreateEntry(changelogId: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: ChangelogEntryCreate) =>
      unwrap(api.POST("/v1/changelogs/{changelogId}/entries", { params: { path: { changelogId } }, body })),
    onSuccess: refresh,
  })
}

export function useBulkEntries(changelogId: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: ChangelogEntryBulkChange) =>
      unwrap(api.POST("/v1/changelogs/{changelogId}/entries/bulk", { params: { path: { changelogId } }, body })),
    onSuccess: () => refresh(),
  })
}

export function useEntryMutations(changelogId: string, entryId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = useRefresh()
  const path = { changelogId, entryId }
  const put = (e: ChangelogEntry) => {
    qc.setQueryData(keys.changelogEntry(ws, changelogId, entryId), e)
    void refresh()
  }
  const withTranslation = (tr: ChangelogEntryTranslation) => {
    qc.setQueryData<ChangelogEntry>(keys.changelogEntry(ws, changelogId, entryId), (old) =>
      old
        ? {
            ...old,
            translations: old.translations.some((x) => x.locale === tr.locale)
              ? old.translations.map((x) => (x.locale === tr.locale ? tr : x))
              : [...old.translations, tr],
          }
        : old
    )
    void qc.invalidateQueries({ queryKey: ["ws", ws, "changelog", "entries", changelogId] })
  }
  return {
    update: useMutation({
      mutationFn: (body: ChangelogEntryUpdate) =>
        unwrap(api.PATCH("/v1/changelogs/{changelogId}/entries/{entryId}", { params: { path }, body })),
      onSuccess: put,
    }),
    remove: useMutation({
      mutationFn: () => unwrap(api.DELETE("/v1/changelogs/{changelogId}/entries/{entryId}", { params: { path } })),
      onSuccess: () => {
        qc.removeQueries({ queryKey: keys.changelogEntry(ws, changelogId, entryId) })
        return refresh()
      },
    }),
    save: useMutation({
      mutationFn: (v: { locale: string; base_version: number; title: string; body: string; slug?: string }) => {
        const { locale, ...body } = v
        return unwrap(
          api.PUT("/v1/changelogs/{changelogId}/entries/{entryId}/translations/{locale}", {
            params: { path: { ...path, locale } },
            body,
          })
        )
      },
      onSuccess: withTranslation,
    }),
    removeTranslation: useMutation({
      mutationFn: (locale: string) =>
        unwrap(
          api.DELETE("/v1/changelogs/{changelogId}/entries/{entryId}/translations/{locale}", {
            params: { path: { ...path, locale } },
          })
        ),
      onSuccess: () => refresh(),
    }),
    publish: useMutation({
      mutationFn: (body: { locale?: string; base_version?: number }) =>
        unwrap(api.POST("/v1/changelogs/{changelogId}/entries/{entryId}/publish", { params: { path }, body })),
      onSuccess: put,
    }),
    schedule: useMutation({
      mutationFn: (publish_at: string) =>
        unwrap(
          api.POST("/v1/changelogs/{changelogId}/entries/{entryId}/schedule", {
            params: { path },
            body: { publish_at },
          })
        ),
      onSuccess: put,
    }),
    unschedule: useMutation({
      mutationFn: () =>
        unwrap(api.POST("/v1/changelogs/{changelogId}/entries/{entryId}/unschedule", { params: { path } })),
      onSuccess: put,
    }),
    unpublish: useMutation({
      mutationFn: () =>
        unwrap(api.POST("/v1/changelogs/{changelogId}/entries/{entryId}/unpublish", { params: { path } })),
      onSuccess: put,
    }),
    broadcast: useMutation({
      mutationFn: (body: ChangelogBroadcast) =>
        unwrap(api.POST("/v1/changelogs/{changelogId}/entries/{entryId}/broadcast", { params: { path }, body })),
    }),
  }
}

export function useChangelogPreview(changelogId: string, view: { entry?: string; locale?: string }) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.changelogs(ws), "preview", changelogId, view.entry ?? "", view.locale ?? ""],
    queryFn: async () => {
      const { data, response } = await api.GET("/v1/changelogs/{changelogId}/preview", {
        params: {
          path: { changelogId },
          query: view.entry
            ? { view: "entry", id: view.entry, locale: view.locale, draft: true }
            : { view: "home", draft: true },
        },
        parseAs: "text",
      })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      return data as string
    },
    gcTime: 0,
  })
}

export function useRenderedMarkdown(body: string) {
  return useQuery({
    queryKey: ["markdown-render", body],
    queryFn: () => unwrap(api.POST("/v1/markdown/render", { body: { body } })).then((r) => r.html),
    staleTime: Infinity,
    gcTime: 60_000,
    placeholderData: (previous) => previous,
    retry: (count, err) => !(err instanceof ApiError) && count < 2,
  })
}

export function useUploadEntryImage(changelogId: string) {
  return useMutation({
    mutationFn: (file: File) =>
      unwrap(
        api.POST("/v1/changelogs/{changelogId}/images", {
          params: { path: { changelogId } },
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
