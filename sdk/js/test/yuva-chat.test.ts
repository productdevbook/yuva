import { afterEach, beforeAll, describe, expect, test } from "bun:test"
import { define, YuvaChatElement } from "../src/element"
import { directionOf, resolveLocale } from "../src/i18n"
import type {
  ClientConversationCreated,
  ClientInbox,
  ClientMessage,
  ClientRealtimeMessage,
  ClientSession,
} from "../src/client/api"

let loads = 0

beforeAll(() => {
  YuvaChatElement.loadPanel = () => {
    loads++
    return import("../src/yuva-chat")
  }
  define()
})

afterEach(() => {
  document.body.replaceChildren()
  loads = 0
})

function mount(attributes: Record<string, string>): YuvaChatElement {
  const element = document.createElement("yuva-chat") as YuvaChatElement
  for (const [name, value] of Object.entries(attributes)) element.setAttribute(name, value)
  document.body.append(element)
  return element
}

const shadow = (element: YuvaChatElement) => element.shadowRoot!
const panel = (element: YuvaChatElement) => shadow(element).querySelector<HTMLElement>(".panel")!
const launcher = (element: YuvaChatElement) => shadow(element).querySelector<HTMLButtonElement>(".launcher")
const compose = async (element: YuvaChatElement) => {
  await until(() => panel(element).querySelector(".home .primary") !== null)
  panel(element).querySelector<HTMLButtonElement>(".home .primary")!.click()
  await until(() => panel(element).querySelector("textarea") !== null)
}
const until = async (condition: () => boolean) => {
  for (let i = 0; i < 50 && !condition(); i++) await new Promise((resolve) => setTimeout(resolve, 5))
}

describe("<yuva-chat> launcher", () => {
  test("renders a closed launcher without loading the panel", () => {
    const element = mount({ locale: "en" })
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Open chat")
    expect(launcher(element)?.getAttribute("aria-expanded")).toBe("false")
    expect(panel(element).hidden).toBe(true)
    expect(loads).toBe(0)
  })

  test("opens and closes, loading the panel once", async () => {
    let opened = 0
    const count = () => opened++
    document.addEventListener("yuva-open", count)
    const element = mount({ locale: "en" })
    launcher(element)!.click()
    await until(() => !panel(element).hidden)
    expect(element.isOpen).toBe(true)
    expect(panel(element).querySelector(".title")?.textContent).toBe("Messages")
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Close chat")

    panel(element).querySelector<HTMLButtonElement>('button[aria-label="Close"]')!.click()
    expect(element.isOpen).toBe(false)
    expect(panel(element).hidden).toBe(true)

    await element.open()
    expect(panel(element).hidden).toBe(false)
    expect(loads).toBe(1)
    expect(opened).toBe(2)
    document.removeEventListener("yuva-open", count)
  })

  test("translates to Turkish and follows locale changes", async () => {
    const element = mount({ locale: "tr-TR" })
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Sohbeti aç")
    await element.open()
    expect(panel(element).querySelector(".title")?.textContent).toBe("Mesajlar")

    element.setAttribute("locale", "en")
    await until(() => panel(element).querySelector(".title")?.textContent === "Messages")
    expect(panel(element).querySelector(".title")?.textContent).toBe("Messages")
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Close chat")
  })

  test("closes on Escape", async () => {
    const element = mount({ locale: "en" })
    await element.open()
    panel(element).dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }))
    expect(element.isOpen).toBe(false)
  })
})

describe("<yuva-chat> embedded", () => {
  test("shows the panel without a launcher or close button", async () => {
    const element = mount({ layout: "embedded", locale: "tr" })
    await until(() => !panel(element).hidden)
    expect(launcher(element)).toBeNull()
    expect(panel(element).querySelector('button[aria-label="Kapat"]')).toBeNull()
    await until(() => panel(element).querySelector(".empty") !== null)
    expect(panel(element).querySelector(".empty")?.textContent).toBe("Sohbet şu anda kullanılamıyor.")
    element.close()
    expect(panel(element).hidden).toBe(false)
  })
})

