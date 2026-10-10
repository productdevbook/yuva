import { msg } from "@lingui/core/macro";
import type { ClientPageAnswer } from "../client/api";
import { richText } from "../panel/text";
import { YuvaDocsElement, el } from "./base";

type Phase = "ask" | "sending" | "sent";

export interface PageQuestionDetail {
  page: string;
  conversation_id: string;
}

const styles = `
.frame { display: flex; flex-direction: column; gap: 16px; }
h2 { margin: 0; font-size: 1.15em; font-weight: 600; }
h3 { margin: 0; font-size: 1em; font-weight: 600; }
.answers { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
.answer { padding: 12px 14px; border: 1px solid var(--yuva-border); border-radius: 10px; }
.answer h3 { margin-block-end: 6px; }
.text { overflow-wrap: anywhere; }
.text a { color: var(--yuva-accent); }
.text code { font-family: ui-monospace, Menlo, monospace; font-size: 0.9em; padding: 1px 4px; border-radius: 4px; background: var(--yuva-soft); }
.text pre { margin: 6px 0; padding: 8px 10px; border-radius: 8px; background: var(--yuva-soft); font-family: ui-monospace, Menlo, monospace; font-size: 0.9em; white-space: pre-wrap; overflow-x: auto; }
.ask { display: flex; flex-direction: column; gap: 8px; }
`;

export class YuvaPageQuestionsElement extends YuvaDocsElement {
  #answers: ClientPageAnswer[] = [];
  #loadedFor = "";
  #phase: Phase = "ask";
  #sentTo = "";
  #error = "";
  #question = el("textarea");
  #email = el("input");

  constructor() {
    super();
    this.#question.rows = 3;
    this.#question.maxLength = 5000;
    this.#question.dir = "auto";
    this.#question.addEventListener("input", () => this.#syncSend());
    this.#email.type = "email";
    this.#email.autocomplete = "email";
  }

  get answers(): ClientPageAnswer[] {
    return this.#answers;
  }

  protected reset(): void {
    this.#answers = [];
    this.#loadedFor = "";
    this.#phase = "ask";
    this.#error = "";
    this.#question.value = "";
  }

  protected pageChanged(): void {
    void this.#load(false);
  }

  reload(): Promise<void> {
    return this.#load(true);
  }

  async #load(force: boolean): Promise<void> {
    const page = this.pageUrl;
    const key = `${this.server} ${this.channel} ${page}`;
    if (!this.channel || !page || this.refused || (!force && this.#loadedFor === key)) return;
    this.#loadedFor = key;
    try {
      const { items } = await this.client().pageAnswers(page);
      if (this.#loadedFor !== key) return;
      this.#answers = items;
      this.render();
    } catch (error) {
      if (this.#loadedFor === key && !this.refusal(error)) this.#loadedFor = "";
    }
  }

  async #send(): Promise<void> {
    const body = this.#question.value.trim();
    const email = this.#email.value.trim();
    if (!body || this.#phase !== "ask") return;
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
      const { conversation } = await this.client().askQuestion({
        page_url: page,
        page_title: this.pageTitle || undefined,
        body,
        email: !this.identified && email ? email : undefined,
      });
      this.emit("yuva-question", { page, conversation_id: conversation.id } satisfies PageQuestionDetail);
      if (this.pageUrl !== page) return;
      this.#question.value = "";
      this.#sentTo = this.identified ? "" : email;
      this.#phase = "sent";
    } catch (error) {
      if (this.pageUrl !== page || this.refusal(error, false)) return;
      this.#phase = "ask";
      this.#error = this.errorText(error);
    }
    this.render();
  }

  #syncSend(): void {
    const send = this.root.querySelector<HTMLButtonElement>(".send");
    if (send) send.disabled = this.#phase !== "ask" || this.#question.value.trim() === "";
  }

  protected render(): void {
    if (this.refused || !this.channel) {
      this.root.replaceChildren();
      return;
    }
    const children: Node[] = [];
    if (this.#answers.length > 0) {
      children.push(el("h2", "", this.t(msg`Questions and answers`)));
      const list = el("ul", "answers");
      for (const item of this.#answers) {
        const entry = el("li", "answer");
        const question = el("h3", "text", item.question);
        question.dir = "auto";
        const answer = el("div", "text");
        answer.dir = "auto";
        answer.append(richText(item.answer));
        entry.append(question, answer);
        list.append(entry);
      }
      children.push(list);
    }

    const ask = el("div", "ask");
    ask.append(el("h2", "", this.t(msg`Ask a question`)));
    if (!this.canWrite) {
      ask.append(el("p", "muted", this.t(msg`Sign in to ask a question.`)));
    } else if (this.#phase === "sent") {
      const email = this.#sentTo;
      const done = el(
        "p",
        "",
        email ? this.t(msg`Thanks! We'll e-mail the answer to ${email}.`) : this.t(msg`Thanks! Your question was sent.`),
      );
      done.setAttribute("role", "status");
      const again = el("button", "link", this.t(msg`Ask another question`));
      again.type = "button";
      again.addEventListener("click", () => {
        this.#phase = "ask";
        this.render();
        this.#question.focus();
      });
      const actions = el("div", "actions");
      actions.style.justifyContent = "flex-start";
      actions.append(again);
      ask.append(done, actions);
    } else {
      const form = el("form");
      form.noValidate = true;
      form.addEventListener("submit", (event) => {
        event.preventDefault();
        void this.#send();
      });
      const prompt = this.t(msg`What would you like to know about this page?`);
      this.#question.placeholder = prompt;
      this.#question.setAttribute("aria-label", prompt);
      this.#question.disabled = this.#phase === "sending";
      form.append(this.#question);
      if (!this.identified) {
        this.#email.placeholder = this.t(msg`E-mail for the answer (optional)`);
        this.#email.setAttribute("aria-label", this.t(msg`E-mail address`));
        this.#email.disabled = this.#phase === "sending";
        form.append(this.#email);
      }
      const send = el("button", "send", this.#phase === "sending" ? this.t(msg`Sending…`) : this.t(msg`Send question`));
      send.type = "submit";
      const actions = el("div", "actions");
      actions.append(send);
      form.append(actions);
      ask.append(form);
      if (this.#error) {
        const error = el("p", "error", this.#error);
        error.setAttribute("role", "alert");
        ask.append(error);
      }
    }
    children.push(ask);
    this.frame(styles, ...children);
    this.#syncSend();
  }
}
