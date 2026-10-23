import { useQueries, useQuery } from "@tanstack/react-query"
import { useMemo } from "react"

import { api, unwrap, type Channel, type CustomFieldObject } from "@/lib/api"
import { keys } from "@/lib/keys"
import type { LiveContext } from "@/lib/live"
import { useSession } from "@/lib/session"

export function useLiveContext(): LiveContext {
  const { workspaceId, membership } = useSession()
  return { ws: workspaceId, memberId: membership.member_id }
}

export function useMembers() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.members(ws),
    queryFn: () => unwrap(api.GET("/v1/members")).then((r) => r.items),
    staleTime: 60_000,
    refetchOnWindowFocus: "always",
  })
}

export function useMemberMap() {
  const members = useMembers().data
  return new Map((members ?? []).map((m) => [m.id, m]))
}

export function useInboxes() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.inboxes(ws),
    queryFn: () => unwrap(api.GET("/v1/inboxes")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useInboxMembers(inboxId: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.inboxMembers(ws, inboxId ?? ""),
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/members", { params: { path: { inboxId: inboxId! } } })).then(
        (r) => r.items
      ),
    enabled: !!inboxId,
    staleTime: 60_000,
  })
}

export function useAssignableMembers(inboxId: string | undefined) {
  const members = useMembers().data ?? []
  const granted = new Set((useInboxMembers(inboxId).data ?? []).map((m) => m.id))
  return members.filter((m) => m.role !== "agent" || granted.has(m.id))
}

export function useLabels() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.labels(ws),
    queryFn: () => unwrap(api.GET("/v1/labels")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useCannedReplies() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.cannedReplies(ws),
    queryFn: () => unwrap(api.GET("/v1/canned-replies")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useMacros() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.macros(ws),
    queryFn: () => unwrap(api.GET("/v1/macros")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useTeams() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.teams(ws),
    queryFn: () => unwrap(api.GET("/v1/teams")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useCustomFields(object?: CustomFieldObject) {
  const { workspaceId: ws } = useSession()
  const q = useQuery({
    queryKey: keys.customFields(ws),
    queryFn: () => unwrap(api.GET("/v1/custom-fields")).then((r) => r.items),
    staleTime: 60_000,
  })
  const data = object && q.data ? q.data.filter((f) => f.object === object) : q.data
  return { ...q, data }
}

export function useMyTeams() {
  const { membership } = useSession()
  return (useTeams().data ?? []).filter((team) => team.member_ids.includes(membership.member_id))
}

export function useChannel(id: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.channel(ws, id ?? ""),
    queryFn: () => unwrap(api.GET("/v1/channels/{channelId}", { params: { path: { channelId: id! } } })),
    enabled: !!id,
    staleTime: 5 * 60_000,
    retry: false,
  })
}

const channelLists = (results: { data?: Channel[] }[]) => results.map((r) => r.data)

export function useChannelMap() {
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data ?? []
  const lists = useQueries({
    queries: inboxes.map((i) => ({
      queryKey: keys.channels(ws, i.id),
      queryFn: () =>
        unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: i.id } } })).then((r) => r.items),
      staleTime: 5 * 60_000,
    })),
    combine: channelLists,
  })
  return useMemo(() => {
    const map = new Map<string, Channel>()
    for (const list of lists) for (const ch of list ?? []) map.set(ch.id, ch)
    return map
  }, [lists])
}
