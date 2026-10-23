import {
  ApiError,
  call,
  createSubscription,
  mailingLists,
  type Fetch,
  type ClientMailingLists,
  type ClientSubscriptionAccepted,
  type ClientSubscriptionCreate,
  type ClientSubscriptionList,
  type ClientSubscriptionPut,
  type ClientSubscriptionStatus,
  type ClientContact,
  type ClientConversation,
  type ClientConversationCreated,
  type ClientConversationPage,
  type ClientFeedbackCreate,
  type ClientHelp,
  type ClientHelpArticle,
  type ClientHelpCollectionDetail,
  type ClientHelpReaction,
  type ClientHelpSearchClick,
  type ClientHelpSearchResults,
  type ClientInbox,
  type ClientRoadmap,
  type ClientWhatsNew,
  type ClientWhatsNewEntry,
  type IdeaStage,
  type ClientMessage,
  type ClientMessagePage,
  type ClientDiscussionCreate,
  type ClientDiscussionPostUpdate,
  type Discussion,
  type DiscussionDetail,
  type DiscussionPage,
  type DiscussionPost,
  type DiscussionPostCreate,
  type DiscussionSort,
  type ClientRatingCreate,
  type ClientReadState,
  type ClientRealtimeMessage,
  type ClientSession,
  type ClientSessionInfo,
  type PageRatingCreate,
  type Rating,
  type Request,
} from "./api"
import { Realtime } from "./realtime"
import { openReaderPopup, type ReaderSession, type ReaderSignInOptions } from "./reader"

export type IdentityTokenSource = () => string | null | undefined | Promise<string | null | undefined>

export interface YuvaStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export interface YuvaClientOptions {
  server: string
  channel: string
  identityToken?: IdentityTokenSource | null
  storage?: YuvaStorage | null
  fetch?: Fetch
  WebSocket?: typeof WebSocket
  idempotencyKeys?: boolean
}

export interface StoredState {
  visitor_id?: string
  session?: { token: string; expires_at: string; sub: string | null }
  [key: string]: unknown
}

export interface ConnectionState {
  state: "connecting" | "open" | "reconnecting" | "closed"
  attempt: number
}

export interface YuvaClientEvents {
  event: ClientRealtimeMessage
  conversationRemoved: { id: string }
  connection: ConnectionState
  session: { session: ClientSessionInfo; renewed: boolean }
  reader: ReaderSession | null
}

export interface MessageInput {
  body?: string
  files?: Blob[]
  client_id?: string
}

export interface ConversationInput extends MessageInput {
  subject?: string
  page_url?: string | null
  page_title?: string
  help_article_id?: string
}

export interface HelpOptions {
  locale?: string
}

export interface HelpGetOptions extends HelpOptions {
  page?: string
}

export interface HelpSearchOptions extends HelpOptions {
  limit?: number
  final?: boolean
}

export interface Help {
  get(options?: HelpGetOptions): Promise<ClientHelp>
  search(q: string, options?: HelpSearchOptions): Promise<ClientHelpSearchResults>
  collection(id: string, options?: HelpOptions): Promise<ClientHelpCollectionDetail>
  article(id: string, options?: HelpOptions): Promise<ClientHelpArticle>
  react(articleId: string, input: ClientHelpReaction): Promise<void>
  click(input: ClientHelpSearchClick): Promise<void>
}

export interface Page {
  cursor?: string
  limit?: number
}

export interface DiscussionListOptions extends Page {
  sort?: DiscussionSort
}

export interface IdeaListOptions extends Page {
  stage?: IdeaStage
  sort?: "top" | "new"
  q?: string
}

export interface WhatsNewOptions extends Page {
  locale?: string
}

export interface IdeaInput {
  title: string
  body: string
}

export function storageKey(channel: string): string {
  return `yuva:${channel}`
}

export function readerStorageKey(server: string, channel: string): string {
  return `yuva:reader:${server} ${channel}`
}

export function randomId(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID()
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, "0")).join("")
}

function jwtSubject(token: string): string | null {
  try {
    const part = (token.split(".")[1] ?? "").replace(/-/g, "+").replace(/_/g, "/")
    const payload = JSON.parse(atob(part)) as { sub?: unknown }
    return typeof payload.sub === "string" ? payload.sub : null
  } catch {
    return null
  }
}

