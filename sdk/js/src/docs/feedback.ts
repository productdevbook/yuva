import { msg } from "@lingui/core/macro";
import type { PageRating } from "../client/api";
import { YuvaDocsElement, el } from "./base";

type Phase = "ask" | "rating" | "form" | "sending" | "thanks";

export interface PageRatingDetail {
  page: string;
  rating: PageRating;
  previous: PageRating | null;
}

export interface PageFeedbackDetail {
  page: string;
  rating: PageRating | null;
  conversation_id: string;
}

const maxRemembered = 500;

const thumb = (down: boolean) =>
  `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"${down ? ' style="transform:scaleY(-1)"' : ""}><path d="M7 10v11H3V10zM7 10l4-8a3 3 0 013 3v4h6a2 2 0 012 2.3l-1.4 8A2 2 0 0118.6 21H7"/></svg>`;

const styles = `
.frame { display: flex; flex-direction: column; gap: 12px; }
.ask { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; }
.question { font-weight: 600; }
.choices { display: flex; gap: 8px; }
.choice {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px solid var(--yuva-border);
  border-radius: 999px;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.choice:hover:not(:disabled), .choice[aria-pressed="true"] { border-color: var(--yuva-accent); box-shadow: inset 0 0 0 1px var(--yuva-accent); }
.choice[aria-pressed="true"] { background: var(--yuva-soft); }
.choice:disabled { cursor: default; }
.choice:focus-visible { outline: 2px solid var(--yuva-accent); outline-offset: 2px; }
`;

function storageKey(channel: string): string {
  return `yuva:${channel}:page-ratings`;
}

function remembered(channel: string): Record<string, PageRating> {
  try {
    const raw = localStorage.getItem(storageKey(channel));
    return raw ? (JSON.parse(raw) as Record<string, PageRating>) : {};
  } catch {
    return {};
  }
}

function remember(channel: string, page: string, rating: PageRating): void {
  try {
    const ratings = remembered(channel);
    delete ratings[page];
    ratings[page] = rating;
    const pages = Object.keys(ratings);
    for (const old of pages.slice(0, Math.max(0, pages.length - maxRemembered))) delete ratings[old];
    localStorage.setItem(storageKey(channel), JSON.stringify(ratings));
  } catch {}
}

export class YuvaPageFeedbackElement extends YuvaDocsElement {
  #phase: Phase = "ask";
  #rating: PageRating | undefined;
  #error = "";
  #comment = el("textarea");
  #email = el("input");

  constructor() {
    super();
    this.#comment.rows = 3;
    this.#comment.maxLength = 5000;
    this.#comment.dir = "auto";
    this.#comment.addEventListener("input", () => this.#syncSend());
    this.#email.type = "email";
    this.#email.autocomplete = "email";
  }

  get rating(): PageRating | null {
    return this.#rating ?? null;
  }

  protected reset(): void {
    this.#phase = "ask";
    this.#error = "";
    this.#comment.value = "";
    this.#rating = this.channel ? remembered(this.channel)[this.pageUrl] : undefined;
  }

