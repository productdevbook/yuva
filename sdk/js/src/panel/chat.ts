import { setupI18n, type I18n, type MessageDescriptor, type Messages } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import { messages as en } from "../locales/en/panel";
import { messages as tr } from "../locales/tr/panel";
import { launcherOf, readStored, writeStored, type PanelController, type PanelHost } from "../types";
import {
  ApiError,
  call,
  type ClientAttachment,
  type ClientContact,
  type ClientConversation,
  type ClientConversationCreated,
  type ClientConversationPage,
  type ClientInbox,
  type ClientMessage,
  type ClientMessagePage,
  type ClientRealtimeMessage,
  type ClientSession,
  type ClientSessionInfo,
  type Request,
} from "./api";
import { Realtime } from "./realtime";
import { styles } from "./styles";
import { richText } from "./text";

const catalogs: Record<string, Messages> = { en, tr };
const maxFiles = 10;
const groupGap = 5 * 60_000;

type View = { kind: "list" } | { kind: "thread"; id: string | null };

interface Pending {
  client_id: string;
  conversation: string | null;
  body: string;
  files: File[];
  state: "sending" | "failed" | "refused";
  status?: number;
}

interface Thread {
  messages: ClientMessage[];
  cursor?: string;
  complete: boolean;
  loading: boolean;
}

const icons = {
  close: `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>`,
  back: `<svg class="flip" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 6l-6 6 6 6"/></svg>`,
  attach: `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 11.5l-8.3 8.3a5 5 0 01-7-7L13 4.5a3.3 3.3 0 014.7 4.7l-8.3 8.3a1.7 1.7 0 01-2.4-2.4L14.5 7.6"/></svg>`,
  send: `<svg class="flip" width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3.4 20.4l17.5-7.5a1 1 0 000-1.8L3.4 3.6a1 1 0 00-1.4 1.2L4.5 12l-2.5 7.2a1 1 0 001.4 1.2zM4.5 12h7"/></svg>`,
  file: `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" aria-hidden="true"><path d="M14 3H6v18h12V7z M14 3v4h4"/></svg>`,
};

function el<K extends keyof HTMLElementTagNameMap>(tag: K, className?: string, text?: string): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

function iconButton(icon: string, label: string, onClick: () => void, className = "icon"): HTMLButtonElement {
  const button = el("button", className);
  button.type = "button";
  button.innerHTML = icon;
  button.setAttribute("aria-label", label);
  button.title = label;
  button.addEventListener("click", onClick);
  return button;
}