function currentPage(): { page_url?: string; page_title?: string } {
  const location = globalThis.location
  if (!location || !/^https?:$/.test(location.protocol)) return {}
  const page_url = location.origin + location.pathname
  if (page_url.length > 2000) return {}
  const title = Array.from(globalThis.document?.title.trim() ?? "")
    .slice(0, 300)
    .join("")
    .trim()
  return title ? { page_url, page_title: title } : { page_url }
}

function defaultStorage(): YuvaStorage {
  try {
    const storage = globalThis.localStorage
    if (storage) return storage
  } catch {}
  const memory = new Map<string, string>()
  return { getItem: (key) => memory.get(key) ?? null, setItem: (key, value) => void memory.set(key, value) }
}

function query(params: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams()
  for (const [name, value] of Object.entries(params)) if (value !== undefined) search.set(name, String(value))
  const text = search.toString()
  return text ? `?${text}` : ""
}

function quoted(value: string): string {
  return value.replace(/"/g, "%22").replace(/\r/g, "%0D").replace(/\n/g, "%0A")
}

// Built by hand with a boundary derived from client_id: a retry must send the same bytes, or the server
// treats the reused Idempotency-Key as a different request.
function multipart(fields: [string, string][], files: Blob[], boundary: string): Blob {
  const parts: BlobPart[] = []
  for (const [name, value] of fields)
    parts.push(`--${boundary}\r\nContent-Disposition: form-data; name="${name}"\r\n\r\n${value}\r\n`)
  for (const file of files) {
    const filename = (file as File).name || "file"
    parts.push(
      `--${boundary}\r\nContent-Disposition: form-data; name="files"; filename="${quoted(filename)}"\r\nContent-Type: ${file.type || "application/octet-stream"}\r\n\r\n`,
      file,
      "\r\n"
    )
  }
  parts.push(`--${boundary}--\r\n`)
  return new Blob(parts)
}

export class YuvaClient {
  readonly server: string
  readonly channel: string
  #fetch: Fetch
  #socketClass: typeof WebSocket | undefined
  #storage: YuvaStorage
  #identity: IdentityTokenSource | null
  #idempotencyKeys: boolean
  #token: string | null = null
  #session: ClientSessionInfo | null = null
  #starting: Promise<ClientSessionInfo> | null = null
  #renewing: Promise<void> | null = null
  #realtime: Realtime | null = null
  #reader: ReaderSession | null | undefined
  #handlers = new Map<keyof YuvaClientEvents, Set<(value: never) => void>>()

  readonly help: Help = {
    get: (options = {}) => this.#helpCall(`${query({ ...options })}`),
    search: (q, options = {}) => this.#helpCall(`/search${query({ q, ...options })}`),
    collection: (id, options = {}) => this.#helpCall(`/collections/${encodeURIComponent(id)}${query({ ...options })}`),
    article: (id, options = {}) => this.#helpCall(`/articles/${encodeURIComponent(id)}${query({ ...options })}`),
    react: (articleId, input) =>
      this.#helpCall(`/articles/${encodeURIComponent(articleId)}/reactions`, { method: "POST", json: input }),
    click: (input) => this.#helpCall("/searches/click", { method: "POST", json: input }),
  }

  constructor(options: YuvaClientOptions) {
    this.server = options.server.replace(/\/+$/, "")
    this.channel = options.channel
    this.#fetch = options.fetch ?? ((url, init) => fetch(url, init))
    this.#socketClass = options.WebSocket ?? globalThis.WebSocket
    this.#storage = options.storage ?? defaultStorage()
    this.#identity = options.identityToken ?? null
    // Servers up to 0.0.3 do not list Idempotency-Key in their /client/v1 CORS headers, so browsers would fail the preflight.
    this.#idempotencyKeys =
      options.idempotencyKeys ?? typeof (globalThis as { document?: unknown }).document === "undefined"
  }

  get session(): ClientSessionInfo | null {
    return this.#session
  }

  get token(): string | null {
    return this.#token
  }

  get connected(): boolean {
    return this.#realtime?.connected ?? false
  }

  on<K extends keyof YuvaClientEvents>(type: K, handler: (value: YuvaClientEvents[K]) => void): () => void {
    let set = this.#handlers.get(type)
    if (!set) this.#handlers.set(type, (set = new Set()))
    set.add(handler as (value: never) => void)
    return () => void set.delete(handler as (value: never) => void)
  }

  #emit<K extends keyof YuvaClientEvents>(type: K, value: YuvaClientEvents[K]): void {
    for (const handler of this.#handlers.get(type) ?? []) (handler as (value: YuvaClientEvents[K]) => void)(value)
  }

  stored(): StoredState {
    try {
      const raw = this.#storage.getItem(storageKey(this.channel))
      return raw ? (JSON.parse(raw) as StoredState) : {}
    } catch {
      return {}
    }
  }

  #store(state: StoredState): void {
    try {
      this.#storage.setItem(storageKey(this.channel), JSON.stringify(state))
    } catch {}
  }

  channelSettings(): Promise<ClientInbox> {
    return this.#call<ClientInbox>(`/client/v1/channels/${encodeURIComponent(this.channel)}`)
  }

  ratePage(input: PageRatingCreate): Promise<void> {
    return this.#call(`/client/v1/channels/${encodeURIComponent(this.channel)}/page-ratings`, {
      method: "POST",
      json: input,
    })
  }

  lists(): Promise<ClientMailingLists> {
    return mailingLists(this.#fetch, this.server, this.channel)
  }

  subscribe(input: ClientSubscriptionCreate): Promise<ClientSubscriptionAccepted> {
    return createSubscription(this.#fetch, this.server, this.channel, input)
  }

  subscriptions(): Promise<ClientSubscriptionList> {
    return this.#authed("/client/v1/subscriptions")
  }

  subscribeList(listId: string, input: ClientSubscriptionPut): Promise<ClientSubscriptionStatus> {
    return this.#authed(`/client/v1/subscriptions/${encodeURIComponent(listId)}`, { method: "PUT", json: input })
  }

  unsubscribeList(listId: string): Promise<void> {
    return this.#authed(`/client/v1/subscriptions/${encodeURIComponent(listId)}`, { method: "DELETE" })
  }

  setIdentityToken(source: IdentityTokenSource | null): void {
    this.#identity = source
    this.reset()
  }

  start(): Promise<ClientSessionInfo> {
    if (this.#session) return Promise.resolve(this.#session)
    this.#starting ??= this.#open(false)
      .then((session) => {
        this.#emit("session", { session, renewed: false })
        return session
      })
      .finally(() => {
        this.#starting = null
      })
    return this.#starting
  }

  async refresh(): Promise<ClientSessionInfo> {
    const info = await this.#authed<ClientSessionInfo>("/client/v1/session")
    this.#session = { ...this.#session, ...info }
    this.#emit("session", { session: this.#session, renewed: false })
    return this.#session
  }

  reset(): void {
    this.#realtime?.stop()
    this.#realtime?.reset()
    this.#starting = null
    this.#renewing = null
    this.#token = null
    this.#session = null
  }

  async signOut(): Promise<void> {
    const token = this.#token
    this.reset()
    const stored = this.stored()
    delete stored.session
    this.#store(stored)
    if (token) await this.#call("/client/v1/session", { method: "DELETE", token }).catch(() => undefined)
  }

  async connect(): Promise<void> {
    if (!this.#socketClass) throw new Error("Yuva: no WebSocket in this runtime; pass options.WebSocket")
    await this.start()
    this.#realtime ??= new Realtime(this.server, this.#socketClass, {
      token: () => this.#token,
      event: (message) => {
        if (message.type === "ready") this.#emit("connection", { state: "open", attempt: 0 })
        this.#emit("event", message)
        if (message.type === "conversation.removed") this.#emit("conversationRemoved", { id: message.data.id })
      },
      sessionEnded: () => void this.#renew().catch(() => this.#realtime?.nudge()),
      reconnecting: (attempt) => {
        this.#emit("connection", { state: "reconnecting", attempt })
        if (attempt === 2) void this.#authed("/client/v1/session").catch(() => undefined)
      },
    })
    if (!this.#realtime.connected) this.#emit("connection", { state: "connecting", attempt: 0 })
    this.#realtime.start()
  }

  disconnect(): void {
    this.#realtime?.stop()
    this.#emit("connection", { state: "closed", attempt: 0 })
  }

  reconnect(): void {
    this.#realtime?.nudge()
  }

  listConversations(page: Page = {}): Promise<ClientConversationPage> {
    return this.#authed(`/client/v1/conversations${query({ ...page })}`)
  }

  getConversation(id: string): Promise<ClientConversation> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(id)}`)
  }

  startConversation(input: ConversationInput): Promise<ClientConversationCreated> {
    const request = input.page_url === undefined ? { ...input, ...currentPage() } : input
    return this.#authed("/client/v1/conversations", this.#messageRequest(request))
  }

  listMessages(conversationId: string, page: Page & { order?: "asc" | "desc" } = {}): Promise<ClientMessagePage> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/messages${query({ ...page })}`)
  }

  sendMessage(conversationId: string, input: MessageInput): Promise<ClientMessage> {
    return this.#authed(
      `/client/v1/conversations/${encodeURIComponent(conversationId)}/messages`,
      this.#messageRequest(input)
    )
  }

  markRead(conversationId: string, messageId?: string): Promise<ClientReadState> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/read`, {
      method: "POST",
      json: messageId ? { message_id: messageId } : {},
    })
  }

  rate(conversationId: string, rating: Rating, comment?: string): Promise<ClientConversation> {
    const body: ClientRatingCreate = { rating, comment: comment?.trim() || undefined }
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/rating`, {
      method: "POST",
      json: body,
    })
  }

  setTyping(conversationId: string, typing: boolean): Promise<void> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/typing`, {
      method: "POST",
      json: { typing },
    })
  }

  setEmail(email: string): Promise<ClientContact> {
    return this.#authed("/client/v1/contact/email", { method: "PUT", json: { email } })
  }

  attachment(id: string): Promise<Blob> {
    return this.#authed(`/client/v1/attachments/${encodeURIComponent(id)}`, { blob: true })
  }

  sendFeedback(input: ClientFeedbackCreate & { files?: Blob[] }): Promise<ClientConversationCreated> {
    const { files = [], ...fields } = input
    const client_id = fields.client_id ?? randomId()
    const headers = this.#idempotency(client_id)
    if (files.length === 0)
      return this.#authed("/client/v1/feedback", { method: "POST", headers, json: { ...fields, client_id } })
    const scalars = Object.entries({ ...fields, client_id })
      .filter(([, value]) => value !== undefined && value !== null)
      .map(([name, value]): [string, string] => [name, String(value)])
    const boundary = `yuva-${client_id.replace(/[^\w-]/g, "").slice(0, 60)}-${client_id.length}`
    headers["Content-Type"] = `multipart/form-data; boundary=${boundary}`
    return this.#authed("/client/v1/feedback", {
      method: "POST",
      headers,
      body: multipart(scalars, files, boundary),
    })
  }

  get reader(): ReaderSession | null {
    if (this.#reader === undefined) {
      try {
        const raw = this.#storage.getItem(readerStorageKey(this.server, this.channel))
        this.#reader = raw ? (JSON.parse(raw) as ReaderSession) : null
      } catch {
        this.#reader = null
      }
    }
    if (this.#reader && Date.parse(this.#reader.expires_at) <= Date.now() + 30_000) this.#setReader(null)
    return this.#reader
  }

  #setReader(reader: ReaderSession | null): void {
    const changed = this.#reader?.token !== reader?.token
    this.#reader = reader
    try {
      this.#storage.setItem(readerStorageKey(this.server, this.channel), reader ? JSON.stringify(reader) : "")
    } catch {}
    if (changed) this.#emit("reader", reader)
  }

  readerSignIn(options?: ReaderSignInOptions): Promise<ReaderSession> {
    return openReaderPopup(this.server, this.channel, options).then((reader) => {
      this.#setReader(reader)
      return reader
    })
  }

  setReader(reader: ReaderSession | null): void {
    this.#setReader(reader)
  }

  async readerSignOut(): Promise<void> {
    const token = this.reader?.token
    this.#setReader(null)
    if (token) await this.#call("/client/v1/reader-token", { method: "DELETE", token }).catch(() => undefined)
  }

  listDiscussions(page: string, options: DiscussionListOptions = {}): Promise<DiscussionPage> {
    return this.#discussionCall(
      `/client/v1/channels/${encodeURIComponent(this.channel)}/discussions${query({ page, ...options })}`,
      {},
      false
    )
  }

  roadmap(): Promise<ClientRoadmap> {
    return this.#call(`/client/v1/channels/${encodeURIComponent(this.channel)}/roadmap`)
  }

  whatsNew(options: WhatsNewOptions = {}): Promise<ClientWhatsNew> {
    return this.#call(`/client/v1/channels/${encodeURIComponent(this.channel)}/whats-new${query({ ...options })}`)
  }

  whatsNewEntry(id: string, options: HelpOptions = {}): Promise<ClientWhatsNewEntry> {
    return this.#call(
      `/client/v1/channels/${encodeURIComponent(this.channel)}/whats-new/${encodeURIComponent(id)}${query({ ...options })}`
    )
  }

  listIdeas(options: IdeaListOptions = {}): Promise<DiscussionPage> {
    return this.#discussionCall(
      `/client/v1/channels/${encodeURIComponent(this.channel)}/discussions${query({ kind: "idea", ...options })}`,
      {},
      false
    )
  }

  suggestIdea(input: IdeaInput): Promise<Discussion> {
    return this.startDiscussion({ kind: "idea", ...input })
  }

  getDiscussion(id: string): Promise<DiscussionDetail> {
    return this.#discussionCall(
      `/client/v1/channels/${encodeURIComponent(this.channel)}/discussions/${encodeURIComponent(id)}`,
      {},
      false
    )
  }

  startDiscussion(input: ClientDiscussionCreate): Promise<Discussion> {
    return this.#discussionCall("/client/v1/discussions", { method: "POST", json: input })
  }

  replyToDiscussion(discussionId: string, input: DiscussionPostCreate): Promise<DiscussionPost> {
    return this.#discussionCall(`/client/v1/discussions/${encodeURIComponent(discussionId)}/posts`, {
      method: "POST",
      json: input,
    })
  }

  editDiscussionPost(postId: string, input: ClientDiscussionPostUpdate): Promise<DiscussionPost> {
    return this.#discussionCall(`/client/v1/discussion-posts/${encodeURIComponent(postId)}`, {
      method: "PATCH",
      json: input,
    })
  }

  deleteDiscussionPost(postId: string): Promise<void> {
    return this.#discussionCall(`/client/v1/discussion-posts/${encodeURIComponent(postId)}`, { method: "DELETE" })
  }

  voteDiscussionPost(postId: string, voted = true): Promise<DiscussionPost> {
    return this.#discussionCall(`/client/v1/discussion-posts/${encodeURIComponent(postId)}/vote`, {
      method: voted ? "PUT" : "DELETE",
    })
  }

  followDiscussion(discussionId: string, following = true): Promise<void> {
    return this.#discussionCall(`/client/v1/discussions/${encodeURIComponent(discussionId)}/follow`, {
      method: following ? "PUT" : "DELETE",
    })
  }

  acceptDiscussionAnswer(discussionId: string, postId: string | null): Promise<Discussion> {
    const path = `/client/v1/discussions/${encodeURIComponent(discussionId)}/answer`
    return this.#discussionCall(path, postId ? { method: "PUT", json: { post_id: postId } } : { method: "DELETE" })
  }

  async discussionWriter(): Promise<"reader" | "contact" | null> {
    if (this.reader) return "reader"
    if (this.#session) return this.#session.contact.identified ? "contact" : null
    return (await this.#identity?.()) ? "contact" : null
  }

  async #discussionCall<T>(path: string, request: Request, write = true): Promise<T> {
    const reader = this.reader
    if (reader) {
      try {
        return await this.#call<T>(path, { ...request, token: reader.token })
      } catch (error) {
        if (!(error instanceof ApiError) || error.status !== 401 || this.#reader?.token !== reader.token) throw error
        this.#setReader(null)
        if (write) throw new ApiError(403, "sign_in_required")
      }
    }
    const writer = await this.discussionWriter()
    if (writer === "contact") return this.#authed<T>(path, request)
    if (write) throw new ApiError(403, "sign_in_required")
    return this.#call<T>(path, request)
  }

  #helpCall<T>(path: string, request?: Request): Promise<T> {
    return this.#call<T>(`/client/v1/channels/${encodeURIComponent(this.channel)}/help${path}`, request)
  }

  #idempotency(client_id: string): Record<string, string> {
    return this.#idempotencyKeys && /^[\x20-\x7e]{1,255}$/.test(client_id) ? { "Idempotency-Key": client_id } : {}
  }

  #messageRequest({ body, subject, files = [], client_id = randomId(), ...page }: ConversationInput): Request {
    const headers = this.#idempotency(client_id)
    const page_url = page.page_url ?? undefined
    const page_title = page_url ? page.page_title : undefined
    const help_article_id = page.help_article_id
    if (files.length === 0)
      return { method: "POST", headers, json: { body, subject, client_id, page_url, page_title, help_article_id } }
    const fields: [string, string][] = []
    if (subject) fields.push(["subject", subject])
    if (body) fields.push(["body", body])
    fields.push(["client_id", client_id])
    if (page_url) fields.push(["page_url", page_url])
    if (page_title) fields.push(["page_title", page_title])
    if (help_article_id) fields.push(["help_article_id", help_article_id])
    const boundary = `yuva-${client_id.replace(/[^\w-]/g, "").slice(0, 60)}-${client_id.length}`
    headers["Content-Type"] = `multipart/form-data; boundary=${boundary}`
    return { method: "POST", headers, body: multipart(fields, files, boundary) }
  }

  #call<T>(path: string, request?: Request): Promise<T> {
    return call<T>(this.#fetch, this.server, path, request)
  }

  async #authed<T>(path: string, request: Request = {}, retry = true): Promise<T> {
    if (!this.#token) await this.start()
    try {
      return await this.#call<T>(path, { ...request, token: this.#token })
    } catch (error) {
      if (!retry || !(error instanceof ApiError) || error.status !== 401) throw error
      await this.#renew()
      return this.#authed<T>(path, request, false)
    }
  }

  #renew(): Promise<void> {
    this.#renewing ??= (async () => {
      const session = await this.#open(true)
      if (this.#realtime) {
        this.#realtime.stop()
        this.#realtime.reset()
        this.#realtime.start()
      }
      this.#emit("session", { session, renewed: true })
    })().finally(() => {
      this.#renewing = null
    })
    return this.#renewing
  }

  async #open(fresh: boolean): Promise<ClientSessionInfo> {
    const stored = this.stored()
    const identity = (await this.#identity?.()) || null
    const subject = identity ? jwtSubject(identity) : null
    const saved = stored.session
    if (!fresh && saved && saved.sub === subject && Date.parse(saved.expires_at) > Date.now() + 60_000) {
      try {
        const info = await this.#call<ClientSessionInfo>("/client/v1/session", { token: saved.token })
        return this.#apply(info, saved.token, subject)
      } catch (error) {
        if (!(error instanceof ApiError) || (error.status !== 401 && error.status !== 403)) throw error
      }
    }
    const session = await this.#call<ClientSession>("/client/v1/session", {
      method: "POST",
      json: { channel_key: this.channel, identity_token: identity ?? undefined, visitor_id: stored.visitor_id },
    })
    const { token, ...info } = session
    return this.#apply(info, token, subject)
  }

  #apply(info: ClientSessionInfo, token: string, subject: string | null): ClientSessionInfo {
    this.#token = token
    this.#session = info
    const stored = this.stored()
    stored.session = { token, expires_at: info.expires_at, sub: subject }
    if (info.visitor_id) stored.visitor_id = info.visitor_id
    else if (info.contact.identified) delete stored.visitor_id
    this.#store(stored)
    return info
  }
}

export function createYuvaClient(options: YuvaClientOptions): YuvaClient {
  return new YuvaClient(options)
}
