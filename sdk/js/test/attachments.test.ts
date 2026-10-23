import { expect, test } from "bun:test"
import { acceptAttribute, allowsType, canAttach, fileProblem } from "../src/attachments"

const limits = { max_bytes: 1000, types: ["image/*", "application/pdf"] }

test("matches exact types and wildcards", () => {
  expect(allowsType(limits, "image/png")).toBe(true)
  expect(allowsType(limits, "IMAGE/WEBP")).toBe(true)
  expect(allowsType(limits, "application/pdf")).toBe(true)
  expect(allowsType(limits, "application/zip")).toBe(false)
  expect(allowsType(limits, "")).toBe(false)
  expect(allowsType(limits, "imagex/png")).toBe(false)
})

test("names the first problem of a file", () => {
  expect(fileProblem(limits, { size: 1000, type: "application/pdf" })).toBeNull()
  expect(fileProblem(limits, { size: 1001, type: "application/pdf" })).toBe("size")
  expect(fileProblem(limits, { size: 1, type: "text/plain" })).toBe("type")
})

test("no types means no files", () => {
  const none = { max_bytes: 1000, types: [] }
  expect(canAttach(none)).toBe(false)
  expect(fileProblem(none, { size: 1, type: "image/png" })).toBe("type")
  expect(acceptAttribute(limits)).toBe("image/*,application/pdf")
})

test("the headless client exposes the limits and follows inbox.updated", async () => {
  const sockets: { onmessage: ((event: { data: string }) => void) | null; readyState: number }[] = []
  class FakeSocket {
    readyState = 1
    onmessage: ((event: { data: string }) => void) | null = null
    onclose = null
    constructor() {
      sockets.push(this)
    }
    close(): void {}
  }
  const inbox = { id: "i1", name: "Support", attachments: { max_bytes: 1000, types: ["image/*"] } }
  const { createYuvaClient } = await import("../src/index")
  const client = createYuvaClient({
    server: "https://yuva.example",
    channel: "pk_test",
    storage: null,
    WebSocket: FakeSocket as unknown as typeof WebSocket,
    fetch: async () =>
      new Response(
        JSON.stringify({
          token: "t1",
          expires_at: "2099-01-01T00:00:00Z",
          visitor_id: "v1",
          contact: { id: "c1", identified: false },
          inbox,
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      ),
  })
  expect(client.attachments).toBeNull()
  await client.start()
  expect(client.attachments).toEqual({ max_bytes: 1000, types: ["image/*"] })
  await client.connect()
  sockets[0]!.onmessage?.({
    data: JSON.stringify({
      id: 1,
      type: "inbox.updated",
      created_at: "2026-10-13T10:00:00Z",
      data: { ...inbox, attachments: { max_bytes: 500, types: [] } },
    }),
  })
  expect(client.attachments).toEqual({ max_bytes: 500, types: [] })
  client.disconnect()
})
