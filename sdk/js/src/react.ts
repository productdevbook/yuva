import {
  createContext,
  createElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement,
  type ReactNode,
} from "react"
import type { ClientChangelog, ClientConversation, ClientHelp, ClientMessage, ClientWhatsNewEntry } from "./client/api"
import type { Help, IdentityTokenSource, MessageInput, YuvaClient } from "./client/client"
import type { DiscussionDetailEvent, DiscussionPostDetail, PageFeedbackDetail, PageRatingDetail } from "./docs"
import type { SubscribeDetail, SubscribeErrorDetail } from "./subscribe"
import type { ReaderProfile } from "./client/api"
import type { Analytics, AnalyticsMethods, AnalyticsOptions } from "./analytics"
import { analytics, attachAnalytics } from "./analytics/handle"

const YuvaContext = createContext<YuvaClient | null>(null)

export interface YuvaProviderProps {
  client: YuvaClient
  connect?: boolean
  children?: ReactNode
}

export function YuvaProvider({ client, connect = true, children }: YuvaProviderProps): ReactElement {
  useEffect(() => {
    if (!connect) return
    client.connect().catch(() => undefined)
    return () => client.disconnect()
  }, [client, connect])
  return createElement(YuvaContext.Provider, { value: client }, children)
}

export function useYuva(): YuvaClient {
  const client = useContext(YuvaContext)
  if (!client) throw new Error("Yuva: wrap your components in <YuvaProvider client={...}>")
  return client
}

const time = (value: string) => Date.parse(value)
const byCreated = (a: ClientMessage, b: ClientMessage) => time(a.created_at) - time(b.created_at)
const byActivity = (a: ClientConversation, b: ClientConversation) =>
  time(b.last_message_at ?? b.created_at) - time(a.last_message_at ?? a.created_at)

interface ListState<T> {
  items: T[]
  cursor?: string
  loading: boolean
  error: unknown
}

const initial = { items: [], loading: true, error: null }

export interface ConversationsResult {
  conversations: ClientConversation[]
  loading: boolean
  error: unknown
  hasMore: boolean
  loadMore(): Promise<void>
  reload(): Promise<void>
}

export function useConversations(): ConversationsResult {
  const client = useYuva()
  const [state, setState] = useState<ListState<ClientConversation>>(initial)
  const cursor = useRef<string | undefined>(undefined)
  const current = useRef(state)
  useEffect(() => {
    current.current = state
  })

  const load = useCallback(
    async (more: boolean) => {
      setState((s) => ({ ...s, loading: true }))
      try {
        const page = await client.listConversations({ limit: 50, cursor: more ? cursor.current : undefined })
        cursor.current = page.next_cursor
        setState((s) => {
          const items = new Map((more ? s.items : []).map((c) => [c.id, c]))
          for (const conversation of page.items) items.set(conversation.id, conversation)
          return { items: [...items.values()].sort(byActivity), cursor: page.next_cursor, loading: false, error: null }
        })
      } catch (error) {
        setState((s) => ({ ...s, loading: false, error }))
      }
    },
    [client]
  )

  useEffect(() => {
    // oxlint-disable-next-line react/set-state-in-effect -- fetching from the client is syncing with an external system
    void load(false)
    const offSession = client.on("session", ({ renewed }) => void (renewed && load(false)))
    const offEvent = client.on("event", (message) => {
      if (message.type === "resync_required") return void load(false)
      if (message.type === "conversation.created") {
        setState((s) =>
          s.items.some((c) => c.id === message.data.id)
            ? s
            : { ...s, items: [message.data, ...s.items].sort(byActivity) }
        )
      } else if (message.type === "conversation.removed") {
        setState((s) => ({ ...s, items: s.items.filter((c) => c.id !== message.data.id) }))
      } else if (message.type === "conversation.updated") {
        setState((s) => ({
          ...s,
          items: s.items.map((c) => (c.id === message.conversation_id ? { ...c, status: message.data.status } : c)),
        }))
      } else if (message.type === "message.created") {
        const m = message.data
        if (!current.current.items.some((c) => c.id === m.conversation_id)) return void load(false)
        setState((s) => {
          const items = s.items.map((c) =>
            c.id === m.conversation_id
              ? {
                  ...c,
                  last_message: {
                    id: m.id,
                    author_type: m.author.type,
                    text: m.body.slice(0, 140),
                    created_at: m.created_at,
                  },
                  last_message_at: m.created_at,
                  unread: c.unread || m.author.type !== "contact",
                }
              : c
          )
          return { ...s, items: items.sort(byActivity) }
        })
      }
    })
    return () => {
      offSession()
      offEvent()
    }
  }, [client, load])

  return {
    conversations: state.items,
    loading: state.loading,
    error: state.error,
    hasMore: !!state.cursor,
    loadMore: () => load(true),
    reload: () => load(false),
  }
}

