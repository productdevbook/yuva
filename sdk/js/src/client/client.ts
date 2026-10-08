import {
  ApiError,
  call,
  type Fetch,
  type ClientContact,
  type ClientConversation,
  type ClientConversationCreated,
  type ClientConversationPage,
  type ClientInbox,
  type ClientMessage,
  type ClientMessagePage,
  type ClientRatingCreate,
  type ClientReadState,
  type ClientRealtimeMessage,
  type ClientSession,
  type ClientSessionInfo,
  type Rating,
  type Request,
} from "./api";
import { Realtime } from "./realtime";

export type IdentityTokenSource = () => string | null | undefined | Promise<string | null | undefined>;

export interface YuvaStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export interface YuvaClientOptions {
  server: string;
  channel: string;
  identityToken?: IdentityTokenSource | null;
  storage?: YuvaStorage | null;
  fetch?: Fetch;
  WebSocket?: typeof WebSocket;
  idempotencyKeys?: boolean;
}

export interface StoredState {
  visitor_id?: string;
  session?: { token: string; expires_at: string; sub: string | null };
  [key: string]: unknown;
}

export interface ConnectionState {
  state: "connecting" | "open" | "reconnecting" | "closed";
  attempt: number;
}

export interface YuvaClientEvents {
  event: ClientRealtimeMessage;
  connection: ConnectionState;
  session: { session: ClientSessionInfo; renewed: boolean };
}

export interface MessageInput {
  body?: string;
  files?: Blob[];
  client_id?: string;
}

export interface ConversationInput extends MessageInput {
  subject?: string;
}

export interface Page {
  cursor?: string;
  limit?: number;
}

export function storageKey(channel: string): string {
  return `yuva:${channel}`;
}

export function randomId(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, "0")).join("");
}

function jwtSubject(token: string): string | null {
  try {
    const part = (token.split(".")[1] ?? "").replace(/-/g, "+").replace(/_/g, "/");
    const payload = JSON.parse(atob(part)) as { sub?: unknown };
    return typeof payload.sub === "string" ? payload.sub : null;
  } catch {
    return null;
  }
}

function defaultStorage(): YuvaStorage {
  try {
    const storage = globalThis.localStorage;
    if (storage) return storage;
  } catch {}
  const memory = new Map<string, string>();
  return { getItem: (key) => memory.get(key) ?? null, setItem: (key, value) => void memory.set(key, value) };
}

function query(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams();
  for (const [name, value] of Object.entries(params)) if (value !== undefined) search.set(name, String(value));
  const text = search.toString();
  return text ? `?${text}` : "";
}

