import { useSyncExternalStore } from "react"

import type { ConversationSla, ConversationSlaTarget, SlaPolicy, SlaState, SlaTargetState } from "@/lib/api"

const listeners = new Set<() => void>()
let tick = Date.now()
let timer: ReturnType<typeof setInterval> | undefined

function subscribe(l: () => void) {
  listeners.add(l)
  if (!timer) {
    timer = setInterval(() => {
      tick = Date.now()
      listeners.forEach((f) => f())
    }, 60_000)
  }
  return () => {
    listeners.delete(l)
    if (listeners.size === 0 && timer) {
      clearInterval(timer)
      timer = undefined
    }
  }
}

export function useMinute() {
  return useSyncExternalStore(subscribe, () => tick)
}

function running(state: SlaTargetState) {
  return state === "on_track" || state === "due_soon" || state === "breached"
}

export function targetState(t: ConversationSlaTarget, warnMinutes: number | undefined, now: number): SlaTargetState {
  if (!running(t.state) || !t.due_at) return t.state
  const left = Date.parse(t.due_at) - now
  if (left <= 0) return "breached"
  if (t.state === "on_track" && warnMinutes && left <= warnMinutes * 60_000) return "due_soon"
  return t.state
}

export function slaState(sla: ConversationSla, policy: SlaPolicy | undefined, now: number): SlaState {
  if (!running(sla.state) || !sla.due_at) return sla.state
  const left = Date.parse(sla.due_at) - now
  if (left <= 0) return "breached"
  if (sla.state === "on_track" && policy?.warn_minutes && left <= policy.warn_minutes * 60_000) return "due_soon"
  return sla.state
}
