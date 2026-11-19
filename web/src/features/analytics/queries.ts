import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { createContext, useContext } from "react"

import { useAnalyticsProperties } from "@/features/analytics/properties"
import {
  type AnalyticsPlatform,
  api,
  ApiError,
  unwrap,
  type AnalyticsExport,
  type AnalyticsExportCreate,
  type AnalyticsFilter,
  type AnalyticsGeoIP,
  type AnalyticsGoal,
  type AnalyticsGoalCreate,
  type AnalyticsInsight,
  type AnalyticsProperty,
  type AnalyticsPropertyUpdate,
  type AnalyticsQuery,
  type AnalyticsResult,
  type AnalyticsShare,
  type AnalyticsShareCreate,
  type AnalyticsSharedProperty,
  type AnalyticsShareUpdate,
  type AnalyticsShareView,
  type AnalyticsSubscriptionUpdate,
} from "@/lib/api"
import { saveDownload } from "@/lib/download"
import type { paths } from "@/lib/schema.gen"
import { keys } from "@/lib/keys"
import { useOptionalSession, useSession } from "@/lib/session"

export type ShareScope = {
  token: string
  access?: string
  views: AnalyticsShareView[]
  property: AnalyticsSharedProperty
  goals: AnalyticsGoal[]
  geo?: AnalyticsGeoIP
  locked: () => void
}

export const ShareContext = createContext<ShareScope | null>(null)

export function useShare() {
  return useContext(ShareContext)
}

function useDataKey(id: string) {
  const share = useShare()
  const ws = useOptionalSession()?.workspaceId ?? ""
  return share ? (["share", share.token, share.access ?? ""] as const) : keys.analyticsData(ws, id)
}

export function shareHeaders(access?: string) {
  return access ? { Authorization: `Bearer ${access}` } : undefined
}

async function shared<T>(share: ShareScope, call: Promise<{ data?: T; error?: unknown; response: Response }>) {
  try {
    return await unwrap(call)
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) share.locked()
    throw err
  }
}

function runQuery(share: ShareScope | null, id: string, body: AnalyticsQuery): Promise<AnalyticsResult> {
  if (share) {
    return shared(
      share,
      api.POST("/client/v1/analytics-shares/{token}/query", {
        params: { path: { token: share.token } },
        headers: shareHeaders(share.access),
        body: { insights: false, ...body },
      })
    ) as Promise<AnalyticsResult>
  }
  return unwrap(
    api.POST("/v1/analytics/properties/{propertyId}/query", {
      params: { path: { propertyId: id } },
      body: { insights: false, ...body },
    })
  ) as Promise<AnalyticsResult>
}

export function useProperty(id: string) {
  const ws = useOptionalSession()?.workspaceId ?? ""
  const list = useAnalyticsProperties()
  return useQuery({
    queryKey: keys.analyticsProperty(ws, id),
    queryFn: () => unwrap(api.GET("/v1/analytics/properties/{propertyId}", { params: { path: { propertyId: id } } })),
    initialData: () => list.data?.items.find((p) => p.id === id),
    initialDataUpdatedAt: () => list.dataUpdatedAt,
    staleTime: 30_000,
    enabled: !!id && !!ws,
  })
}

export function useWaitForFirstEvent(id: string, waiting: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.analyticsProperty(ws, id), "wait"],
    queryFn: () => unwrap(api.GET("/v1/analytics/properties/{propertyId}", { params: { path: { propertyId: id } } })),
    refetchInterval: (q) => (q.state.data?.first_event_at ? false : 3_000),
    enabled: !!id && waiting,
  })
}

export type Query = Omit<AnalyticsQuery, "limit"> & { limit?: number }

export function useAnalytics(id: string, query: Query, enabled = true) {
  const body: AnalyticsQuery = { limit: 100, ...query }
  const share = useShare()
  const base = useDataKey(id)
  return useQuery({
    queryKey: [...base, "query", body],
    queryFn: () => runQuery(share, id, body),
    placeholderData: keepPreviousData,
    staleTime: 30_000,
    enabled: !!id && enabled,
  })
}

