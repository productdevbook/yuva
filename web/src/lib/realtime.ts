import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useSyncExternalStore } from "react"

import type { RealtimeMessage } from "@/lib/api"
import { applyEvent, type LiveEvent } from "@/lib/live"
import { meKey } from "@/lib/session"
import { applyTyping, clearTyping, stopTyping } from "@/lib/typing"

export type RealtimeStatus = "connecting" | "live" | "offline"

let status: { ws: string | null; value: RealtimeStatus } = { ws: null, value: "offline" }
const listeners = new Set<() => void>()
let retryNow: (() => void) | null = null

export function reconnectNow() {
  retryNow?.()
}

function setStatus(ws: string, value: RealtimeStatus) {
  status = { ws, value }
  listeners.forEach((l) => l())
}

export function isLive(ws: string) {
  return status.ws === ws && status.value === "live"
}

export function useRealtimeStatus(): RealtimeStatus {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => status.value,
  )
}

export function useRealtime(ws: string, memberId: string) {
  const qc = useQueryClient()
  useEffect(() => {
    let socket: WebSocket | null = null
    let cursor: number | null = null
    let attempt = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    let stopped = false
    let hidden = false
    const ctx = { ws, memberId }

    const connect = () => {
      if (stopped || hidden) return
      if (attempt === 0) setStatus(ws, "connecting")
      const url = new URL("/v1/realtime", window.location.href)
      url.protocol = url.protocol === "https:" ? "wss:" : "ws:"
      url.searchParams.set("workspace_id", ws)
      if (cursor !== null) url.searchParams.set("last_event_id", String(cursor))
      const s = new WebSocket(url)
      socket = s
      let opened = false
      s.onopen = () => {
        opened = true
      }
      s.onmessage = (e) => {
        const msg = JSON.parse(e.data as string) as RealtimeMessage
        if (msg.type === "ready") {
          cursor = Math.max(cursor ?? 0, msg.last_event_id)
          if (attempt > 0) void qc.refetchQueries({ predicate: (q) => q.state.status === "error", type: "active" })
          attempt = 0
          setStatus(ws, "live")
          return
        }
        if (msg.type === "resync_required") {
          void qc.invalidateQueries({ queryKey: ["ws", ws] })
          return
        }
        if (msg.type === "typing") {
          applyTyping(msg.data)
          return
        }
        cursor = Math.max(cursor ?? 0, msg.id)
        if (msg.type === "message.created" && msg.data.kind === "message" && msg.data.author.type === "contact") {
          stopTyping(msg.data.conversation_id, { type: "contact", contact_id: msg.data.author.contact_id })
        }
        applyEvent(qc, ctx, { type: msg.type, data: msg.data } as LiveEvent)
      }
      s.onclose = (e) => {
        if (socket !== s) return
        socket = null
        clearTyping()
        if (stopped || hidden) return
        setStatus(ws, "offline")
        if (e.code === 1008) {
          void qc.invalidateQueries({ queryKey: meKey })
          return
        }
        if (!opened) void qc.invalidateQueries({ queryKey: meKey })
        const delay = e.code === 1012 || e.code === 1013 ? 500 : Math.min(10_000, 1000 * 2 ** attempt)
        attempt++
        timer = setTimeout(connect, delay * (0.75 + Math.random() / 2))
      }
    }

    const retry = () => {
      if (socket || stopped || hidden) return
      clearTimeout(timer)
      connect()
    }
    const onVisible = () => {
      if (document.visibilityState === "visible") retry()
    }
    const onPageHide = () => {
      hidden = true
      clearTimeout(timer)
      const s = socket
      socket = null
      s?.close(1000)
      clearTyping()
    }
    const onPageShow = () => {
      hidden = false
      retry()
    }
    retryNow = retry
    window.addEventListener("online", retry)
    window.addEventListener("pagehide", onPageHide)
    window.addEventListener("pageshow", onPageShow)
    document.addEventListener("visibilitychange", onVisible)
    connect()
    return () => {
      stopped = true
      clearTimeout(timer)
      if (retryNow === retry) retryNow = null
      window.removeEventListener("online", retry)
      window.removeEventListener("pagehide", onPageHide)
      window.removeEventListener("pageshow", onPageShow)
      document.removeEventListener("visibilitychange", onVisible)
      socket?.close(1000)
      setStatus(ws, "offline")
    }
  }, [qc, ws, memberId])
}
