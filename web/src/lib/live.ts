import type { InfiniteData, QueryClient, QueryKey } from "@tanstack/react-query"

import type { Contact, Conversation, Inbox, Message, RealtimeMessage } from "@/lib/api"
import { keys, type ConversationFilters } from "@/lib/keys"

type ConversationPage = { items: Conversation[]; next_cursor?: string }
type Lists = InfiniteData<ConversationPage, string | undefined>

type Event = Exclude<RealtimeMessage, { type: "ready" } | { type: "resync_required" }>
export type LiveEvent = Event extends infer E ? (E extends { type: infer T; data: infer D } ? { type: T; data: D } : never) : never

export type LiveContext = { ws: string; memberId: string }

function matches(c: Conversation, f: ConversationFilters, memberId: string) {
  if (f.status && c.status !== f.status) return false
  if (f.inbox_id && c.inbox_id !== f.inbox_id) return false
  if (f.label_id && !c.labels.includes(f.label_id)) return false
  if (f.assignee === "unassigned" && c.assignee_id) return false
  if (f.assignee === "me" && c.assignee_id !== memberId) return false
  if (f.assignee && f.assignee !== "me" && f.assignee !== "unassigned" && c.assignee_id !== f.assignee) return false
  return true
}

function byActivity(a: Conversation, b: Conversation) {
  if (a.last_activity_at !== b.last_activity_at) return a.last_activity_at < b.last_activity_at ? 1 : -1
  return a.id < b.id ? 1 : -1
}

function placeInList(old: Lists, c: Conversation, keep: boolean): Lists {
  const pages = old.pages.map((p) => ({ ...p, items: p.items.filter((x) => x.id !== c.id) }))
  if (keep && pages.length > 0) {
    const all = pages.flatMap((p) => p.items)
    const last = all[all.length - 1]
    const lastPage = pages[pages.length - 1]
    const fits = !lastPage.next_cursor || !last || byActivity(c, last) <= 0
    if (fits) {
      const target = pages.findIndex((p, i) => {
        const tail = p.items[p.items.length - 1]
        return i === pages.length - 1 || (tail && byActivity(c, tail) <= 0)
      })
      const page = pages[target]
      page.items = [...page.items, c].sort(byActivity)
    }
  }
  return { ...old, pages }
}

function updateLists(qc: QueryClient, ctx: LiveContext, c: Conversation) {
  for (const [key, data] of qc.getQueriesData<Lists | Conversation[]>({ queryKey: keys.conversationLists(ctx.ws) })) {
    if (!data) continue
    const filters = (key as QueryKey)[3]
    if (Array.isArray(data) || typeof filters !== "object" || filters === null) {
      void qc.invalidateQueries({ queryKey: key, exact: true })
      continue
    }
    const f = filters as ConversationFilters
    if (f.q) {
      void qc.invalidateQueries({ queryKey: key, exact: true })
      continue
    }
    qc.setQueryData<Lists>(key, (old) => (old ? placeInList(old, c, matches(c, f, ctx.memberId)) : old))
  }
}

function setConversation(qc: QueryClient, ctx: LiveContext, c: Conversation) {
  const current = qc.getQueryData<Conversation>(keys.conversation(ctx.ws, c.id))
  if (current && current.updated_at > c.updated_at) return
  qc.setQueryData(keys.conversation(ctx.ws, c.id), c)
  updateLists(qc, ctx, c)
}

function addMessage(qc: QueryClient, ctx: LiveContext, m: Message) {
  qc.setQueryData<Message[]>(keys.messages(ctx.ws, m.conversation_id), (old) =>
    old && !old.some((x) => x.id === m.id)
      ? [...old, m].sort((a, b) => (a.created_at < b.created_at ? -1 : a.created_at > b.created_at ? 1 : 0))
      : old,
  )
  if (m.kind === "event") return
  const c = qc.getQueryData<Conversation>(keys.conversation(ctx.ws, m.conversation_id))
  const cached =
    c ??
    qc
      .getQueriesData<Lists>({ queryKey: keys.conversationLists(ctx.ws) })
      .flatMap(([, d]) => (d && !Array.isArray(d) ? d.pages.flatMap((p) => p.items) : []))
      .find((x) => x.id === m.conversation_id)
  if (!cached) {
    void qc.invalidateQueries({ queryKey: keys.conversationLists(ctx.ws) })
    return
  }
  const next: Conversation = {
    ...cached,
    last_activity_at: m.created_at > cached.last_activity_at ? m.created_at : cached.last_activity_at,
    last_message_at:
      m.kind === "message" && (!cached.last_message_at || m.created_at > cached.last_message_at)
        ? m.created_at
        : cached.last_message_at,
  }
  if (c) qc.setQueryData(keys.conversation(ctx.ws, c.id), next)
  updateLists(qc, ctx, next)
}

export function applyEvent(qc: QueryClient, ctx: LiveContext, event: LiveEvent) {
  switch (event.type) {
    case "conversation.created":
    case "conversation.updated":
      setConversation(qc, ctx, event.data)
      return
    case "message.created":
      addMessage(qc, ctx, event.data)
      return
    case "contact.updated":
      qc.setQueryData<Contact>(keys.contact(ctx.ws, event.data.id), event.data)
      return
    case "contact.deleted":
      qc.removeQueries({ queryKey: keys.contact(ctx.ws, event.data.id) })
      void qc.invalidateQueries({ queryKey: keys.conversationLists(ctx.ws) })
      return
    case "inbox.updated": {
      const inbox = event.data
      qc.setQueryData<Inbox[]>(keys.inboxes(ctx.ws), (old) => old?.map((x) => (x.id === inbox.id ? inbox : x)))
      qc.setQueryData([...keys.inboxes(ctx.ws), inbox.id], inbox)
      return
    }
    case "inbox_access.changed":
      void qc.invalidateQueries({ queryKey: keys.inboxMembers(ctx.ws, event.data.inbox_id) })
      if (event.data.member_id === ctx.memberId) {
        void qc.invalidateQueries({ queryKey: keys.inboxes(ctx.ws) })
        void qc.invalidateQueries({ queryKey: keys.conversationLists(ctx.ws) })
        void qc.invalidateQueries({ queryKey: ["ws", ctx.ws, "conversation"] })
        void qc.invalidateQueries({ queryKey: ["ws", ctx.ws, "messages"] })
      }
      return
  }
}