export function useAnalyticsTable(id: string, body: AnalyticsQuery) {
  const share = useShare()
  const base = useDataKey(id)
  return useInfiniteQuery({
    queryKey: [...base, "table", body],
    queryFn: ({ pageParam }) => runQuery(share, id, { ...body, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  })
}

export function downloadCsv(id: string, body: Query, name: string) {
  return saveDownload(
    api.POST("/v1/analytics/properties/{propertyId}/query", {
      params: { path: { propertyId: id }, query: { format: "csv" } },
      body: { insights: false, ...body, limit: 1000, cursor: undefined },
      parseAs: "blob",
    }),
    `${name}.csv`
  )
}

export function useInsights(id: string, bodies: AnalyticsQuery[], enabled = true) {
  const share = useShare()
  const base = useDataKey(id)
  return useQuery({
    queryKey: [...base, "insights", bodies],
    queryFn: async () => {
      const out = new Map<string, AnalyticsInsight>()
      for (const body of bodies) {
        const rows = (body.dimensions ?? []).some((d) => !d.startsWith("time:"))
        try {
          const r = await runQuery(share, id, { ...body, insights: true })
          for (const x of r.insights ?? []) {
            const total = x.kind === "metric_change" || x.kind === "metric_new"
            if (total === rows) continue
            const key = [x.kind, x.metric, x.dimension ?? "", x.value ?? ""].join("\u0000")
            if (!out.has(key)) out.set(key, x)
          }
        } catch {
          continue
        }
      }
      return [...out.values()]
    },
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    enabled: !!id && enabled && bodies.length > 0,
  })
}

export function useRealtime(id: string, enabled = true) {
  const share = useShare()
  const base = useDataKey(id)
  return useQuery({
    queryKey: [...base, "realtime"],
    queryFn: () =>
      share
        ? shared(
            share,
            api.GET("/client/v1/analytics-shares/{token}/realtime", {
              params: { path: { token: share.token } },
              headers: shareHeaders(share.access),
            })
          )
        : unwrap(api.GET("/v1/analytics/properties/{propertyId}/realtime", { params: { path: { propertyId: id } } })),
    refetchInterval: 15_000,
    placeholderData: keepPreviousData,
    enabled,
  })
}

export function useHealth(id: string, days: number, enabled = true) {
  const base = useDataKey(id)
  return useQuery({
    queryKey: [...base, "health", days],
    queryFn: () =>
      unwrap(
        api.GET("/v1/analytics/properties/{propertyId}/health", {
          params: { path: { propertyId: id }, query: { days } },
        })
      ),
    staleTime: days === 1 ? 10 * 60_000 : 30_000,
    enabled,
  })
}

export function useHealthWatch(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.analyticsData(ws, id), "health-watch"],
    queryFn: () =>
      unwrap(
        api.GET("/v1/analytics/properties/{propertyId}/health", {
          params: { path: { propertyId: id }, query: { days: 2 } },
        })
      ),
    refetchInterval: 5_000,
    staleTime: 0,
    gcTime: 0,
  })
}

export function useGeoAttribution(id: string) {
  const share = useShare()
  const health = useHealth(id, 1, !share)
  const geo = share ? share.geo : health.data?.geoip
  return geo?.attribution ? { text: geo.attribution, url: geo.attribution_url } : null
}

function useStore() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return {
    put: (p: AnalyticsProperty) => {
      qc.setQueryData(keys.analyticsProperty(ws, p.id), p)
      void qc.invalidateQueries({ queryKey: keys.analyticsProperties(ws) })
    },
    refresh: (id: string) => {
      void qc.invalidateQueries({ queryKey: keys.analyticsData(ws, id) })
      void qc.invalidateQueries({ queryKey: keys.analyticsProperties(ws) })
    },
    drop: (id: string) => {
      qc.removeQueries({ queryKey: keys.analyticsProperty(ws, id) })
      qc.removeQueries({ queryKey: keys.analyticsData(ws, id) })
      void qc.invalidateQueries({ queryKey: keys.analyticsProperties(ws) })
    },
  }
}

export function useCreateProperty() {
  const store = useStore()
  return useMutation({
    mutationFn: (body: {
      name: string
      domains: string[]
      platforms?: AnalyticsPlatform[]
      inbox_id?: string
      timezone: string
    }) => unwrap(api.POST("/v1/analytics/properties", { body: { currency: "EUR", ...body } })),
    onSuccess: store.put,
  })
}

export function useUpdateProperty(id: string) {
  const store = useStore()
  return useMutation({
    mutationFn: (body: AnalyticsPropertyUpdate) =>
      unwrap(api.PATCH("/v1/analytics/properties/{propertyId}", { params: { path: { propertyId: id } }, body })),
    onSuccess: store.put,
  })
}

