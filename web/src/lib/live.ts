import type { InfiniteData, QueryClient, QueryKey } from "@tanstack/react-query"

import type {
  Contact,
  Conversation,
  ConversationListItem,
  ConversationRead,
  Inbox,
  Member,
  Message,
  RealtimeMessage,
} from "@/lib/api"
import { keys, type ConversationFilters } from "@/lib/keys"

type ConversationPage = { items: ConversationListItem[]; next_cursor?: string }
type Lists = InfiniteData<ConversationPage, string | undefined>
export type MessagePage = { items: Message[]; next_cursor?: string }
export type Messages = InfiniteData<MessagePage, string | undefined>

type Event = Exclude<RealtimeMessage, { type: "ready" } | { type: "resync_required" }>
export type LiveEvent = Event extends infer E ? (E extends { type: infer T; data: infer D } ? { type: T; data: D } : never) : never

export type LiveContext = { ws: string; memberId: string }

const PREVIEW_RUNES = 140

export function previewText(body: string) {
  const text = body.split(/\s+/).filter(Boolean).join(" ")
  const runes = [...text]
  return runes.length > PREVIEW_RUNES ? runes.slice(0, PREVIEW_RUNES).join("").trim() + "…" : text
}

function matches(c: Conversation, f: ConversationFilters, memberId: string) {
  if (!!f.spam !== c.spam) return false
  if (f.status && c.status !== f.status) return false
  if (f.inbox_id && c.inbox_id !== f.inbox_id) return false
  if (f.contact_id && c.contact_id !== f.contact_id) return false
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

function placeInList(old: Lists, c: ConversationListItem, keep: boolean): Lists {
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

function listCaches(qc: QueryClient, ws: string) {
  return qc.getQueriesData<Lists>({ queryKey: keys.conversationLists(ws) })
}

function findItem(qc: QueryClient, ws: string, id: string) {
  for (const [, data] of listCaches(qc, ws)) {
    const item = data?.pages.flatMap((p) => p.items).find((x) => x.id === id)
    if (item) return item
  }
  return undefined
}

function updateLists(qc: QueryClient, ctx: LiveContext, c: Conversation, patch: Partial<ConversationListItem> = {}) {
  const base = findItem(qc, ctx.ws, c.id)
  for (const [key, data] of listCaches(qc, ctx.ws)) {
    if (!data) continue
    const f = (key as QueryKey)[3] as ConversationFilters
    if (f.q || (!base && matches(c, f, ctx.memberId))) {
      void qc.invalidateQueries({ queryKey: key, exact: true })
      continue
    }
    if (!base) continue
    const item: ConversationListItem = { ...base, ...c, ...patch }
    qc.setQueryData<Lists>(key, (old) => (old ? placeInList(old, item, matches(c, f, ctx.memberId)) : old))
  }
}

function patchItem(qc: QueryClient, ws: string, id: string, patch: Partial<ConversationListItem>) {
  for (const [key, data] of listCaches(qc, ws)) {
    if (!data?.pages.some((p) => p.items.some((x) => x.id === id))) continue
    qc.setQueryData<Lists>(key, (old) =>
      old
        ? { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.map((x) => (x.id === id ? { ...x, ...patch } : x)) })) }
        : old,
    )
  }
}

const countTimers = new Map<string, ReturnType<typeof setTimeout>>()

function refreshCounts(qc: QueryClient, ws: string) {
  clearTimeout(countTimers.get(ws))
  countTimers.set(
    ws,
    setTimeout(() => {
      countTimers.delete(ws)
      void qc.invalidateQueries({ queryKey: keys.counts(ws) })
    }, 400),
  )
}

function setConversation(qc: QueryClient, ctx: LiveContext, c: Conversation) {
  const current = qc.getQueryData<Conversation>(keys.conversation(ctx.ws, c.id))
  if (current && current.updated_at > c.updated_at) return
  qc.setQueryData(keys.conversation(ctx.ws, c.id), c)
  updateLists(qc, ctx, c)
  refreshCounts(qc, ctx.ws)
}

function newestFirst(a: Message, b: Message) {
  if (a.created_at !== b.created_at) return a.created_at < b.created_at ? 1 : -1
  return a.id < b.id ? 1 : -1
}

function addMessage(qc: QueryClient, ctx: LiveContext, m: Message) {
  qc.setQueryData<Messages>(keys.messages(ctx.ws, m.conversation_id), (old) => {
    if (!old || old.pages.length === 0 || old.pages.some((p) => p.items.some((x) => x.id === m.id))) return old
    const [first, ...rest] = old.pages
    return { ...old, pages: [{ ...first, items: [m, ...first.items].sort(newestFirst) }, ...rest] }
  })
  if (m.kind === "event") return
  const c = qc.getQueryData<Conversation>(keys.conversation(ctx.ws, m.conversation_id))
  const cached = c ?? findItem(qc, ctx.ws, m.conversation_id)
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
  const patch: Partial<ConversationListItem> = {}
  if (m.kind === "message" && next.last_message_at === m.created_at) {
    patch.last_message = {
      id: m.id,
      kind: m.kind,
      author_type: m.author.type,
      text: previewText(m.body),
      created_at: m.created_at,
    }
  }
  if (m.author.member_id !== ctx.memberId) patch.unread = true
  if (c) qc.setQueryData(keys.conversation(ctx.ws, c.id), next)
  updateLists(qc, ctx, next, patch)
}

