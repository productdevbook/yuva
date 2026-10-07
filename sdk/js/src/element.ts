import type { Messages } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import { catalogFor, directionOf, resolveLocale, translatePlain } from "./i18n";
import type { Layout, PanelModule } from "./types";

const styles = `
:host {
  --yuva-accent: #2563eb;
  --yuva-on-accent: #ffffff;
  --yuva-bg: #ffffff;
  --yuva-fg: #111827;
  --yuva-muted: #6b7280;
  --yuva-border: #e5e7eb;
  --yuva-shadow: 0 12px 40px rgb(15 23 42 / 0.18);
  color-scheme: light;
  position: fixed;
  inset-block-end: 20px;
  inset-inline-end: 20px;
  z-index: 2147483000;
  font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  font-size: 15px;
  line-height: 1.45;
  color: var(--yuva-fg);
}
@media (prefers-color-scheme: dark) {
  :host {
    --yuva-accent: #60a5fa;
    --yuva-on-accent: #0b1220;
    --yuva-bg: #1c1f24;
    --yuva-fg: #f3f4f6;
    --yuva-muted: #9ca3af;
    --yuva-border: #30343b;
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
  inset-block-end: 72px;
  inset-inline-end: 0;
  inline-size: min(380px, calc(100vw - 32px));
  block-size: min(560px, calc(100vh - 112px));
  box-shadow: var(--yuva-shadow);
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
  static observedAttributes = ["inbox", "layout", "locale"];
  static loadPanel: () => Promise<PanelModule> = () =>
    import(/* @vite-ignore */ new URL("yuva-chat.js", scriptSrc || location.href).href);

  #root: ShadowRoot;
  #launcher: HTMLButtonElement | null = null;
  #panel: HTMLElement;
  #panelModule: Promise<PanelModule> | null = null;
  #messages: Messages = {};
  #open = false;

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

  get inbox(): string | null {
    return this.getAttribute("inbox");
  }

  get locale(): string {
    return resolveLocale(this.getAttribute("locale"), navigator.language);
  }

  get isOpen(): boolean {
    return this.#open;
  }

  connectedCallback(): void {
    this.#render();
  }

  attributeChangedCallback(): void {
    if (this.isConnected) this.#render();
  }

  async open(): Promise<void> {
    if (this.#open) return;
    this.#open = true;
    this.#syncLauncher();
    await this.#renderPanel();
    if (!this.#open) return;
    this.#panel.hidden = false;
    if (this.layout === "launcher") this.#panel.querySelector<HTMLElement>("button")?.focus();
  }

  close(): void {
    if (!this.#open || this.layout === "embedded") return;
    this.#open = false;
    this.#panel.hidden = true;
    this.#syncLauncher();
    this.#launcher?.focus();
  }

  toggle(): Promise<void> | void {
    return this.#open ? this.close() : this.open();
  }

  #render(): void {
    const locale = this.locale;
    this.#messages = catalogFor(locale);
    this.#root.host.setAttribute("lang", locale);
    const style = document.createElement("style");
    style.textContent = styles;
    this.#panel.dir = this.getAttribute("dir") ?? directionOf(locale);
    this.#panel.setAttribute("aria-modal", "false");

    if (this.layout === "embedded") {
      this.#launcher = null;
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
    this.#root.replaceChildren(style, this.#panel, launcher);
    this.#syncLauncher();
    if (this.#open) void this.#renderPanel();
  }

  #syncLauncher(): void {
    const launcher = this.#launcher;
    if (!launcher) return;
    launcher.setAttribute("aria-expanded", String(this.#open));
    launcher.setAttribute("aria-label", translatePlain(this.#messages, this.#open ? msg`Close chat` : msg`Open chat`));
    launcher.innerHTML = this.#open ? closeIcon : chatIcon;
  }

  async #renderPanel(): Promise<void> {
    this.#panelModule ??= YuvaChatElement.loadPanel();
    const { renderPanel } = await this.#panelModule;
    renderPanel(this.#panel, {
      locale: this.locale,
      messages: this.#messages,
      layout: this.layout,
      inbox: this.inbox,
      onClose: () => this.close(),
    });
  }
}

export function define(): void {
  if (!customElements.get("yuva-chat")) customElements.define("yuva-chat", YuvaChatElement);
}
