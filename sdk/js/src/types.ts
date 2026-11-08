import type { IdentityTokenSource, YuvaClient } from "./client/client"
import type { components } from "./schema.gen"

export type { IdentityTokenSource }
export type Rating = components["schemas"]["Rating"]
export type ClientInfo = components["schemas"]["ClientInfo"]
export type ClientConversation = components["schemas"]["ClientConversation"]

export type Layout = "launcher" | "embedded"

export interface LauncherStyle {
  position: "right" | "left"
  color?: string
}

export function launcherOf(inbox: components["schemas"]["ClientInbox"]): LauncherStyle {
  return { position: inbox.chat.launcher_position, color: inbox.chat.launcher_color ?? inbox.branding.color }
}

export function contrastOn(color: string): string {
  const n = Number.parseInt(color.slice(1), 16)
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }) as [number, number, number]
  return 0.2126 * r + 0.7152 * g + 0.0722 * b > 0.4 ? "#111827" : "#ffffff"
}

export interface ChatConfig {
  channel: string | null
  server: string
  layout: Layout
  locale: string
  clientInfo: ClientInfo | false
}

export interface PanelHost {
  panel: HTMLElement
  config(): ChatConfig
  identityToken(): Promise<string | null>
  isOpen(): boolean
  close(): void
  setUnread(count: number): void
  setUpdates(count: number): void
  setLauncher(style: LauncherStyle): void
  refused(): void
}

export interface PanelController {
  ready: Promise<void>
  update(): void
  opened(): void
  closed(): void
  identityChanged(): void
  signOut(): Promise<void>
  openApp(kind: AppKind, params?: AppParams): Promise<void>
  rate(conversationId: string, rating: Rating, comment?: string): Promise<ClientConversation>
  destroy(): void
}

export interface PanelModule {
  mount(host: PanelHost): PanelController
}

export type AppKind = "feedback" | "status" | "subscribe" | "help" | "roadmap" | "whats-new"

export interface AppParams {
  article?: string
  query?: string
  idea?: string
  entry?: string
}

export interface AppContext {
  root: Node
  client: YuvaClient
  inbox: components["schemas"]["ClientInbox"]
  locale: string
  mailingLists: boolean
  params: AppParams
  back(): void
  open(kind: AppKind, params?: AppParams): void
  ask(articleId?: string): void
  created(result: components["schemas"]["ClientConversationCreated"]): void
  viewConversation(id: string): void
  seenUpdates(): void
}

export interface AppHandle {
  element: HTMLElement
  destroy(): void
}

export interface AppsModule {
  mountApp(kind: AppKind, context: AppContext): Promise<AppHandle>
}

export interface StoredState {
  visitor_id?: string
  session?: { token: string; expires_at: string; sub: string | null }
  launcher?: LauncherStyle
  dismissed_status?: Record<string, string>
  whats_new_seen?: string
}

export function storageKey(channel: string): string {
  return `yuva:${channel}`
}

export function readStored(channel: string): StoredState {
  try {
    const raw = localStorage.getItem(storageKey(channel))
    return raw ? (JSON.parse(raw) as StoredState) : {}
  } catch {
    return {}
  }
}

export function writeStored(channel: string, state: StoredState): void {
  try {
    localStorage.setItem(storageKey(channel), JSON.stringify(state))
  } catch {}
}
