import type { InfiniteData, QueryClient, QueryKey } from "@tanstack/react-query"

import { ApiError } from "@/lib/api"
import type {
  Broadcast,
  BroadcastProgress,
  Contact,
  Conversation,
  ConversationListItem,
  ConversationPin,
  ConversationRead,
  DraftItem,
  Inbox,
  ListImport,
  ListImportProgress,
  Member,
  Message,
  Migration,
  MigrationFolder,
  MigrationForwardingConfirmationData,
  MigrationProgress,
  RealtimeMessage,
  StatusNotice,
} from "@/lib/api"
import { keys, type ConversationFilters } from "@/lib/keys"

type ConversationPage = { items: ConversationListItem[]; next_cursor?: string }
type Lists = InfiniteData<ConversationPage, string | undefined>
type MessagePage = { items: Message[]; next_cursor?: string }
export type Messages = InfiniteData<MessagePage, string | undefined>

type Event = Exclude<RealtimeMessage, { type: "ready" } | { type: "resync_required" }>
export type LiveEvent = Event extends infer E
  ? E extends { type: infer T; data: infer D }
    ? { type: T; data: D }
    : never
  : never

export type LiveContext = { ws: string; memberId: string }

const PREVIEW_RUNES = 140

function previewText(body: string) {
  const text = body.split(/\s+/).filter(Boolean).join(" ")
  const runes = [...text]
  return runes.length > PREVIEW_RUNES ? runes.slice(0, PREVIEW_RUNES).join("").trim() + "…" : text
}

function pageOrigin(url?: string) {
  if (!url) return ""
  try {
    return new URL(url).origin
  } catch {
    return ""
  }
}

function matches(c: Conversation & { pinned_at?: string }, f: ConversationFilters, memberId: string) {
  if (!!f.spam !== c.spam) return false
  if (!!f.trashed !== !!c.trashed_at) return false
  if (f.pinned !== undefined && !!c.pinned_at !== f.pinned) return false
  if (f.status?.length && !f.status.includes(c.status)) return false
  if (f.kind?.length && !f.kind.includes(c.kind)) return false
  if (f.category && c.kind !== "feedback") return false
  if (f.channel_kind?.length && !(c.channel_kind && f.channel_kind.includes(c.channel_kind))) return false
  if (f.site && pageOrigin(c.page?.page_url ?? c.feedback?.page_url) !== f.site) return false
  if (f.category && c.feedback?.category !== f.category) return false
  if (f.page && (c.page?.page_url ?? c.feedback?.page_url) !== f.page) return false
  if (f.inbox_id && c.inbox_id !== f.inbox_id) return false
  if (f.broadcast_id && c.broadcast_id !== f.broadcast_id) return false
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
    if (f.trashed) {
      void qc.invalidateQueries({ queryKey: key, exact: true })
      continue
    }
    if (f.q || (!base && matches(c, f, ctx.memberId))) {
      void qc.invalidateQueries({ queryKey: key, exact: true })
      continue
    }
    if (!base) continue
    const item: ConversationListItem = { ...base, ...c, ...patch }
    if (!c.trashed_at) {
      delete item.trashed_at
      delete item.trashed_by
      delete item.deletes_at
    }
    qc.setQueryData<Lists>(key, (old) => (old ? placeInList(old, item, matches(item, f, ctx.memberId)) : old))
  }
}

function patchItem(qc: QueryClient, ws: string, id: string, patch: Partial<ConversationListItem>) {
  for (const [key, data] of listCaches(qc, ws)) {
    if (!data?.pages.some((p) => p.items.some((x) => x.id === id))) continue
    qc.setQueryData<Lists>(key, (old) =>
      old
        ? {
            ...old,
            pages: old.pages.map((p) => ({ ...p, items: p.items.map((x) => (x.id === id ? { ...x, ...patch } : x)) })),
          }
        : old
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
      void qc.invalidateQueries({ queryKey: keys.stats(ws) })
      void qc.invalidateQueries({ queryKey: keys.docs(ws) })
      void qc.invalidateQueries({ queryKey: keys.home(ws) })
    }, 400)
  )
}

const listTimers = new Map<string, ReturnType<typeof setTimeout>>()