export interface HelpResult {
  help: ClientHelp | null
  loading: boolean
  error: unknown
  reload(): Promise<void>
  search: Help["search"]
  collection: Help["collection"]
  article: Help["article"]
  react: Help["react"]
  click: Help["click"]
}

export interface UseHelpOptions {
  locale?: string
  page?: string
}

export function useHelp({ locale, page }: UseHelpOptions = {}): HelpResult {
  const client = useYuva()
  const [state, setState] = useState<{ help: ClientHelp | null; loading: boolean; error: unknown }>({
    help: null,
    loading: true,
    error: null,
  })

  const load = useCallback(async () => {
    setState((s) => ({ ...s, loading: true }))
    try {
      const help = await client.help.get({ locale, page })
      setState({ help, loading: false, error: null })
    } catch (error) {
      setState((s) => ({ ...s, loading: false, error }))
    }
  }, [client, locale, page])

  useEffect(() => {
    // oxlint-disable-next-line react/set-state-in-effect -- fetching from the client is syncing with an external system
    void load()
  }, [load])

  const bound = useMemo<Pick<HelpResult, "search" | "collection" | "article" | "react" | "click">>(
    () => ({
      search: (q, options) => client.help.search(q, { locale, ...options }),
      collection: (id, options) => client.help.collection(id, { locale, ...options }),
      article: (id, options) => client.help.article(id, { locale, ...options }),
      react: client.help.react,
      click: client.help.click,
    }),
    [client, locale]
  )

  return { ...state, reload: load, ...bound }
}

export interface WhatsNewResult {
  changelog: ClientChangelog | null
  items: ClientWhatsNewEntry[]
  loading: boolean
  error: unknown
  hasMore: boolean
  loadMore(): Promise<void>
  reload(): Promise<void>
}

export function useWhatsNew({ locale }: { locale?: string } = {}): WhatsNewResult {
  const client = useYuva()
  const [state, setState] = useState<{
    changelog: ClientChangelog | null
    items: ClientWhatsNewEntry[]
    cursor?: string
    loading: boolean
    error: unknown
  }>({ changelog: null, items: [], loading: true, error: null })
  const cursor = useRef<string | undefined>(undefined)

  const fetchPage = useCallback(
    async (more: boolean) => {
      setState((s) => ({ ...s, loading: true }))
      try {
        const page = await client.whatsNew({ locale, cursor: more ? cursor.current : undefined })
        cursor.current = page.next_cursor
        setState((s) => ({
          changelog: page.changelog,
          items: more
            ? [...s.items, ...page.items.filter((item) => !s.items.some((known) => known.id === item.id))]
            : page.items,
          cursor: page.next_cursor,
          loading: false,
          error: null,
        }))
      } catch (error) {
        setState((s) => ({ ...s, loading: false, error }))
      }
    },
    [client, locale]
  )

  useEffect(() => {
    // oxlint-disable-next-line react/set-state-in-effect -- fetching from the client is syncing with an external system
    void fetchPage(false)
  }, [fetchPage])

  return {
    changelog: state.changelog,
    items: state.items,
    loading: state.loading,
    error: state.error,
    hasMore: state.cursor !== undefined,
    loadMore: () => fetchPage(true),
    reload: () => fetchPage(false),
  }
}

