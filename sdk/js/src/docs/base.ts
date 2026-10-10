import type { MessageDescriptor, Messages } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import { ApiError, type ClientInbox } from "../client/api";
import { createYuvaClient, type IdentityTokenSource, type YuvaClient } from "../client/client";
import { directionOf, resolveLocale, translatePlain } from "../i18n";
import { messages as en } from "../locales/en/docs";
import { messages as tr } from "../locales/tr/docs";
import { contrastOn, launcherOf, readStored, writeStored } from "../types";
import { followPage, pageOf } from "./page";

const catalogs: Record<string, Messages> = { en, tr };

const settings = new Map<string, Promise<ClientInbox>>();

const Base = (typeof HTMLElement === "undefined" ? class {} : HTMLElement) as typeof HTMLElement;

const scriptSrc = typeof document !== "undefined" && document.currentScript instanceof HTMLScriptElement ? document.currentScript.src : "";

export const baseStyles = `
:host {
  --yuva-accent: #2563eb;
  --yuva-on-accent: #ffffff;
  --yuva-border: rgb(127 127 127 / 0.35);
  --yuva-soft: rgb(127 127 127 / 0.1);
  --yuva-danger: #dc2626;
  display: block;
  color: inherit;
  font: inherit;
  line-height: 1.5;
}
:host([hidden]) { display: none; }
@media (prefers-color-scheme: dark) {
  :host { --yuva-danger: #f87171; }
}
[hidden] { display: none !important; }
* { box-sizing: border-box; }
p { margin: 0; }
.muted { opacity: 0.72; font-size: 0.9em; }
.error { color: var(--yuva-danger); font-size: 0.9em; }
form { display: flex; flex-direction: column; gap: 8px; }
textarea, input {
  inline-size: 100%;
  padding: 8px 10px;
  border: 1px solid var(--yuva-border);
  border-radius: 8px;
  background: transparent;
  color: inherit;
  font: inherit;
}
textarea { resize: vertical; min-block-size: 72px; }
textarea:focus-visible, input:focus-visible { outline: 2px solid var(--yuva-accent); outline-offset: -1px; border-color: transparent; }
.actions { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; justify-content: flex-end; }
.send {
  padding: 8px 14px;
  border: 0;
  border-radius: 8px;
  background: var(--yuva-accent);
  color: var(--yuva-on-accent);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
.send:disabled { opacity: 0.6; cursor: default; }
.send:focus-visible, .link:focus-visible { outline: 2px solid var(--yuva-accent); outline-offset: 2px; }
.link { padding: 0; border: 0; background: none; color: inherit; font: inherit; text-decoration: underline; cursor: pointer; opacity: 0.8; }
@media (max-width: 480px) { textarea, input { font-size: 16px; } }
`;

export function el<K extends keyof HTMLElementTagNameMap>(tag: K, className?: string, text?: string): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

export abstract class YuvaDocsElement extends Base {
  static observedAttributes = ["channel", "server", "locale", "dir", "page", "page-title", "identity-token"];