export function useRotateKey(id: string) {
  const store = useStore()
  return useMutation({
    mutationFn: () =>
      unwrap(api.POST("/v1/analytics/properties/{propertyId}/rotate-key", { params: { path: { propertyId: id } } })),
    onSuccess: store.put,
  })
}

export function useDeleteData(id: string) {
  const store = useStore()
  return useMutation({
    mutationFn: (body: { from: string; to: string } | { all: true }) =>
      unwrap(
        api.POST("/v1/analytics/properties/{propertyId}/delete-data", { params: { path: { propertyId: id } }, body })
      ),
    onSuccess: () => store.refresh(id),
  })
}

export function useDeleteProperty(id: string) {
  const store = useStore()
  return useMutation({
    mutationFn: (name: string) =>
      unwrap(
        api.DELETE("/v1/analytics/properties/{propertyId}", { params: { path: { propertyId: id } }, body: { name } })
      ),
    onSuccess: () => store.drop(id),
  })
}

function useMeta(id: string) {
  const qc = useQueryClient()
  const base = useDataKey(id)
  return {
    goals: [...base, "goals"] as const,
    funnels: [...base, "funnels"] as const,
    segments: [...base, "segments"] as const,
    annotations: [...base, "annotations"] as const,
    base,
    invalidate: (key: readonly unknown[]) => void qc.invalidateQueries({ queryKey: key }),
  }
}

const path = (id: string) => ({ params: { path: { propertyId: id } } })

export function useGoals(id: string) {
  const m = useMeta(id)
  const share = useShare()
  return useQuery({
    queryKey: m.goals,
    queryFn: () =>
      share
        ? Promise.resolve({ items: share.goals })
        : unwrap(api.GET("/v1/analytics/properties/{propertyId}/goals", path(id))),
    select: (d) => d.items,
    staleTime: 60_000,
  })
}

export function useSaveGoal(id: string, goalId?: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (body: AnalyticsGoalCreate) =>
      goalId
        ? unwrap(
            api.PATCH("/v1/analytics/properties/{propertyId}/goals/{goalId}", {
              params: { path: { propertyId: id, goalId } },
              body: { ...body, prop_key: body.kind === "event" ? (body.prop_key ?? null) : undefined },
            })
          )
        : unwrap(api.POST("/v1/analytics/properties/{propertyId}/goals", { ...path(id), body })),
    onSuccess: () => m.invalidate(m.base),
  })
}

export function useDeleteGoal(id: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (goalId: string) =>
      unwrap(
        api.DELETE("/v1/analytics/properties/{propertyId}/goals/{goalId}", {
          params: { path: { propertyId: id, goalId } },
        })
      ),
    onSuccess: () => m.invalidate(m.base),
  })
}

export function useGoalTotals(id: string, goals: AnalyticsGoal[], query: Omit<Query, "metrics" | "goal_id">) {
  const m = useMeta(id)
  const share = useShare()
  const ids = goals.map((g) => g.id)
  return useQuery({
    queryKey: [...m.base, "goal-totals", ids, query],
    queryFn: async () => {
      const out = new Map<string, AnalyticsResult["totals"]>()
      const queue = [...ids]
      const worker = async () => {
        for (let goal = queue.shift(); goal; goal = queue.shift()) {
          const r = await runQuery(share, id, {
            limit: 1,
            ...query,
            goal_id: goal,
            metrics: ["conversions", "conversion_rate", "revenue"],
          })
          out.set(goal, r.totals)
        }
      }
      await Promise.all([worker(), worker()])
      return out
    },
    placeholderData: keepPreviousData,
    staleTime: 30_000,
    enabled: ids.length > 0,
  })
}

export function useFunnels(id: string) {
  const m = useMeta(id)
  return useQuery({
    queryKey: m.funnels,
    queryFn: () => unwrap(api.GET("/v1/analytics/properties/{propertyId}/funnels", path(id))),
    select: (d) => d.items,
    staleTime: 60_000,
  })
}

export function useSaveFunnel(id: string, funnelId?: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (body: { name: string; steps: string[] }) =>
      funnelId
        ? unwrap(
            api.PATCH("/v1/analytics/properties/{propertyId}/funnels/{funnelId}", {
              params: { path: { propertyId: id, funnelId } },
              body,
            })
          )
        : unwrap(api.POST("/v1/analytics/properties/{propertyId}/funnels", { ...path(id), body })),
    onSuccess: () => m.invalidate(m.base),
  })
}

