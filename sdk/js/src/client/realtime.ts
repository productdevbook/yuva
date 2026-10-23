import type { ClientRealtimeMessage } from "./api"

export interface RealtimeHandlers {
  token(): string | null
  event(message: ClientRealtimeMessage): void
  sessionEnded(): void
  reconnecting(attempt: number): void
}

const stableAfter = 60_000
const maxDelay = 180_000
const maxRetryAfter = 300_000
const kicked = 1013

function retryAfter(reason: string): number {
  const found = /retry_after\D{0,4}(\d+(?:\.\d+)?)/.exec(reason)
  return found ? Math.min(Number(found[1]) * 1000, maxRetryAfter) : 0
}

export class Realtime {
  #socket: WebSocket | null = null
  #lastEventId = 0
  #attempt = 0
  #timer: ReturnType<typeof setTimeout> | null = null
  #stopped = true
  #readyAt = 0
  #notBefore = 0
  #heldHidden = false

  constructor(
    readonly server: string,
    readonly socketClass: typeof WebSocket,
    readonly handlers: RealtimeHandlers
  ) {}

  get connected(): boolean {
    return this.#socket?.readyState === 1
  }

  start(): void {
    this.#stopped = false
    if (!this.#socket && !this.#timer) this.#connect()
  }

  stop(): void {
    this.#stopped = true
    if (this.#timer) clearTimeout(this.#timer)
    this.#timer = null
    this.#heldHidden = false
    this.#notBefore = 0
    globalThis.document?.removeEventListener("visibilitychange", this.#visible)
    const socket = this.#socket
    this.#socket = null
    socket?.close(1000)
  }

  reset(): void {
    this.#lastEventId = 0
  }

  nudge(): void {
    if (this.#stopped || this.#socket) return
    if (this.#timer && Date.now() < this.#notBefore) return
    if (this.#heldHidden) {
      if (this.#hidden()) return
      this.#heldHidden = false
      globalThis.document?.removeEventListener("visibilitychange", this.#visible)
    }
    if (this.#timer) clearTimeout(this.#timer)
    this.#timer = null
    this.#connect()
  }

  #visible = (): void => {
    if (globalThis.document?.visibilityState !== "visible" || !this.#heldHidden) return
    this.#heldHidden = false
    globalThis.document.removeEventListener("visibilitychange", this.#visible)
    this.#delay(Math.random() * 2000)
  }

  #hidden(): boolean {
    return globalThis.document?.visibilityState === "hidden"
  }

  #connect(): void {
    const token = this.handlers.token()
    if (!token || this.#stopped) return
    const url = new URL(`${this.server}/client/v1/realtime`, globalThis.location?.href)
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:"
    if (this.#lastEventId > 0) url.searchParams.set("last_event_id", String(this.#lastEventId))
    let socket: WebSocket
    try {
      socket = new this.socketClass(url, ["yuva", `yuva.token.${token}`])
    } catch {
      this.#schedule()
      return
    }
    this.#socket = socket
    socket.onmessage = (event) => {
      let message: ClientRealtimeMessage
      try {
        message = JSON.parse(String(event.data)) as ClientRealtimeMessage
      } catch {
        return
      }
      if (message.type === "resync_required") {
        this.#lastEventId = 0
      } else if (message.type === "ready") {
        this.#readyAt = Date.now()
        this.#lastEventId = Math.max(this.#lastEventId, message.last_event_id)
      } else if ("id" in message && typeof message.id === "number") {
        this.#lastEventId = Math.max(this.#lastEventId, message.id)
      }
      this.handlers.event(message)
    }
    socket.onclose = (event) => {
      if (this.#socket !== socket) return
      this.#socket = null
      if (this.#stopped) return
      if (event.code === 1008) {
        this.#lastEventId = 0
        this.handlers.sessionEnded()
        return
      }
      this.#schedule(event.code === kicked ? String(event.reason ?? "") : null)
    }
  }

  #schedule(kickReason: string | null = null): void {
    if (this.#stopped || this.#timer || this.#heldHidden) return
    if (this.#readyAt && Date.now() - this.#readyAt >= stableAfter) this.#attempt = 0
    this.#readyAt = 0
    const attempt = this.#attempt++
    this.handlers.reconnecting(attempt)
    if (kickReason !== null && this.#hidden()) {
      this.#heldHidden = true
      globalThis.document.addEventListener("visibilitychange", this.#visible)
      return
    }
    const backoff = Math.min(maxDelay, 1000 * 2 ** attempt) * (0.75 + Math.random() * 0.5)
    const wait = kickReason === null ? backoff : Math.max(backoff, retryAfter(kickReason))
    this.#notBefore = Date.now() + (kickReason === null ? 0 : wait)
    this.#delay(wait)
  }

  #delay(wait: number): void {
    this.#timer = setTimeout(() => {
      this.#timer = null
      this.#connect()
    }, wait)
  }
}
