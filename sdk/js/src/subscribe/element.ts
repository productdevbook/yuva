import type { MessageDescriptor, Messages } from "@lingui/core"
import { msg } from "@lingui/core/macro"
import {
  ApiError,
  call,
  createSubscription,
  mailingLists,
  type ClientInbox,
  type ClientMailingList,
  type Fetch,
  type SubscriptionState,
} from "../client/api"
import type { IdentityTokenSource, YuvaClient, YuvaClientOptions } from "../client/client"
import { el } from "../docs/dom"
import { messages as en } from "../locales/en/subscribe"
import { messages as de } from "../locales/de/subscribe"
import { messages as tr } from "../locales/tr/subscribe"
import { directionOf, localeIn, translate } from "../translate"
import { setStyles } from "../styles"
import { contrastOn } from "../types"

export interface SubscribeDetail {
  lists: string[]
  confirmation: boolean
}

export interface SubscribeErrorDetail {
  code: string
}

export interface SubscribeClientModule {
  createYuvaClient(options: YuvaClientOptions): YuvaClient
}

type Phase = "loading" | "idle" | "submitting" | "done"

interface Offered {
  list: ClientMailingList
  state?: SubscriptionState
}

const catalogs: Record<string, Messages> = { en, tr, de }
const tokenMinimumAge = 2100
const fetcher: Fetch = (url, init) => fetch(url, init)

const Base = (typeof HTMLElement === "undefined" ? class {} : HTMLElement) as typeof HTMLElement

export const scriptSrc =
  typeof document !== "undefined" && document.currentScript instanceof HTMLScriptElement
    ? document.currentScript.src
    : ""

const styles = `
:host{display:block;line-height:1.5}
:host([hidden]),[hidden]{display:none!important}
*{box-sizing:border-box}
p,form,fieldset{margin:0}
.frame,form,.row,fieldset,.text{display:flex;flex-direction:column}
.frame{--a:var(--yuva-accent,var(--brand,#2563eb));--on:var(--yuva-on-accent,var(--brand-on,#fff));--r:var(--yuva-radius,8px);--d:#dc2626;font-family:var(--yuva-font,inherit);gap:10px}
@media (prefers-color-scheme:dark){.frame{--d:#f87171}}
form{gap:10px}
.row,fieldset{gap:8px}
.inline .row{flex-flow:row wrap}
.inline .row input{flex:1 1 180px}
input{inline-size:100%;padding:8px 10px;border:1px solid rgb(127 127 127/.35);border-radius:var(--r);background:none;color:inherit;font:inherit}
[aria-invalid=true]{border-color:var(--d)}
[type=checkbox]{inline-size:18px;block-size:18px;margin:2px 0 0;flex:none;accent-color:var(--a)}
[role=switch]{appearance:none;position:relative;inline-size:36px;block-size:20px;margin:1px 0 0;border-radius:10px;background:rgb(127 127 127/.45);cursor:pointer;transition:background .15s}
[role=switch]::before{content:"";position:absolute;inset-block-start:2px;inset-inline-start:2px;inline-size:16px;block-size:16px;border-radius:50%;background:#fff;box-shadow:0 1px 2px rgb(0 0 0/.3);transition:inset-inline-start .15s}
[role=switch]:checked{background:var(--a)}
[role=switch]:checked::before{inset-inline-start:18px;background:var(--on)}
[role=switch]:disabled{opacity:.5;cursor:default}
button{padding:8px 16px;border:0;border-radius:var(--r);background:var(--a);color:var(--on);font:inherit;font-weight:600;cursor:pointer}
button:disabled{opacity:.6;cursor:default}
:focus-visible{outline:2px solid var(--a);outline-offset:2px}
fieldset{padding:0;border:0}
legend{padding:0;margin-block-end:4px;font-weight:600}
.check{display:flex;gap:8px;align-items:flex-start;cursor:pointer}
.muted,.consent,.error{font-size:.9em}
.muted{opacity:.72}
.consent a{color:inherit}
.error{color:var(--d)}
.trap{position:absolute;inset-inline-start:-1e4px;inline-size:1px;overflow:hidden}
@media (max-width:480px){input{font-size:16px}}
`

export class YuvaSubscribeElement extends Base {
  static observedAttributes = [
    "channel",
    "server",
    "locale",
    "dir",
    "lists",
    "layout",
    "name",
    "button-text",
    "consent",
    "privacy-url",
    "identity-token",
  ]

