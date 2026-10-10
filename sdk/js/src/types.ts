import type { IdentityTokenSource } from "./client/client";
import type { components } from "./schema.gen";

export type { IdentityTokenSource };
export type Rating = components["schemas"]["Rating"];
export type ClientConversation = components["schemas"]["ClientConversation"];

export type Layout = "launcher" | "embedded";

export interface LauncherStyle {
  position: "right" | "left";
  color?: string;
}

export function launcherOf(inbox: components["schemas"]["ClientInbox"]): LauncherStyle {
  return { position: inbox.chat.launcher_position, color: inbox.chat.launcher_color ?? inbox.branding.color };
}

export function contrastOn(color: string): string {
  const n = Number.parseInt(color.slice(1), 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b > 0.4 ? "#111827" : "#ffffff";
}

export interface ChatConfig {
  channel: string | null;
  server: string;
  layout: Layout;
  locale: string;
}

export interface PanelHost {
  panel: HTMLElement;
  config(): ChatConfig;
  identityToken(): Promise<string | null>;
  isOpen(): boolean;
  close(): void;
  setUnread(count: number): void;
  setLauncher(style: LauncherStyle): void;
  refused(): void;
}

export interface PanelController {
  update(): void;
  opened(): void;
  closed(): void;
  identityChanged(): void;
  signOut(): Promise<void>;
  rate(conversationId: string, rating: Rating, comment?: string): Promise<ClientConversation>;
  destroy(): void;
}

export interface PanelModule {
  mount(host: PanelHost): PanelController;
}

export interface StoredState {
  visitor_id?: string;
  session?: { token: string; expires_at: string; sub: string | null };
  launcher?: LauncherStyle;
}

export function storageKey(channel: string): string {
  return `yuva:${channel}`;
}

export function readStored(channel: string): StoredState {
  try {
    const raw = localStorage.getItem(storageKey(channel));
    return raw ? (JSON.parse(raw) as StoredState) : {};
  } catch {
    return {};
  }
}

export function writeStored(channel: string, state: StoredState): void {
  try {
    localStorage.setItem(storageKey(channel), JSON.stringify(state));
  } catch {}
}
