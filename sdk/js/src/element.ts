import type { Messages } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import { catalogFor, directionOf, resolveLocale, translatePlain } from "./i18n";
import type { components } from "./schema.gen";
import {
  contrastOn,
  launcherOf,
  readStored,
  writeStored,
  type ChatConfig,
  type IdentityTokenSource,
  type ClientConversation,
  type Layout,
  type LauncherStyle,
  type PanelController,
  type PanelModule,
  type Rating,
} from "./types";

const styles = `
:host {
  --yuva-accent: #2563eb;
  --yuva-on-accent: #ffffff;
  --yuva-bg: #ffffff;
  --yuva-fg: #111827;
  --yuva-muted: #6b7280;
  --yuva-border: #e5e7eb;
  --yuva-soft: #f3f4f6;
  --yuva-danger: #dc2626;
  --yuva-shadow: 0 12px 40px rgb(15 23 42 / 0.18);
  color-scheme: light;
  position: fixed;
  bottom: 20px;
  right: 20px;
  z-index: 2147483000;
  font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  font-size: 15px;
  line-height: 1.45;
  color: var(--yuva-fg);
}
:host([position="left"]) { right: auto; left: 20px; }
@media (prefers-color-scheme: dark) {
  :host {
    --yuva-bg: #1c1f24;
    --yuva-fg: #f3f4f6;
    --yuva-muted: #9ca3af;
    --yuva-border: #30343b;
    --yuva-soft: #2a2e35;
    --yuva-danger: #f87171;
    --yuva-shadow: 0 12px 40px rgb(0 0 0 / 0.5);
    color-scheme: dark;
  }
}
:host([layout="embedded"]) {
  position: relative;
  inset: auto;
  z-index: auto;
  display: block;
  block-size: 100%;
}
.launcher {
  position: relative;
  display: grid;
  place-items: center;
  inline-size: 56px;
  block-size: 56px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: var(--yuva-accent);
  color: var(--yuva-on-accent);
  box-shadow: var(--yuva-shadow);
  cursor: pointer;
}
.launcher:focus-visible { outline: 3px solid var(--yuva-accent); outline-offset: 3px; }
.badge {
  position: absolute;
  top: -4px;
  right: -4px;
  min-inline-size: 20px;
  block-size: 20px;
  padding-inline: 5px;
  box-sizing: border-box;
  border-radius: 10px;
  background: var(--yuva-danger);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  line-height: 20px;
  text-align: center;
}
.badge[hidden] { display: none; }
.panel {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-sizing: border-box;
  background: var(--yuva-bg);
  border: 1px solid var(--yuva-border);
  border-radius: 16px;
}
.panel[hidden] { display: none; }
:host(:not([layout="embedded"])) .panel {
  position: absolute;
  bottom: 72px;
  right: 0;
  inline-size: min(380px, calc(100vw - 32px));
  block-size: min(600px, calc(100vh - 112px));
  box-shadow: var(--yuva-shadow);
}
:host([position="left"]:not([layout="embedded"])) .panel { right: auto; left: 0; }
@media (max-width: 480px) {
  :host(:not([layout="embedded"])) .panel {
    position: fixed;
    inset: 0;
    inline-size: auto;
    block-size: auto;
    border: 0;
    border-radius: 0;
  }
  :host([open-panel]:not([layout="embedded"])) .launcher { display: none; }
}
:host([layout="embedded"]) .panel {
  inline-size: 100%;
  block-size: 100%;
  min-block-size: 320px;
}
`;

const chatIcon = `<svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" aria-hidden="true"><path d="M4 5h16v11H9l-5 4z"/></svg>`;
const closeIcon = `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>`;

const scriptSrc = document.currentScript instanceof HTMLScriptElement ? document.currentScript.src : "";

export class YuvaChatElement extends HTMLElement {
  static observedAttributes = ["channel", "server", "layout", "locale", "dir", "identity-token", "open"];
  static loadPanel: () => Promise<PanelModule> = () =>
    import(/* @vite-ignore */ new URL("yuva-chat.js", scriptSrc || location.href).href);