  static loadClient: (() => Promise<SubscribeClientModule>) | null = null

  #root: ShadowRoot
  #messages: Messages = en
  #connected = false
  #generation = 0
  #phase: Phase = "loading"
  #refused = false
  #inbox: ClientInbox | null = null
  #offered: Offered[] = []
  #chosen = new Set<string>()
  #token = ""
  #tokenAt = 0
  #confirmation = false
  #error = ""
  #emailError = ""
  #nameError = ""
  #consentError = ""
  #identity: IdentityTokenSource | null = null
  #client: YuvaClient | null = null
  #busy = new Set<string>()
  #email = el("input")
  #name = el("input")
  #trap = el("input")
  #consent = el("input")
  #status = el("p")

  constructor() {
    super()
    this.#root = this.attachShadow({ mode: "open" })
    this.#email.type = "email"
    this.#email.autocomplete = "email"
    this.#email.required = true
    this.#email.maxLength = 320
    this.#email.id = "email"
    this.#name.autocomplete = "name"
    this.#name.maxLength = 200
    this.#name.id = "name"
    this.#trap.name = "website"
    this.#trap.tabIndex = -1
    this.#trap.autocomplete = "off"
    this.#consent.type = "checkbox"
    this.#consent.id = "consent"
    this.#consent.addEventListener("change", () => {
      this.#consentError = ""
      this.#render()
    })
    for (const input of [this.#email, this.#name]) {
      input.setAttribute("part", "field")
      input.dir = "auto"
    }
    this.#status.setAttribute("part", "status")
    this.#status.setAttribute("role", "status")
    this.#status.setAttribute("aria-live", "polite")
  }

  get channel(): string | null {
    return this.getAttribute("channel")
  }

  set channel(value: string | null) {
    this.#attribute("channel", value)
  }

  get server(): string {
    const base = this.getAttribute("server") || (scriptSrc ? new URL(scriptSrc).origin : location.origin)
    return base.replace(/\/+$/, "")
  }

  set server(value: string | null) {
    this.#attribute("server", value)
  }

  get locale(): string {
    return localeIn(catalogs, [this.getAttribute("locale"), navigator.language])
  }

  set locale(value: string | null) {
    this.#attribute("locale", value)
  }

  get lists(): ClientMailingList[] {
    return this.#offered.map((item) => item.list)
  }

  setIdentityToken(source: IdentityTokenSource | null): void {
    this.#identity = source
    if (this.#connected) void this.#load()
  }

  connectedCallback(): void {
    this.#connected = true
    void this.#load()
  }

  disconnectedCallback(): void {
    this.#connected = false
  }

  attributeChangedCallback(name: string, previous: string | null, value: string | null): void {
    if (!this.#connected || previous === value) return
    if (["channel", "server", "lists", "identity-token"].includes(name)) return void this.#load()
    this.#render()
  }

  #attribute(name: string, value: string | null): void {
    if (value === null || value === undefined || value === "") this.removeAttribute(name)
    else this.setAttribute(name, value)
  }

  #t(descriptor: MessageDescriptor): string {
    return translate(this.#messages, descriptor, en)
  }

  get #identified(): boolean {
    return !!this.#identity || this.hasAttribute("identity-token")
  }

  async #identityToken(): Promise<string | null> {
    return this.#identity ? ((await this.#identity()) ?? null) : this.getAttribute("identity-token")
  }

  #wanted(lists: ClientMailingList[]): ClientMailingList[] {
    const ids = (this.getAttribute("lists") ?? "")
      .split(",")
      .map((id) => id.trim())
      .filter((id) => id && id !== "all")
    if (ids.length === 0) return lists
    const found = ids.map((id) => lists.find((list) => list.id === id)).filter((list) => !!list)
    if (found.length < ids.length) console.warn(`Yuva: some lists in "${ids.join(",")}" are not public lists`)
    return found
  }

  async #load(): Promise<void> {
    const generation = ++this.#generation
    const channel = this.channel
    this.#refused = false
    this.#phase = "loading"
    this.#offered = []
    this.#error = ""
    this.#client = null
    this.#render()
    if (!channel) return
    const server = this.server
    try {
      const inbox = call<ClientInbox>(fetcher, server, `/client/v1/channels/${encodeURIComponent(channel)}`)
      let offered: Offered[] | null = null
      if (this.#identified && YuvaSubscribeElement.loadClient && (await this.#identityToken())) {
        const { createYuvaClient } = await YuvaSubscribeElement.loadClient()
        const client = createYuvaClient({ server, channel, identityToken: () => this.#identityToken() })
        try {
          const { items } = await client.subscriptions()
          const wanted = new Set(this.#wanted(items.map((item) => item.list)))
          offered = items.filter((item) => wanted.has(item.list))
          this.#client = client
        } catch (error) {
          if (!(error instanceof ApiError) || error.code !== "identity_required") throw error
        }
      }
      if (!offered) {
        const { lists, form_token } = await mailingLists(fetcher, server, channel)
        offered = this.#wanted(lists).map((list) => ({ list }))
        this.#token = form_token
        this.#tokenAt = Date.now()
      }
      this.#inbox = await inbox
      if (generation !== this.#generation) return
      this.#offered = offered
      this.#chosen = new Set(offered.length === 1 ? [offered[0]!.list.id] : [])
      this.#phase = "idle"
      if (offered.length === 0) console.warn(`Yuva: channel ${channel} has no public mailing lists to offer`)
    } catch (error) {
      if (generation !== this.#generation) return
      if (this.#refusal(error)) return
      this.#phase = "idle"
      this.#error = this.#errorText(error)
    }
    this.#render()
  }

  #refusal(error: unknown): boolean {
    const refused =
      error instanceof ApiError &&
      (error.status === 404 || error.code === "origin_not_allowed" || error.code === "page_origin")
    if (refused) {
      this.#refused = true
      console.warn(`Yuva: channel ${this.channel} is unknown, offers no such list or does not allow ${location.origin}`)
      this.#render()
    }
    return refused
  }

  #errorText(error: unknown): string {
    const code = error instanceof ApiError ? error.code : ""
    if (code === "rate_limited") return this.#t(msg`Too many attempts. Wait a moment and try again.`)
    if (code === "network") return this.#t(msg`You seem to be offline.`)
    if (code === "no_email") return this.#t(msg`Your account has no e-mail address yet.`)
    return this.#t(msg`Could not subscribe. Try again.`)
  }

  #emit(type: string, detail: SubscribeDetail | SubscribeErrorDetail): void {
    this.dispatchEvent(new CustomEvent(type, { detail, bubbles: true, composed: true }))
  }

  #failed(error: unknown): void {
    this.#emit("yuva-subscribe-error", { code: error instanceof ApiError ? error.code : "error" })
  }

  consentText(lists: ClientMailingList[] = this.#selected()): string {
    const custom = this.getAttribute("consent")?.trim()
    if (custom) return custom
    const names = (lists.length ? lists : this.lists).map((list) => list.name)
    const list = new Intl.ListFormat(this.locale, { type: "conjunction" }).format(names)
    const inbox = this.#inbox?.name ?? ""
    return this.#t(msg`I want to receive e-mails about ${list} from ${inbox}. I can unsubscribe at any time.`)
  }

  #selected(): ClientMailingList[] {
    return this.lists.filter((list) => this.#chosen.has(list.id))
  }

  async #submit(): Promise<void> {
    if (this.#phase !== "idle" || !this.channel) return
    const lists = this.#selected()
    const email = this.#email.value.trim()
    const name = this.#name.value.trim()
    const asksName = this.getAttribute("name")
    this.#error = lists.length === 0 ? this.#t(msg`Choose at least one list.`) : ""
    this.#emailError = this.#email.checkValidity() && email ? "" : this.#t(msg`Enter a valid e-mail address.`)
    this.#nameError = asksName === "required" && !name ? this.#t(msg`Enter your name.`) : ""
    this.#consentError = this.#consent.checked ? "" : this.#t(msg`Tick the box to agree.`)
    if (this.#error || this.#emailError || this.#nameError || this.#consentError) {
      this.#render()
      const invalid = [
        this.#emailError && this.#email,
        this.#nameError && this.#name,
        this.#consentError && this.#consent,
      ]
      for (const input of invalid) if (input) return input.focus()
      return
    }
    const channel = this.channel
    const generation = this.#generation
    const consent_text = this.consentText(lists)
    this.#phase = "submitting"
    this.#render()
    const send = async (retry: boolean): Promise<boolean> => {
      const wait = this.#tokenAt + tokenMinimumAge - Date.now()
      if (wait > 0) await new Promise((resolve) => setTimeout(resolve, wait))
      try {
        const { confirmation } = await createSubscription(fetcher, this.server, channel, {
          email,
          name: asksName === "optional" || asksName === "required" ? name || undefined : undefined,
          list_ids: lists.map((list) => list.id),
          locale: this.locale,
          page_url: location.href.split("#")[0]!.slice(0, 2000),
          consent_text,
          form_token: this.#token,
          website: this.#trap.value || undefined,
        })
        this.#confirmation = confirmation
        return true
      } catch (error) {
        if (retry && error instanceof ApiError && error.code === "form_token") {
          const fresh = await mailingLists(fetcher, this.server, channel)
          this.#token = fresh.form_token
          this.#tokenAt = Date.now()
          return send(false)
        }
        throw error
      }
    }
    try {
      await send(true)
    } catch (error) {
      if (generation !== this.#generation) return
      this.#failed(error)
      this.#phase = "idle"
      if (this.#refusal(error)) return
      if (error instanceof ApiError && error.code === "invalid_email") {
        this.#emailError = this.#t(msg`Enter a valid e-mail address.`)
        this.#render()
        return this.#email.focus()
      }
      this.#error = this.#errorText(error)
      return this.#render()
    }
    if (generation !== this.#generation) return
    this.#phase = "done"
    this.#emit("yuva-subscribe", { lists: lists.map((list) => list.id), confirmation: this.#confirmation })
    this.#render()
  }

  async #toggle(item: Offered, on: boolean): Promise<void> {
    const client = this.#client
    if (!client || this.#busy.has(item.list.id)) return
    this.#busy.add(item.list.id)
    this.#error = ""
    this.#render()
    try {
      if (on) {
        const { state } = await client.subscribeList(item.list.id, {
          consent_text: this.consentText([item.list]),
          page_url: location.href.split("#")[0]!.slice(0, 2000),
        })
        item.state = state
        this.#emit("yuva-subscribe", { lists: [item.list.id], confirmation: state === "pending" })
      } else {
        await client.unsubscribeList(item.list.id)
        item.state = "unsubscribed"
      }
    } catch (error) {
      this.#failed(error)
      this.#error = this.#errorText(error)
    }
    this.#busy.delete(item.list.id)
    if (client === this.#client) this.#render()
  }

  #errorFor(input: HTMLInputElement, text: string): HTMLElement | null {
    input.setAttribute("aria-invalid", String(!!text))
    if (!text) {
      input.removeAttribute("aria-describedby")
      return null
    }
    const id = `${input.id}-error`
    input.setAttribute("aria-describedby", id)
    const error = el("p", "error", text)
    error.id = id
    return error
  }

  #check(text: Node[], checked: boolean, onChange: (checked: boolean) => void, key: string): HTMLLabelElement {
    const label = el("label", "check")
    const box = el("input")
    box.type = "checkbox"
    box.checked = checked
    box.dataset.key = key
    box.addEventListener("change", () => onChange(box.checked))
    const body = el("span", "text")
    body.append(...text)
    label.append(box, body)
    return label
  }

  #lists(identity: boolean): HTMLElement | null {
    if (!identity && this.#offered.length < 2) return null
    const group = el("fieldset")
    group.append(el("legend", "", this.#t(msg`Lists`)))
    for (const item of this.#offered) {
      const text: Node[] = [el("span", "", item.list.name)]
      if (item.list.description) text.push(el("span", "muted", item.list.description))
      if (identity) {
        text.push(el("span", "muted consent", this.consentText([item.list])))
        if (item.state === "pending") text.push(el("span", "muted", this.#t(msg`Check your inbox to confirm.`)))
      }
      const on = identity ? item.state === "subscribed" || item.state === "pending" : this.#chosen.has(item.list.id)
      const label = this.#check(
        text,
        on,
        (checked) => {
          if (identity) return void this.#toggle(item, checked)
          if (checked) this.#chosen.add(item.list.id)
          else this.#chosen.delete(item.list.id)
          this.#error = ""
          this.#render()
        },
        item.list.id
      )
      const box = label.querySelector("input")!
      if (identity) {
        box.setAttribute("role", "switch")
        box.setAttribute("aria-checked", String(box.checked))
        box.addEventListener("keydown", (event) => {
          if (event.key !== "Enter") return
          event.preventDefault()
          box.click()
        })
        box.disabled = this.#busy.has(item.list.id)
      } else box.disabled = this.#phase === "submitting"
      group.append(label)
    }
    return group
  }

  #form(): HTMLElement {
    const form = el("form", this.getAttribute("layout") === "inline" ? "inline" : "")
    form.noValidate = true
    form.addEventListener("submit", (event) => {
      event.preventDefault()
      void this.#submit()
    })
    const busy = this.#phase === "submitting"
    const lists = this.#lists(false)
    if (lists) form.append(lists)

    const row = el("div", "row")
    const errors: Node[] = []
    const asksName = this.getAttribute("name")
    if (asksName === "optional" || asksName === "required") {
      this.#name.placeholder = asksName === "required" ? this.#t(msg`Your name`) : this.#t(msg`Your name (optional)`)
      this.#name.setAttribute("aria-label", this.#name.placeholder)
      this.#name.required = asksName === "required"
      this.#name.disabled = busy
      row.append(this.#name)
      const error = this.#errorFor(this.#name, this.#nameError)
      if (error) errors.push(error)
    }
    this.#email.placeholder = this.#t(msg`E-mail address`)
    this.#email.setAttribute("aria-label", this.#email.placeholder)
    this.#email.disabled = busy
    row.append(this.#email)
    const emailError = this.#errorFor(this.#email, this.#emailError)
    if (emailError) errors.push(emailError)
    const button = el(
      "button",
      "",
      busy ? this.#t(msg`Subscribing…`) : this.getAttribute("button-text") || this.#t(msg`Subscribe`)
    )
    button.type = "submit"
    button.disabled = busy
    button.setAttribute("part", "button")
    row.append(button)

    const trap = el("div", "trap")
    trap.setAttribute("aria-hidden", "true")
    trap.append(this.#trap)

    const consentText = el("span", "", this.consentText())
    const consent = el("label", "check consent")
    consent.setAttribute("part", "consent")
    this.#consent.disabled = busy
    const body = el("span")
    body.append(consentText)
    const privacy = this.getAttribute("privacy-url")
    if (privacy && /^https?:\/\//i.test(privacy)) {
      const link = el("a", "", this.#t(msg`Privacy policy`))
      link.href = privacy
      link.target = "_blank"
      link.rel = "noopener noreferrer"
      body.append(" ", link)
    }
    consent.append(this.#consent, body)
    const consentError = this.#errorFor(this.#consent, this.#consentError)

    form.append(row, ...errors, consent)
    if (consentError) form.append(consentError)
    form.append(trap)
    return form
  }

  #render(): void {
    if (this.#refused || !this.channel || (this.#phase !== "loading" && this.#offered.length === 0 && !this.#error)) {
      this.#root.replaceChildren()
      return
    }
    const locale = this.locale
    this.#messages = catalogs[locale] ?? en
    const dir = this.getAttribute("dir")
    const wrapper = el("div", "frame")
    wrapper.dir = dir === "rtl" || dir === "ltr" ? dir : directionOf(locale)
    wrapper.lang = locale
    const brand = this.#inbox?.chat.launcher_color ?? this.#inbox?.branding.color
    if (brand && /^#[0-9a-f]{6}$/i.test(brand)) {
      wrapper.style.setProperty("--brand", brand)
      wrapper.style.setProperty("--brand-on", contrastOn(brand))
    }

    let status = ""
    if (this.#phase === "done")
      status = this.#confirmation
        ? this.#t(msg`Almost done. Check your inbox and confirm your subscription.`)
        : this.#t(msg`Thanks! You are subscribed.`)
    this.#status.textContent = status

    if (this.#phase === "idle" || this.#phase === "submitting") {
      const content = this.#client ? this.#lists(true) : this.#form()
      if (content) wrapper.append(content)
    }
    wrapper.append(this.#status)
    if (this.#error) {
      const error = el("p", "error", this.#error)
      error.setAttribute("role", "alert")
      wrapper.append(error)
    }

    let focusedKey: string | undefined
    let focused: Element | null = null
    try {
      focused = this.#root.activeElement
    } catch {}
    if (focused instanceof HTMLInputElement) focusedKey = focused.dataset.key
    const fallback = setStyles(this.#root, styles)
    this.#root.replaceChildren(...(fallback ? [fallback] : []), wrapper)
    const again = focusedKey ? this.#root.querySelector<HTMLElement>(`[data-key="${focusedKey}"]`) : focused
    if (again instanceof HTMLElement && again.isConnected) again.focus({ preventScroll: true })
  }
}

export function defineSubscribe(): void {
  if (typeof customElements === "undefined") return
  if (!customElements.get("yuva-subscribe")) customElements.define("yuva-subscribe", YuvaSubscribeElement)
}
