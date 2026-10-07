import { setupI18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import type { PanelContext } from "./types";

const styles = `
.header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-block: 14px;
  padding-inline: 18px 10px;
  background: var(--yuva-accent);
  color: var(--yuva-on-accent);
}
.title {
  flex: 1;
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.close {
  display: grid;
  place-items: center;
  inline-size: 32px;
  block-size: 32px;
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.close:hover { background: rgb(255 255 255 / 0.15); }
.close:focus-visible { outline: 2px solid currentColor; outline-offset: -4px; }
.body {
  flex: 1;
  display: grid;
  place-content: center;
  gap: 6px;
  padding: 24px;
  text-align: center;
}
.empty { margin: 0; font-weight: 600; }
.hint { margin: 0; color: var(--yuva-muted); font-size: 14px; }
`;

const closeIcon = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>`;

export function renderPanel(target: HTMLElement, { locale, messages, layout, onClose }: PanelContext): void {
  const i18n = setupI18n({ locale, messages: { [locale]: messages } });
  const style = document.createElement("style");
  style.textContent = styles;

  const header = document.createElement("header");
  header.className = "header";
  const title = document.createElement("h2");
  title.className = "title";
  title.id = "yuva-title";
  title.textContent = i18n._(msg`Messages`);
  header.append(title);

  if (layout === "launcher") {
    const close = document.createElement("button");
    close.type = "button";
    close.className = "close";
    close.setAttribute("aria-label", i18n._(msg`Close`));
    close.innerHTML = closeIcon;
    close.addEventListener("click", onClose);
    header.append(close);
  }

  const body = document.createElement("div");
  body.className = "body";
  const empty = document.createElement("p");
  empty.className = "empty";
  empty.textContent = i18n._(msg`No conversations yet`);
  const hint = document.createElement("p");
  hint.className = "hint";
  hint.textContent = i18n._(msg`Send us a message and we will reply here.`);
  body.append(empty, hint);

  target.setAttribute("aria-labelledby", title.id);
  target.replaceChildren(style, header, body);
}