function refreshLists(qc: QueryClient, ws: string) {
  clearTimeout(listTimers.get(ws))
  listTimers.set(
    ws,
    setTimeout(() => {
      listTimers.delete(ws)
      void qc.invalidateQueries({ queryKey: keys.lists(ws) })
      void qc.invalidateQueries({ queryKey: keys.home(ws) })
    }, 400)
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

function putMessage(qc: QueryClient, ws: string, m: Message) {
  qc.setQueryData<Messages>(keys.messages(ws, m.conversation_id), (old) => {
    if (!old || old.pages.length === 0) return old
    const known = old.pages.flatMap((p) => p.items).find((x) => x.id === m.id)
    if (known && (m.draft || !known.draft)) return old
    const pages = old.pages.map((p) => ({ ...p, items: p.items.filter((x) => x.id !== m.id) }))
    const [first, ...rest] = pages
    return { ...old, pages: [{ ...first, items: [m, ...first.items].sort(newestFirst) }, ...rest] }
  })
}

function dropMessage(qc: QueryClient, ws: string, m: Message) {
  qc.setQueryData<Messages>(keys.messages(ws, m.conversation_id), (old) =>
    old?.pages.some((p) => p.items.some((x) => x.id === m.id))
      ? { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.filter((x) => x.id !== m.id) })) }
      : old
  )
}

function updateDraft(qc: QueryClient, ws: string, m: Message) {
  qc.setQueryData<Messages>(keys.messages(ws, m.conversation_id), (old) =>
    old?.pages.some((p) => p.items.some((x) => x.id === m.id && x.draft))
      ? {
          ...old,
          pages: old.pages.map((p) => ({ ...p, items: p.items.map((x) => (x.id === m.id && x.draft ? m : x)) })),
        }
      : old
  )
}

function addMessage(qc: QueryClient, ctx: LiveContext, m: Message) {
  putMessage(qc, ctx.ws, m)
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
  if ((m.sent_by ?? m.author).member_id !== ctx.memberId) patch.unread = true
  if (m.kind === "message" && m.direction === "out") refreshCounts(qc, ctx.ws)
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

export function applyPin(qc: QueryClient, ws: string, pin: ConversationPin) {
  patchItem(qc, ws, pin.conversation_id, { pinned_at: pin.pinned_at })
  for (const [key] of listCaches(qc, ws)) {
    if (((key as QueryKey)[3] as ConversationFilters).pinned !== undefined)
      void qc.invalidateQueries({ queryKey: key, exact: true })
  }
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

function followMerge(qc: QueryClient, ws: string, from: string, to: string) {
  for (const q of qc.getQueryCache().findAll({ queryKey: ["ws", ws, "conversation"] })) {
    const c = q.state.data as Conversation | undefined
    if (c?.contact_id === from) qc.setQueryData<Conversation>(q.queryKey, { ...c, contact_id: to })
  }
}

function refreshMentions(qc: QueryClient, ws: string) {
  void qc.invalidateQueries({ queryKey: keys.mentions(ws) })
}

function refreshDrafts(qc: QueryClient, ws: string) {
  void qc.invalidateQueries({ queryKey: keys.draftLists(ws) })
}

function listsDraftsOf(qc: QueryClient, ws: string, test: (item: DraftItem) => boolean) {
  return qc
    .getQueriesData<InfiniteData<{ items: DraftItem[] }, string | undefined>>({ queryKey: keys.draftLists(ws) })
    .some(([, data]) => data?.pages.some((p) => p.items.some(test)))
}

type BroadcastPages = InfiniteData<{ items: Broadcast[]; next_cursor?: string }, string | undefined>

function patchBroadcastPages(qc: QueryClient, ws: string, id: string, patch: (b: Broadcast) => Broadcast) {
  for (const [key, data] of qc.getQueriesData<BroadcastPages>({ queryKey: [...keys.broadcasts(ws), "pages"] })) {
    if (!data?.pages.some((p) => p.items.some((x) => x.id === id))) continue
    qc.setQueryData<BroadcastPages>(key, (old) =>
      old
        ? { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.map((x) => (x.id === id ? patch(x) : x)) })) }
        : old
    )
  }
}

const broadcastTimers = new Map<string, ReturnType<typeof setTimeout>>()

