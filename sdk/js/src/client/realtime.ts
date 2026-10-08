import type { ClientRealtimeMessage } from "./api";

export interface RealtimeHandlers {
  token(): string | null;
  event(message: ClientRealtimeMessage): void;
  sessionEnded(): void;
  reconnecting(attempt: number): void;
}

export class Realtime {
  #socket: WebSocket | null = null;
  #lastEventId = 0;
  #attempt = 0;
  #timer: ReturnType<typeof setTimeout> | null = null;
  #stopped = true;

  constructor(
    readonly server: string,
    readonly socketClass: typeof WebSocket,
    readonly handlers: RealtimeHandlers,
  ) {}

  get connected(): boolean {
    return this.#socket?.readyState === 1;
  }

  start(): void {
    this.#stopped = false;
    if (!this.#socket) this.#connect();
  }

  stop(): void {
    this.#stopped = true;
    if (this.#timer) clearTimeout(this.#timer);
    this.#timer = null;
    const socket = this.#socket;
    this.#socket = null;
    socket?.close(1000);
  }

  reset(): void {
    this.#lastEventId = 0;
  }

  nudge(): void {
    if (this.#stopped || this.#socket) return;
    if (this.#timer) clearTimeout(this.#timer);
    this.#timer = null;
    this.#connect();
  }

  #connect(): void {
    const token = this.handlers.token();
    if (!token || this.#stopped) return;
    const url = new URL(`${this.server}/client/v1/realtime`, globalThis.location?.href);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    if (this.#lastEventId > 0) url.searchParams.set("last_event_id", String(this.#lastEventId));
    let socket: WebSocket;
    try {
      socket = new this.socketClass(url, ["yuva", `yuva.token.${token}`]);
    } catch {
      this.#schedule();
      return;
    }
    this.#socket = socket;
    socket.onmessage = (event) => {
      let message: ClientRealtimeMessage;
      try {
        message = JSON.parse(String(event.data)) as ClientRealtimeMessage;
      } catch {
        return;
      }
      if (message.type === "ready") {
        this.#attempt = 0;
        this.#lastEventId = Math.max(this.#lastEventId, message.last_event_id);
      } else if ("id" in message && typeof message.id === "number") {
        this.#lastEventId = Math.max(this.#lastEventId, message.id);
      }
      this.handlers.event(message);
    };
    socket.onclose = (event) => {
      if (this.#socket !== socket) return;
      this.#socket = null;
      if (this.#stopped) return;
      if (event.code === 1008) {
        this.#lastEventId = 0;
        this.handlers.sessionEnded();
        return;
      }
      this.#schedule();
    };
  }

  #schedule(): void {
    if (this.#stopped || this.#timer) return;
    const attempt = this.#attempt++;
    this.handlers.reconnecting(attempt);
    const delay = Math.min(30_000, 1000 * 2 ** attempt) * (0.75 + Math.random() * 0.5);
    this.#timer = setTimeout(() => {
      this.#timer = null;
      this.#connect();
    }, delay);
  }
}