export interface MessagesResult {
  messages: ClientMessage[]
  loading: boolean
  error: unknown
  hasMore: boolean
  loadOlder(): Promise<void>
  send(input: MessageInput | string): Promise<ClientMessage>
  markRead(): Promise<void>
}

export function useMessages(conversationId: string | null | undefined): MessagesResult {
  const client = useYuva()
  const [state, setState] = useState<ListState<ClientMessage>>(initial)
  const cursor = useRef<string | undefined>(undefined)

  const upsert = useCallback((message: ClientMessage) => {
    setState((s) => {
      const items = s.items.filter(
        (m) => m.id !== message.id && (!message.client_id || m.client_id !== message.client_id)
      )
      return { ...s, items: [...items, message].sort(byCreated) }
    })
  }, [])

  const load = useCallback(
    async (older: boolean) => {
      if (!conversationId) {
        cursor.current = undefined
        setState({ items: [], loading: false, error: null })
        return
      }
      if (older && !cursor.current) return
      setState((s) => ({ ...s, loading: true }))
      try {
        const page = await client.listMessages(conversationId, {
          order: "desc",
          limit: 30,
          cursor: older ? cursor.current : undefined,
        })
        cursor.current = page.next_cursor
        setState((s) => {
          const items = new Map((older ? s.items : []).map((m) => [m.id, m]))
          for (const message of page.items) items.set(message.id, message)
          return { items: [...items.values()].sort(byCreated), cursor: page.next_cursor, loading: false, error: null }
        })
      } catch (error) {
        setState((s) => ({ ...s, loading: false, error }))
      }
    },
    [client, conversationId]
  )

  useEffect(() => {
    // oxlint-disable-next-line react/set-state-in-effect -- fetching from the client is syncing with an external system
    void load(false)
    if (!conversationId) return
    const offSession = client.on("session", ({ renewed }) => void (renewed && load(false)))
    const offEvent = client.on("event", (message) => {
      if (message.type === "resync_required") void load(false)
      else if (
        (message.type === "message.created" || message.type === "message.updated") &&
        message.data.conversation_id === conversationId
      )
        upsert(message.data)
    })
    return () => {
      offSession()
      offEvent()
    }
  }, [client, conversationId, load, upsert])

  return {
    messages: state.items,
    loading: state.loading,
    error: state.error,
    hasMore: !!state.cursor,
    loadOlder: () => load(true),
    async send(input) {
      if (!conversationId) throw new Error("Yuva: no conversation; start one with client.startConversation()")
      const message = await client.sendMessage(conversationId, typeof input === "string" ? { body: input } : input)
      upsert(message)
      return message
    },
    async markRead() {
      if (conversationId) await client.markRead(conversationId)
    },
  }
}

export interface YuvaDocsProps {
  channel: string
  server?: string
  locale?: string
  dir?: "ltr" | "rtl"
  page?: string
  pageTitle?: string
  identityToken?: string | IdentityTokenSource | null
  className?: string
  style?: CSSProperties
}

export interface YuvaPageFeedbackProps extends YuvaDocsProps {
  onRating?: (detail: PageRatingDetail) => void
  onFeedback?: (detail: PageFeedbackDetail) => void
}

export interface YuvaDiscussionsProps extends YuvaDocsProps {
  onDiscussion?: (detail: DiscussionDetailEvent) => void
  onPost?: (detail: DiscussionPostDetail) => void
  onReader?: (detail: ReaderProfile | null) => void
}

export interface YuvaSubscribeProps extends Omit<YuvaDocsProps, "page" | "pageTitle"> {
  lists?: string[]
  layout?: "stacked" | "inline"
  name?: "off" | "optional" | "required"
  buttonText?: string
  consent?: string
  privacyUrl?: string
  onSubscribe?: (detail: SubscribeDetail) => void
  onError?: (detail: SubscribeErrorDetail) => void
}

type Handlers = Record<string, ((detail: never) => void) | undefined>

interface ElementProps {
  channel: string
  server?: string
  locale?: string
  dir?: "ltr" | "rtl"
  identityToken?: string | IdentityTokenSource | null
  className?: string
  style?: CSSProperties
}