function refreshBroadcasts(qc: QueryClient, ws: string, id: string) {
  const timerKey = `${ws}:${id}`
  clearTimeout(broadcastTimers.get(timerKey))
  broadcastTimers.set(
    timerKey,
    setTimeout(() => {
      broadcastTimers.delete(timerKey)
      void qc.invalidateQueries({ queryKey: [...keys.broadcasts(ws), "pages"] })
      void qc.invalidateQueries({ queryKey: keys.broadcastReview(ws, id) })
      void qc.invalidateQueries({ queryKey: keys.broadcastReport(ws, id) })
      void qc.invalidateQueries({ queryKey: [...keys.broadcasts(ws), "deliveries", id] })
      void qc.invalidateQueries({ queryKey: keys.home(ws) })
    }, 400)
  )
}

export function setBroadcast(qc: QueryClient, ws: string, b: Broadcast) {
  const current = qc.getQueryData<Broadcast>(keys.broadcast(ws, b.id))
  if (current && current.updated_at > b.updated_at) return
  qc.setQueryData(keys.broadcast(ws, b.id), b)
  patchBroadcastPages(qc, ws, b.id, () => b)
  refreshBroadcasts(qc, ws, b.id)
}

function applyProgress(qc: QueryClient, ws: string, p: BroadcastProgress) {
  const patch = (b: Broadcast): Broadcast => ({
    ...b,
    state: p.state,
    counts: p.counts,
    waiting_until: p.waiting_until,
  })
  qc.setQueryData<Broadcast>(keys.broadcast(ws, p.id), (old) => (old ? patch(old) : old))
  patchBroadcastPages(qc, ws, p.id, patch)
  void qc.invalidateQueries({ queryKey: keys.broadcastReport(ws, p.id) })
}

type ImportPages = InfiniteData<{ items: ListImport[]; next_cursor?: string }, string | undefined>

function patchImport(qc: QueryClient, ws: string, listId: string, id: string, patch: (i: ListImport) => ListImport) {
  qc.setQueryData<ListImport>(keys.listImport(ws, listId, id), (old) => (old ? patch(old) : old))
  const pages = qc.getQueryData<ImportPages>(keys.listImports(ws, listId))
  if (!pages) return
  if (!pages.pages.some((p) => p.items.some((x) => x.id === id))) {
    void qc.invalidateQueries({ queryKey: keys.listImports(ws, listId) })
    return
  }
  qc.setQueryData<ImportPages>(keys.listImports(ws, listId), {
    ...pages,
    pages: pages.pages.map((p) => ({ ...p, items: p.items.map((x) => (x.id === id ? patch(x) : x)) })),
  })
}

function setImport(qc: QueryClient, ws: string, i: ListImport) {
  const current = qc.getQueryData<ListImport>(keys.listImport(ws, i.list_id, i.id))
  if (current && current.updated_at > i.updated_at) return
  qc.setQueryData(keys.listImport(ws, i.list_id, i.id), i)
  patchImport(qc, ws, i.list_id, i.id, () => i)
  if (i.state === "done" || i.state === "cancelled" || i.state === "failed") {
    void qc.invalidateQueries({ queryKey: keys.list(ws, i.list_id) })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "subscriptions", i.list_id] })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "growth", i.list_id] })
    refreshLists(qc, ws)
  }
}

function applyImportProgress(qc: QueryClient, ws: string, p: ListImportProgress) {
  patchImport(qc, ws, p.list_id, p.id, (i) => ({
    ...i,
    state: p.state,
    rows_total: p.rows_total,
    rows_done: p.rows_done,
    counts: p.counts,
  }))
}

type MigrationPages = InfiniteData<{ items: Migration[]; next_cursor?: string }, string | undefined>

function patchMigration(qc: QueryClient, ws: string, id: string, patch: (m: Migration) => Migration) {
  qc.setQueryData<Migration>(keys.migration(ws, id), (old) => (old ? patch(old) : old))
  const pages = qc.getQueryData<MigrationPages>(keys.migrationPages(ws))
  if (!pages) return
  if (!pages.pages.some((p) => p.items.some((x) => x.id === id))) {
    void qc.invalidateQueries({ queryKey: keys.migrationPages(ws) })
    return
  }
  qc.setQueryData<MigrationPages>(keys.migrationPages(ws), {
    ...pages,
    pages: pages.pages.map((p) => ({ ...p, items: p.items.map((x) => (x.id === id ? patch(x) : x)) })),
  })
}

export function setMigration(qc: QueryClient, ws: string, m: Migration) {
  const current = qc.getQueryData<Migration>(keys.migration(ws, m.id))
  if (current && current.updated_at > m.updated_at) return
  qc.setQueryData(keys.migration(ws, m.id), m)
  patchMigration(qc, ws, m.id, () => m)
  void qc.invalidateQueries({ queryKey: keys.migrationFolders(ws, m.id) })
}

