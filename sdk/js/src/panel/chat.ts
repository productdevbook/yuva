import { setupI18n, type I18n, type MessageDescriptor, type Messages } from "@lingui/core"
import { msg } from "@lingui/core/macro"
import { messages as en } from "../locales/en/panel"
import {
  launcherOf,
  readStored,
  writeStored,
  type AppHandle,
  type AppKind,
  type AppParams,
  type AppsModule,
  type PanelController,
  type PanelHost,
} from "../types"
import {
  ApiError,
  type ClientAttachment,
  type MessageCard,
  type ClientContact,
  type ClientConversation,
  type ClientInbox,
  type ClientMessage,
  type ClientRealtimeMessage,
  type Rating,
} from "../client/api"
import { createYuvaClient, randomId, type YuvaClient } from "../client/client"
import { addStyles } from "../styles"
import {
  authorKey,
  byActivity,
  byCreated,
  dayLabel,
  el,
  fileKind,
  fileSize,
  iconButton,
  initialsOf,
  isImage,
  limitSize,
  sameDay,
  textButton,
  time,
} from "./helpers"
import { acceptAttribute, allowsType, canAttach, fileProblem } from "../attachments"
import { canCapture, captureScreen } from "./capture"
import { countUpdates } from "../updates"
import { appTitle, buildHome } from "./home"
import { icons } from "./icons"
import { styles } from "./styles"
import { richText } from "./text"

declare const YUVA_CHAT_APPS: string | undefined
declare const YUVA_PANEL_LOCALES: string | undefined

const loadMessages = (locale: string): Promise<Messages> =>
  typeof YUVA_PANEL_LOCALES === "string"
    ? import(/* @vite-ignore */ new URL(localeFile(YUVA_PANEL_LOCALES, locale), import.meta.url).href)
        .then((module: { messages: Messages }) => module.messages)
        .catch(() => en)
    : import("../catalogs/panel").then((module) => module.catalogs[locale] ?? en)

const localeFile = (map: string, locale: string): string =>
  (JSON.parse(map) as Record<string, string>)[locale] ?? "about:blank"

const loadApps = (): Promise<AppsModule> =>
  typeof YUVA_CHAT_APPS === "string"
    ? (import(/* @vite-ignore */ new URL(YUVA_CHAT_APPS, import.meta.url).href) as Promise<AppsModule>)
    : import("../apps/apps")

const impactRank = { none: 0, minor: 1, major: 2, critical: 3 }
const maxFiles = 10
const groupGap = 5 * 60_000
const inboxRefreshGap = 60_000
const maxTimer = 2 ** 31 - 1

type View = { kind: "home" } | { kind: "list" } | { kind: "thread"; id: string | null } | { kind: "app"; app: AppKind }

interface Pending {
  client_id: string
  conversation: string | null
  body: string
  files: File[]
  state: "sending" | "failed" | "refused"
  status?: number
  help_article_id?: string
}

interface RatingDraft {
  choice: Rating
  state: "choosing" | "sending" | "failed"
}

interface Thread {
  messages: ClientMessage[]
  cursor?: string
  complete: boolean
  loading: boolean
}

type Entry = { kind: "message"; message: ClientMessage } | { kind: "pending"; pending: Pending }

type EmailMode = "sent" | "bad" | "ask" | "confirmed" | "saved"

export class Chat implements PanelController {
  #host: PanelHost
  #i18n!: I18n
  #client: YuvaClient
  #inbox: ClientInbox | null = null
  #updates = 0
  #updatesKey = ""
  #inboxAt = 0
  #contact: ClientContact | null = null
  #phase: "loading" | "ready" | "error" = "loading"
  #error = ""
  #starting: Promise<void> | null = null
  #beginning: Promise<void> | null = null
  #conversations = new Map<string, ClientConversation>()
  #conversationCursor: string | undefined
  #threads = new Map<string, Thread>()
  #pending: Pending[] = []
  #typing = new Map<string, { name: string; timer: ReturnType<typeof setTimeout> }>()
  #view: View | null = null
  #reconnecting = false
  #files: File[] = []
  #typingSentAt = 0
  #typingIdle: ReturnType<typeof setTimeout> | null = null
  #blobs = new Map<string, string>()
  #blobLoads = new Map<string, Promise<string>>()
  #avatars = new Map<string, boolean>()
  #emailSaved = false
  #emailError = ""
  #emailNote = ""
  #emailEditing = false
  #emailBusy = false
  #emailKey = ""
  #emailTimer: ReturnType<typeof setTimeout> | null = null
  #connectedBefore = false
  #ratings = new Map<string, RatingDraft>()
  #rateCard = el("div", "card rate")
  #rateComment = el("textarea")
  #rateKey = ""
  #rateFor: string | null = null
  #lists = false
  #listsLoaded = false
  #app: { kind: AppKind; handle: AppHandle } | null = null
  #appLoad = 0
  #appError = false
  #helpArticle: string | null = null
  #missed = 0
  #loaded = ""
  ready: Promise<void>

  #header = el("header", "header")
  #banner = el("div", "banner")
  #body = el("div", "body")
  #scroll = el("div", "scroll")
  #flash = ""
  #notice = el("div", "notice")
  #email = el("div", "")
  #emailInput = el("input", "field")
  #composer = el("div", "sec composer")
  #chips = el("div", "chips")
  #fileNote = el("p", "error")
  #textarea = el("textarea")
  #fileInput = el("input")
  #sendButton: HTMLButtonElement
  #attachButton: HTMLButtonElement
  #shotButton: HTMLButtonElement
  #jump = el("button", "jump")
  #hint = el("span", "hintkey")
  #live = el("div", "sr")
  #scrollThread: string | null | undefined
  #atEnd = true

