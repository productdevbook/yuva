"use client"

import * as React from "react"
import {
  ApiError,
  createYuvaClient,
  type ClientAttachment,
  type ClientContact,
  type ClientConversation,
  type ClientInbox,
  type ClientMessage,
  type ClientRealtimeMessage,
  type ConnectionState,
  type IdentityTokenSource,
  type YuvaClient,
} from "useyuva"

type Rating = Parameters<YuvaClient["rate"]>[1]

const PAGE_SIZE = 30
const TYPING_REPEAT_MS = 3000
const TYPING_IDLE_MS = 5000
const TYPING_SHOWN_MS = 6000

export interface UseYuvaChatOptions {
  server?: string
  channel?: string
  client?: YuvaClient
  identityToken?: IdentityTokenSource | null
  helpArticleId?: string
}

export type YuvaChatMessageStatus = "sent" | "sending" | "error"

export interface YuvaChatFile {
  name: string
  size: number
  type: string
}

export interface YuvaChatMessage {
  id: string
  clientId?: string
  direction: ClientMessage["direction"]
  author: ClientMessage["author"]
  body: string
  attachments: ClientAttachment[]
  files: YuvaChatFile[]
  createdAt: string
  status: YuvaChatMessageStatus
}

export interface YuvaChatSendInput {
  body?: string
  files?: File[]
}

export interface YuvaChatState {
  loading: boolean
  error: unknown
  inbox: ClientInbox | null
  contact: ClientContact | null
  connection: ConnectionState["state"]
  conversations: ClientConversation[]
  hasMoreConversations: boolean
  loadMoreConversations(): Promise<void>
  activeId: string | null
  conversation: ClientConversation | null
  open(id: string | null): Promise<void>
  removed: boolean
  threadLoading: boolean
  messages: YuvaChatMessage[]
  hasEarlier: boolean
  loadEarlier(): Promise<void>
  typing: ClientMessage["author"] | null
  memberReadAt: string | null
  send(input: YuvaChatSendInput): Promise<void>
  retry(id: string): Promise<void>
  discard(id: string): void
  setTyping(on: boolean): void
  markRead(): Promise<void>
  rate(rating: Rating, comment?: string): Promise<void>
  openAttachment(attachment: ClientAttachment): Promise<void>
  saveEmail(email: string): Promise<void>
}

function newId(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID()
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, "0")).join("")
}

function fromServer(message: ClientMessage): YuvaChatMessage {
  return {
    id: message.id,
    clientId: message.client_id,
    direction: message.direction,
    author: message.author,
    body: message.body,
    attachments: message.attachments,
    files: [],
    createdAt: message.created_at,
    status: "sent",
  }
}

function preview(body: string): string {
  const text = body.replace(/\s+/g, " ").trim()
  return text.length > 140 ? `${text.slice(0, 137)}...` : text
}

function merge(list: YuvaChatMessage[], incoming: YuvaChatMessage): YuvaChatMessage[] {
  const at = list.findIndex(
    (item) => item.id === incoming.id || (!!incoming.clientId && item.clientId === incoming.clientId)
  )
  if (at < 0) return [...list, incoming].sort(byTime)
  const next = list.slice()
  next[at] = incoming
  return next.sort(byTime)
}

function byTime(a: YuvaChatMessage, b: YuvaChatMessage): number {
  return Date.parse(a.createdAt) - Date.parse(b.createdAt)
}

function upsertConversation(list: ClientConversation[], next: ClientConversation): ClientConversation[] {
  const rest = list.filter((item) => item.id !== next.id)
  return [next, ...rest].sort(
    (a, b) => Date.parse(b.last_message_at ?? b.created_at) - Date.parse(a.last_message_at ?? a.created_at)
  )
}

