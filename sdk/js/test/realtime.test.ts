import { afterEach, beforeEach, expect, test } from "bun:test"
import { Realtime } from "../src/client/realtime"
import type { ClientRealtimeMessage } from "../src/client/api"

interface Timer {
  at: number
  run: () => void
}

class FakeSocket {
  static all: FakeSocket[] = []
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: ((event: { code: number; reason: string }) => void) | null = null
  readyState = 1
  constructor(readonly url: URL) {
    FakeSocket.all.push(this)
  }
  send(message: ClientRealtimeMessage): void {
    this.onmessage?.({ data: JSON.stringify(message) })
  }
  drop(code = 1006, reason = ""): void {
    this.onclose?.({ code, reason })
  }
  close(): void {}
}

let now = 0
let timers: Timer[] = []
const real = {
  setTimeout: globalThis.setTimeout,
  clearTimeout: globalThis.clearTimeout,
  now: Date.now,
  random: Math.random,
}
let visibility = "visible"

beforeEach(() => {
  now = 1_000_000
  timers = []
  FakeSocket.all = []
  visibility = "visible"
  Date.now = () => now
  Math.random = () => 0.5
  globalThis.setTimeout = ((run: () => void, ms: number) => {
    const timer = { at: now + ms, run }
    timers.push(timer)
    return timer
  }) as unknown as typeof setTimeout
  globalThis.clearTimeout = ((timer: Timer) => {
    timers = timers.filter((t) => t !== timer)
  }) as unknown as typeof clearTimeout
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => visibility })
})

afterEach(() => {
  Object.assign(globalThis, { setTimeout: real.setTimeout, clearTimeout: real.clearTimeout })
  Date.now = real.now
  Math.random = real.random
})

function setup() {
  const events: ClientRealtimeMessage[] = []
  const attempts: number[] = []
  const realtime = new Realtime("https://yuva.example", FakeSocket as unknown as typeof WebSocket, {
    token: () => "t1",
    event: (message) => events.push(message),
    sessionEnded: () => undefined,
    reconnecting: (attempt) => attempts.push(attempt),
  })
  realtime.start()
  return { realtime, events, attempts }
}

function advance(ms: number): void {
  now += ms
  const due = timers.filter((t) => t.at <= now)
  timers = timers.filter((t) => t.at > now)
  for (const timer of due) timer.run()
}

const ready = { type: "ready", last_event_id: 5 } as ClientRealtimeMessage

test("ready alone does not reset the backoff", () => {
  const { attempts } = setup()
  for (let i = 0; i < 3; i++) {
    const socket = FakeSocket.all.at(-1)!
    socket.send(ready)
    socket.drop()
    advance(200_000)
  }
  expect(attempts).toEqual([0, 1, 2])
})

test("a socket that stayed up for a minute resets the backoff", () => {
  const { attempts } = setup()
  FakeSocket.all[0]!.drop()
  advance(200_000)
  const socket = FakeSocket.all[1]!
  socket.send(ready)
  now += 61_000
  socket.drop()
  expect(attempts).toEqual([0, 0])
})

test("retry_after on 1013 is honoured and a nudge does not skip it", () => {
  const { realtime } = setup()
  FakeSocket.all[0]!.drop(1013, "too_many_connections;retry_after=45")
  realtime.nudge()
  advance(44_000)
  expect(FakeSocket.all.length).toBe(1)
  advance(1_500)
  expect(FakeSocket.all.length).toBe(2)
})

test("a hidden page does not reconnect after 1013 until it is visible", () => {
  setup()
  visibility = "hidden"
  FakeSocket.all[0]!.drop(1013, "too_many_connections")
  advance(600_000)
  expect(FakeSocket.all.length).toBe(1)
  visibility = "visible"
  document.dispatchEvent(new Event("visibilitychange"))
  advance(3_000)
  expect(FakeSocket.all.length).toBe(2)
})

test("resync_required drops the replay position so the next connect does not replay", () => {
  const { events } = setup()
  const socket = FakeSocket.all[0]!
  socket.send({ type: "message.created", id: 9 } as unknown as ClientRealtimeMessage)
  socket.send({ type: "resync_required" } as ClientRealtimeMessage)
  socket.send({ type: "ready", last_event_id: 40 } as ClientRealtimeMessage)
  expect(events.map((e) => e.type)).toEqual(["message.created", "resync_required", "ready"])
  socket.drop()
  advance(200_000)
  expect(FakeSocket.all[1]!.url.searchParams.get("last_event_id")).toBe("40")
})

test("a drop between resync_required and ready reconnects without a replay position", () => {
  setup()
  const socket = FakeSocket.all[0]!
  socket.send({ type: "message.created", id: 9 } as unknown as ClientRealtimeMessage)
  socket.send({ type: "resync_required" } as ClientRealtimeMessage)
  socket.drop()
  advance(200_000)
  expect(FakeSocket.all[1]!.url.searchParams.has("last_event_id")).toBe(false)
})
