import type { QueryClient } from "@tanstack/react-query"
import { useSyncExternalStore } from "react"

import type { Member } from "@/lib/api"
import { keys } from "@/lib/keys"

type Presence = { member_id: string; availability: Member["availability"]; online: boolean }
type Viewing = { member_id: string; conversation_id: string; viewing: boolean }

const viewers = new Map<string, string[]>()
const listeners = new Set<() => void>()
const EMPTY: string[] = []
let all: Map<string, string[]> = new Map()

function changed() {
  all = new Map(viewers)
  listeners.forEach((l) => l())
}

export function applyViewing(v: Viewing) {
  const now = viewers.get(v.conversation_id) ?? []
  const next = v.viewing ? (now.includes(v.member_id) ? now : [...now, v.member_id]) : now.filter((x) => x !== v.member_id)
  if (next.length) viewers.set(v.conversation_id, next)
  else viewers.delete(v.conversation_id)
  changed()
}

export function clearViewing() {
  if (viewers.size === 0) return
  viewers.clear()
  changed()
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

export function useViewers(conversationId: string | undefined): string[] {
  return useSyncExternalStore(subscribe, () => (conversationId ? (viewers.get(conversationId) ?? EMPTY) : EMPTY))
}

export function useAllViewers(): Map<string, string[]> {
  return useSyncExternalStore(subscribe, () => all)
}

export function applyPresence(qc: QueryClient, ws: string, p: Presence) {
  qc.setQueryData<Member[]>(keys.members(ws), (old) =>
    old?.map((m) => (m.id === p.member_id ? { ...m, availability: p.availability, online: p.online } : m)),
  )
}