function randomId(): string {
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

const time = (value: string) => Date.parse(value);
const byCreated = (a: ClientMessage, b: ClientMessage) => time(a.created_at) - time(b.created_at);
const byActivity = (a: ClientConversation, b: ClientConversation) =>
  time(b.last_message_at ?? b.created_at) - time(a.last_message_at ?? a.created_at);
const authorKey = (m: ClientMessage) => (m.author.type === "contact" ? "contact" : `${m.author.type}:${m.author.name ?? ""}`);
const isImage = (a: ClientAttachment) => /^image\/(png|jpeg|gif|webp)$/.test(a.content_type);

export class Chat implements PanelController {
  #host: PanelHost;
  #i18n!: I18n;
  #realtime: Realtime | null = null;
  #token: string | null = null;
  #inbox: ClientInbox | null = null;
  #contact: ClientContact | null = null;
  #phase: "loading" | "ready" | "error" = "loading";
  #error = "";
  #starting: Promise<void> | null = null;
  #beginning: Promise<void> | null = null;
  #conversations = new Map<string, ClientConversation>();
  #conversationCursor: string | undefined;
  #threads = new Map<string, Thread>();
  #pending: Pending[] = [];
  #typing = new Map<string, { name: string; timer: ReturnType<typeof setTimeout> }>();
  #view: View | null = null;
  #reconnecting = false;
  #files: File[] = [];
  #typingSentAt = 0;
  #typingIdle: ReturnType<typeof setTimeout> | null = null;
  #blobs = new Map<string, string>();
  #blobLoads = new Map<string, Promise<string>>();
  #emailSaved = false;
  #emailError = "";
  #connectedBefore = false;

  #header = el("header", "header");
  #banner = el("div", "banner");
  #body = el("div", "body");
  #scroll = el("div", "scroll");
  #email = el("div", "card");
  #emailInput = el("input");
  #composer = el("div", "composer");
  #chips = el("div", "chips");
  #textarea = el("textarea");
  #fileInput = el("input");
  #sendButton: HTMLButtonElement;
  #attachButton: HTMLButtonElement;
  #live = el("div", "sr");
  #scrollThread: string | null | undefined;
  #atEnd = true;

  constructor(host: PanelHost) {
    this.#host = host;
    this.#setLocale();
    this.#live.setAttribute("aria-live", "polite");
    this.#scroll.addEventListener("scroll", () => {
      this.#atEnd = this.#scroll.scrollHeight - this.#scroll.scrollTop - this.#scroll.clientHeight < 48;
      if (this.#scroll.scrollTop < 60 && this.#view?.kind === "thread" && this.#view.id) void this.#loadMessages(this.#view.id, true);
    });
    this.#textarea.rows = 1;
    this.#textarea.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
        event.preventDefault();
        void this.#send();
      }
    });
    this.#textarea.addEventListener("input", () => {
      this.#autosize();
      this.#syncSend();
      this.#typingInput();
    });
    this.#textarea.addEventListener("paste", (event) => {
      const files = Array.from(event.clipboardData?.files ?? []);
      if (files.length === 0) return;
      event.preventDefault();
      this.#addFiles(files);
    });
    this.#fileInput.type = "file";
    this.#fileInput.multiple = true;
    this.#fileInput.hidden = true;
    this.#fileInput.addEventListener("change", () => {
      this.#addFiles(Array.from(this.#fileInput.files ?? []));
      this.#fileInput.value = "";
    });
    this.#attachButton = iconButton(icons.attach, "", () => this.#fileInput.click());
    this.#sendButton = iconButton(icons.send, "", () => void this.#send(), "icon send");
    const form = el("form");
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      void this.#send();
    });
    form.append(this.#attachButton, this.#textarea, this.#sendButton, this.#fileInput);
    this.#composer.append(this.#chips, form);

    this.#emailInput.type = "email";
    this.#emailInput.required = true;
    this.#emailInput.autocomplete = "email";

    document.addEventListener("visibilitychange", this.#onVisible);
    window.addEventListener("online", this.#onVisible);
    this.#renderAll();
    void this.#start();
  }

  update(): void {
    this.#setLocale();
    this.#renderAll();
  }

  opened(): void {
    if (this.#phase === "error") {
      this.#starting = null;
      void this.#start();
      return;
    }
    if (this.#phase === "ready") void (this.#token ? this.#refreshSession() : this.#preview().then(() => this.#render()).catch(() => undefined));
    this.#render();
    this.#scrollToEnd();
    this.#markRead();
    if (this.#host.config().layout === "launcher") this.#textarea.focus({ preventScroll: true });
  }

  closed(): void {
    this.#stopTyping();
  }

  identityChanged(): void {
    this.#reset();
    void this.#start();
  }

  async signOut(): Promise<void> {
    const { channel, server } = this.#host.config();
    const token = this.#token;
    this.#reset();
    if (channel) {
      const stored = readStored(channel);
      delete stored.session;
      writeStored(channel, stored);
    }
    if (token) await call(server, "/client/v1/session", { method: "DELETE", token }).catch(() => undefined);
    if (this.#host.isOpen()) void this.#start();
    else this.#render();
  }

  destroy(): void {
    this.#realtime?.stop();
    this.#stopTyping();
    document.removeEventListener("visibilitychange", this.#onVisible);
    window.removeEventListener("online", this.#onVisible);
    for (const url of this.#blobs.values()) URL.revokeObjectURL(url);
  }

  #reset(): void {
    this.#realtime?.stop();
    this.#realtime = null;
    this.#starting = null;
    this.#beginning = null;
    this.#token = null;
    this.#inbox = null;
    this.#contact = null;
    this.#phase = "loading";
    this.#conversations.clear();
    this.#threads.clear();
    this.#pending = [];
    this.#view = null;
    this.#connectedBefore = false;
    this.#host.setUnread(0);
  }

  #onVisible = () => {
    if (document.visibilityState !== "visible") return;
    this.#realtime?.nudge();
    this.#markRead();
    void this.#refreshInbox();
  };

  #setLocale(): void {
    const { locale } = this.#host.config();
    this.#i18n = setupI18n({ locale, messages: { [locale]: catalogs[locale] ?? en } });
  }

  #t(descriptor: MessageDescriptor): string {
    return this.#i18n._(descriptor);
  }


  #start(): Promise<void> {
    this.#starting ??= (async () => {
      this.#phase = "loading";
      this.#render();
      const { channel } = this.#host.config();
      if (!channel) throw new ApiError(0, "not_configured");
      if ((await this.#host.identityToken()) || readStored(channel).session) await this.#begin();
      else {
        await this.#preview();
        if (!this.#inbox?.chat.allow_anonymous) throw new ApiError(403, "anonymous_not_allowed");
      }
      this.#phase = "ready";
      if (!this.#view) {
        const list = this.#sorted();
        this.#view = list.length > 1 ? { kind: "list" } : { kind: "thread", id: list[0]?.id ?? null };
        if (this.#view.kind === "thread" && this.#view.id) await this.#loadMessages(this.#view.id);
      }
      this.#render();
      this.#scrollToEnd();
      this.#markRead();
    })().catch((error: unknown) => {
      this.#starting = null;
      if (this.#refusal(error)) return;
      this.#phase = "error";
      this.#error = error instanceof ApiError ? error.code : "error";
      this.#render();
    });
    return this.#starting;
  }

  async #preview(): Promise<void> {
    const inbox = await this.#fetchInbox();
    if (!this.#token) this.#setInbox(inbox);
  }

  #fetchInbox(): Promise<ClientInbox> {
    const { channel, server } = this.#host.config();
    if (!channel) throw new ApiError(0, "not_configured");
    return call<ClientInbox>(server, `/client/v1/channels/${encodeURIComponent(channel)}`);
  }

  #setInbox(inbox: ClientInbox): void {
    const { channel } = this.#host.config();
    this.#inbox = inbox;
    const launcher = launcherOf(inbox);
    this.#host.setLauncher(launcher);
    if (channel) writeStored(channel, { ...readStored(channel), launcher });
  }

  async #refreshInbox(): Promise<void> {
    if (this.#phase !== "ready") return;
    try {
      this.#setInbox(await this.#fetchInbox());
      this.#render();
    } catch (error) {
      this.#refusal(error, true);
    }
  }

  #refusal(error: unknown, channelRequest = !this.#token): boolean {
    const refused = error instanceof ApiError && (error.code === "origin_not_allowed" || (error.status === 404 && channelRequest));
    if (refused) this.#host.refused();
    return refused;
  }

  #begin(): Promise<void> {
    this.#beginning ??= (async () => {
      await this.#session(false);
      await this.#loadConversations();
      this.#connect();
    })().finally(() => {
      this.#beginning = null;
    });
    return this.#beginning;
  }

  async #session(fresh: boolean): Promise<void> {
    const { channel, server } = this.#host.config();
    if (!channel) throw new ApiError(0, "not_configured");
    const stored = readStored(channel);
    const identity = await this.#host.identityToken();
    const subject = identity ? jwtSubject(identity) : null;
    const saved = stored.session;
    if (!fresh && saved && saved.sub === subject && time(saved.expires_at) > Date.now() + 60_000) {
      try {
        const info = await call<ClientSessionInfo>(server, "/client/v1/session", { token: saved.token });
        this.#apply(info, saved.token, subject);
        return;
      } catch (error) {
        if (!(error instanceof ApiError) || (error.status !== 401 && error.status !== 403)) throw error;
      }
    }
    const session = await call<ClientSession>(server, "/client/v1/session", {
      method: "POST",
      json: { channel_key: channel, identity_token: identity ?? undefined, visitor_id: stored.visitor_id },
    });
    this.#apply(session, session.token, subject);
  }

  #apply(info: ClientSessionInfo, token: string, subject: string | null): void {
    const { channel } = this.#host.config();
    this.#token = token;
    this.#inbox = info.inbox;
    this.#contact = info.contact;
    const launcher = launcherOf(info.inbox);
    this.#host.setLauncher(launcher);
    if (!channel) return;
    const stored = readStored(channel);
    stored.session = { token, expires_at: info.expires_at, sub: subject };
    if (info.visitor_id) stored.visitor_id = info.visitor_id;
    else if (info.contact.identified) delete stored.visitor_id;
    stored.launcher = launcher;
    writeStored(channel, stored);
  }

  async #refreshSession(): Promise<void> {
    try {
      const info = await this.#authed<ClientSessionInfo>("/client/v1/session");
      this.#inbox = info.inbox;
      this.#contact = info.contact;
      this.#render();
    } catch {}
  }

  async #renew(): Promise<void> {
    await this.#session(true);
    this.#realtime?.stop();
    this.#realtime?.reset();
    this.#connect();
    await this.#reload();
  }

  async #authed<T>(path: string, request: Request = {}, retry = true): Promise<T> {
    const { server } = this.#host.config();
    if (!this.#token) await this.#begin();
    try {
      return await call<T>(server, path, { ...request, token: this.#token });
    } catch (error) {
      if (!retry || !(error instanceof ApiError) || error.status !== 401) throw error;
      await this.#renew();
      return this.#authed<T>(path, request, false);
    }
  }

  async #loadConversations(more = false): Promise<void> {
    const cursor = more && this.#conversationCursor ? `&cursor=${encodeURIComponent(this.#conversationCursor)}` : "";
    const page = await this.#authed<ClientConversationPage>(`/client/v1/conversations?limit=50${cursor}`);
    if (!more) this.#conversations.clear();
    for (const conversation of page.items) this.#conversations.set(conversation.id, conversation);
    this.#conversationCursor = page.next_cursor;
    this.#syncUnread();
  }

  async #reload(): Promise<void> {
    await this.#loadConversations();
    const current = this.#view?.kind === "thread" ? this.#view.id : null;
    for (const id of [...this.#threads.keys()]) if (id !== current) this.#threads.delete(id);
    if (current) {
      this.#threads.delete(current);
      await this.#loadMessages(current);
    }
    this.#render();
  }

  async #loadMessages(id: string, older = false): Promise<void> {
    let thread = this.#threads.get(id);
    if (!thread) {
      thread = { messages: [], complete: false, loading: false };
      this.#threads.set(id, thread);
    }
    if (thread.loading || (older && (thread.complete || !thread.cursor))) return;
    thread.loading = true;
    const before = this.#scroll.scrollHeight - this.#scroll.scrollTop;
    try {
      const cursor = older && thread.cursor ? `&cursor=${encodeURIComponent(thread.cursor)}` : "";
      const page = await this.#authed<ClientMessagePage>(`/client/v1/conversations/${id}/messages?order=desc&limit=30${cursor}`);
      const known = new Set(thread.messages.map((m) => m.id));
      thread.messages = [...thread.messages, ...page.items.filter((m) => !known.has(m.id))].sort(byCreated);
      thread.cursor = page.next_cursor;
      thread.complete = !page.next_cursor;
    } finally {
      thread.loading = false;
    }
    if (older) {
      this.#render();
      this.#scroll.scrollTop = this.#scroll.scrollHeight - before;
    }
  }

  #connect(): void {
    const { server } = this.#host.config();
    this.#realtime ??= new Realtime(server, {
      token: () => this.#token,
      event: (message) => this.#event(message),
      sessionEnded: () => void this.#renew().catch(() => this.#realtime?.nudge()),
      reconnecting: (attempt) => {
        this.#reconnecting = attempt > 0;
        this.#renderBanner();
        if (attempt === 2) void this.#authed("/client/v1/session").catch(() => undefined);
      },
    });
    this.#realtime.start();
  }

  #event(message: ClientRealtimeMessage): void {
    switch (message.type) {
      case "ready":
        this.#reconnecting = false;
        this.#renderBanner();
        if (this.#connectedBefore) void this.#refreshInbox();
        this.#connectedBefore = true;
        return;
      case "inbox.updated":
        this.#setInbox(message.data);
        break;
      case "resync_required":
        void this.#reload();
        return;
      case "presence":
        if (this.#inbox) this.#inbox = { ...this.#inbox, presence: message.data };
        break;
      case "conversation.created":
        if (!this.#conversations.has(message.data.id)) this.#conversations.set(message.data.id, message.data);
        if (this.#view?.kind === "thread" && this.#view.id === null && this.#conversations.size > 1) {
          this.#view = this.#pending.some((p) => p.conversation === null) ? this.#view : { kind: "list" };
        }
        this.#syncUnread();
        break;
      case "conversation.updated": {
        const conversation = this.#conversations.get(message.data.id);
        if (conversation) this.#conversations.set(conversation.id, { ...conversation, status: message.data.status });
        break;
      }
      case "message.created":
      case "message.updated":
        this.#upsertMessage(message.data, message.type === "message.created");
        break;
      case "read": {
        const conversation = this.#conversations.get(message.data.conversation_id);
        const previous = conversation?.last_read_by_member_at;
        if (conversation && (!previous || time(message.data.read_at) > time(previous))) {
          this.#conversations.set(conversation.id, { ...conversation, last_read_by_member_at: message.data.read_at });
        }
        break;
      }
      case "typing": {
        const id = message.data.conversation_id;
        const previous = this.#typing.get(id);
        if (previous) clearTimeout(previous.timer);
        if (message.data.typing) {
          const name = message.data.author.name ?? "";
          this.#typing.set(id, { name, timer: setTimeout(() => this.#typing.delete(id) && this.#render(), 8000) });
        } else this.#typing.delete(id);
        break;
      }
    }
    this.#render();
  }

  #upsertMessage(message: ClientMessage, created: boolean): void {
    const id = message.conversation_id;
    const thread = this.#threads.get(id);
    if (thread) {
      const index = thread.messages.findIndex((m) => m.id === message.id);
      if (index >= 0) thread.messages[index] = message;
      else thread.messages = [...thread.messages, message].sort(byCreated);
    }
    if (message.client_id) this.#pending = this.#pending.filter((p) => p.client_id !== message.client_id);
    const conversation = this.#conversations.get(id);
    if (!conversation) {
      void this.#authed<ClientConversation>(`/client/v1/conversations/${id}`)
        .then((c) => {
          this.#conversations.set(c.id, c);
          this.#syncUnread();
          this.#render();
        })
        .catch(() => undefined);
      return;
    }
    if (created) {
      const text = message.body.replace(/\s+/g, " ").trim();
      this.#conversations.set(id, {
        ...conversation,
        last_message: { id: message.id, author_type: message.author.type, text: text.length > 140 ? `${text.slice(0, 139)}…` : text, created_at: message.created_at },
        last_message_at: message.created_at,
        unread: conversation.unread || message.author.type !== "contact",
      });
      if (message.author.type !== "contact") {
        const typing = this.#typing.get(id);
        if (typing) clearTimeout(typing.timer);
        this.#typing.delete(id);
        this.#announce(message);
      }
    }
    this.#syncUnread();
    this.#markRead();
  }

  #announce(message: ClientMessage): void {
    this.#live.textContent = `${message.author.name || this.#inbox?.name || ""}: ${message.body.slice(0, 200)}`;
  }

  #syncUnread(): void {
    let count = 0;
    for (const conversation of this.#conversations.values()) if (conversation.unread) count++;
    this.#host.setUnread(count);
  }

  #visibleThread(): string | null {
    if (!this.#host.isOpen() || document.visibilityState !== "visible") return null;
    return this.#view?.kind === "thread" ? this.#view.id : null;
  }

  #markRead(): void {
    const id = this.#visibleThread();
    const conversation = id ? this.#conversations.get(id) : undefined;
    if (!id || !conversation?.unread) return;
    this.#conversations.set(id, { ...conversation, unread: false });
    this.#syncUnread();
    void this.#authed(`/client/v1/conversations/${id}/read`, { method: "POST", json: {} }).catch(() => {
      const current = this.#conversations.get(id);
      if (current) this.#conversations.set(id, { ...current, unread: true });
      this.#syncUnread();
    });
  }


  #addFiles(files: File[]): void {
    this.#files = [...this.#files, ...files].slice(0, maxFiles);
    this.#renderChips();
    this.#syncSend();
    this.#textarea.focus();
  }

  #autosize(): void {
    this.#textarea.style.blockSize = "";
    if (this.#textarea.value === "") return;
    this.#textarea.style.blockSize = "auto";
    this.#textarea.style.blockSize = `${Math.max(38, Math.min(this.#textarea.scrollHeight, 140))}px`;
  }

  #syncSend(): void {
    this.#sendButton.disabled = this.#textarea.value.trim() === "" && this.#files.length === 0;
  }

  #typingInput(): void {
    const id = this.#view?.kind === "thread" ? this.#view.id : null;
    if (!id || this.#textarea.value.trim() === "") {
      this.#stopTyping();
      return;
    }
    if (Date.now() - this.#typingSentAt > 3000) {
      this.#typingSentAt = Date.now();
      void this.#authed(`/client/v1/conversations/${id}/typing`, { method: "POST", json: { typing: true } }).catch(() => undefined);
    }
    if (this.#typingIdle) clearTimeout(this.#typingIdle);
    this.#typingIdle = setTimeout(() => this.#stopTyping(), 4000);
  }

  #stopTyping(): void {
    if (this.#typingIdle) clearTimeout(this.#typingIdle);
    this.#typingIdle = null;
    if (this.#typingSentAt === 0) return;
    this.#typingSentAt = 0;
    const id = this.#view?.kind === "thread" ? this.#view.id : null;
    if (id) void this.#authed(`/client/v1/conversations/${id}/typing`, { method: "POST", json: { typing: false } }).catch(() => undefined);
  }

  async #send(): Promise<void> {
    if (this.#phase !== "ready" || this.#view?.kind !== "thread") return;
    const body = this.#textarea.value.trim();
    const files = this.#files;
    if (!body && files.length === 0) return;
    if (this.#view.id === null && this.#pending.some((p) => p.conversation === null)) return;
    this.#stopTyping();
    this.#textarea.value = "";
    this.#files = [];
    this.#autosize();
    this.#renderChips();
    this.#syncSend();
    const pending: Pending = { client_id: randomId(), conversation: this.#view.id, body, files, state: "sending" };
    this.#pending.push(pending);
    this.#render();
    this.#scrollToEnd();
    await this.#deliver(pending);
  }

  async #deliver(pending: Pending): Promise<void> {
    pending.state = "sending";
    this.#render();
    let request: Request;
    if (pending.files.length > 0) {
      const form = new FormData();
      if (pending.body) form.append("body", pending.body);
      form.append("client_id", pending.client_id);
      for (const file of pending.files) form.append("files", file, file.name || "pasted-image.png");
      request = { method: "POST", form };
    } else request = { method: "POST", json: { body: pending.body, client_id: pending.client_id } };
    try {
      if (pending.conversation === null) {
        const created = await this.#authed<ClientConversationCreated>("/client/v1/conversations", request);
        const id = created.conversation.id;
        if (!this.#conversations.has(id)) this.#conversations.set(id, created.conversation);
        if (!this.#threads.has(id)) this.#threads.set(id, { messages: [], complete: true, loading: false });
        if (this.#view?.kind === "thread" && this.#view.id === null) this.#view = { kind: "thread", id };
        this.#upsertMessage(created.message, false);
      } else {
        const message = await this.#authed<ClientMessage>(`/client/v1/conversations/${pending.conversation}/messages`, request);
        this.#upsertMessage(message, true);
      }
      this.#pending = this.#pending.filter((p) => p !== pending);
    } catch (error) {
      const status = error instanceof ApiError ? error.status : 0;
      pending.state = status === 413 || status === 415 ? "refused" : "failed";
      pending.status = status;
    }
    this.#render();
    this.#scrollToEnd();
  }

  async #saveEmail(): Promise<void> {
    const email = this.#emailInput.value.trim();
    if (!email || !this.#emailInput.checkValidity()) {
      this.#emailError = this.#t(msg`Enter a valid e-mail address.`);
      this.#renderEmail();
      return;
    }
    try {
      this.#contact = await this.#authed<ClientContact>("/client/v1/contact/email", { method: "PUT", json: { email } });
      this.#emailSaved = true;
      this.#emailError = "";
    } catch {
      this.#emailError = this.#t(msg`Could not save your e-mail address. Try again.`);
    }
    this.#renderEmail();
  }

  #attachmentUrl(attachment: ClientAttachment): Promise<string> {
    const cached = this.#blobs.get(attachment.id);
    if (cached) return Promise.resolve(cached);
    let load = this.#blobLoads.get(attachment.id);
    if (!load) {
      load = this.#authed<Blob>(`/client/v1/attachments/${attachment.id}`, { blob: true }).then((blob) => {
        const url = URL.createObjectURL(blob);
        this.#blobs.set(attachment.id, url);
        return url;
      });
      load.catch(() => this.#blobLoads.delete(attachment.id));
      this.#blobLoads.set(attachment.id, load);
    }
    return load;
  }

  async #download(attachment: ClientAttachment): Promise<void> {
    const url = await this.#attachmentUrl(attachment);
    const a = el("a");
    a.href = url;
    a.download = attachment.filename;
    a.rel = "noopener";
    a.click();
  }


  #sorted(): ClientConversation[] {
    return [...this.#conversations.values()].sort(byActivity);
  }

  #nobodyAvailable(): boolean {
    const inbox = this.#inbox;
    if (!inbox) return false;
    return !inbox.open_now || inbox.mode === "async" || !inbox.presence?.available;
  }

  #scrollToEnd(): void {
    this.#atEnd = true;
    this.#scroll.scrollTop = this.#scroll.scrollHeight;
    requestAnimationFrame(() => {
      if (this.#atEnd) this.#scroll.scrollTop = this.#scroll.scrollHeight;
    });
  }

  #renderAll(): void {
    this.#textarea.placeholder = this.#t(msg`Write a message…`);
    this.#textarea.setAttribute("aria-label", this.#t(msg`Message`));
    this.#attachButton.setAttribute("aria-label", this.#t(msg`Attach files`));
    this.#attachButton.title = this.#t(msg`Attach files`);
    this.#sendButton.setAttribute("aria-label", this.#t(msg`Send`));
    this.#sendButton.title = this.#t(msg`Send`);
    this.#emailInput.placeholder = this.#t(msg`you@example.com`);
    this.#emailInput.setAttribute("aria-label", this.#t(msg`E-mail address`));
    this.#syncSend();
    this.#renderChips();
    this.#render();
  }

  #render(): void {
    const panel = this.#host.panel;
    if (panel.childElementCount === 0) {
      const style = el("style");
      style.textContent = styles;
      panel.append(style, this.#header, this.#banner, this.#body, this.#live);
      this.#header.id = "yuva-title";
      panel.setAttribute("aria-labelledby", "yuva-title");
    }
    this.#renderHeader();
    this.#renderBanner();
    if (this.#phase !== "ready") {
      this.#scrollThread = undefined;
      const center = el("div", "center");
      if (this.#phase === "loading") center.append(el("p", "hint", this.#t(msg`Loading…`)));
      else {
        center.append(el("p", "empty", this.#errorText()));
        const retry = el("button", "primary", this.#t(msg`Try again`));
        retry.type = "button";
        retry.addEventListener("click", () => {
          this.#starting = null;
          void this.#start();
        });
        center.append(retry);
      }
      this.#body.replaceChildren(center);
      return;
    }
    if (this.#view?.kind === "list") this.#renderList();
    else this.#renderThread();
  }

  #errorText(): string {
    switch (this.#error) {
      case "anonymous_not_allowed":
        return this.#t(msg`Sign in to chat with us.`);
      case "invalid_identity_token":
        return this.#t(msg`Your sign-in has expired. Reload the page and try again.`);
      case "rate_limited":
        return this.#t(msg`Too many attempts. Wait a moment and try again.`);
      case "network":
        return this.#t(msg`You seem to be offline.`);
      default:
        return this.#t(msg`Chat is not available right now.`);
    }
  }

  #renderHeader(): void {
    const { layout } = this.#host.config();
    const inbox = this.#inbox;
    const children: Node[] = [];
    const view = this.#view;
    if (this.#phase === "ready" && view?.kind === "thread" && (this.#conversations.size > 1 || (view.id === null && this.#conversations.size > 0))) {
      children.push(
        iconButton(icons.back, this.#t(msg`All conversations`), () => {
          this.#stopTyping();
          this.#view = { kind: "list" };
          this.#render();
        }),
      );
    }
    const heading = el("div", "heading");
    heading.append(el("h2", "title", inbox?.name || this.#t(msg`Messages`)));
    if (inbox) {
      const status = el("div", "status");
      const live = inbox.mode === "live" && inbox.open_now && inbox.presence?.available;
      if (live) {
        const avatars = el("div", "avatars");
        for (const member of inbox.presence?.members.slice(0, 3) ?? []) {
          const avatar = el("span", "avatar", member.initials || "•");
          avatar.title = member.name;
          avatars.append(avatar);
        }
        status.append(avatars, el("span", "dot"), el("span", "", this.#t(msg`Online`)));
      } else if (!inbox.open_now || inbox.mode === "live") {
        status.append(el("span", "dot away"), el("span", "", this.#t(msg`We're away right now`)));
      } else {
        status.append(el("span", "", this.#replyTime(inbox.expected_reply_minutes)));
      }
      heading.append(status);
    }
    children.push(heading);
    if (layout === "launcher") children.push(iconButton(icons.close, this.#t(msg`Close`), () => this.#host.close()));
    this.#header.replaceChildren(...children);
  }

  #replyTime(minutes: number | undefined): string {
    if (!minutes) return this.#t(msg`We'll reply as soon as we can`);
    const { locale } = this.#host.config();
    const [value, unit] = minutes < 60 ? [minutes, "minute"] : minutes < 1440 ? [Math.round(minutes / 60), "hour"] : [Math.round(minutes / 1440), "day"];
    const duration = new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "long" }).format(value);
    return this.#t(msg`Usually replies in ${duration}`);
  }

  #renderBanner(): void {
    this.#banner.hidden = !this.#reconnecting;
    this.#banner.textContent = this.#t(msg`Reconnecting…`);
  }

  #renderList(): void {
    this.#scrollThread = undefined;
    const { locale } = this.#host.config();
    const list = el("ul", "list");
    const today = new Date().toDateString();
    for (const conversation of this.#sorted()) {
      const button = el("button", conversation.unread ? "item unread" : "item");
      button.type = "button";
      const main = el("div", "item-main");
      const top = el("div", "item-top");
      top.append(el("span", "item-subject", conversation.subject || this.#inbox?.name || this.#t(msg`Conversation`)));
      const at = new Date(conversation.last_message_at ?? conversation.created_at);
      top.append(
        el(
          "span",
          "item-time",
          new Intl.DateTimeFormat(locale, at.toDateString() === today ? { timeStyle: "short" } : { month: "short", day: "numeric" }).format(at),
        ),
      );
      const preview = conversation.last_message
        ? `${conversation.last_message.author_type === "contact" ? this.#t(msg`You:`) + " " : ""}${conversation.last_message.text || this.#t(msg`Attachment`)}`
        : "";
      const previewLine = el("div", "item-preview", preview.replace(/\*\*|`/g, ""));
      previewLine.dir = "auto";
      main.append(top, previewLine);
      button.append(main);
      if (conversation.unread) button.append(el("span", "unread-dot"));
      button.addEventListener("click", () => void this.#openThread(conversation.id));
      const item = el("li");
      item.append(button);
      list.append(item);
    }
    if (this.#conversationCursor) {
      const more = el("button", "link", this.#t(msg`Show older conversations`));
      more.type = "button";
      more.style.margin = "8px auto";
      more.style.display = "block";
      more.addEventListener("click", () => void this.#loadConversations(true).then(() => this.#render()));
      list.append(more);
    }
    const start = el("button", "primary", this.#t(msg`New conversation`));
    start.type = "button";
    start.addEventListener("click", () => {
      this.#view = { kind: "thread", id: null };
      this.#render();
      this.#textarea.focus();
    });
    this.#body.replaceChildren(list, start);
  }

  async #openThread(id: string): Promise<void> {
    this.#view = { kind: "thread", id };
    this.#render();
    if (!this.#threads.get(id)?.complete && !this.#threads.get(id)?.messages.length) await this.#loadMessages(id).catch(() => undefined);
    this.#render();
    this.#scrollToEnd();
    this.#markRead();
  }

  #renderThread(): void {
    const view = this.#view;
    if (view?.kind !== "thread") return;
    const id = view.id;
    const sameThread = this.#scrollThread === id;
    const atEnd = this.#atEnd;
    const thread = id ? this.#threads.get(id) : undefined;
    const messages = thread?.messages ?? [];
    const pending = this.#pending.filter((p) => p.conversation === id);
    const nodes: Node[] = [];
    const greeting = this.#inbox?.chat.greeting;
    if (!thread || thread.complete) {
      if (greeting) {
        const bubble = el("div", "bubble greeting");
        bubble.append(richText(greeting));
        nodes.push(bubble);
      } else if (messages.length === 0 && pending.length === 0) {
        const center = el("div", "center");
        center.append(el("p", "empty", this.#t(msg`No conversations yet`)), el("p", "hint", this.#t(msg`Send us a message and we will reply here.`)));
        nodes.push(center);
      }
    } else if (thread.loading) nodes.push(el("p", "hint", this.#t(msg`Loading…`)));

    messages.forEach((message, i) => {
      const previous = messages[i - 1];
      const next = messages[i + 1] ?? (pending.length > 0 ? null : undefined);
      const mine = message.author.type === "contact";
      const starts = !previous || authorKey(previous) !== authorKey(message) || time(message.created_at) - time(previous.created_at) > groupGap;
      const ends =
        next === undefined ||
        (next === null ? !mine : authorKey(next) !== authorKey(message) || time(next.created_at) - time(message.created_at) > groupGap);
      if (!mine && starts && message.author.name) nodes.push(el("div", "author", message.author.name));
      const row = el("div", `row ${mine ? "mine" : "theirs"}${starts && i > 0 ? " group-gap" : ""}`);
      if (!mine) {
        const avatar = el("span", "avatar", message.author.initials || "");
        if (!ends) avatar.style.visibility = "hidden";
        row.append(avatar);
      }
      const bubble = el("div", "bubble");
      bubble.dir = "auto";
      bubble.title = new Date(message.created_at).toLocaleString(this.#host.config().locale);
      if (message.body.trim()) bubble.append(richText(message.body));
      if (message.attachments.length > 0) bubble.append(this.#renderAttachments(message.attachments));
      row.append(bubble);
      nodes.push(row);
      if (ends) {
        let meta = this.#formatTime(message.created_at);
        if (mine && i === messages.length - 1 && pending.length === 0) {
          const readAt = id ? this.#conversations.get(id)?.last_read_by_member_at : undefined;
          const seen = readAt && time(readAt) >= time(message.created_at);
          meta += ` · ${seen ? this.#t(msg`Seen`) : this.#t(msg`Sent`)}`;
        }
        const line = el("div", "meta", meta);
        if (!mine) line.style.alignSelf = "flex-start";
        if (!mine) line.style.marginInlineStart = "36px";
        nodes.push(line);
      }
    });

    pending.forEach((item, i) => {
      const row = el("div", `row mine${i === 0 && messages.length > 0 && messages[messages.length - 1]?.author.type !== "contact" ? " group-gap" : ""}`);
      const bubble = el("div", "bubble");
      bubble.dir = "auto";
      if (item.body) bubble.append(richText(item.body));
      if (item.files.length > 0) {
        const files = el("div", "files");
        for (const file of item.files) files.append(el("span", "file", file.name || "image"));
        bubble.append(files);
      }
      row.append(bubble);
      nodes.push(row);
      if (item.state === "failed") {
        const line = el("div", "meta failed", `${this.#t(msg`Not sent.`)} `);
        const retry = el("button", "link", this.#t(msg`Retry`));
        retry.type = "button";
        retry.addEventListener("click", () => void this.#deliver(item));
        line.append(retry);
        nodes.push(line);
      } else if (item.state === "refused") {
        const reason = item.status === 413 ? this.#t(msg`The file is too large.`) : this.#t(msg`This file type is not accepted.`);
        nodes.push(el("div", "meta failed", `${this.#t(msg`Not sent.`)} ${reason}`));
      } else if (i === pending.length - 1) nodes.push(el("div", "meta", this.#t(msg`Sending…`)));
    });

    const typing = id ? this.#typing.get(id) : undefined;
    if (typing) {
      const line = el("div", "typing");
      line.append(el("i"), el("i"), el("i"));
      const name = typing.name;
      line.append(el("span", "", name ? this.#t(msg`${name} is typing…`) : this.#t(msg`Typing…`)));
      nodes.push(line);
    }

    this.#scroll.replaceChildren(...nodes);
    this.#renderEmail();
    if (this.#body.firstChild !== this.#scroll) this.#body.replaceChildren(this.#scroll, this.#email, this.#composer);
    this.#scrollThread = id;
    if (!sameThread || atEnd) this.#scrollToEnd();
  }

  #renderAttachments(attachments: ClientAttachment[]): HTMLElement {
    const files = el("div", "files");
    for (const attachment of attachments) {
      if (isImage(attachment)) {
        const img = el("img");
        img.alt = attachment.filename;
        const cached = this.#blobs.get(attachment.id);
        if (cached) img.src = cached;
        else
          void this.#attachmentUrl(attachment)
            .then((url) => {
              img.src = url;
            })
            .catch(() => undefined);
        img.addEventListener("load", () => {
          if (this.#atEnd) this.#scroll.scrollTop = this.#scroll.scrollHeight;
        });
        img.addEventListener("click", () => void this.#download(attachment));
        files.append(img);
      } else {
        const button = el("button", "file");
        button.type = "button";
        button.innerHTML = icons.file;
        button.append(el("span", "", attachment.filename));
        button.addEventListener("click", () => void this.#download(attachment));
        files.append(button);
      }
    }
    return files;
  }

  #formatTime(value: string): string {
    return new Intl.DateTimeFormat(this.#host.config().locale, { timeStyle: "short" }).format(new Date(value));
  }

  #renderEmail(): void {
    const contact = this.#contact;
    const chat = this.#inbox?.chat;
    const id = this.#view?.kind === "thread" ? this.#view.id : null;
    const sent = id !== null || this.#pending.length > 0;
    const known = contact?.email || contact?.typed_email;
    const show = !!chat?.ask_email_offline && this.#nobodyAvailable() && sent && (!known || this.#emailSaved);
    this.#email.hidden = !show;
    if (!show) return;
    if (known && this.#emailSaved) {
      this.#email.replaceChildren(el("p", "", this.#t(msg`Thanks! We'll e-mail you at ${known} if you're gone when we reply.`)));
      return;
    }
    if (this.#email.querySelector("form")) {
      const error = this.#email.querySelector(".error");
      if (error) error.textContent = this.#emailError;
      else if (this.#emailError) this.#email.append(el("p", "error", this.#emailError));
      return;
    }
    const form = el("form");
    form.noValidate = true;
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      void this.#saveEmail();
    });
    const save = el("button", "", this.#t(msg`Save`));
    save.type = "submit";
    form.append(this.#emailInput, save);
    this.#email.replaceChildren(el("p", "", this.#t(msg`Nobody is available right now. Leave your e-mail and we'll send the reply there too.`)), form);
    if (this.#emailError) this.#email.append(el("p", "error", this.#emailError));
  }

  #renderChips(): void {
    this.#chips.hidden = this.#files.length === 0;
    this.#chips.replaceChildren(
      ...this.#files.map((file, index) => {
        const chip = el("span", "chip");
        chip.append(
          el("span", "", file.name || "image"),
          iconButton(icons.close, this.#t(msg`Remove ${file.name}`), () => {
            this.#files = this.#files.filter((_, i) => i !== index);
            this.#renderChips();
            this.#syncSend();
          }),
        );
        return chip;
      }),
    );
  }
}

export function mount(host: PanelHost): PanelController {
  return new Chat(host);
}