describe("locale", () => {
  test("resolves to a supported locale", () => {
    expect(resolveLocale("tr-TR")).toBe("tr")
    expect(resolveLocale("de-DE")).toBe("de")
    expect(resolveLocale("fa", "tr")).toBe("tr")
    expect(resolveLocale("fa", null)).toBe("en")
  })

  test("matches regional and script tags to catalogs", () => {
    expect(resolveLocale("pt-BR")).toBe("pt-br")
    expect(resolveLocale("pt_PT")).toBe("pt-pt")
    expect(resolveLocale("pt")).toBe("pt-br")
    expect(resolveLocale("zh-TW")).toBe("zh-hant")
    expect(resolveLocale("zh-Hant-HK")).toBe("zh-hant")
    expect(resolveLocale("zh-CN")).toBe("zh-hans")
    expect(resolveLocale("zh")).toBe("zh-hans")
    expect(resolveLocale("iw")).toBe("he")
    expect(resolveLocale("no")).toBe("nb")
    expect(resolveLocale("es-MX")).toBe("es-mx")
    expect(resolveLocale("es-419")).toBe("es-mx")
    expect(resolveLocale("es-ES")).toBe("es")
    expect(resolveLocale("fr-CA")).toBe("fr-ca")
    expect(resolveLocale("fr-BE")).toBe("fr")
  })

  test("knows text direction", () => {
    expect(directionOf("ar-EG")).toBe("rtl")
    expect(directionOf("tr")).toBe("ltr")
    expect(directionOf("he")).toBe("rtl")
    expect(directionOf("ur")).toBe("rtl")
    expect(directionOf("zh-hant")).toBe("ltr")
  })
})