  protected pageChanged(): void {
    if (this.#phase === "ask" && !this.#error) this.#rating = this.channel ? remembered(this.channel)[this.pageUrl] : undefined;
    this.render();
  }

  async rate(rating: PageRating): Promise<void> {
    const channel = this.channel;
    if (!channel || this.refused || this.#phase === "rating" || this.#phase === "sending") return;
    const page = this.pageUrl;
    const previous = remembered(channel)[page];
    this.#error = "";
    if (previous === rating) {
      this.#rating = rating;
      this.#phase = this.canWrite ? "form" : "thanks";
      this.render();
      this.#comment.focus({ preventScroll: true });
      return;
    }
    this.#phase = "rating";
    this.#rating = rating;
    this.render();
    try {
      await this.client().ratePage({ page, title: this.pageTitle || undefined, rating, previous });
    } catch (error) {
      if (this.pageUrl !== page || this.refusal(error)) return;
      this.#rating = previous;
      this.#phase = "ask";
      this.#error = this.errorText(error);
      this.render();
      return;
    }
    remember(channel, page, rating);
    this.emit("yuva-rating", { page, rating, previous: previous ?? null } satisfies PageRatingDetail);
    if (this.pageUrl !== page) return;
    this.#phase = this.canWrite ? "form" : "thanks";
    this.render();
    this.#comment.focus({ preventScroll: true });
  }

  async #send(): Promise<void> {
    const body = this.#comment.value.trim();
    const email = this.#email.value.trim();
    const rating = this.#rating;
    if (!body || this.#phase !== "form") return;
    if (email && !this.#email.checkValidity()) {
      this.#error = this.t(msg`Enter a valid e-mail address.`);
      this.render();
      return;
    }
    const page = this.pageUrl;
    this.#phase = "sending";
    this.#error = "";
    this.render();
    try {
      const { conversation } = await this.client().sendFeedback({
        page_url: page,
        page_title: this.pageTitle || undefined,
        rating,
        body,
        locale: this.locale,
        allow_email: this.identified || !!email,
        email: !this.identified && email ? email : undefined,
      });
      this.emit("yuva-feedback", { page, rating: rating ?? null, conversation_id: conversation.id } satisfies PageFeedbackDetail);
      if (this.pageUrl !== page) return;
      this.#comment.value = "";
      this.#phase = "thanks";
    } catch (error) {
      if (this.pageUrl !== page || this.refusal(error, false)) return;
      this.#phase = "form";
      this.#error = this.errorText(error);
    }
    this.render();
  }

  #syncSend(): void {
    const send = this.root.querySelector<HTMLButtonElement>(".send");
    if (send) send.disabled = this.#phase !== "form" || this.#comment.value.trim() === "";
  }

  protected render(): void {
    if (this.refused || !this.channel) {
      this.root.replaceChildren();
      return;
    }
    const question = this.t(msg`Was this page helpful?`);
    const ask = el("div", "ask");
    ask.setAttribute("role", "group");
    ask.setAttribute("aria-label", question);
    const choices = el("div", "choices");
    for (const value of ["up", "down"] as const) {
      const button = el("button", "choice");
      button.type = "button";
      button.innerHTML = thumb(value === "down");
      button.append(el("span", "", value === "up" ? this.t(msg`Yes`) : this.t(msg`No`)));
      button.setAttribute("aria-pressed", String(this.#rating === value));
      button.disabled = this.#phase === "rating" || this.#phase === "sending";
      button.addEventListener("click", () => void this.rate(value));
      choices.append(button);
    }
    ask.append(el("p", "question", question), choices);
    const children: Node[] = [ask];

    if (this.#phase === "form" || this.#phase === "sending") {
      const form = el("form");
      form.noValidate = true;
      form.addEventListener("submit", (event) => {
        event.preventDefault();
        void this.#send();
      });
      const prompt = this.#rating === "down" ? this.t(msg`Sorry about that. What was missing or unclear?`) : this.t(msg`Thanks! Anything we could add? (optional)`);
      this.#comment.placeholder = prompt;
      this.#comment.setAttribute("aria-label", prompt);
      this.#comment.disabled = this.#phase === "sending";
      form.append(this.#comment);
      if (!this.identified) {
        this.#email.placeholder = this.t(msg`E-mail for a reply (optional)`);
        this.#email.setAttribute("aria-label", this.t(msg`E-mail address`));
        this.#email.disabled = this.#phase === "sending";
        form.append(this.#email);
      }
      const skip = el("button", "link", this.t(msg`No thanks`));
      skip.type = "button";
      skip.disabled = this.#phase === "sending";
      skip.addEventListener("click", () => {
        this.#phase = "thanks";
        this.#error = "";
        this.render();
      });
      const send = el("button", "send", this.#phase === "sending" ? this.t(msg`Sending…`) : this.t(msg`Send`));
      send.type = "submit";
      const actions = el("div", "actions");
      actions.append(skip, send);
      form.append(actions);
      children.push(form);
    } else if (this.#phase === "thanks") {
      const thanks = el("p", "muted", this.t(msg`Thanks for your feedback.`));
      thanks.setAttribute("role", "status");
      children.push(thanks);
    }
    if (this.#error) {
      const error = el("p", "error", this.#error);
      error.setAttribute("role", "alert");
      children.push(error);
    }
    this.frame(styles, ...children);
    this.#syncSend();
  }
}