  constructor(host: PanelHost) {
    this.#host = host
    const { server, channel } = host.config()
    this.#client = createYuvaClient({ server, channel: channel ?? "", identityToken: () => host.identityToken() })
    this.#client.on("event", (message) => this.#event(message))
    this.#client.on("connection", ({ state, attempt }) => {
      if (state !== "reconnecting") return
      this.#reconnecting = attempt > 0
      this.#renderBanner()
    })
    this.#client.on("session", ({ session, renewed }) => {
      this.#setInbox(session.inbox)
      this.#contact = session.contact
      if (renewed) void this.#reload()
      else if (!this.#beginning) void this.#client.connect().catch(() => undefined)
    })
    this.ready = this.#setLocale()
    this.#live.setAttribute("aria-live", "polite")
    this.#scroll.setAttribute("role", "log")
    this.#scroll.addEventListener("scroll", () => {
      this.#atEnd = this.#scroll.scrollHeight - this.#scroll.scrollTop - this.#scroll.clientHeight < 48
      if (this.#atEnd) this.#missed = 0
      this.#syncJump()
      if (this.#scroll.scrollTop < 60 && this.#view?.kind === "thread" && this.#view.id)
        void this.#loadMessages(this.#view.id, true)
    })
    this.#jump.type = "button"
    this.#jump.addEventListener("click", () => {
      this.#missed = 0
      this.#scrollToEnd()
      this.#syncJump()
    })
    this.#textarea.rows = 2
    this.#textarea.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
        event.preventDefault()
        void this.#send()
      }
    })
    this.#textarea.addEventListener("input", () => {
      this.#autosize()
      this.#syncSend()
      this.#typingInput()
    })
    this.#textarea.addEventListener("paste", (event) => {
      const files = Array.from(event.clipboardData?.files ?? [])
      if (files.length === 0) return
      event.preventDefault()
      this.#addFiles(files)
    })
    this.#fileInput.type = "file"
    this.#fileInput.multiple = true
    this.#fileInput.hidden = true
    this.#fileInput.addEventListener("change", () => {
      this.#addFiles(Array.from(this.#fileInput.files ?? []))
      this.#fileInput.value = ""
    })
    this.#attachButton = iconButton(icons.attach, "", () => this.#fileInput.click(), "icon tool")
    this.#shotButton = iconButton(icons.shot, "", () => void this.#capture(), "icon tool")
    this.#shotButton.hidden = !canCapture()
    this.#sendButton = iconButton(icons.send, "", () => void this.#send(), "send")
    const tools = el("div", "tools")
    tools.append(
      this.#attachButton,
      this.#shotButton,
      el("span", "grow"),
      this.#hint,
      this.#sendButton,
      this.#fileInput
    )
    const box = el("form", "box")
    box.addEventListener("submit", (event) => {
      event.preventDefault()
      void this.#send()
    })
    this.#fileNote.hidden = true
    this.#fileNote.setAttribute("role", "alert")
    box.append(this.#chips, this.#textarea, tools)
    this.#composer.append(this.#jump, this.#fileNote, box)

    this.#emailInput.type = "email"
    this.#emailInput.required = true
    this.#emailInput.autocomplete = "email"
    this.#rateComment.rows = 2
    this.#rateComment.maxLength = 2000
    this.#rateComment.dir = "auto"

    document.addEventListener("visibilitychange", this.#onVisible)
    window.addEventListener("online", this.#onVisible)
    void this.ready.then(() => {
      this.#renderAll()
      void this.#start()
    })
  }

  update(): void {
    void this.#setLocale().then(() => {
      this.#emailKey = ""
      this.#rateKey = ""
      this.#renderAll()
    })
  }

  opened(): void {
    if (this.#phase === "error") {
      this.#starting = null
      void this.#start()
      return
    }
    if (this.#phase === "ready") {
      if (this.#client.token) void this.#refreshSession()
      else if (this.#inboxIsStale())
        void this.#preview()
          .then(() => this.#render())
          .catch(() => undefined)
      this.#loadLists()
    }
    this.#render()
    this.#scrollToEnd()
    this.#markRead()
    if (this.#host.config().layout === "launcher" && this.#view?.kind === "thread")
      this.#textarea.focus({ preventScroll: true })
  }

  async openApp(kind: AppKind, params: AppParams = {}): Promise<void> {
    await this.ready
    await this.#start()
    if (this.#phase === "ready") await this.#openApp(kind, params)
  }

  closed(): void {
    this.#stopTyping()
  }

  identityChanged(): void {
    this.#reset()
    void this.#start()
  }

  async signOut(): Promise<void> {
    const signingOut = this.#client.signOut()
    this.#reset()
    await signingOut
    if (this.#host.isOpen()) void this.#start()
    else this.#render()
  }

  destroy(): void {
    this.#client.reset()
    this.#stopTyping()
    this.#closeApp()
    if (this.#emailTimer) clearTimeout(this.#emailTimer)
    document.removeEventListener("visibilitychange", this.#onVisible)
    window.removeEventListener("online", this.#onVisible)
    for (const url of this.#blobs.values()) URL.revokeObjectURL(url)
  }

  #reset(): void {
    this.#client.reset()
    this.#closeApp()
    this.#starting = null
    this.#beginning = null
    this.#inbox = null
    this.#contact = null
    this.#phase = "loading"
    this.#conversations.clear()
    this.#threads.clear()
    this.#pending = []
    this.#ratings.clear()
    this.#rateKey = ""
    this.#view = null
    this.#lists = false
    this.#listsLoaded = false
    this.#emailSaved = false
    this.#emailEditing = false
    this.#emailKey = ""
    this.#connectedBefore = false
    this.#missed = 0
    this.#host.setUnread(0)
    this.#updates = 0
    this.#updatesKey = ""
    this.#inboxAt = 0
    this.#host.setUpdates(0)
  }

  #onVisible = () => {
    if (document.visibilityState !== "visible") return
    this.#client.reconnect()
    this.#markRead()
    if (this.#client.connected || this.#inboxIsStale()) void this.#refreshInbox()
  }

  #inboxIsStale(): boolean {
    return !this.#client.connected && Date.now() - this.#inboxAt >= inboxRefreshGap
  }

  async #setLocale(): Promise<void> {
    const { locale } = this.#host.config()
    this.#i18n ??= setupI18n({ locale: "en", messages: { en } })
    if (this.#loaded === locale) return
    const messages = await loadMessages(locale)
    if (locale !== this.#host.config().locale) return
    this.#i18n = setupI18n({ locale, messages: { [locale]: messages } })
    this.#loaded = locale
  }

  #t(descriptor: MessageDescriptor): string {
    return this.#i18n._(descriptor)
  }

  #start(): Promise<void> {
    this.#starting ??= (async () => {
      this.#phase = "loading"
      this.#render()
      const { channel } = this.#host.config()
      if (!channel) throw new ApiError(0, "not_configured")
      if ((await this.#host.identityToken()) || readStored(channel).session) await this.#begin()
      else {
        await this.#preview()
        if (!this.#inbox?.chat.allow_anonymous) throw new ApiError(403, "anonymous_not_allowed")
      }
      this.#phase = "ready"
      this.#view ??= { kind: "home" }
      this.#loadLists()
      this.#render()
      this.#scrollToEnd()
      this.#markRead()
    })().catch((error: unknown) => {
      this.#starting = null
      if (this.#refusal(error)) return
      this.#phase = "error"
      this.#error = error instanceof ApiError ? error.code : "error"
      this.#render()
    })
    return this.#starting
  }

  #loadLists(): void {
    if (this.#listsLoaded || !this.#host.config().channel) return
    this.#listsLoaded = true
    void this.#client
      .lists()
      .then((result) => {
        this.#lists = (result?.lists.length ?? 0) > 0
        this.#render()
      })
      .catch(() => undefined)
  }

  async #preview(): Promise<void> {
    const inbox = await this.#fetchInbox()
    if (!this.#client.token) this.#setInbox(inbox)
  }

  async #fetchInbox(): Promise<ClientInbox> {
    if (!this.#host.config().channel) throw new ApiError(0, "not_configured")
    const inbox = await this.#client.channelSettings()
    this.#inboxAt = Date.now()
    return inbox
  }

  #setInbox(inbox: ClientInbox): void {
    const { channel } = this.#host.config()
    this.#inbox = inbox
    this.#syncAttach()
    const launcher = launcherOf(inbox)
    this.#host.setLauncher(launcher)
    if (!channel) return
    const stored = readStored(channel)
    const open = new Set(inbox.status?.open.map((incident) => incident.id))
    const kept = Object.fromEntries(Object.entries(stored.dismissed_status ?? {}).filter(([id]) => open.has(id)))
    const next = { ...stored, launcher }
    if (Object.keys(kept).length > 0) next.dismissed_status = kept
    else delete next.dismissed_status
    writeStored(channel, next)
    this.#syncUpdates()
  }

  #syncUpdates(): void {
    const { channel, server, locale } = this.#host.config()
    const inbox = this.#inbox
    if (!channel || !inbox?.whats_new) return this.#setUpdates(0)
    const key = `${inbox.whats_new.latest_published_at}|${readStored(channel).whats_new_seen ?? ""}`
    if (key === this.#updatesKey) return
    this.#updatesKey = key
    countUpdates(server, channel, inbox, locale)
      .then((count) => {
        if (key === this.#updatesKey) this.#setUpdates(count)
      })
      .catch(() => {
        if (key === this.#updatesKey) this.#updatesKey = ""
      })
  }

  #setUpdates(count: number): void {
    if (count === this.#updates) return
    this.#updates = count
    this.#host.setUpdates(this.#inbox?.chat.announce_updates ? count : 0)
    if (this.#view?.kind === "home") this.#render()
  }

  #renderNotice(): void {
    const { channel } = this.#host.config()
    const dismissed = (channel && readStored(channel).dismissed_status) || {}
    const shown = [...(this.#inbox?.status?.open ?? [])]
      .filter((incident) => dismissed[incident.id] !== incident.updated_at)
      .sort((a, b) => impactRank[b.impact] - impactRank[a.impact])[0]
    this.#notice.hidden = !shown
    if (!shown) return this.#notice.replaceChildren()
    const states = {
      investigating: msg`Investigating`,
      identified: msg`Identified`,
      monitoring: msg`Monitoring`,
      resolved: msg`Resolved`,
      scheduled: msg`Scheduled`,
      in_progress: msg`In progress`,
      completed: msg`Completed`,
    }
    const link = el("a", "", shown.title)
    link.href = shown.url
    link.target = "_blank"
    link.rel = "noopener noreferrer"
    link.dir = "auto"
    const text = el("div", "notice-text")
    text.append(el("strong", "", this.#t(states[shown.state])), " ", link)
    this.#notice.className = `notice impact-${shown.impact}`
    this.#notice.replaceChildren(
      text,
      iconButton(
        icons.closeSmall,
        this.#t(msg`Dismiss`),
        () => {
          if (!channel) return
          const stored = readStored(channel)
          writeStored(channel, {
            ...stored,
            dismissed_status: { ...stored.dismissed_status, [shown.id]: shown.updated_at },
          })
          this.#render()
        },
        "icon sm"
      )
    )
  }

  async #refreshInbox(): Promise<void> {
    if (this.#phase !== "ready") return
    try {
      this.#setInbox(await this.#fetchInbox())
      this.#render()
    } catch (error) {
      this.#refusal(error, true)
    }
  }

  #refusal(error: unknown, channelRequest = !this.#client.token): boolean {
    const refused =
      error instanceof ApiError && (error.code === "origin_not_allowed" || (error.status === 404 && channelRequest))
    if (refused) this.#host.refused()
    return refused
  }

  #begin(): Promise<void> {
    this.#beginning ??= (async () => {
      await this.#client.start()
      await this.#loadConversations()
      await this.#client.connect()
    })().finally(() => {
      this.#beginning = null
    })
    return this.#beginning
  }

  async #refreshSession(): Promise<void> {
    try {
      await this.#client.refresh()
      this.#render()
    } catch {}
  }

  async #loadConversations(more = false): Promise<void> {
    const page = await this.#client.listConversations({
      limit: 50,
      cursor: more ? this.#conversationCursor : undefined,
    })
    if (!more) this.#conversations.clear()
    for (const conversation of page.items) this.#conversations.set(conversation.id, conversation)
    this.#conversationCursor = page.next_cursor
    this.#syncUnread()
  }

  async #reload(): Promise<void> {
    await this.#loadConversations()
    const current = this.#view?.kind === "thread" ? this.#view.id : null
    for (const id of this.#threads.keys()) if (id !== current) this.#threads.delete(id)
    if (current) {
      this.#threads.delete(current)
      await this.#loadMessages(current)
    }
    this.#render()
  }

  async #loadMessages(id: string, older = false): Promise<void> {
    let thread = this.#threads.get(id)
    if (!thread) {
      thread = { messages: [], complete: false, loading: false }
      this.#threads.set(id, thread)
    }
    if (thread.loading || (older && (thread.complete || !thread.cursor))) return
    thread.loading = true
    const before = this.#scroll.scrollHeight - this.#scroll.scrollTop
    try {
      const page = await this.#client.listMessages(id, {
        order: "desc",
        limit: 30,
        cursor: older ? thread.cursor : undefined,
      })
      const known = new Set(thread.messages.map((m) => m.id))
      thread.messages = [...thread.messages, ...page.items.filter((m) => !known.has(m.id))].sort(byCreated)
      thread.cursor = page.next_cursor
      thread.complete = !page.next_cursor
    } finally {
      thread.loading = false
    }
    if (older) {
      this.#render()
      this.#scroll.scrollTop = this.#scroll.scrollHeight - before
    }
  }

  #event(message: ClientRealtimeMessage): void {
    switch (message.type) {
      case "ready":
        this.#reconnecting = false
        this.#renderBanner()
        if (this.#connectedBefore) void this.#refreshInbox()
        this.#connectedBefore = true
        return
      case "inbox.updated": {
        const asked = this.#inbox?.ask_for_rating
        this.#setInbox(message.data)
        if (asked !== undefined && asked !== message.data.ask_for_rating)
          void this.#loadConversations()
            .then(() => this.#render())
            .catch(() => undefined)
        break
      }
      case "contact.updated":
        this.#contact = message.data
        break
      case "resync_required":
        void this.#reload()
        return
      case "presence":
        if (this.#inbox) this.#inbox = { ...this.#inbox, presence: message.data }
        break
      case "conversation.created":
        if (!this.#conversations.has(message.data.id)) this.#conversations.set(message.data.id, message.data)
        if (this.#view?.kind === "thread" && this.#view.id === null && this.#conversations.size > 1) {
          this.#view = this.#pending.some((p) => p.conversation === null) ? this.#view : { kind: "home" }
        }
        this.#syncUnread()
        break
      case "conversation.removed":
        this.#removeConversation(message.data.id)
        break
      case "conversation.updated": {
        const conversation = this.#conversations.get(message.data.id)
        const { status, can_rate, rating } = message.data
        if (conversation) this.#conversations.set(conversation.id, { ...conversation, status, can_rate, rating })
        if (!can_rate) this.#ratings.delete(message.data.id)
        break
      }
      case "message.created":
      case "message.updated":
        this.#upsertMessage(message.data, message.type === "message.created")
        break
      case "read": {
        const conversation = this.#conversations.get(message.data.conversation_id)
        const previous = conversation?.last_read_by_member_at
        if (conversation && (!previous || time(message.data.read_at) > time(previous))) {
          this.#conversations.set(conversation.id, { ...conversation, last_read_by_member_at: message.data.read_at })
        }
        break
      }
      case "typing": {
        const id = message.data.conversation_id
        const previous = this.#typing.get(id)
        if (previous) clearTimeout(previous.timer)
        if (message.data.typing) {
          const name = message.data.author.name ?? ""
          this.#typing.set(id, { name, timer: setTimeout(() => this.#typing.delete(id) && this.#render(), 8000) })
        } else this.#typing.delete(id)
        break
      }
    }
    this.#render()
  }

  #removeConversation(id: string): void {
    const open = this.#view?.kind === "thread" && this.#view.id === id
    this.#conversations.delete(id)
    this.#threads.delete(id)
    this.#ratings.delete(id)
    const typing = this.#typing.get(id)
    if (typing) clearTimeout(typing.timer)
    this.#typing.delete(id)
    this.#pending = this.#pending.filter((p) => p.conversation !== id || p.state === "sending")
    this.#syncUnread()
    if (!open) return
    this.#stopTyping()
    this.#view = { kind: "home" }
    this.#flash = this.#t(msg`This conversation is no longer available.`)
    this.#renderBanner()
  }

  #upsertMessage(message: ClientMessage, created: boolean): void {
    const id = message.conversation_id
    const thread = this.#threads.get(id)
    if (thread) {
      const index = thread.messages.findIndex((m) => m.id === message.id)
      if (index >= 0) thread.messages[index] = message
      else thread.messages = [...thread.messages, message].sort(byCreated)
    }
    if (message.client_id) this.#pending = this.#pending.filter((p) => p.client_id !== message.client_id)
    const conversation = this.#conversations.get(id)
    if (!conversation) {
      void this.#client
        .getConversation(id)
        .then((c) => {
          this.#conversations.set(c.id, c)
          this.#syncUnread()
          this.#render()
        })
        .catch(() => undefined)
      return
    }
    if (created) {
      const text = message.body.replace(/\s+/g, " ").trim()
      this.#conversations.set(id, {
        ...conversation,
        last_message: {
          id: message.id,
          author_type: message.author.type,
          text: text.length > 140 ? `${text.slice(0, 139)}…` : text,
          created_at: message.created_at,
        },
        last_message_at: message.created_at,
        unread: conversation.unread || message.author.type !== "contact",
      })
      if (message.author.type !== "contact") {
        const typing = this.#typing.get(id)
        if (typing) clearTimeout(typing.timer)
        this.#typing.delete(id)
        this.#announce(message)
        if (!this.#atEnd && this.#visibleThread() === id) this.#missed++
      }
    }
    this.#syncUnread()
    this.#markRead()
  }

  #announce(message: ClientMessage): void {
    this.#live.textContent = `${message.author.name || this.#inbox?.name || ""}: ${message.body.slice(0, 200)}`
  }

  #syncUnread(): void {
    let count = 0
    for (const conversation of this.#conversations.values()) if (conversation.unread) count++
    this.#host.setUnread(count)
  }

  #visibleThread(): string | null {
    if (!this.#host.isOpen() || document.visibilityState !== "visible") return null
    return this.#view?.kind === "thread" ? this.#view.id : null
  }

  #markRead(): void {
    const id = this.#visibleThread()
    const conversation = id ? this.#conversations.get(id) : undefined
    if (!id || !conversation?.unread) return
    this.#conversations.set(id, { ...conversation, unread: false })
    this.#syncUnread()
    void this.#client.markRead(id).catch(() => {
      const current = this.#conversations.get(id)
      if (current) this.#conversations.set(id, { ...current, unread: true })
      this.#syncUnread()
    })
  }

  #addFiles(files: File[]): void {
    const limits = this.#inbox?.attachments
    const { locale } = this.#host.config()
    let note = ""
    const accepted = limits
      ? files.filter((file) => {
          const problem = fileProblem(limits, file)
          const name = file.name || "image"
          const limit = limitSize(locale, limits.max_bytes)
          if (problem === "size") note ||= this.#t(msg`${name} is too large. The limit is ${limit}.`)
          else if (problem === "type") note ||= this.#t(msg`${name} is not an accepted file type.`)
          return !problem
        })
      : files
    this.#fileNote.textContent = note
    this.#fileNote.hidden = !note
    this.#files = [...this.#files, ...accepted].slice(0, maxFiles)
    this.#renderChips()
    this.#syncSend()
    this.#textarea.focus()
  }

  async #capture(): Promise<void> {
    const file = await captureScreen(this.#host.panel)
    if (file) this.#addFiles([file])
  }

  #autosize(): void {
    this.#textarea.style.blockSize = ""
    if (this.#textarea.value === "") return
    this.#textarea.style.blockSize = "auto"
    this.#textarea.style.blockSize = `${Math.max(44, Math.min(this.#textarea.scrollHeight, 140))}px`
  }

  #syncSend(): void {
    this.#sendButton.disabled = this.#textarea.value.trim() === "" && this.#files.length === 0
  }

  #typingInput(): void {
    const id = this.#view?.kind === "thread" ? this.#view.id : null
    if (!id || this.#textarea.value.trim() === "") {
      this.#stopTyping()
      return
    }
    if (Date.now() - this.#typingSentAt > 3000) {
      this.#typingSentAt = Date.now()
      void this.#client.setTyping(id, true).catch(() => undefined)
    }
    if (this.#typingIdle) clearTimeout(this.#typingIdle)
    this.#typingIdle = setTimeout(() => this.#stopTyping(), 4000)
  }

  #stopTyping(): void {
    if (this.#typingIdle) clearTimeout(this.#typingIdle)
    this.#typingIdle = null
    if (this.#typingSentAt === 0) return
    this.#typingSentAt = 0
    const id = this.#view?.kind === "thread" ? this.#view.id : null
    if (id) void this.#client.setTyping(id, false).catch(() => undefined)
  }

  async #send(): Promise<void> {
    if (this.#phase !== "ready" || this.#view?.kind !== "thread") return
    const body = this.#textarea.value.trim()
    const files = this.#files
    if (!body && files.length === 0) return
    if (this.#view.id === null && this.#pending.some((p) => p.conversation === null)) return
    this.#stopTyping()
    this.#fileNote.hidden = true
    this.#textarea.value = ""
    this.#files = []
    this.#autosize()
    this.#renderChips()
    this.#syncSend()
    const pending: Pending = { client_id: randomId(), conversation: this.#view.id, body, files, state: "sending" }
    if (this.#view.id === null && this.#helpArticle) {
      pending.help_article_id = this.#helpArticle
      this.#helpArticle = null
    }
    this.#pending.push(pending)
    this.#render()
    this.#scrollToEnd()
    await this.#deliver(pending)
  }

  async #deliver(pending: Pending): Promise<void> {
    pending.state = "sending"
    this.#render()
    const input = { body: pending.body, files: pending.files, client_id: pending.client_id }
    try {
      if (pending.conversation === null) {
        const created = await this.#client.startConversation({ ...input, help_article_id: pending.help_article_id })
        const id = created.conversation.id
        if (!this.#conversations.has(id)) this.#conversations.set(id, created.conversation)
        if (!this.#threads.has(id)) this.#threads.set(id, { messages: [], complete: true, loading: false })
        if (this.#view?.kind === "thread" && this.#view.id === null) this.#view = { kind: "thread", id }
        this.#upsertMessage(created.message, false)
      } else {
        const message = await this.#client.sendMessage(pending.conversation, input)
        this.#upsertMessage(message, true)
      }
      this.#pending = this.#pending.filter((p) => p !== pending)
    } catch (error) {
      const status = error instanceof ApiError ? error.status : 0
      pending.state = status === 413 || status === 415 ? "refused" : "failed"
      pending.status = status
    }
    this.#render()
    this.#scrollToEnd()
  }

  async rate(id: string, rating: Rating, comment?: string): Promise<ClientConversation> {
    try {
      const conversation = await this.#client.rate(id, rating, comment)
      this.#conversations.set(conversation.id, conversation)
      this.#ratings.delete(id)
      this.#render()
      return conversation
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        const current = await this.#client.getConversation(id).catch(() => null)
        if (current) this.#conversations.set(current.id, current)
        this.#ratings.delete(id)
        this.#render()
      }
      throw error
    }
  }

  #choose(id: string, choice: Rating): void {
    const draft = this.#ratings.get(id)
    if (draft?.state === "sending") return
    this.#ratings.set(id, { choice, state: "choosing" })
    this.#render()
    this.#rateComment.focus({ preventScroll: true })
    if (this.#atEnd) this.#scrollToEnd()
  }

  async #submitRating(id: string, withComment: boolean): Promise<void> {
    const draft = this.#ratings.get(id)
    if (!draft || draft.state === "sending") return
    draft.state = "sending"
    this.#render()
    try {
      await this.rate(id, draft.choice, withComment ? this.#rateComment.value : undefined)
      this.#rateComment.value = ""
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) return
      draft.state = "failed"
      this.#render()
    }
  }

  async #saveEmail(address?: string): Promise<void> {
    const email = (address ?? this.#emailInput.value).trim()
    if (!email || (address === undefined && !this.#emailInput.checkValidity())) {
      this.#emailError = this.#t(msg`Enter a valid e-mail address.`)
      this.#renderEmail()
      return
    }
    if (this.#emailBusy) return
    this.#emailBusy = true
    try {
      this.#contact = await this.#client.setEmail(email)
      this.#emailSaved = true
      this.#emailEditing = false
      this.#emailError = ""
      this.#emailNote = address === undefined ? "" : this.#t(msg`We sent it again.`)
    } catch (error) {
      this.#emailError =
        error instanceof ApiError && error.status === 429
          ? this.#t(msg`Too many attempts. Wait a moment and try again.`)
          : this.#t(msg`Could not save your e-mail address. Try again.`)
    } finally {
      this.#emailBusy = false
    }
    this.#renderEmail()
    this.#render()
  }

  #attachmentUrl(attachment: ClientAttachment): Promise<string> {
    const cached = this.#blobs.get(attachment.id)
    if (cached) return Promise.resolve(cached)
    let load = this.#blobLoads.get(attachment.id)
    if (!load) {
      load = this.#client.attachment(attachment.id).then((blob) => {
        const url = URL.createObjectURL(blob)
        this.#blobs.set(attachment.id, url)
        return url
      })
      load.catch(() => this.#blobLoads.delete(attachment.id))
      this.#blobLoads.set(attachment.id, load)
    }
    return load
  }

  async #download(attachment: ClientAttachment): Promise<void> {
    const url = await this.#attachmentUrl(attachment)
    const a = el("a")
    a.href = url
    a.download = attachment.filename
    a.rel = "noopener"
    a.click()
  }

  #sorted(): ClientConversation[] {
    return [...this.#conversations.values()].sort(byActivity)
  }

  #nobodyAvailable(): boolean {
    const inbox = this.#inbox
    if (!inbox) return false
    return !inbox.open_now || inbox.mode === "async" || !inbox.presence?.available
  }

  #scrollToEnd(): void {
    this.#atEnd = true
    this.#missed = 0
    this.#scroll.scrollTop = this.#scroll.scrollHeight
    requestAnimationFrame(() => {
      if (this.#atEnd) this.#scroll.scrollTop = this.#scroll.scrollHeight
    })
  }

  #syncJump(): void {
    const show = !this.#atEnd && this.#view?.kind === "thread"
    this.#jump.hidden = !show
    if (!show) return
    this.#jump.setAttribute("aria-label", this.#t(msg`Jump to latest`))
    const count = this.#missed
    this.#jump.replaceChildren()
    this.#jump.insertAdjacentHTML("beforeend", icons.down)
    this.#jump.append(count > 0 ? this.#t(msg`${count} new`) : this.#t(msg`Jump to latest`))
  }

  #syncAttach(): void {
    const limits = this.#inbox?.attachments
    const attach = !limits || canAttach(limits)
    this.#attachButton.hidden = !attach
    this.#shotButton.hidden = !canCapture() || !attach || (!!limits && !allowsType(limits, "image/png"))
    if (limits) this.#fileInput.accept = acceptAttribute(limits)
    else this.#fileInput.removeAttribute("accept")
  }

  #renderAll(): void {
    this.#textarea.placeholder = this.#t(msg`Write a message…`)
    this.#textarea.setAttribute("aria-label", this.#t(msg`Message`))
    this.#syncAttach()
    this.#attachButton.setAttribute("aria-label", this.#t(msg`Attach files`))
    this.#attachButton.title = this.#t(msg`Attach files`)
    this.#shotButton.setAttribute("aria-label", this.#t(msg`Take a screenshot`))
    this.#shotButton.title = this.#t(msg`Take a screenshot`)
    this.#sendButton.setAttribute("aria-label", this.#t(msg`Send`))
    this.#sendButton.title = this.#t(msg`Send`)
    this.#hint.replaceChildren(el("kbd", "", this.#t(msg`Enter`)), this.#t(msg`to send`))
    this.#emailInput.placeholder = this.#t(msg`you@example.com`)
    this.#emailInput.setAttribute("aria-label", this.#t(msg`E-mail address`))
    this.#scroll.setAttribute("aria-label", this.#t(msg`Conversation`))
    this.#syncSend()
    this.#renderChips()
    this.#render()
  }

  #go(view: View): void {
    this.#stopTyping()
    this.#clearFlash()
    if (view.kind !== "thread" || view.id !== null) this.#helpArticle = null
    if (view.kind !== "app") this.#closeApp()
    this.#view = view
    this.#render()
    if (view.kind === "thread") {
      this.#scrollToEnd()
      this.#textarea.focus({ preventScroll: true })
    }
  }

  #closeApp(): void {
    this.#appLoad++
    this.#appError = false
    this.#app?.handle.destroy()
    this.#app = null
  }

  async #openApp(kind: AppKind, params: AppParams = {}): Promise<void> {
    this.#closeApp()
    const load = this.#appLoad
    this.#view = { kind: "app", app: kind }
    this.#render()
    try {
      const apps = await loadApps()
      const inbox = this.#inbox
      if (load !== this.#appLoad || !inbox) return
      const { locale } = this.#host.config()
      const handle = await apps.mountApp(kind, {
        root: this.#host.panel.getRootNode(),
        client: this.#client,
        inbox,
        locale,
        mailingLists: this.#lists,
        params,
        back: () => this.#go({ kind: "home" }),
        open: (next, nextParams) => void this.#openApp(next, nextParams),
        ask: (articleId) => {
          this.#go({ kind: "thread", id: null })
          this.#helpArticle = articleId ?? null
        },
        created: (result) => {
          const id = result.conversation.id
          if (!this.#conversations.has(id)) this.#conversations.set(id, result.conversation)
          if (!this.#threads.has(id)) this.#threads.set(id, { messages: [], complete: true, loading: false })
          this.#upsertMessage(result.message, false)
        },
        viewConversation: (id) => void this.#openThread(id),
        seenUpdates: () => this.#setUpdates(0),
      })
      if (load !== this.#appLoad) return handle.destroy()
      this.#app = { kind, handle }
    } catch {
      if (load === this.#appLoad) this.#appError = true
    }
    if (load === this.#appLoad) this.#render()
  }

  #render(): void {
    const panel = this.#host.panel
    if (panel.childElementCount === 0) {
      const fallback = addStyles(panel.getRootNode(), styles)
      panel.append(...(fallback ? [fallback] : []), this.#header, this.#banner, this.#body, this.#live)
      this.#header.id = "yuva-title"
      panel.setAttribute("aria-labelledby", "yuva-title")
    }
    this.#renderHeader()
    this.#renderBanner()
    if (this.#phase !== "ready") {
      this.#scrollThread = undefined
      const center = el("div", "center")
      if (this.#phase === "loading") center.append(el("p", "hint", this.#t(msg`Loading…`)))
      else {
        center.append(el("p", "empty", this.#errorText()))
        const retry = textButton(this.#t(msg`Try again`), "primary", () => {
          this.#starting = null
          void this.#start()
        })
        center.append(retry)
      }
      this.#body.replaceChildren(center)
      return
    }
    this.#renderNotice()
    const view = this.#view ?? { kind: "home" }
    if (view.kind === "home") this.#renderHome()
    else if (view.kind === "list") this.#renderList()
    else if (view.kind === "app") this.#renderApp()
    else this.#renderThread()
    this.#syncJump()
  }

  #errorText(): string {
    switch (this.#error) {
      case "anonymous_not_allowed":
        return this.#t(msg`Sign in to chat with us.`)
      case "invalid_identity_token":
        return this.#t(msg`Your sign-in has expired. Reload the page and try again.`)
      case "rate_limited":
        return this.#t(msg`Too many attempts. Wait a moment and try again.`)
      case "network":
        return this.#t(msg`You seem to be offline.`)
      default:
        return this.#t(msg`Chat is not available right now.`)
    }
  }

  #presenceLine(inbox: ClientInbox): Node[] {
    const live = inbox.mode === "live" && inbox.open_now && inbox.presence?.available
    if (live) return [el("span", "", this.#t(msg`Online`))]
    if (!inbox.open_now || inbox.mode === "live") return [el("span", "", this.#t(msg`We're away right now`))]
    return [el("span", "", this.#replyTime(inbox.expected_reply_minutes))]
  }

  #renderHeader(): void {
    const { layout } = this.#host.config()
    const inbox = this.#inbox
    const view = this.#phase === "ready" ? (this.#view ?? { kind: "home" }) : ({ kind: "home" } as View)
    const children: Node[] = []
    const close = layout === "launcher" ? iconButton(icons.close, this.#t(msg`Close`), () => this.#host.close()) : null
    if (view.kind === "home") {
      this.#header.className = "header home"
      const brand = el("div", "headbrand")
      const mark = el("span", "mark")
      const logo = inbox?.branding.logo_url
      if (logo && /^https?:/i.test(logo)) {
        const img = el("img")
        img.alt = ""
        img.referrerPolicy = "no-referrer"
        img.addEventListener("error", () => mark.replaceChildren((inbox?.name || "Y").slice(0, 1)))
        img.src = logo
        mark.append(img)
      } else mark.textContent = (inbox?.name || "Y").slice(0, 1)
      brand.append(mark, el("h2", "title", inbox?.name || this.#t(msg`Messages`)))
      children.push(brand)
    } else {
      this.#header.className = "header"
      children.push(iconButton(icons.back, this.#t(msg`Back`), () => this.#go({ kind: "home" })))
      if (view.kind === "thread" && inbox) {
        const who = el("div", "who")
        const live = inbox.mode === "live" && inbox.open_now && inbox.presence?.available
        const first = live ? inbox.presence?.members[0] : undefined
        who.append(
          el("span", "avatar lg", first?.initials || (inbox.name || "").slice(0, 2)),
          el("span", live ? "dot" : "dot away")
        )
        who.querySelector(".dot")?.setAttribute("aria-hidden", "true")
        const heading = el("div", "heading")
        heading.append(el("h2", "title", inbox.name || this.#t(msg`Messages`)))
        const status = el("div", "status")
        status.append(...this.#presenceLine(inbox))
        heading.append(status)
        children.push(who, heading)
      } else {
        const heading = el("div", "heading")
        heading.append(
          el("h2", "title", view.kind === "app" ? this.#t(appTitle(view.app)) : this.#t(msg`All conversations`))
        )
        children.push(heading)
      }
    }
    if (close) children.push(close)
    this.#header.replaceChildren(...children)
  }

  #replyTime(minutes: number | undefined): string {
    if (!minutes) return this.#t(msg`We'll reply as soon as we can`)
    const { locale } = this.#host.config()
    const [value, unit] =
      minutes < 60
        ? [minutes, "minute"]
        : minutes < 1440
          ? [Math.round(minutes / 60), "hour"]
          : [Math.round(minutes / 1440), "day"]
    const duration = new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "long" }).format(value)
    return this.#t(msg`Usually replies in ${duration}`)
  }

  #renderBanner(): void {
    const text = this.#reconnecting ? this.#t(msg`Reconnecting…`) : this.#flash
    this.#banner.hidden = !text
    this.#banner.textContent = text
  }

  #renderHome(): void {
    this.#scrollThread = undefined
    const inbox = this.#inbox
    if (!inbox) return
    const home = buildHome({
      inbox,
      conversations: this.#sorted(),
      mailingLists: this.#lists,
      updates: this.#updates,
      locale: this.#host.config().locale,
      t: (descriptor) => this.#t(descriptor),
      notice: this.#notice,
      openConversation: (id) => void this.#openThread(id),
      allConversations: () => this.#go({ kind: "list" }),
      newConversation: () => this.#go({ kind: "thread", id: null }),
      openApp: (kind, params) => void this.#openApp(kind, params),
    })
    this.#body.replaceChildren(home)
  }

  #renderApp(): void {
    this.#scrollThread = undefined
    const app = this.#app
    if (app) {
      if (this.#body.firstChild !== app.handle.element || this.#body.childElementCount !== 1)
        this.#body.replaceChildren(app.handle.element)
      return
    }
    const center = el("div", "center")
    if (this.#appError) {
      center.append(
        el("p", "empty", this.#t(msg`Could not load this. Try again.`)),
        textButton(this.#t(msg`Try again`), "primary", () => {
          const view = this.#view
          if (view?.kind === "app") void this.#openApp(view.app)
        })
      )
    } else center.append(el("p", "hint", this.#t(msg`Loading…`)))
    this.#body.replaceChildren(center)
  }

  #renderList(): void {
    this.#scrollThread = undefined
    const { locale } = this.#host.config()
    const list = el("ul", "list")
    const today = new Date().toDateString()
    for (const conversation of this.#sorted()) {
      const button = el("button", conversation.unread ? "item unread" : "item")
      button.type = "button"
      const main = el("div", "item-main")
      const top = el("div", "item-top")
      top.append(el("span", "item-subject", conversation.subject || this.#inbox?.name || this.#t(msg`Conversation`)))
      if (conversation.kind === "feedback" && conversation.status === "closed")
        top.append(el("span", "item-state", this.#t(msg`Resolved`)))
      const at = new Date(conversation.last_message_at ?? conversation.created_at)
      top.append(
        el(
          "span",
          "item-time",
          new Intl.DateTimeFormat(
            locale,
            at.toDateString() === today ? { timeStyle: "short" } : { month: "short", day: "numeric" }
          ).format(at)
        )
      )
      const preview = conversation.last_message
        ? `${conversation.last_message.author_type === "contact" ? this.#t(msg`You:`) + " " : ""}${conversation.last_message.text || this.#t(msg`Attachment`)}`
        : ""
      const previewLine = el("div", "item-preview", preview.replace(/\*\*|`/g, ""))
      previewLine.dir = "auto"
      main.append(top, previewLine)
      button.append(main)
      if (conversation.unread) button.append(el("span", "unread-dot"))
      button.addEventListener("click", () => void this.#openThread(conversation.id))
      const item = el("li")
      item.append(button)
      list.append(item)
    }
    const page = el("div", "page")
    page.append(this.#notice, list)
    if (this.#conversationCursor) {
      const more = textButton(
        this.#t(msg`Show older conversations`),
        "link soft",
        () => void this.#loadConversations(true).then(() => this.#render())
      )
      more.style.margin = "8px auto"
      page.append(more)
    }
    const start = textButton(this.#t(msg`New conversation`), "primary", () => this.#go({ kind: "thread", id: null }))
    start.style.margin = "16px 32px"
    page.append(el("div", "fill"), start)
    this.#body.replaceChildren(page)
  }

  #clearFlash(): void {
    if (!this.#flash) return
    this.#flash = ""
    this.#renderBanner()
  }

  async #openThread(id: string): Promise<void> {
    this.#clearFlash()
    this.#closeApp()
    this.#view = { kind: "thread", id }
    this.#render()
    if (!this.#threads.get(id)?.complete && !this.#threads.get(id)?.messages.length)
      await this.#loadMessages(id).catch(() => undefined)
    this.#render()
    this.#scrollToEnd()
    this.#markRead()
  }

  #avatar(author: { name?: string; initials?: string; avatar_url?: string }): HTMLSpanElement {
    const avatar = el("span", "avatar", author.initials || initialsOf(author.name ?? ""))
    const url = author.avatar_url
    const state = url ? this.#avatars.get(url) : false
    if (!url || state === false) return avatar
    const img = el("img")
    img.alt = ""
    img.referrerPolicy = "no-referrer"
    if (state) {
      img.src = url
      avatar.replaceChildren(img)
      return avatar
    }
    img.addEventListener("load", () => {
      this.#avatars.set(url, true)
      if (avatar.isConnected) avatar.replaceChildren(img)
    })
    img.addEventListener("error", () => this.#avatars.set(url, false))
    img.src = url
    return avatar
  }

  #renderThread(): void {
    const view = this.#view
    if (view?.kind !== "thread") return
    const id = view.id
    const { locale } = this.#host.config()
    const sameThread = this.#scrollThread === id
    const atEnd = this.#atEnd
    const thread = id ? this.#threads.get(id) : undefined
    const messages = thread?.messages ?? []
    const pending = this.#pending.filter((p) => p.conversation === id)
    const conversation = id ? this.#conversations.get(id) : undefined
    const nodes: Node[] = []
    const greeting = this.#inbox?.chat.greeting
    if (!thread || thread.complete) {
      if (greeting) {
        const bubble = el("div", "bubble greeting")
        bubble.append(richText(greeting))
        nodes.push(bubble)
      } else if (messages.length === 0 && pending.length === 0) {
        const center = el("div", "center")
        center.append(
          el("p", "empty", this.#t(msg`No conversations yet`)),
          el("p", "hint", this.#t(msg`Send us a message and we will reply here.`))
        )
        nodes.push(center)
      }
    } else if (thread.loading) nodes.push(el("p", "hint", this.#t(msg`Loading…`)))

    const entries: Entry[] = [
      ...messages.map((message): Entry => ({ kind: "message", message })),
      ...pending.map((item): Entry => ({ kind: "pending", pending: item })),
    ]
    const known = new Set<string>()
    let lastDay = ""
    let group: { mine: boolean; key: string; at: string; node: HTMLElement; stack: HTMLElement } | null = null
    const flush = (last: boolean) => {
      if (!group) return
      const { node, stack, mine, at } = group
      const meta = this.#groupMeta(entries, last, mine, at, id)
      if (meta) {
        if (!mine) meta.classList.add("theirs-meta")
        stack.append(meta)
      }
      nodes.push(node)
      group = null
    }

    entries.forEach((entry, i) => {
      const message = entry.kind === "message" ? entry.message : null
      const created = message?.created_at ?? ""
      const mine = !message || message.author.type === "contact"
      const key = message ? authorKey(message) : "contact"
      if (message && created && !sameDay(created, lastDay || "1970-01-01")) {
        flush(false)
        nodes.push(el("div", "day", dayLabel(locale, created)))
        lastDay = created
      }
      if (message && !mine && message.author.type === "member" && message.author.name) {
        const name = message.author.name
        if (!known.has(name)) {
          known.add(name)
          if (thread?.complete && i > 0) {
            flush(false)
            const line = el("div", "sys")
            line.insertAdjacentHTML("beforeend", icons.person)
            line.append(this.#t(msg`${name} joined the conversation`))
            nodes.push(line)
          }
        }
      }
      const startsGroup = !group || group.key !== key || (message && time(created) - time(group.at) > groupGap) || false
      if (startsGroup) {
        flush(false)
        const node = el("div", mine ? "group mine" : "row theirs")
        const stack = mine ? node : el("div", "stack")
        if (!mine && message) {
          const avatar = this.#avatar(message.author)
          avatar.style.marginBlockEnd = "22px"
          node.append(avatar, stack)
          if (message.author.name) stack.append(el("div", "author", message.author.name))
        }
        group = { mine, key, at: created || new Date().toISOString(), node, stack }
      }
      if (!group) return
      if (message) {
        group.at = created
        if (message.body.trim()) {
          const bubble = el("div", "bubble")
          bubble.dir = "auto"
          bubble.title = new Date(created).toLocaleString(locale)
          bubble.append(richText(message.body))
          group.stack.append(bubble)
        }
        if (message.attachments.length > 0) group.stack.append(this.#renderAttachments(message.attachments, mine))
        for (const card of message.cards ?? []) group.stack.append(this.#renderCard(card))
      } else if (entry.kind === "pending") {
        const item = entry.pending
        if (item.body) {
          const bubble = el("div", "bubble")
          bubble.dir = "auto"
          bubble.append(richText(item.body))
          group.stack.append(bubble)
        }
        if (item.files.length > 0) {
          const files = el("div", "files")
          for (const file of item.files) files.append(this.#pendingFile(file, item))
          group.stack.append(files)
        }
      }
    })
    flush(true)

    if (conversation?.kind === "feedback" && conversation.status === "closed")
      nodes.push(el("p", "hint resolved", this.#t(msg`Resolved`)))
    else if (conversation?.status === "closed" && messages.length > 0) {
      const line = el("div", "sys")
      line.insertAdjacentHTML("beforeend", icons.check)
      line.append(this.#t(msg`Conversation closed`))
      nodes.push(line)
    }
    const rate = conversation ? this.#renderRating(conversation) : null
    if (rate) nodes.push(rate)

    this.#renderEmail()
    if (!this.#email.hidden) nodes.push(this.#email)

    const typing = id ? this.#typing.get(id) : undefined
    if (typing) {
      const line = el("div", "typing")
      line.setAttribute("role", "status")
      const name = typing.name
      const dots = el("div", "dots")
      dots.append(el("i"), el("i"), el("i"))
      line.append(
        this.#avatar({ name }),
        dots,
        el("span", "sr", name ? this.#t(msg`${name} is typing…`) : this.#t(msg`Typing…`))
      )
      nodes.push(line)
    }

    const root = this.#scroll.getRootNode() as Document | ShadowRoot
    const focused = root.activeElement === this.#rateComment || root.activeElement === this.#emailInput
    const active = root.activeElement
    const { selectionStart, selectionEnd } = this.#rateComment
    this.#scroll.replaceChildren(...nodes)
    if (focused && active instanceof HTMLElement && active.isConnected) {
      active.focus({ preventScroll: true })
      if (active === this.#rateComment) this.#rateComment.setSelectionRange(selectionStart, selectionEnd)
    }
    if (this.#body.firstChild !== this.#scroll) this.#body.replaceChildren(this.#scroll, this.#notice, this.#composer)
    this.#scrollThread = id
    if (!sameThread || atEnd) this.#scrollToEnd()
  }

  #groupMeta(entries: Entry[], last: boolean, mine: boolean, at: string, id: string | null): HTMLElement | null {
    const tail = entries[entries.length - 1]
    if (!mine) return el("div", "meta", this.#formatTime(at))
    if (last && tail?.kind === "pending") {
      const item = tail.pending
      if (item.state === "failed") {
        const line = el("div", "meta failed", `${this.#t(msg`Not sent.`)} `)
        line.append(textButton(this.#t(msg`Retry`), "link", () => void this.#deliver(item)))
        return line
      }
      if (item.state === "refused") {
        const reason =
          item.status === 413 ? this.#t(msg`The file is too large.`) : this.#t(msg`This file type is not accepted.`)
        return el("div", "meta failed", `${this.#t(msg`Not sent.`)} ${reason}`)
      }
      return el("div", "meta", this.#t(msg`Sending…`))
    }
    const meta = el("div", "meta", this.#formatTime(at))
    if (last && tail?.kind === "message") {
      const readAt = id ? this.#conversations.get(id)?.last_read_by_member_at : undefined
      const seen = !!readAt && time(readAt) >= time(tail.message.created_at)
      meta.textContent = `${seen ? this.#t(msg`Seen`) : this.#t(msg`Sent`)} ${this.#formatTime(at)}`
      if (seen) {
        meta.classList.add("seen")
        meta.insertAdjacentHTML("beforeend", icons.seen)
      }
    }
    return meta
  }

  #pendingFile(file: File, item: Pending): HTMLElement {
    const failed = item.state !== "sending"
    const card = el("div", failed ? "att bad" : "att busy")
    const thumb = el("span", "thumb")
    thumb.innerHTML = icons.file
    const info = el("span", "info")
    const { locale } = this.#host.config()
    info.append(
      el("span", "name", file.name || "image"),
      el(
        "span",
        "sub",
        failed ? this.#t(msg`Not sent.`) : this.#t(msg`Uploading…`) + " · " + fileSize(locale, file.size)
      )
    )
    card.append(thumb, info)
    return card
  }

  #renderRating(conversation: ClientConversation): HTMLElement | null {
    const rated = conversation.status === "closed" ? conversation.rating : undefined
    const draft = conversation.can_rate && this.#inbox?.ask_for_rating ? this.#ratings.get(conversation.id) : undefined
    if (!rated && !(conversation.can_rate && this.#inbox?.ask_for_rating)) return null
    const card = this.#rateCard
    const key = [conversation.id, rated, draft?.choice, draft?.state, this.#host.config().locale].join(":")
    if (key === this.#rateKey) return card
    this.#rateKey = key
    if (this.#rateFor !== conversation.id) {
      this.#rateFor = conversation.id
      this.#rateComment.value = ""
    }
    const question = this.#t(msg`How did we do?`)
    card.setAttribute("role", "group")
    card.setAttribute("aria-label", question)
    const choices = el("div", "choices")
    for (const value of ["good", "bad"] as const) {
      if (rated && rated !== value) continue
      const button = el("button", "choice", value === "good" ? "👍" : "👎")
      button.type = "button"
      const label = value === "good" ? this.#t(msg`Good`) : this.#t(msg`Bad`)
      button.setAttribute("aria-label", label)
      button.title = label
      button.setAttribute("aria-pressed", String((rated ?? draft?.choice) === value))
      button.disabled = !!rated || draft?.state === "sending"
      button.addEventListener("click", () => this.#choose(conversation.id, value))
      choices.append(button)
    }
    if (rated) {
      card.replaceChildren(choices, el("p", "", this.#t(msg`Thanks for your feedback`)))
      return card
    }
    card.replaceChildren(el("p", "", question), choices)
    if (!draft) return card
    this.#rateComment.placeholder = this.#t(msg`Anything you'd like to add? (optional)`)
    this.#rateComment.setAttribute("aria-label", this.#t(msg`Comment`))
    this.#rateComment.disabled = draft.state === "sending"
    const form = el("form")
    form.addEventListener("submit", (event) => {
      event.preventDefault()
      void this.#submitRating(conversation.id, true)
    })
    const skip = el("button", "link", this.#t(msg`Skip`))
    skip.type = "button"
    skip.disabled = draft.state === "sending"
    skip.addEventListener("click", () => void this.#submitRating(conversation.id, false))
    const send = el("button", "", draft.state === "sending" ? this.#t(msg`Sending…`) : this.#t(msg`Send`))
    send.type = "submit"
    send.disabled = draft.state === "sending"
    const actions = el("div", "actions")
    actions.append(skip, send)
    form.append(this.#rateComment, actions)
    card.append(form)
    if (draft.state === "failed") card.append(el("p", "error", this.#t(msg`Could not send your rating. Try again.`)))
    return card
  }

  #renderCard(card: MessageCard): HTMLElement {
    const link = el("a", "article-card")
    link.href = card.url
    link.target = "_blank"
    link.rel = "noopener noreferrer"
    link.dir = "auto"
    const text = el("span", "article-card-text")
    text.append(el("b", "", card.title))
    if (card.excerpt) text.append(el("span", "", card.excerpt))
    link.insertAdjacentHTML("beforeend", icons.book)
    link.append(text)
    link.insertAdjacentHTML("beforeend", icons.chevron)
    link.addEventListener("click", (event) => {
      if (!this.#inbox?.help || event.metaKey || event.ctrlKey || event.shiftKey) return
      event.preventDefault()
      void this.#openApp("help", { article: card.article_id })
    })
    return link
  }

  #renderAttachments(attachments: ClientAttachment[], mine: boolean): HTMLElement {
    const files = el("div", "files")
    const { locale } = this.#host.config()
    for (const attachment of attachments) {
      if (isImage(attachment)) {
        const img = el("img", "preview")
        img.alt = attachment.filename
        const cached = this.#blobs.get(attachment.id)
        if (cached) img.src = cached
        else
          void this.#attachmentUrl(attachment)
            .then((url) => {
              img.src = url
            })
            .catch(() => undefined)
        img.addEventListener("load", () => {
          if (this.#atEnd) this.#scroll.scrollTop = this.#scroll.scrollHeight
        })
        img.addEventListener("click", () => void this.#download(attachment))
        files.append(img)
      } else {
        const card = el("button", "att")
        card.type = "button"
        const thumb = el("span", "thumb")
        thumb.innerHTML = icons.file
        const info = el("span", "info")
        info.append(
          el("span", "name", attachment.filename),
          el(
            "span",
            "sub",
            [fileKind(attachment.filename, attachment.content_type), fileSize(locale, attachment.size)]
              .filter(Boolean)
              .join(" · ")
          )
        )
        card.append(thumb, info)
        card.addEventListener("click", () => void this.#download(attachment))
        files.append(card)
      }
    }
    if (!mine) files.style.alignItems = "flex-start"
    return files
  }

  #formatTime(value: string): string {
    return new Intl.DateTimeFormat(this.#host.config().locale, { timeStyle: "short" }).format(new Date(value))
  }

  #emailMode(): EmailMode | null {
    const contact = this.#contact
    const view = this.#view
    if (view?.kind !== "thread") return null
    const sent = view.id !== null || this.#pending.length > 0
    if (!sent) return null
    const confirmation = contact?.email_confirmation
    const live =
      confirmation?.state === "sent" && (!confirmation.expires_at || Date.parse(confirmation.expires_at) > Date.now())
    if (confirmation?.state === "undeliverable" && !this.#emailEditing) return "bad"
    if (live && !this.#emailEditing) return "sent"
    if (confirmation?.state === "confirmed" && !this.#emailEditing) return "confirmed"
    const verified = !!contact?.email
    if (this.#emailEditing || (confirmation?.state === "sent" && !verified)) return "ask"
    if (this.#emailSaved && (verified || contact?.typed_email)) return "saved"
    if (this.#inbox?.chat.ask_email_offline && this.#nobodyAvailable() && !verified) return "ask"
    return null
  }

  #renderEmail(): void {
    const mode = this.#emailMode()
    this.#email.hidden = !mode
    if (this.#emailTimer) clearTimeout(this.#emailTimer)
    this.#emailTimer = null
    if (!mode) {
      this.#emailKey = ""
      return
    }
    const confirmation = this.#contact?.email_confirmation
    const address = confirmation?.email ?? this.#contact?.typed_email ?? this.#contact?.email ?? ""
    if (mode === "sent" && confirmation?.expires_at) {
      const wait = Math.min(Date.parse(confirmation.expires_at) - Date.now() + 100, maxTimer)
      this.#emailTimer = setTimeout(() => this.#render(), Math.max(wait, 0))
    }
    const key = [mode, address, this.#emailError, this.#emailNote, this.#emailBusy, this.#host.config().locale].join(
      "|"
    )
    if (key === this.#emailKey) return
    if (this.#emailKey.split("|")[0] !== mode) this.#emailInput.value = mode === "bad" || mode === "ask" ? address : ""
    this.#emailKey = key

    const note = () => (this.#emailNote ? [el("span", "soft", this.#emailNote)] : [])
    const failure = () => (this.#emailError ? [el("p", "error", this.#emailError)] : [])
    const form = () => {
      const f = el("form", "inline-form")
      f.noValidate = true
      f.addEventListener("submit", (event) => {
        event.preventDefault()
        void this.#saveEmail()
      })
      const save = el("button", "btn", this.#t(msg`Save`))
      save.type = "submit"
      save.disabled = this.#emailBusy
      f.append(this.#emailInput, save)
      return f
    }
    const strong = (text: string) => el("b", "", text)

    if (mode === "sent") {
      this.#email.className = "mail"
      this.#email.innerHTML = icons.mail
      const text = el("div", "text")
      const soft = el("span", "soft")
      const marker = "\u0001"
      const sentTo = ((address) => this.#t(msg`We sent a link to ${address}. Open it so replies reach you by e-mail.`))(
        marker
      )
      const [before = "", after = ""] = sentTo.split(marker)
      soft.append(before, strong(address), after)
      const acts = el("div", "acts")
      acts.append(
        textButton(this.#t(msg`Change address`), "link brand", () => {
          this.#emailEditing = true
          this.#emailError = ""
          this.#emailNote = ""
          this.#render()
          this.#emailInput.focus()
        }),
        textButton(this.#t(msg`Send again`), "link soft", () => void this.#saveEmail(address))
      )
      text.append(el("b", "", this.#t(msg`Check your inbox`)), soft, acts, ...note(), ...failure())
      this.#email.append(text)
    } else if (mode === "bad") {
      this.#email.className = "mail bad"
      const head = el("div", "head")
      head.insertAdjacentHTML("beforeend", icons.alert)
      head.append(this.#t(msg`We couldn't reach ${address}`))
      this.#email.replaceChildren(
        head,
        el("span", "soft", this.#t(msg`That address doesn't accept mail. Check it for a typo.`)),
        form(),
        ...failure()
      )
    } else if (mode === "confirmed") {
      this.#email.className = "mail ok"
      this.#email.innerHTML = icons.check
      this.#email.append(this.#t(msg`Replies will also reach ${address}`))
    } else if (mode === "saved") {
      this.#email.className = "mail ok"
      this.#email.innerHTML = icons.check
      const known = address
      this.#email.append(this.#t(msg`Thanks! We'll e-mail you at ${known} if you're gone when we reply.`))
    } else {
      this.#email.className = "card"
      const cancel = this.#emailEditing
        ? [
            textButton(this.#t(msg`Cancel`), "link soft", () => {
              this.#emailEditing = false
              this.#emailError = ""
              this.#render()
            }),
          ]
        : []
      this.#email.replaceChildren(
        el("p", "", this.#t(msg`Nobody is available right now. Leave your e-mail and we'll send the reply there too.`)),
        form(),
        ...failure(),
        ...cancel
      )
    }
  }

  #renderChips(): void {
    const { locale } = this.#host.config()
    this.#chips.hidden = this.#files.length === 0
    this.#chips.replaceChildren(
      ...this.#files.map((file, index) => {
        const chip = el("span", "chip")
        chip.insertAdjacentHTML("beforeend", icons.file)
        const two = el("span", "two")
        two.append(el("b", "", file.name || "image"), el("span", "", fileSize(locale, file.size)))
        chip.append(
          two,
          iconButton(
            icons.closeSmall,
            this.#t(msg`Remove ${file.name}`),
            () => {
              this.#files = this.#files.filter((_, i) => i !== index)
              this.#renderChips()
              this.#syncSend()
            },
            "icon sm"
          )
        )
        return chip
      })
    )
  }
}

export function mount(host: PanelHost): PanelController {
  return new Chat(host)
}
