import { useCallback, useEffect, useRef, useSyncExternalStore } from "react"

import { api, type Typing, type TypingAuthor } from "@/lib/api"

const EXPIRE_MS = 6000
const SEND_EVERY_MS = 3000
const IDLE_MS = 5000

type Entry = { author: TypingAuthor; timer: ReturnType<typeof setTimeout> }

const typists = new Map<string, Map<string, Entry>>()
const snapshots = new Map<string, TypingAuthor[]>()
const listeners = new Set<() => void>()
const EMPTY: TypingAuthor[] = []

function authorKey(a: TypingAuthor) {
  return a.type === "contact" ? `c:${a.contact_id}` : `m:${a.member_id}`
}

function changed(conversationId: string) {
  const entries = typists.get(conversationId)
  if (entries && entries.size > 0) snapshots.set(conversationId, [...entries.values()].map((e) => e.author))
  else {
    typists.delete(conversationId)
    snapshots.delete(conversationId)
  }
  listeners.forEach((l) => l())
}

function remove(conversationId: string, key: string) {
  const entries = typists.get(conversationId)
  const entry = entries?.get(key)
  if (!entries || !entry) return
  clearTimeout(entry.timer)
  entries.delete(key)
  changed(conversationId)
}

export function applyTyping(t: Typing) {
  const key = authorKey(t.author)
  if (!t.typing) {
    remove(t.conversation_id, key)
    return
  }
  let entries = typists.get(t.conversation_id)
  if (!entries) {
    entries = new Map()
    typists.set(t.conversation_id, entries)
  }
  const prev = entries.get(key)
  if (prev) clearTimeout(prev.timer)
  entries.set(key, { author: t.author, timer: setTimeout(() => remove(t.conversation_id, key), EXPIRE_MS) })
  changed(t.conversation_id)
}

export function stopTyping(conversationId: string, author: TypingAuthor) {
  remove(conversationId, authorKey(author))
}

export function clearTyping() {
  for (const entries of typists.values()) for (const e of entries.values()) clearTimeout(e.timer)
  typists.clear()
  snapshots.clear()
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

export function useTyping(conversationId: string): TypingAuthor[] {
  return useSyncExternalStore(subscribe, () => snapshots.get(conversationId) ?? EMPTY)
}

export function useContactTyping(conversationId: string) {
  return useTyping(conversationId).some((a) => a.type === "contact")
}

function post(conversationId: string, typing: boolean) {
  void api
    .POST("/v1/conversations/{conversationId}/typing", { params: { path: { conversationId } }, body: { typing } })
    .catch(() => undefined)
}

export function useTypingSender(conversationId: string) {
  const state = useRef({ sentAt: 0, idle: undefined as ReturnType<typeof setTimeout> | undefined })

  const stop = useCallback(() => {
    const s = state.current
    clearTimeout(s.idle)
    if (s.sentAt === 0) return
    s.sentAt = 0
    post(conversationId, false)
  }, [conversationId])

  const typed = useCallback(() => {
    const s = state.current
    const now = Date.now()
    if (now - s.sentAt >= SEND_EVERY_MS) {
      s.sentAt = now
      post(conversationId, true)
    }
    clearTimeout(s.idle)
    s.idle = setTimeout(stop, IDLE_MS)
  }, [conversationId, stop])

  useEffect(() => stop, [stop])

  return { typed, stop }
}