  protected root: ShadowRoot;
  protected inbox: ClientInbox | null = null;
  protected pageUrl = "";
  protected refused = false;
  protected direction: "ltr" | "rtl" = "ltr";
  #messages: Messages = en;
  #client: YuvaClient | null = null;
  #identity: IdentityTokenSource | null = null;
  #unfollow: (() => void) | null = null;
  #settingsFor = "";
  #connected = false;
  #accentColor = "";

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
  }

  get channel(): string | null {
    return this.getAttribute("channel");
  }

  set channel(value: string | null) {
    this.#attribute("channel", value);
  }

  get server(): string {
    const base = this.getAttribute("server") || (scriptSrc ? new URL(scriptSrc).origin : location.origin);
    return base.replace(/\/+$/, "");
  }

  set server(value: string | null) {
    this.#attribute("server", value);
  }

  get locale(): string {
    return resolveLocale(this.getAttribute("locale"), navigator.language);
  }

  set locale(value: string | null) {
    this.#attribute("locale", value);
  }

  get page(): string {
    return this.pageUrl || pageOf(this.getAttribute("page"));
  }

  set page(value: string | null) {
    this.#attribute("page", value);
  }

  get pageTitle(): string {
    return this.getAttribute("page-title") || document.title;
  }

  set pageTitle(value: string | null) {
    this.#attribute("page-title", value);
  }

  #attribute(name: string, value: string | null): void {
    if (value === null || value === undefined || value === "") this.removeAttribute(name);
    else this.setAttribute(name, value);
  }

  setIdentityToken(source: IdentityTokenSource | null): void {
    this.#identity = source;
    this.#client?.setIdentityToken(() => this.#identityToken());
    if (this.#connected && !this.refused) this.render();
  }

  connectedCallback(): void {
    this.#connected = true;
    this.#unfollow ??= followPage(() => this.#syncPage());
    this.pageUrl = pageOf(this.getAttribute("page"));
    this.#applyLocale();
    this.#loadSettings();
    this.render();
    this.pageChanged();
  }

  disconnectedCallback(): void {
    this.#connected = false;
    this.#unfollow?.();
    this.#unfollow = null;
  }

  attributeChangedCallback(name: string, previous: string | null, value: string | null): void {
    if (!this.#connected || previous === value) return;
    if (name === "channel" || name === "server") {
      this.#client?.reset();
      this.#client = null;
      this.refused = false;
      this.inbox = null;
      this.#settingsFor = "";
      this.#loadSettings();
      this.reset();
      this.render();
      this.pageChanged();
      return;
    }
    if (name === "page") return this.#syncPage();
    if (name === "identity-token") this.#client?.setIdentityToken(() => this.#identityToken());
    if (name === "locale" || name === "dir") this.#applyLocale();
    this.render();
  }

  protected abstract render(): void;
  protected abstract pageChanged(): void;
  protected abstract reset(): void;

  protected t(descriptor: MessageDescriptor): string {
    return translatePlain(this.#messages, descriptor, en);
  }

  protected client(): YuvaClient {
    this.#client ??= createYuvaClient({ server: this.server, channel: this.channel ?? "", identityToken: () => this.#identityToken() });
    return this.#client;
  }

  protected get identified(): boolean {
    return !!this.#identity || this.hasAttribute("identity-token");
  }

  protected get canWrite(): boolean {
    return this.identified || this.inbox?.chat.allow_anonymous !== false;
  }

  protected emit(type: string, detail: unknown): void {
    this.dispatchEvent(new CustomEvent(type, { detail, bubbles: true, composed: true }));
  }

  protected refusal(error: unknown, channelRequest = true): boolean {
    const refused =
      error instanceof ApiError && (error.code === "origin_not_allowed" || error.code === "page_origin" || (channelRequest && error.status === 404));
    if (refused && !this.refused) {
      this.refused = true;
      console.warn(`Yuva: channel ${this.channel} is unknown or does not allow ${this.pageUrl}`);
      this.root.replaceChildren();
    }
    return refused;
  }

  protected errorText(error: unknown): string {
    const code = error instanceof ApiError ? error.code : "";
    if (code === "rate_limited") return this.t(msg`Too many attempts. Wait a moment and try again.`);
    if (code === "network") return this.t(msg`You seem to be offline.`);
    if (code === "anonymous_not_allowed" || code === "invalid_identity_token") return this.t(msg`Sign in to write to us.`);
    return this.t(msg`Could not send. Try again.`);
  }

  protected frame(css: string, ...children: Node[]): void {
    const style = el("style");
    style.textContent = baseStyles + css;
    const wrapper = el("div", "frame");
    wrapper.dir = this.direction;
    wrapper.lang = this.locale;
    if (this.#accentColor) {
      wrapper.style.setProperty("--yuva-accent", this.#accentColor);
      wrapper.style.setProperty("--yuva-on-accent", contrastOn(this.#accentColor));
    }
    wrapper.append(...children);
    let focused: Element | null = null;
    try {
      focused = this.root.activeElement;
    } catch {}
    this.root.replaceChildren(style, wrapper);
    if (focused instanceof HTMLElement && focused.isConnected) focused.focus({ preventScroll: true });
  }

  async #identityToken(): Promise<string | null> {
    return this.#identity ? ((await this.#identity()) ?? null) : this.getAttribute("identity-token");
  }

  #syncPage(): void {
    if (!this.#connected) return;
    const page = pageOf(this.getAttribute("page"));
    if (page === this.pageUrl) return;
    this.pageUrl = page;
    this.reset();
    this.render();
    this.pageChanged();
  }

  #applyLocale(): void {
    const locale = this.locale;
    this.#messages = catalogs[locale] ?? en;
    this.direction = this.getAttribute("dir") === "rtl" ? "rtl" : this.getAttribute("dir") === "ltr" ? "ltr" : directionOf(locale);
  }

  #loadSettings(): void {
    const channel = this.channel;
    const key = `${this.server} ${channel}`;
    if (!channel || this.#settingsFor === key) return;
    this.#settingsFor = key;
    const stored = readStored(channel).launcher;
    if (stored?.color) this.#accent(stored.color);
    let request = settings.get(key);
    if (!request) {
      request = this.client().channelSettings();
      settings.set(key, request);
      request.catch(() => settings.delete(key));
    }
    request
      .then((inbox) => {
        if (this.channel !== channel) return;
        this.inbox = inbox;
        const launcher = launcherOf(inbox);
        if (launcher.color) this.#accent(launcher.color);
        writeStored(channel, { ...readStored(channel), launcher });
        if (!this.refused) this.render();
      })
      .catch((error: unknown) => {
        if (this.channel === channel) this.refusal(error);
      });
  }

  #accent(color: string): void {
    if (!/^#[0-9a-f]{6}$/i.test(color)) return;
    this.#accentColor = color;
  }
}