export function useDeleteFunnel(id: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (funnelId: string) =>
      unwrap(
        api.DELETE("/v1/analytics/properties/{propertyId}/funnels/{funnelId}", {
          params: { path: { propertyId: id, funnelId } },
        })
      ),
    onSuccess: () => m.invalidate(m.funnels),
  })
}

export type FunnelRange = { period: NonNullable<FunnelPeriod> } | { from: string; to: string }
type FunnelPeriod = NonNullable<
  paths["/v1/analytics/properties/{propertyId}/funnels/{funnelId}/report"]["get"]["parameters"]["query"]
>["period"]

export function useFunnelReport(id: string, funnelId: string | undefined, range: FunnelRange | null) {
  const m = useMeta(id)
  return useQuery({
    queryKey: [...m.base, "funnel", funnelId, range],
    queryFn: () =>
      unwrap(
        api.GET("/v1/analytics/properties/{propertyId}/funnels/{funnelId}/report", {
          params: { path: { propertyId: id, funnelId: funnelId! }, query: range ?? {} },
        })
      ),
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    enabled: !!funnelId && !!range,
  })
}

export type RetentionGrain = "day" | "week" | "month"

export function useRetention(id: string, grain: RetentionGrain, enabled: boolean) {
  const m = useMeta(id)
  return useQuery({
    queryKey: [...m.base, "retention", grain],
    queryFn: () =>
      unwrap(
        api.GET("/v1/analytics/properties/{propertyId}/retention", {
          params: { path: { propertyId: id }, query: { grain } },
        })
      ),
    placeholderData: keepPreviousData,
    staleTime: 5 * 60_000,
    enabled,
  })
}

export function useSegments(id: string) {
  const m = useMeta(id)
  return useQuery({
    queryKey: m.segments,
    queryFn: () => unwrap(api.GET("/v1/analytics/properties/{propertyId}/segments", path(id))),
    select: (d) => d.items,
    staleTime: 60_000,
  })
}

export function useSaveSegment(id: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: ({ segmentId, ...body }: { segmentId?: string; name?: string; filters?: AnalyticsFilter[] }) =>
      segmentId
        ? unwrap(
            api.PATCH("/v1/analytics/properties/{propertyId}/segments/{segmentId}", {
              params: { path: { propertyId: id, segmentId } },
              body,
            })
          )
        : unwrap(
            api.POST("/v1/analytics/properties/{propertyId}/segments", {
              ...path(id),
              body: { name: body.name ?? "", filters: body.filters ?? [] },
            })
          ),
    onSuccess: () => m.invalidate(m.base),
  })
}

export function useDeleteSegment(id: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (segmentId: string) =>
      unwrap(
        api.DELETE("/v1/analytics/properties/{propertyId}/segments/{segmentId}", {
          params: { path: { propertyId: id, segmentId } },
        })
      ),
    onSuccess: () => m.invalidate(m.segments),
  })
}

export function useAnnotations(id: string, from?: string, to?: string) {
  const m = useMeta(id)
  const share = useShare()
  return useQuery({
    queryKey: [...m.annotations, from, to],
    queryFn: () =>
      unwrap(
        api.GET("/v1/analytics/properties/{propertyId}/annotations", {
          params: { path: { propertyId: id }, query: { from, to } },
        })
      ),
    select: (d) => d.items,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    enabled: !!from && !!to && !share,
  })
}

export function useSaveAnnotation(id: string, annotationId?: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (body: { starts_on: string; ends_on: string; text: string }) =>
      annotationId
        ? unwrap(
            api.PATCH("/v1/analytics/properties/{propertyId}/annotations/{annotationId}", {
              params: { path: { propertyId: id, annotationId } },
              body,
            })
          )
        : unwrap(api.POST("/v1/analytics/properties/{propertyId}/annotations", { ...path(id), body })),
    onSuccess: () => m.invalidate(m.annotations),
  })
}

export function useDeleteAnnotation(id: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (annotationId: string) =>
      unwrap(
        api.DELETE("/v1/analytics/properties/{propertyId}/annotations/{annotationId}", {
          params: { path: { propertyId: id, annotationId } },
        })
      ),
    onSuccess: () => m.invalidate(m.annotations),
  })
}