function useYuvaElement(
  tag: string,
  load: () => Promise<unknown>,
  props: ElementProps,
  extra: Record<string, string | undefined>,
  handlers: Handlers
): ReactElement {
  const [element, setElement] = useState<HTMLElement | null>(null)
  const latest = useRef(handlers)
  const loader = useRef(load)
  useEffect(() => {
    latest.current = handlers
  })
  const { channel, server, locale, dir, identityToken, className, style } = props

  useEffect(() => {
    void loader.current()
  }, [])

  const attributes = JSON.stringify({
    server,
    locale,
    dir,
    ...extra,
    "identity-token": typeof identityToken === "string" ? identityToken : undefined,
    channel,
  })
  useEffect(() => {
    if (!element) return
    for (const [name, value] of Object.entries(JSON.parse(attributes) as Record<string, string | undefined>)) {
      if (value) element.setAttribute(name, value)
      else element.removeAttribute(name)
    }
  }, [element, attributes])

  useEffect(() => {
    if (!element || typeof identityToken !== "function") return
    let current = true
    void customElements.whenDefined(tag).then(() => {
      if (current)
        (element as HTMLElement & { setIdentityToken(source: IdentityTokenSource | null): void }).setIdentityToken(
          identityToken
        )
    })
    return () => {
      current = false
    }
  }, [element, tag, identityToken])

  const events = Object.keys(handlers).join(" ")
  useEffect(() => {
    if (!element) return
    const listeners = events.split(" ").map((type) => {
      const listener = (event: Event) => latest.current[type]?.((event as CustomEvent).detail as never)
      element.addEventListener(type, listener)
      return () => element.removeEventListener(type, listener)
    })
    return () => listeners.forEach((off) => off())
  }, [element, events])

  return createElement(tag, { ref: setElement, className, style })
}

const loadDocs = () => import("./docs")

export function YuvaPageFeedback({ onRating, onFeedback, ...props }: YuvaPageFeedbackProps): ReactElement {
  return useYuvaElement(
    "yuva-page-feedback",
    loadDocs,
    props,
    { page: props.page, "page-title": props.pageTitle },
    { "yuva-rating": onRating, "yuva-feedback": onFeedback }
  )
}

export function YuvaDiscussions({ onDiscussion, onPost, onReader, ...props }: YuvaDiscussionsProps): ReactElement {
  return useYuvaElement(
    "yuva-discussions",
    loadDocs,
    props,
    { page: props.page, "page-title": props.pageTitle },
    {
      "yuva-discussion": onDiscussion,
      "yuva-post": onPost,
      "yuva-reader": onReader,
    }
  )
}

export function YuvaSubscribe({ onSubscribe, onError, ...props }: YuvaSubscribeProps): ReactElement {
  return useYuvaElement(
    "yuva-subscribe",
    () => import("./subscribe"),
    props,
    {
      lists: props.lists?.join(","),
      layout: props.layout,
      name: props.name,
      "button-text": props.buttonText,
      consent: props.consent,
      "privacy-url": props.privacyUrl,
    },
    { "yuva-subscribe": onSubscribe, "yuva-subscribe-error": onError }
  )
}

export interface YuvaAnalyticsProps extends Omit<AnalyticsOptions, "key"> {
  publicKey: string
}

export function YuvaAnalytics({
  server,
  publicKey: key,
  hash,
  consent,
  manual,
  outbound,
  files,
  chat,
}: YuvaAnalyticsProps): null {
  useEffect(() => {
    let instance: Analytics | undefined
    let mounted = true
    void import("./analytics").then(({ createAnalytics }) => {
      if (!mounted) return
      instance = createAnalytics({ server, key, hash, consent, manual, outbound, files, chat })
      attachAnalytics(instance)
    })
    return () => {
      mounted = false
      instance?.destroy()
      attachAnalytics(undefined)
    }
  }, [server, key, hash, consent, manual, outbound, files, chat])
  return null
}

export function useYuvaAnalytics(): AnalyticsMethods {
  return analytics
}