function replaceMessage(qc: QueryClient, ctx: LiveContext, m: Message) {
  qc.setQueryData<Messages>(keys.messages(ctx.ws, m.conversation_id), (old) => {
    if (!old) return old
    let changed = false
    const pages = old.pages.map((p) => ({
      ...p,
      items: p.items.map((x) => {
        if (x.id !== m.id) return x
        if (x.delivery && m.delivery && x.delivery.updated_at > m.delivery.updated_at) return x
        changed = true
        return m
      }),
    }))
    return changed ? { ...old, pages } : old
  })
}

export function applyRead(qc: QueryClient, ws: string, read: ConversationRead) {
  patchItem(qc, ws, read.conversation_id, { unread: read.unread })
}

function knowMember(qc: QueryClient, ws: string, id: string | undefined) {
  if (!id) return
  const members = qc.getQueryData<Member[]>(keys.members(ws))
  if (members && !members.some((m) => m.id === id)) void qc.invalidateQueries({ queryKey: keys.members(ws) })
}

function forgetInbox(qc: QueryClient, ws: string, inboxId: string) {
  for (const q of qc.getQueryCache().findAll({ queryKey: ["ws", ws, "conversation"] })) {
    const c = q.state.data as Conversation | undefined
    if (c?.inbox_id !== inboxId) continue
    void qc.resetQueries({ queryKey: keys.conversation(ws, c.id), exact: true })
    void qc.resetQueries({ queryKey: keys.messages(ws, c.id), exact: true })
  }
}

function byName(a: Inbox, b: Inbox) {
  return a.name.localeCompare(b.name) || (a.id < b.id ? -1 : 1)
}

export function applyEvent(qc: QueryClient, ctx: LiveContext, event: LiveEvent) {
  switch (event.type) {
    case "conversation.created":
    case "conversation.updated":
      knowMember(qc, ctx.ws, event.data.assignee_id)
      setConversation(qc, ctx, event.data)
      return
    case "message.created":
      knowMember(qc, ctx.ws, event.data.author.member_id)
      addMessage(qc, ctx, event.data)
      return
    case "message.updated":
      replaceMessage(qc, ctx, event.data)
      return
    case "conversation.read":
      if (event.data.member_id === ctx.memberId) applyRead(qc, ctx.ws, event.data)
      return
    case "contact.updated": {
      const contact = event.data
      qc.setQueryData<Contact>(keys.contact(ctx.ws, contact.id), contact)
      for (const [key, data] of listCaches(qc, ctx.ws)) {
        if (!data?.pages.some((p) => p.items.some((x) => x.contact.id === contact.id))) continue
        const ref = { id: contact.id, name: contact.name, email: contact.emails[0] }
        qc.setQueryData<Lists>(key, (old) =>
          old
            ? {
                ...old,
                pages: old.pages.map((p) => ({
                  ...p,
                  items: p.items.map((x) => (x.contact.id === contact.id ? { ...x, contact: ref } : x)),
                })),
              }
            : old,
        )
      }
      return
    }
    case "contact.deleted":
      qc.removeQueries({ queryKey: keys.contact(ctx.ws, event.data.id) })
      void qc.invalidateQueries({ queryKey: keys.conversationLists(ctx.ws) })
      refreshCounts(qc, ctx.ws)
      return
    case "inbox.created": {
      const inbox = event.data
      qc.setQueryData<Inbox[]>(keys.inboxes(ctx.ws), (old) =>
        old && !old.some((x) => x.id === inbox.id) ? [...old, inbox].sort(byName) : old,
      )
      return
    }
    case "inbox.updated": {
      const inbox = event.data
      qc.setQueryData<Inbox[]>(keys.inboxes(ctx.ws), (old) =>
        old?.map((x) => (x.id === inbox.id ? inbox : x)).sort(byName),
      )
      qc.setQueryData([...keys.inboxes(ctx.ws), inbox.id], inbox)
      return
    }
    case "inbox.deleted": {
      const id = event.data.id
      qc.setQueryData<Inbox[]>(keys.inboxes(ctx.ws), (old) => old?.filter((x) => x.id !== id))
      qc.removeQueries({ queryKey: [...keys.inboxes(ctx.ws), id] })
      qc.removeQueries({ queryKey: keys.inboxMembers(ctx.ws, id) })
      qc.removeQueries({ queryKey: keys.channels(ctx.ws, id) })
      forgetInbox(qc, ctx.ws, id)
      for (const [key, data] of listCaches(qc, ctx.ws)) {
        if (!data?.pages.some((p) => p.items.some((x) => x.inbox_id === id))) continue
        qc.setQueryData<Lists>(key, (old) =>
          old ? { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.filter((x) => x.inbox_id !== id) })) } : old,
        )
      }
      refreshCounts(qc, ctx.ws)
      return
    }
    case "inbox_access.changed":
      void qc.invalidateQueries({ queryKey: keys.inboxMembers(ctx.ws, event.data.inbox_id) })
      knowMember(qc, ctx.ws, event.data.member_id)
      if (event.data.member_id === ctx.memberId) {
        if (!event.data.granted) forgetInbox(qc, ctx.ws, event.data.inbox_id)
        void qc.invalidateQueries({ queryKey: keys.inboxes(ctx.ws) })
        void qc.invalidateQueries({ queryKey: keys.conversationLists(ctx.ws) })
        void qc.invalidateQueries({ queryKey: ["ws", ctx.ws, "conversation"] })
        void qc.invalidateQueries({ queryKey: ["ws", ctx.ws, "messages"] })
        refreshCounts(qc, ctx.ws)
      }
      return
  }
}