export function useShares(id: string) {
  const m = useMeta(id)
  return useQuery({
    queryKey: [...m.base, "shares"],
    queryFn: () => unwrap(api.GET("/v1/analytics/properties/{propertyId}/shares", path(id))),
    select: (d) => d.items,
  })
}

export function useCreateShare(id: string) {
  const m = useMeta(id)
  return useMutation({
    mutationFn: (body: AnalyticsShareCreate) =>
      unwrap(api.POST("/v1/analytics/properties/{propertyId}/shares", { ...path(id), body })),
    onSuccess: () => m.invalidate([...m.base, "shares"]),
  })
}

export function useShareActions(id: string) {
  const m = useMeta(id)
  const done = () => m.invalidate([...m.base, "shares"])
  const at = (shareId: string) => ({ params: { path: { propertyId: id, shareId } } })
  return {
    update: useMutation({
      mutationFn: ({ shareId, body }: { shareId: string; body: AnalyticsShareUpdate }) =>
        unwrap(api.PATCH("/v1/analytics/properties/{propertyId}/shares/{shareId}", { ...at(shareId), body })),
      onSuccess: done,
    }),
    revoke: useMutation({
      mutationFn: (s: AnalyticsShare) =>
        unwrap(api.POST("/v1/analytics/properties/{propertyId}/shares/{shareId}/revoke", at(s.id))),
      onSuccess: done,
    }),
    remove: useMutation({
      mutationFn: (s: AnalyticsShare) =>
        unwrap(api.DELETE("/v1/analytics/properties/{propertyId}/shares/{shareId}", at(s.id))),
      onSuccess: done,
    }),
  }
}

const unfinished = (e: AnalyticsExport) => e.status === "queued" || e.status === "running"

export function useExports(id: string) {
  const m = useMeta(id)
  return useQuery({
    queryKey: [...m.base, "exports"],
    queryFn: () => unwrap(api.GET("/v1/analytics/properties/{propertyId}/exports", path(id))),
    select: (d) => d.items,
    refetchInterval: (q) => (q.state.data?.items.some(unfinished) ? 2_000 : false),
  })
}

export function useExportActions(id: string) {
  const m = useMeta(id)
  const done = () => m.invalidate([...m.base, "exports"])
  return {
    create: useMutation({
      mutationFn: (body: AnalyticsExportCreate) =>
        unwrap(api.POST("/v1/analytics/properties/{propertyId}/exports", { ...path(id), body })),
      onSuccess: done,
    }),
    remove: useMutation({
      mutationFn: (exportId: string) =>
        unwrap(
          api.DELETE("/v1/analytics/properties/{propertyId}/exports/{exportId}", {
            params: { path: { propertyId: id, exportId } },
          })
        ),
      onSuccess: done,
    }),
  }
}

export function exportDownloadUrl(propertyId: string, exportId: string, ws: string) {
  return `/v1/analytics/properties/${encodeURIComponent(propertyId)}/exports/${encodeURIComponent(exportId)}/download?workspace_id=${encodeURIComponent(ws)}`
}

export function useAlerts(id: string, enabled = true) {
  const m = useMeta(id)
  return useInfiniteQuery({
    queryKey: [...m.base, "alerts"],
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/analytics/properties/{propertyId}/alerts", {
          params: { path: { propertyId: id }, query: { before: pageParam } },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.items.length >= 100 ? last.items[last.items.length - 1]?.created_at : undefined),
    staleTime: 60_000,
    enabled,
  })
}

export function useSubscription(id: string) {
  const m = useMeta(id)
  return useQuery({
    queryKey: [...m.base, "subscription"],
    queryFn: () => unwrap(api.GET("/v1/analytics/properties/{propertyId}/subscription", path(id))),
  })
}

export function useUpdateSubscription(id: string) {
  const qc = useQueryClient()
  const m = useMeta(id)
  return useMutation({
    mutationFn: (body: AnalyticsSubscriptionUpdate) =>
      unwrap(api.PUT("/v1/analytics/properties/{propertyId}/subscription", { ...path(id), body })),
    onSuccess: (data) => qc.setQueryData([...m.base, "subscription"], data),
  })
}

export function useSendReport(id: string) {
  return useMutation({
    mutationFn: () => unwrap(api.POST("/v1/analytics/properties/{propertyId}/subscription/send-report", path(id))),
  })
}