  #root: ShadowRoot;
  #launcher: HTMLButtonElement | null = null;
  #badge: HTMLElement | null = null;
  #panel: HTMLElement;
  #controller: Promise<PanelController> | null = null;
  #messages: Messages = {};
  #open = false;
  #unread = 0;
  #identity: IdentityTokenSource | null = null;
  #refused = false;

  constructor() {
    super();
    this.#root = this.attachShadow({ mode: "open" });
    this.#panel = document.createElement("section");
    this.#panel.className = "panel";
    this.#panel.setAttribute("role", "dialog");
    this.#panel.hidden = true;
    this.#panel.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && this.layout === "launcher") this.close();
    });
  }

  get layout(): Layout {
    return this.getAttribute("layout") === "embedded" ? "embedded" : "launcher";
  }

  get channel(): string | null {
    return this.getAttribute("channel");
  }

  get server(): string {
    const base = this.getAttribute("server") || (scriptSrc ? new URL(scriptSrc).origin : location.origin);
    return base.replace(/\/+$/, "");
  }

  get locale(): string {
    return resolveLocale(this.getAttribute("locale"), navigator.language);
  }

  get isOpen(): boolean {
    return this.#open;
  }

  get unread(): number {
    return this.#unread;
  }

  setIdentityToken(source: IdentityTokenSource | null): void {
    this.#identity = source;
    if (this.#controller) void this.#controller.then((controller) => controller.identityChanged());
  }

  async signOut(): Promise<void> {
    this.#identity = null;
    this.removeAttribute("identity-token");
    if (this.#controller) await (await this.#controller).signOut();
  }

  async rate(conversationId: string, rating: Rating, comment?: string): Promise<ClientConversation> {
    return (await this.#ensureController()).rate(conversationId, rating, comment);
  }

  connectedCallback(): void {
    this.#render();
    const channel = this.channel;
    if (this.layout === "embedded" || this.hasAttribute("open") || !channel) return;
    const stored = readStored(channel);
    if (stored.launcher) this.#applyLauncher(stored.launcher);
    const idle = () => {
      if (!this.isConnected || this.#controller || this.channel !== channel) return;
      if (stored.session) void this.#ensureController();
      else this.#styleLauncher(channel);
    };
    if ("requestIdleCallback" in window) requestIdleCallback(idle, { timeout: 3000 });
    else setTimeout(idle, 1000);
  }

  #styleLauncher(channel: string): void {
    fetch(`${this.server}/client/v1/channels/${encodeURIComponent(channel)}`, { credentials: "omit" })
      .then((response) => {
        if (response.status === 403 || response.status === 404) this.#refuse();
        return response.ok ? (response.json() as Promise<components["schemas"]["ClientInbox"]>) : null;
      })
      .then((inbox) => {
        if (!inbox || this.channel !== channel || this.#controller) return;
        const launcher = launcherOf(inbox);
        this.#applyLauncher(launcher);
        writeStored(channel, { ...readStored(channel), launcher });
      })
      .catch(() => undefined);
  }

  disconnectedCallback(): void {
    const controller = this.#controller;
    this.#controller = null;
    if (controller) void controller.then((c) => c.destroy());
  }

  attributeChangedCallback(name: string, previous: string | null, value: string | null): void {
    if (!this.isConnected || previous === value) return;
    if (name === "open") {
      void (value === null ? this.close() : this.open());
      return;
    }
    if (name === "identity-token") {
      if (this.#controller) void this.#controller.then((controller) => controller.identityChanged());
      return;
    }
    if (name === "channel" || name === "server") {
      this.disconnectedCallback();
      this.#refused = false;
      this.#render();
      return;
    }
    this.#render();
    if (this.#controller) void this.#controller.then((controller) => controller.update());
  }

  async open(): Promise<void> {
    if (this.#open || this.#refused) return;
    this.#open = true;
    this.toggleAttribute("open-panel", true);
    this.#syncLauncher();
    const controller = await this.#ensureController();
    if (!this.#open) return;
    this.#panel.hidden = false;
    controller.opened();
  }

  close(): void {
    if (!this.#open || this.layout === "embedded") return;
    this.#open = false;
    this.toggleAttribute("open-panel", false);
    this.#panel.hidden = true;
    this.#syncLauncher();
    this.#launcher?.focus();
    if (this.#controller) void this.#controller.then((controller) => controller.closed());
  }

  toggle(): Promise<void> | void {
    return this.#open ? this.close() : this.open();
  }

  #config(): ChatConfig {
    return { channel: this.channel, server: this.server, layout: this.layout, locale: this.locale };
  }

  #ensureController(): Promise<PanelController> {
    this.#controller ??= YuvaChatElement.loadPanel().then(({ mount }) =>
      mount({
        panel: this.#panel,
        config: () => this.#config(),
        identityToken: async () => (this.#identity ? ((await this.#identity()) ?? null) : this.getAttribute("identity-token")),
        isOpen: () => this.#open,
        close: () => this.close(),
        setUnread: (count) => {
          this.#unread = count;
          this.#syncLauncher();
          this.dispatchEvent(new CustomEvent("yuva-unread", { detail: { count } }));
        },
        setLauncher: (style) => this.#applyLauncher(style),
        refused: () => this.#refuse(),
      }),
    );
    return this.#controller;
  }

  #refuse(): void {
    if (this.#refused) return;
    this.#refused = true;
    console.warn(`Yuva: channel ${this.channel} is unknown or does not allow ${location.origin}`);
    this.#open = false;
    this.toggleAttribute("open-panel", false);
    this.disconnectedCallback();
    this.#render();
  }

  #applyLauncher(style: LauncherStyle): void {
    this.setAttribute("position", style.position === "left" ? "left" : "right");
    if (style.color && /^#[0-9a-f]{6}$/i.test(style.color)) {
      this.style.setProperty("--yuva-accent", style.color);
      this.style.setProperty("--yuva-on-accent", contrastOn(style.color));
    }
  }

  #render(): void {
    const locale = this.locale;
    this.#messages = catalogFor(locale);
    this.setAttribute("lang", locale);
    const style = document.createElement("style");
    style.textContent = styles;
    this.#panel.dir = this.getAttribute("dir") ?? directionOf(locale);
    this.#panel.setAttribute("aria-modal", "false");
    if (this.#refused) {
      this.#launcher = null;
      this.#badge = null;
      this.#root.replaceChildren();
      return;
    }

    if (this.layout === "embedded") {
      this.#launcher = null;
      this.#badge = null;
      this.#root.replaceChildren(style, this.#panel);
      this.#open = false;
      void this.open();
      return;
    }

    const launcher = document.createElement("button");
    launcher.type = "button";
    launcher.className = "launcher";
    launcher.addEventListener("click", () => void this.toggle());
    this.#launcher = launcher;
    this.#badge = document.createElement("span");
    this.#badge.className = "badge";
    this.#badge.setAttribute("aria-hidden", "true");
    this.#root.replaceChildren(style, this.#panel, launcher);
    this.#syncLauncher();
    if (this.hasAttribute("open") && !this.#open) void this.open();
  }

  #syncLauncher(): void {
    const launcher = this.#launcher;
    const badge = this.#badge;
    if (!launcher || !badge) return;
    const count = this.#unread;
    launcher.setAttribute("aria-expanded", String(this.#open));
    launcher.setAttribute(
      "aria-label",
      this.#open
        ? translatePlain(this.#messages, msg`Close chat`)
        : count > 0
          ? translatePlain(this.#messages, msg`Open chat, ${count} unread`)
          : translatePlain(this.#messages, msg`Open chat`),
    );
    launcher.innerHTML = this.#open ? closeIcon : chatIcon;
    badge.textContent = count > 9 ? "9+" : String(count);
    badge.hidden = this.#open || count === 0;
    launcher.append(badge);
  }
}

export function define(): void {
  if (!customElements.get("yuva-chat")) customElements.define("yuva-chat", YuvaChatElement);
}
