import { useEffect } from "react"
import { useLocation, useNavigate } from "react-router"

import { useConversations, useCounts } from "@/features/inbox/queries"
import { useSession } from "@/lib/session"

export type SetupKind = "chat" | "email" | "app"
export const SETUP_KINDS: SetupKind[] = ["chat", "email", "app"]

export type SetupState = { secret?: string; back?: string }

const laterKey = (ws: string) => `setup-later:${ws}`

export function putOff(ws: string) {
  try {
    localStorage.setItem(laterKey(ws), "1")
  } catch {
    // Storage can be blocked; the setup then opens again on the next visit.
  }
}

function isPutOff(ws: string) {
  try {
    return localStorage.getItem(laterKey(ws)) === "1"
  } catch {
    return false
  }
}

export function useOpenSetup() {
  const { workspaceId: ws, canManage } = useSession()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const watch = canManage && pathname === "/" && !isPutOff(ws)
  const counts = useCounts().data
  const noOpen = watch && !!counts && counts.all === 0 && counts.spam === 0
  const list = useConversations({}, noOpen).data
  const empty = noOpen && !!list && list.pages[0]?.items.length === 0
  useEffect(() => {
    if (empty) navigate("/setup", { replace: true })
  }, [empty, navigate])
}

export function browserLanguage() {
  const tag = (navigator.language || "en").split("-")[0]!.toLowerCase()
  return /^[a-z]{2,3}$/.test(tag) ? tag : "en"
}

export function browserTimeZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"
}