describe("<yuva-chat> realtime", () => {
  const sockets: FakeSocket[] = []

  class FakeSocket {
    readyState = 0
    onmessage: ((event: { data: string }) => void) | null = null
    onclose: ((event: { code: number }) => void) | null = null
    constructor(readonly url: URL | string) {
      sockets.push(this)
    }
    receive(message: ClientRealtimeMessage): void {
      this.readyState = 1
      this.onmessage?.({ data: JSON.stringify(message) })
    }
    close(): void {
      this.readyState = 3
    }
  }

  const at = "2026-10-13T10:00:00Z"
  const inbox: ClientInbox = {
    id: "i1",
    name: "Support",
    branding: {},
    default_locale: "en",
    timezone: "UTC",
    mode: "live",
    business_hours: { enabled: false, intervals: [] },
    open_now: true,
    chat: {
      greeting: "Hi",
      launcher_position: "right",
      ask_email_offline: false,
      allow_anonymous: true,
      discussions: "off",
    },
    feedback_categories: [],
    feedback_category_items: [],
    ask_for_rating: false,
  }
  const session: ClientSession = {
    token: "t1",
    expires_at: "2099-01-01T00:00:00Z",
    visitor_id: "v1",
    contact: { id: "k1", name: "Visitor", identified: false },
    inbox,
  }
  const message = (id: string, body: string, type: "contact" | "member", client_id?: string): ClientMessage => ({
    id,
    conversation_id: "c1",
    direction: type === "contact" ? "in" : "out",
    author: { type, name: type === "member" ? "Ada" : undefined },
    body,
    client_id,
    attachments: [],
    created_at: at,
  })

  let realFetch: typeof fetch
  let realSocket: typeof WebSocket

  beforeAll(() => {
    realFetch = globalThis.fetch
    realSocket = globalThis.WebSocket
  })

  afterEach(() => {
    globalThis.fetch = realFetch
    globalThis.WebSocket = realSocket
    sockets.length = 0
    localStorage.clear()
  })

  test("opens the realtime socket once the first message creates the session", async () => {
    localStorage.clear()
    const requests: string[] = []
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input))
      const route = `${init?.method ?? "GET"} ${url.pathname}`
      requests.push(route)
      const reply = (status: number, body: unknown) =>
        new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } })
      switch (route) {
        case "GET /client/v1/channels/pk_test":
          return reply(200, inbox)
        case "POST /client/v1/session":
          return reply(201, session)
        case "GET /client/v1/conversations":
          return reply(200, { items: [] })
        case "POST /client/v1/conversations": {
          const { client_id } = JSON.parse(String(init?.body)) as { client_id: string }
          const created: ClientConversationCreated = {
            conversation: {
              id: "c1",
              kind: "conversation",
              subject: "Hello",
              status: "open",
              unread: false,
              can_rate: false,
              created_at: at,
            },
            message: message("m1", "Hello", "contact", client_id),
          }
          return reply(201, created)
        }
        default:
          return reply(204, {})
      }
    }) as typeof fetch
    globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket

    const element = mount({ layout: "embedded", channel: "pk_test", server: "http://yuva.test", locale: "en" })
    const textarea = () => panel(element).querySelector<HTMLTextAreaElement>("textarea")
    await compose(element)
    expect(requests).not.toContain("POST /client/v1/session")
    expect(sockets).toHaveLength(0)

    textarea()!.value = "Hello"
    textarea()!.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }))
    await until(() => requests.includes("POST /client/v1/conversations") && sockets.length > 0)
    expect(sockets).toHaveLength(1)
    expect(String(sockets[0]?.url)).toBe("ws://yuva.test/client/v1/realtime")

    sockets[0]!.receive({ type: "ready", last_event_id: 0 })
    sockets[0]!.receive({
      id: 1,
      type: "message.created",
      conversation_id: "c1",
      created_at: at,
      data: message("m2", "A reply from Ada", "member"),
    })
    await until(() => panel(element).textContent?.includes("A reply from Ada") ?? false)
    expect(panel(element).textContent).toContain("A reply from Ada")
    expect(sockets).toHaveLength(1)
  })

  test("the first message sends the page it starts on, without query and fragment", async () => {
    const window = globalThis as unknown as { happyDOM: { setURL(url: string): void } }
    const previousURL = location.href
    const previousTitle = document.title
    window.happyDOM.setURL("https://shop.example.com/pricing/pro?plan=team&utm_source=x#faq")
    document.title = `  ${"Pricing ".repeat(50)}  `
    const sent: Record<string, unknown>[] = []
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const route = `${init?.method ?? "GET"} ${new URL(String(input)).pathname}`
      const reply = (status: number, body: unknown) =>
        new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } })
      switch (route) {
        case "GET /client/v1/channels/pk_test":
          return reply(200, inbox)
        case "POST /client/v1/session":
          return reply(201, session)
        case "GET /client/v1/conversations":
          return reply(200, { items: [] })
        case "POST /client/v1/conversations": {
          const body = JSON.parse(String(init?.body)) as Record<string, unknown>
          sent.push(body)
          const created: ClientConversationCreated = {
            conversation: {
              id: "c1",
              kind: "conversation",
              subject: "Hello",
              status: "open",
              unread: false,
              can_rate: false,
              created_at: at,
            },
            message: message("m1", "Hello", "contact", String(body.client_id)),
          }
          return reply(201, created)
        }
        default:
          return reply(204, {})
      }
    }) as typeof fetch
    globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket

    try {
      const element = mount({ layout: "embedded", channel: "pk_test", server: "http://yuva.test", locale: "en" })
      const textarea = () => panel(element).querySelector<HTMLTextAreaElement>("textarea")
      await compose(element)
      textarea()!.value = "Hello"
      textarea()!.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }))
      await until(() => sent.length > 0)

      expect(sent).toHaveLength(1)
      expect(sent[0]?.page_url).toBe("https://shop.example.com/pricing/pro")
      expect(sent[0]?.page_title).toBe("Pricing ".repeat(50).trim().slice(0, 300).trim())
      expect(String(sent[0]?.page_title).length).toBeLessThanOrEqual(300)
    } finally {
      window.happyDOM.setURL(previousURL)
      document.title = previousTitle
    }
  })

  describe("status line", () => {
    const incident = (id: string, impact: "none" | "minor" | "major" | "critical", updated_at = at) => ({
      id,
      kind: "incident" as const,
      title: `Incident ${id}`,
      impact,
      state: "investigating" as const,
      updated_at,
      url: `https://status.example.com/incidents/${id}`,
    })
    const withStatus = (open: ReturnType<typeof incident>[]): ClientInbox => ({
      ...inbox,
      status: { url: "https://status.example.com", state: "degraded", open },
    })

    async function connected(first: ClientInbox): Promise<YuvaChatElement> {
      globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
        const route = `${init?.method ?? "GET"} ${new URL(String(input)).pathname}`
        const reply = (status: number, body: unknown) =>
          new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } })
        if (route === "GET /client/v1/channels/pk_test") return reply(200, first)
        if (route === "POST /client/v1/session") return reply(201, { ...session, inbox: first })
        if (route === "GET /client/v1/session") return reply(200, { ...session, token: undefined, inbox: first })
        if (route === "GET /client/v1/conversations") return reply(200, { items: [] })
        return reply(204, {})
      }) as typeof fetch
      globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
      const stored = JSON.parse(localStorage.getItem("yuva:pk_test") ?? "{}")
      localStorage.setItem(
        "yuva:pk_test",
        JSON.stringify({
          ...stored,
          visitor_id: "v1",
          session: { token: "t1", expires_at: session.expires_at, sub: null },
        })
      )
      const element = mount({ layout: "embedded", channel: "pk_test", server: "http://yuva.test", locale: "en" })
      await until(() => panel(element).querySelector(".home") !== null)
      return element
    }

    const notice = (element: YuvaChatElement) => panel(element).querySelector<HTMLElement>(".notice")
    const noticeText = (element: YuvaChatElement) => notice(element)?.textContent ?? ""

    test("shows the worst open incident with its state in words and a safe link", async () => {
      const element = await connected(withStatus([incident("a", "minor"), incident("b", "critical")]))
      expect(notice(element)?.hidden).toBe(false)
      expect(noticeText(element)).toContain("Investigating")
      expect(noticeText(element)).toContain("Incident b")
      const link = notice(element)!.querySelector("a")!
      expect(link.href).toBe("https://status.example.com/incidents/b")
      expect(link.target).toBe("_blank")
      expect(link.rel).toContain("noopener")
    })

    test("is absent without a status or without open incidents", async () => {
      const element = await connected(inbox)
      expect(notice(element)?.hidden ?? true).toBe(true)
      const empty = await connected(withStatus([]))
      expect(notice(empty)?.hidden ?? true).toBe(true)
    })

    test("dismissal hides it until a new update and is remembered", async () => {
      const element = await connected(withStatus([incident("a", "major")]))
      notice(element)!.querySelector<HTMLButtonElement>("button")!.click()
      expect(notice(element)?.hidden).toBe(true)
      expect(JSON.parse(localStorage.getItem("yuva:pk_test")!).dismissed_status).toEqual({ a: at })

      document.body.replaceChildren()
      const again = await connected(withStatus([incident("a", "major")]))
      expect(notice(again)?.hidden).toBe(true)

      sockets.at(-1)!.receive({
        type: "inbox.updated",
        created_at: at,
        data: withStatus([incident("a", "major", "2026-10-13T11:00:00Z")]),
      })
      await until(() => notice(again)?.hidden === false)
      expect(noticeText(again)).toContain("Incident a")
    })

    test("follows inbox.updated and forgets dismissals when the incident closes", async () => {
      const element = await connected(inbox)
      await until(() => sockets.length > 0)
      const live = sockets.at(-1)!
      live.receive({ type: "inbox.updated", created_at: at, data: withStatus([incident("a", "minor")]) })
      await until(() => notice(element)?.hidden === false)
      expect(noticeText(element)).toContain("Incident a")

      notice(element)!.querySelector<HTMLButtonElement>("button")!.click()
      expect(notice(element)?.hidden).toBe(true)

      live.receive({ type: "inbox.updated", created_at: at, data: withStatus([]) })
      await until(() => JSON.parse(localStorage.getItem("yuva:pk_test")!).dismissed_status === undefined)
      expect(notice(element)?.hidden).toBe(true)
      expect(JSON.parse(localStorage.getItem("yuva:pk_test")!).dismissed_status).toBeUndefined()
    })
  })
})