export function useYuvaChat({
  server,
  channel,
  client: given,
  identityToken,
  helpArticleId,
}: UseYuvaChatOptions): YuvaChatState {
  const identity = React.useRef(identityToken)
  const helpArticle = React.useRef(helpArticleId)
  React.useEffect(() => {
    identity.current = identityToken
    helpArticle.current = helpArticleId
  })
  const [own, setOwn] = React.useState<YuvaClient | null>(null)
  React.useEffect(() => {
    if (given || !server || !channel) {
      // oxlint-disable-next-line react/set-state-in-effect -- the client is made after mount, so the server and the first client render agree
      setOwn(null)
      return
    }
    setOwn(createYuvaClient({ server, channel, identityToken: () => identity.current?.() ?? null }))
  }, [given, server, channel])
  const client = given ?? own

  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<unknown>(null)
  const [inbox, setInbox] = React.useState<ClientInbox | null>(null)
  const [contact, setContact] = React.useState<ClientContact | null>(null)
  const [connection, setConnection] = React.useState<ConnectionState["state"]>("closed")
  const [conversations, setConversations] = React.useState<ClientConversation[]>([])
  const [conversationCursor, setConversationCursor] = React.useState<string | undefined>()
  const [activeId, setActiveId] = React.useState<string | null>(null)
  const [removed, setRemoved] = React.useState(false)
  const [threadLoading, setThreadLoading] = React.useState(false)
  const [messages, setMessages] = React.useState<YuvaChatMessage[]>([])
  const [messageCursor, setMessageCursor] = React.useState<string | undefined>()
  const [typing, setTypingAuthor] = React.useState<ClientMessage["author"] | null>(null)

  const active = React.useRef<string | null>(null)
  const generation = React.useRef(0)
  const files = React.useRef(new Map<string, File[]>())
  const typingShown = React.useRef<ReturnType<typeof setTimeout> | null>(null)
  const typingOut = React.useRef({ on: false, at: 0, idle: null as ReturnType<typeof setTimeout> | null })

  const clearTypingShown = () => {
    if (typingShown.current) clearTimeout(typingShown.current)
    typingShown.current = null
  }

  const forget = React.useCallback(() => {
    generation.current++
    active.current = null
    setActiveId(null)
    setMessages([])
    setMessageCursor(undefined)
    setThreadLoading(false)
    clearTypingShown()
    setTypingAuthor(null)
  }, [])

  const loadConversations = React.useCallback(async (target: YuvaClient) => {
    const page = await target.listConversations({ limit: 20 })
    setConversations(page.items)
    setConversationCursor(page.next_cursor)
  }, [])

  const loadThread = React.useCallback(async (target: YuvaClient, id: string) => {
    const mine = ++generation.current
    active.current = id
    setActiveId(id)
    setThreadLoading(true)
    setMessages([])
    setMessageCursor(undefined)
    clearTypingShown()
    setTypingAuthor(null)
    try {
      const [conversation, page] = await Promise.all([
        target.getConversation(id),
        target.listMessages(id, { order: "desc", limit: PAGE_SIZE }),
      ])
      if (generation.current !== mine) return
      setConversations((list) => upsertConversation(list, conversation))
      setMessages((list) =>
        page.items.map(fromServer).reduce(
          merge,
          list.filter((item) => item.status !== "sent")
        )
      )
      setMessageCursor(page.next_cursor)
    } finally {
      if (generation.current === mine) setThreadLoading(false)
    }
  }, [])

  React.useEffect(() => {
    if (!client) return
    let live = true
    const apply = (event: ClientRealtimeMessage) => {
      if (event.type === "inbox.updated") setInbox(event.data)
      else if (event.type === "contact.updated") setContact(event.data)
      else if (event.type === "presence")
        setInbox((current) => (current ? { ...current, presence: event.data } : current))
      else if (event.type === "conversation.created") setConversations((list) => upsertConversation(list, event.data))
      else if (event.type === "conversation.removed") {
        const { id } = event.data
        setConversations((list) => list.filter((item) => item.id !== id))
        if (active.current === id) {
          forget()
          setRemoved(true)
        }
      } else if (event.type === "conversation.updated") {
        const { id, status, can_rate, rating } = event.data
        setConversations((list) => list.map((item) => (item.id === id ? { ...item, status, can_rate, rating } : item)))
      } else if (event.type === "message.created" || event.type === "message.updated") {
        const message = event.data
        const incoming = message.direction === "out"
        if (message.conversation_id === active.current) {
          setMessages((list) => merge(list, fromServer(message)))
          if (incoming) {
            clearTypingShown()
            setTypingAuthor(null)
          }
        }
        if (event.type === "message.created") {
          setConversations((list) => {
            const current = list.find((item) => item.id === message.conversation_id)
            if (!current) return list
            const seen = message.conversation_id === active.current && document.visibilityState === "visible"
            return upsertConversation(list, {
              ...current,
              last_message: {
                id: message.id,
                author_type: message.author.type,
                text: preview(message.body),
                created_at: message.created_at,
              },
              last_message_at: message.created_at,
              unread: incoming && !seen ? true : current.unread,
            })
          })
        }
      } else if (event.type === "read") {
        const { conversation_id, read_at } = event.data
        setConversations((list) =>
          list.map((item) => (item.id === conversation_id ? { ...item, last_read_by_member_at: read_at } : item))
        )
      } else if (event.type === "typing") {
        if (event.data.conversation_id !== active.current) return
        clearTypingShown()
        if (!event.data.typing) return setTypingAuthor(null)
        setTypingAuthor(event.data.author)
        typingShown.current = setTimeout(() => setTypingAuthor(null), TYPING_SHOWN_MS)
      } else if (event.type === "resync_required") {
        void loadConversations(client).catch(() => undefined)
        if (active.current) void loadThread(client, active.current).catch(() => undefined)
      }
    }
    const offs = [
      client.on("event", apply),
      client.on("connection", ({ state }) => setConnection(state)),
      client.on("session", ({ session }) => {
        setInbox(session.inbox)
        setContact(session.contact)
      }),
    ]
    // oxlint-disable-next-line react/set-state-in-effect -- loading from the client is syncing with an external system
    setLoading(true)
    ;(async () => {
      const session = await client.start()
      if (!live) return
      setInbox(session.inbox)
      setContact(session.contact)
      await loadConversations(client)
      if (!live) return
      setError(null)
      setLoading(false)
      await client.connect().catch(() => undefined)
    })().catch((failure: unknown) => {
      if (!live) return
      setError(failure)
      setLoading(false)
    })
    return () => {
      live = false
      for (const off of offs) off()
      client.disconnect()
      clearTypingShown()
      forget()
      setConversations([])
    }
  }, [client, loadConversations, loadThread, forget])

  const open = React.useCallback(
    async (id: string | null) => {
      if (!client) return
      setRemoved(false)
      if (id === null) {
        forget()
        return
      }
      await loadThread(client, id)
    },
    [client, loadThread, forget]
  )

  const loadMoreConversations = React.useCallback(async () => {
    if (!client || !conversationCursor) return
    const page = await client.listConversations({ limit: 20, cursor: conversationCursor })
    setConversations((list) => [...list, ...page.items.filter((item) => !list.some((known) => known.id === item.id))])
    setConversationCursor(page.next_cursor)
  }, [client, conversationCursor])

  const loadEarlier = React.useCallback(async () => {
    const id = active.current
    if (!client || !id || !messageCursor) return
    const mine = generation.current
    const page = await client.listMessages(id, { order: "desc", limit: PAGE_SIZE, cursor: messageCursor })
    if (generation.current !== mine) return
    setMessages((list) => page.items.map(fromServer).reduce(merge, list))
    setMessageCursor(page.next_cursor)
  }, [client, messageCursor])

  const deliver = React.useCallback(
    async (local: YuvaChatMessage) => {
      if (!client) return
      const clientId = local.clientId!
      const input = { body: local.body || undefined, files: files.current.get(clientId), client_id: clientId }
      setMessages((list) => merge(list, { ...local, status: "sending" }))
      try {
        const id = active.current
        let sent: ClientMessage
        if (id) {
          sent = await client.sendMessage(id, input)
        } else {
          const created = await client.startConversation({ ...input, help_article_id: helpArticle.current })
          sent = created.message
          active.current = created.conversation.id
          setActiveId(created.conversation.id)
          setConversations((list) => upsertConversation(list, created.conversation))
        }
        files.current.delete(clientId)
        if (sent.conversation_id === active.current) setMessages((list) => merge(list, fromServer(sent)))
      } catch (failure) {
        if (!(failure instanceof ApiError)) throw failure
        setMessages((list) => merge(list, { ...local, status: "error" }))
      }
    },
    [client]
  )

  const send = React.useCallback(
    async ({ body, files: picked = [] }: YuvaChatSendInput) => {
      const text = body?.trim() ?? ""
      if (!client || (!text && picked.length === 0)) return
      const clientId = newId()
      files.current.set(clientId, picked)
      const local: YuvaChatMessage = {
        id: `local:${clientId}`,
        clientId,
        direction: "in",
        author: { type: "contact" },
        body: text,
        attachments: [],
        files: picked.map((file) => ({ name: file.name, size: file.size, type: file.type })),
        createdAt: new Date().toISOString(),
        status: "sending",
      }
      typingOut.current.on = false
      if (typingOut.current.idle) clearTimeout(typingOut.current.idle)
      await deliver(local)
    },
    [client, deliver]
  )

  const retry = React.useCallback(
    async (id: string) => {
      const local = messages.find((item) => item.id === id && item.status === "error")
      if (local) await deliver({ ...local, createdAt: new Date().toISOString() })
    },
    [messages, deliver]
  )

  const discard = React.useCallback((id: string) => {
    setMessages((list) => {
      const gone = list.find((item) => item.id === id)
      if (gone?.clientId) files.current.delete(gone.clientId)
      return list.filter((item) => item.id !== id)
    })
  }, [])

  const setTyping = React.useCallback(
    (on: boolean) => {
      const id = active.current
      const state = typingOut.current
      if (!client || !id) return
      if (state.idle) clearTimeout(state.idle)
      state.idle = null
      if (on) {
        state.idle = setTimeout(() => {
          state.idle = null
          if (!state.on) return
          state.on = false
          void client.setTyping(id, false).catch(() => undefined)
        }, TYPING_IDLE_MS)
        if (state.on && Date.now() - state.at < TYPING_REPEAT_MS) return
      } else if (!state.on) return
      state.on = on
      state.at = Date.now()
      void client.setTyping(id, on).catch(() => undefined)
    },
    [client]
  )

  const markRead = React.useCallback(async () => {
    const id = active.current
    if (!client || !id) return
    const state = await client.markRead(id)
    setConversations((list) => list.map((item) => (item.id === id ? { ...item, unread: state.unread } : item)))
  }, [client])

  const rate = React.useCallback(
    async (rating: Rating, comment?: string) => {
      const id = active.current
      if (!client || !id) return
      const updated = await client.rate(id, rating, comment)
      setConversations((list) => upsertConversation(list, updated))
    },
    [client]
  )

  const openAttachment = React.useCallback(
    async (attachment: ClientAttachment) => {
      if (!client) return
      const blob = await client.attachment(attachment.id)
      const url = URL.createObjectURL(new Blob([blob], { type: attachment.content_type }))
      const link = document.createElement("a")
      link.href = url
      link.download = attachment.filename
      link.rel = "noopener"
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 60_000)
    },
    [client]
  )

  const saveEmail = React.useCallback(
    async (email: string) => {
      if (!client) return
      setContact(await client.setEmail(email))
    },
    [client]
  )

  const conversation = conversations.find((item) => item.id === activeId) ?? null
  const lastIncoming = [...messages].reverse().find((item) => item.direction === "out" && item.status === "sent")?.id
  const unread = conversation?.unread ?? false
  React.useEffect(() => {
    if (!unread || !lastIncoming || document.visibilityState !== "visible") return
    void markRead().catch(() => undefined)
  }, [unread, lastIncoming, markRead])

  return {
    loading,
    error,
    inbox,
    contact,
    connection,
    conversations,
    hasMoreConversations: !!conversationCursor,
    loadMoreConversations,
    activeId,
    conversation,
    open,
    removed,
    threadLoading,
    messages,
    hasEarlier: !!messageCursor,
    loadEarlier,
    typing,
    memberReadAt: conversation?.last_read_by_member_at ?? null,
    send,
    retry,
    discard,
    setTyping,
    markRead,
    rate,
    openAttachment,
    saveEmail,
  }
}