function applyMigrationProgress(qc: QueryClient, ws: string, p: MigrationProgress) {
  const current = qc.getQueryData<Migration>(keys.migration(ws, p.id))
  if (current && current.state !== p.state) void qc.invalidateQueries({ queryKey: keys.migration(ws, p.id) })
  patchMigration(qc, ws, p.id, (m) => ({
    ...m,
    state: p.state,
    counts: p.counts,
    rate_per_minute: p.rate_per_minute,
    seconds_left: p.seconds_left,
    waiting_until: p.waiting_until,
  }))
  const byId = new Map(p.folders.map((f) => [f.id, f]))
  qc.setQueryData<MigrationFolder[]>(keys.migrationFolders(ws, p.id), (old) =>
    old?.map((f) => {
      const n = byId.get(f.id)
      return n
        ? { ...f, imported: n.imported, duplicate: n.duplicate, skipped: n.skipped, failed: n.failed, bytes: n.bytes }
        : f
    })
  )
}

function applyForwardingConfirmation(qc: QueryClient, ws: string, d: MigrationForwardingConfirmationData) {
  patchMigration(qc, ws, d.migration_id, (m) => ({ ...m, forwarding_confirmation: d.confirmation }))
}

function byName(a: Inbox, b: Inbox) {
  return a.name.localeCompare(b.name) || (a.id < b.id ? -1 : 1)
}

function refreshLinked(qc: QueryClient, ws: string, id: string) {
  for (const root of [keys.statusLinked(ws), keys.roadmapLinked(ws)])
    for (const q of qc.getQueryCache().findAll({ queryKey: root })) {
      const data = q.state.data as { pages?: { items: { id: string }[] }[] } | undefined
      if (data?.pages?.some((p) => p.items.some((x) => x.id === id)))
        void qc.invalidateQueries({ queryKey: q.queryKey, exact: true, refetchType: "active" })
    }
}