function quoted(value: string): string {
  return value.replace(/"/g, "%22").replace(/\r/g, "%0D").replace(/\n/g, "%0A");
}

// Built by hand with a boundary derived from client_id: a retry must send the same bytes, or the server
// treats the reused Idempotency-Key as a different request.
function multipart(fields: [string, string][], files: Blob[], boundary: string): Blob {
  const parts: BlobPart[] = [];
  for (const [name, value] of fields) parts.push(`--${boundary}\r\nContent-Disposition: form-data; name="${name}"\r\n\r\n${value}\r\n`);
  for (const file of files) {
    const filename = (file as File).name || "file";
    parts.push(
      `--${boundary}\r\nContent-Disposition: form-data; name="files"; filename="${quoted(filename)}"\r\nContent-Type: ${file.type || "application/octet-stream"}\r\n\r\n`,
      file,
      "\r\n",
    );
  }
  parts.push(`--${boundary}--\r\n`);
  return new Blob(parts);
}

export class YuvaClient {
  readonly server: string;
  readonly channel: string;
  #fetch: Fetch;
  #socketClass: typeof WebSocket | undefined;
  #storage: YuvaStorage;
  #identity: IdentityTokenSource | null;
  #idempotencyKeys: boolean;
  #token: string | null = null;
  #session: ClientSessionInfo | null = null;
  #starting: Promise<ClientSessionInfo> | null = null;
  #renewing: Promise<void> | null = null;
  #realtime: Realtime | null = null;
  #handlers = new Map<keyof YuvaClientEvents, Set<(value: never) => void>>();

  constructor(options: YuvaClientOptions) {
    this.server = options.server.replace(/\/+$/, "");
    this.channel = options.channel;
    this.#fetch = options.fetch ?? ((url, init) => fetch(url, init));
    this.#socketClass = options.WebSocket ?? globalThis.WebSocket;
    this.#storage = options.storage ?? defaultStorage();
    this.#identity = options.identityToken ?? null;
    // Servers up to 0.0.3 do not list Idempotency-Key in their /client/v1 CORS headers, so browsers would fail the preflight.
    this.#idempotencyKeys = options.idempotencyKeys ?? typeof (globalThis as { document?: unknown }).document === "undefined";
  }

  get session(): ClientSessionInfo | null {
    return this.#session;
  }

  get token(): string | null {
    return this.#token;
  }

  get connected(): boolean {
    return this.#realtime?.connected ?? false;
  }

  on<K extends keyof YuvaClientEvents>(type: K, handler: (value: YuvaClientEvents[K]) => void): () => void {
    let set = this.#handlers.get(type);
    if (!set) this.#handlers.set(type, (set = new Set()));
    set.add(handler as (value: never) => void);
    return () => void set.delete(handler as (value: never) => void);
  }

  #emit<K extends keyof YuvaClientEvents>(type: K, value: YuvaClientEvents[K]): void {
    for (const handler of this.#handlers.get(type) ?? []) (handler as (value: YuvaClientEvents[K]) => void)(value);
  }

  stored(): StoredState {
    try {
      const raw = this.#storage.getItem(storageKey(this.channel));
      return raw ? (JSON.parse(raw) as StoredState) : {};
    } catch {
      return {};
    }
  }

  #store(state: StoredState): void {
    try {
      this.#storage.setItem(storageKey(this.channel), JSON.stringify(state));
    } catch {}
  }

  channelSettings(): Promise<ClientInbox> {
    return this.#call<ClientInbox>(`/client/v1/channels/${encodeURIComponent(this.channel)}`);
  }

  setIdentityToken(source: IdentityTokenSource | null): void {
    this.#identity = source;
    this.reset();
  }

  start(): Promise<ClientSessionInfo> {
    if (this.#session) return Promise.resolve(this.#session);
    this.#starting ??= this.#open(false)
      .then((session) => {
        this.#emit("session", { session, renewed: false });
        return session;
      })
      .finally(() => {
        this.#starting = null;
      });
    return this.#starting;
  }

  async refresh(): Promise<ClientSessionInfo> {
    const info = await this.#authed<ClientSessionInfo>("/client/v1/session");
    this.#session = { ...this.#session, ...info };
    this.#emit("session", { session: this.#session, renewed: false });
    return this.#session;
  }

  reset(): void {
    this.#realtime?.stop();
    this.#realtime?.reset();
    this.#starting = null;
    this.#renewing = null;
    this.#token = null;
    this.#session = null;
  }

  async signOut(): Promise<void> {
    const token = this.#token;
    this.reset();
    const stored = this.stored();
    delete stored.session;
    this.#store(stored);
    if (token) await this.#call("/client/v1/session", { method: "DELETE", token }).catch(() => undefined);
  }

  async connect(): Promise<void> {
    if (!this.#socketClass) throw new Error("Yuva: no WebSocket in this runtime; pass options.WebSocket");
    await this.start();
    this.#realtime ??= new Realtime(this.server, this.#socketClass, {
      token: () => this.#token,
      event: (message) => {
        if (message.type === "ready") this.#emit("connection", { state: "open", attempt: 0 });
        this.#emit("event", message);
      },
      sessionEnded: () => void this.#renew().catch(() => this.#realtime?.nudge()),
      reconnecting: (attempt) => {
        this.#emit("connection", { state: "reconnecting", attempt });
        if (attempt === 2) void this.#authed("/client/v1/session").catch(() => undefined);
      },
    });
    if (!this.#realtime.connected) this.#emit("connection", { state: "connecting", attempt: 0 });
    this.#realtime.start();
  }

  disconnect(): void {
    this.#realtime?.stop();
    this.#emit("connection", { state: "closed", attempt: 0 });
  }

  reconnect(): void {
    this.#realtime?.nudge();
  }

  listConversations(page: Page = {}): Promise<ClientConversationPage> {
    return this.#authed(`/client/v1/conversations${query({ ...page })}`);
  }

  getConversation(id: string): Promise<ClientConversation> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(id)}`);
  }

  startConversation(input: ConversationInput): Promise<ClientConversationCreated> {
    return this.#authed("/client/v1/conversations", this.#messageRequest(input));
  }

  listMessages(conversationId: string, page: Page & { order?: "asc" | "desc" } = {}): Promise<ClientMessagePage> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/messages${query({ ...page })}`);
  }

  sendMessage(conversationId: string, input: MessageInput): Promise<ClientMessage> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/messages`, this.#messageRequest(input));
  }

  markRead(conversationId: string, messageId?: string): Promise<ClientReadState> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/read`, {
      method: "POST",
      json: messageId ? { message_id: messageId } : {},
    });
  }

  rate(conversationId: string, rating: Rating, comment?: string): Promise<ClientConversation> {
    const body: ClientRatingCreate = { rating, comment: comment?.trim() || undefined };
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/rating`, { method: "POST", json: body });
  }

  setTyping(conversationId: string, typing: boolean): Promise<void> {
    return this.#authed(`/client/v1/conversations/${encodeURIComponent(conversationId)}/typing`, { method: "POST", json: { typing } });
  }

  setEmail(email: string): Promise<ClientContact> {
    return this.#authed("/client/v1/contact/email", { method: "PUT", json: { email } });
  }

  attachment(id: string): Promise<Blob> {
    return this.#authed(`/client/v1/attachments/${encodeURIComponent(id)}`, { blob: true });
  }

  #messageRequest({ body, subject, files = [], client_id = randomId() }: ConversationInput): Request {
    const headers: Record<string, string> = this.#idempotencyKeys && /^[\x20-\x7e]{1,255}$/.test(client_id) ? { "Idempotency-Key": client_id } : {};
    if (files.length === 0) return { method: "POST", headers, json: { body, subject, client_id } };
    const fields: [string, string][] = [];
    if (subject) fields.push(["subject", subject]);
    if (body) fields.push(["body", body]);
    fields.push(["client_id", client_id]);
    const boundary = `yuva-${client_id.replace(/[^\w-]/g, "").slice(0, 60)}-${client_id.length}`;
    headers["Content-Type"] = `multipart/form-data; boundary=${boundary}`;
    return { method: "POST", headers, body: multipart(fields, files, boundary) };
  }

  #call<T>(path: string, request?: Request): Promise<T> {
    return call<T>(this.#fetch, this.server, path, request);
  }

  async #authed<T>(path: string, request: Request = {}, retry = true): Promise<T> {
    if (!this.#token) await this.start();
    try {
      return await this.#call<T>(path, { ...request, token: this.#token });
    } catch (error) {
      if (!retry || !(error instanceof ApiError) || error.status !== 401) throw error;
      await this.#renew();
      return this.#authed<T>(path, request, false);
    }
  }

  #renew(): Promise<void> {
    this.#renewing ??= (async () => {
      const session = await this.#open(true);
      if (this.#realtime) {
        this.#realtime.stop();
        this.#realtime.reset();
        this.#realtime.start();
      }
      this.#emit("session", { session, renewed: true });
    })().finally(() => {
      this.#renewing = null;
    });
    return this.#renewing;
  }

  async #open(fresh: boolean): Promise<ClientSessionInfo> {
    const stored = this.stored();
    const identity = (await this.#identity?.()) || null;
    const subject = identity ? jwtSubject(identity) : null;
    const saved = stored.session;
    if (!fresh && saved && saved.sub === subject && Date.parse(saved.expires_at) > Date.now() + 60_000) {
      try {
        const info = await this.#call<ClientSessionInfo>("/client/v1/session", { token: saved.token });
        return this.#apply(info, saved.token, subject);
      } catch (error) {
        if (!(error instanceof ApiError) || (error.status !== 401 && error.status !== 403)) throw error;
      }
    }
    const session = await this.#call<ClientSession>("/client/v1/session", {
      method: "POST",
      json: { channel_key: this.channel, identity_token: identity ?? undefined, visitor_id: stored.visitor_id },
    });
    const { token, ...info } = session;
    return this.#apply(info, token, subject);
  }

  #apply(info: ClientSessionInfo, token: string, subject: string | null): ClientSessionInfo {
    this.#token = token;
    this.#session = info;
    const stored = this.stored();
    stored.session = { token, expires_at: info.expires_at, sub: subject };
    if (info.visitor_id) stored.visitor_id = info.visitor_id;
    else if (info.contact.identified) delete stored.visitor_id;
    this.#store(stored);
    return info;
  }
}

export function createYuvaClient(options: YuvaClientOptions): YuvaClient {
  return new YuvaClient(options);
}
