import { msg } from "@lingui/core/macro"
import type { InfiniteData, QueryClient } from "@tanstack/react-query"
import { useSyncExternalStore } from "react"

import { i18n } from "@/i18n"
import type { Contact, Conversation, ConversationListItem, Me, Message } from "@/lib/api"
import { keys } from "@/lib/keys"
import { serviceWorkerRegistration } from "@/lib/pwa"
import { meKey } from "@/lib/session"

const KEY = "tab-alerts"
const QUIET_MS = 5000
const STALE_MS = 2 * 60_000

const listeners = new Set<() => void>()
const lastAlert = new Map<string, number>()

function read() {
  try {
    return localStorage.getItem(KEY) === "on"
  } catch {
    return false
  }
}

export function setTabAlerts(on: boolean) {
  try {
    if (on) localStorage.setItem(KEY, "on")
    else localStorage.removeItem(KEY)
  } catch {
    // Storage can be blocked; the setting then lasts for this page only.
  }
  listeners.forEach((l) => l())
}

export function useTabAlerts() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    read,
  )
}

export function notificationPermission(): NotificationPermission | "unsupported" {
  return typeof Notification === "undefined" ? "unsupported" : Notification.permission
}

export function playAlertSound() {
  const Ctx = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  if (!Ctx) return
  try {
    const ctx = new Ctx()
    const now = ctx.currentTime
    for (const [i, freq] of [880, 1320].entries()) {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      const start = now + i * 0.13
      osc.type = "sine"
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.0001, start)
      gain.gain.exponentialRampToValueAtTime(0.18, start + 0.015)
      gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.12)
      osc.connect(gain).connect(ctx.destination)
      osc.start(start)
      osc.stop(start + 0.13)
    }
    setTimeout(() => void ctx.close(), 600)
  } catch {
    // Audio can be unavailable until the page has had a user gesture.
  }
}

function cachedConversation(qc: QueryClient, ws: string, id: string): (Conversation & Partial<ConversationListItem>) | undefined {
  const one = qc.getQueryData<Conversation>(keys.conversation(ws, id))
  for (const [, data] of qc.getQueriesData<InfiniteData<{ items: ConversationListItem[] }>>({ queryKey: keys.conversationLists(ws) })) {
    const item = data?.pages.flatMap((p) => p.items).find((x) => x.id === id)
    if (item) return { ...item, ...one }
  }
  return one
}

async function show(title: string, body: string, conversationId: string) {
  const options: NotificationOptions & { renotify?: boolean } = {
    body,
    tag: `conversation-${conversationId}`,
    renotify: true,
    icon: "/icons/icon-192.png",
    badge: "/icons/badge-96.png",
    data: { url: `/conversations/${conversationId}` },
  }
  const reg = await serviceWorkerRegistration().catch(() => null)
  if (reg) return reg.showNotification(title, options)
  const n = new Notification(title, options)
  n.onclick = () => {
    window.focus()
    window.location.assign(`/conversations/${conversationId}`)
  }
}

export function alertNewMessage(qc: QueryClient, ws: string, memberId: string, m: Message) {
  if (m.kind !== "message" || m.author.type !== "contact" || m.draft) return
  if (document.visibilityState !== "hidden" || !read()) return
  const c = cachedConversation(qc, ws, m.conversation_id)
  if (c?.spam) return
  const away = qc.getQueryData<Me>(meKey)?.person.availability === "away"
  const mine = c?.assignee_id === memberId
  if (!mine && (away || c?.assignee_id)) return
  const now = Date.now()
  if (now - Date.parse(m.created_at) > STALE_MS) return
  if (now - (lastAlert.get(m.conversation_id) ?? 0) < QUIET_MS) return
  lastAlert.set(m.conversation_id, now)
  playAlertSound()
  if (notificationPermission() !== "granted") return
  const contact = m.author.contact_id ? qc.getQueryData<Contact>(keys.contact(ws, m.author.contact_id)) : undefined
  const title = c?.contact?.name || contact?.name || c?.contact?.email || i18n._(msg`New message`)
  const text = m.body.trim() || (m.attachments.length ? i18n._(msg`Sent an attachment`) : "")
  void show(title, [...text].slice(0, 140).join(""), m.conversation_id).catch(() => {})
}