export function applyEvent(qc: QueryClient, ctx: LiveContext, event: LiveEvent) {
  switch (event.type) {
    case "conversation.deleted": {
      const id = event.data.id
      for (const [key, data] of listCaches(qc, ctx.ws)) {
        if (!data?.pages.some((p) => p.items.some((x) => x.id === id))) continue
        qc.setQueryData<Lists>(key, (old) =>
          old ? { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.filter((x) => x.id !== id) })) } : old
        )
      }
      for (const key of [keys.conversation(ctx.ws, id), keys.messages(ctx.ws, id)]) {
        void qc.cancelQueries({ queryKey: key, exact: true })
        qc.getQueryCache()
          .find({ queryKey: key, exact: true })
          ?.setState({ data: undefined, error: new ApiError(404), status: "error", fetchStatus: "idle" })
      }
      void qc.invalidateQueries({ queryKey: keys.contactSummary(ctx.ws, event.data.contact_id) })
      void qc.invalidateQueries({ queryKey: keys.contact(ctx.ws, event.data.contact_id), refetchType: "active" })
      refreshCounts(qc, ctx.ws)
      refreshMentions(qc, ctx.ws)
      refreshDrafts(qc, ctx.ws)
      return
    }
    case "conversation.trashed":
    case "conversation.restored":
      setConversation(qc, ctx, event.data)
      void qc.invalidateQueries({ queryKey: keys.contactSummary(ctx.ws, event.data.contact_id) })
      refreshMentions(qc, ctx.ws)
      refreshDrafts(qc, ctx.ws)
      return
    case "conversation.created":
    case "conversation.updated":
      knowMember(qc, ctx.ws, event.data.assignee_id)
      setConversation(qc, ctx, event.data)
      refreshLinked(qc, ctx.ws, event.data.id)
      if (
        event.type === "conversation.updated" &&
        listsDraftsOf(qc, ctx.ws, (x) => x.conversation.id === event.data.id)
      )
        refreshDrafts(qc, ctx.ws)
      void qc.invalidateQueries({ queryKey: keys.contact(ctx.ws, event.data.contact_id), refetchType: "active" })
      void qc.invalidateQueries({ queryKey: keys.contactSummary(ctx.ws, event.data.contact_id) })
      if (event.type === "conversation.created") {
        void qc.invalidateQueries({ queryKey: keys.contactPresence(ctx.ws, event.data.contact_id) })
      }
      return
    case "conversation.moved": {
      const { id, inbox_id, previous_inbox_id } = event.data
      for (const [key, data] of listCaches(qc, ctx.ws)) {
        if (!data?.pages.some((p) => p.items.some((x) => x.id === id && x.inbox_id === previous_inbox_id))) continue
        qc.setQueryData<Lists>(key, (old) =>
          old ? { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.filter((x) => x.id !== id) })) } : old
        )
      }
      const inboxes = qc.getQueryData<Inbox[]>(keys.inboxes(ctx.ws))
      if (inboxes && !inboxes.some((x) => x.id === inbox_id)) {
        void qc.resetQueries({ queryKey: keys.conversation(ctx.ws, id), exact: true })
        void qc.resetQueries({ queryKey: keys.messages(ctx.ws, id), exact: true })
      }
      refreshCounts(qc, ctx.ws)
      refreshMentions(qc, ctx.ws)
      refreshDrafts(qc, ctx.ws)
      return
    }
    case "message.created":
      knowMember(qc, ctx.ws, event.data.author.member_id)
      addMessage(qc, ctx, event.data)
      if (event.data.kind === "note" && event.data.mentions?.includes(ctx.memberId)) refreshMentions(qc, ctx.ws)
      if (listsDraftsOf(qc, ctx.ws, (x) => x.draft.id === event.data.id)) refreshDrafts(qc, ctx.ws)
      if (event.data.author.contact_id) {
        void qc.invalidateQueries({ queryKey: keys.contactPresence(ctx.ws, event.data.author.contact_id) })
        if (event.data.email)
          void qc.invalidateQueries({ queryKey: keys.contact(ctx.ws, event.data.author.contact_id) })
      }
      return
    case "message.updated":
      replaceMessage(qc, ctx, event.data)
      return
    case "draft.created":
      putMessage(qc, ctx.ws, event.data)
      refreshDrafts(qc, ctx.ws)
      return
    case "draft.updated":
      updateDraft(qc, ctx.ws, event.data)
      refreshDrafts(qc, ctx.ws)
      return
    case "draft.deleted":
      dropMessage(qc, ctx.ws, event.data)
      refreshDrafts(qc, ctx.ws)
      return
    case "conversation.read":
      if (event.data.member_id !== ctx.memberId) return
      applyRead(qc, ctx.ws, event.data)
      refreshMentions(qc, ctx.ws)
      return
    case "conversation.pin":
      if (event.data.member_id === ctx.memberId) applyPin(qc, ctx.ws, event.data)
      return
    case "contact.read": {
      const { conversation_id: id, read_at } = event.data
      qc.setQueryData<Conversation>(keys.conversation(ctx.ws, id), (old) =>
        old && (!old.last_read_by_contact_at || Date.parse(old.last_read_by_contact_at) < Date.parse(read_at))
          ? { ...old, last_read_by_contact_at: read_at }
          : old
      )
      patchItem(qc, ctx.ws, id, { last_read_by_contact_at: read_at })
      return
    }
    case "contact.updated": {
      const contact = event.data
      qc.setQueryData<Contact>(keys.contact(ctx.ws, contact.id), (old) => ({
        ...contact,
        activity: contact.activity ?? old?.activity,
      }))
      void qc.invalidateQueries({ queryKey: keys.contactSearch(ctx.ws) })
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
            : old
        )
      }
      return
    }
    case "contact.deleted":
      if (event.data.merged_into_id) followMerge(qc, ctx.ws, event.data.id, event.data.merged_into_id)
      qc.removeQueries({ queryKey: keys.contact(ctx.ws, event.data.id) })
      qc.removeQueries({ queryKey: keys.contactPresence(ctx.ws, event.data.id) })
      void qc.invalidateQueries({ queryKey: keys.contactSearch(ctx.ws) })
      void qc.invalidateQueries({ queryKey: keys.conversationLists(ctx.ws) })
      refreshCounts(qc, ctx.ws)
      return
    case "inbox.created": {
      const inbox = event.data
      qc.setQueryData<Inbox[]>(keys.inboxes(ctx.ws), (old) =>
        old && !old.some((x) => x.id === inbox.id) ? [...old, inbox].sort(byName) : old
      )
      return
    }
    case "inbox.updated": {
      const inbox = event.data
      const keep = (old?: Inbox): Inbox => ({ ...inbox, ses_tenant: inbox.ses_tenant ?? old?.ses_tenant })
      qc.setQueryData<Inbox[]>(keys.inboxes(ctx.ws), (old) =>
        old?.map((x) => (x.id === inbox.id ? keep(x) : x)).sort(byName)
      )
      qc.setQueryData<Inbox>([...keys.inboxes(ctx.ws), inbox.id], keep)
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
          old
            ? { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.filter((x) => x.inbox_id !== id) })) }
            : old
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
        refreshMentions(qc, ctx.ws)
        refreshDrafts(qc, ctx.ws)
      }
      return
    case "subscription.created":
    case "subscription.updated":
      refreshLists(qc, ctx.ws)
      return
    case "broadcast.updated":
      setBroadcast(qc, ctx.ws, event.data.broadcast)
      return
    case "broadcast.progress":
      applyProgress(qc, ctx.ws, event.data)
      return
    case "list_import.updated":
      setImport(qc, ctx.ws, event.data)
      return
    case "list_import.progress":
      applyImportProgress(qc, ctx.ws, event.data)
      return
    case "migration.updated":
      setMigration(qc, ctx.ws, event.data.migration)
      return
    case "migration.progress":
      applyMigrationProgress(qc, ctx.ws, event.data)
      return
    case "migration.forwarding_confirmation":
      applyForwardingConfirmation(qc, ctx.ws, event.data)
      return
    case "discussion.created":
    case "discussion.post_created":
    case "discussion.answered":
    case "discussion.updated":
    case "discussion.stage_changed":
      void qc.invalidateQueries({ queryKey: keys.discussions(ctx.ws) })
      void qc.invalidateQueries({ queryKey: keys.roadmap(ctx.ws) })
      refreshCounts(qc, ctx.ws)
      return
    case "roadmap.board.updated":
      void qc.invalidateQueries({ queryKey: keys.roadmap(ctx.ws) })
      void qc.invalidateQueries({ queryKey: keys.discussions(ctx.ws) })
      return
    case "status.page.updated":
    case "status.component.updated":
    case "status.incident.created":
    case "status.incident.updated":
    case "status.incident.resolved":
    case "status.monitor.down":
    case "status.monitor.up":
      void qc.invalidateQueries({ queryKey: keys.status(ctx.ws) })
      return
    case "help.article.published":
    case "help.article.unpublished":
    case "help.article.deleted":
      void qc.invalidateQueries({ queryKey: keys.help(ctx.ws) })
      return
    case "help.center.updated":
      void qc.invalidateQueries({ queryKey: keys.help(ctx.ws) })
      return
    case "changelog.updated":
      void qc.invalidateQueries({ queryKey: keys.changelog(ctx.ws) })
      void qc.invalidateQueries({ queryKey: keys.changelogs(ctx.ws) })
      return
    case "help.article.updated":
      void qc.invalidateQueries({ queryKey: keys.helpArticle(ctx.ws, event.data.center_id, event.data.article_id) })
      void qc.invalidateQueries({ queryKey: ["ws", ctx.ws, "help", "articles", event.data.center_id] })
      return
    case "changelog.entry.published":
    case "changelog.entry.unpublished":
    case "changelog.entry.deleted":
      void qc.invalidateQueries({ queryKey: keys.changelog(ctx.ws) })
      return
    case "changelog.entry.updated":
      void qc.invalidateQueries({ queryKey: keys.changelogEntry(ctx.ws, event.data.changelog_id, event.data.entry_id) })
      void qc.invalidateQueries({ queryKey: ["ws", ctx.ws, "changelog", "entries", event.data.changelog_id] })
      void qc.invalidateQueries({ queryKey: keys.changelogs(ctx.ws) })
      return
    case "status_notice.updated": {
      const n = event.data
      const key = keys.statusNotices(ctx.ws, n.page_id, n.incident_id)
      const cur = qc.getQueryData<StatusNotice[]>(key)
      if (cur?.some((x) => x.id === n.id))
        qc.setQueryData(
          key,
          cur.map((x) => (x.id === n.id ? n : x))
        )
      else void qc.invalidateQueries({ queryKey: key })
      return
    }
    case "analytics.alert": {
      const data = keys.analyticsData(ctx.ws, event.data.property.id)
      void qc.invalidateQueries({ queryKey: [...data, "alerts"] })
      void qc.invalidateQueries({ queryKey: [...data, "annotations"] })
      return
    }
  }
}
