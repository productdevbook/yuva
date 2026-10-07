import type { components } from "./schema.gen";

export type Layout = "launcher" | "embedded";

export type IdentityTokenSource = () => string | null | undefined | Promise<string | null | undefined>;

export interface LauncherStyle {
  position: "right" | "left";
  color?: string;
}

export function launcherOf(inbox: components["schemas"]["ClientInbox"]): LauncherStyle {
  return { position: inbox.chat.launcher_position, color: inbox.chat.launcher_color ?? inbox.branding.color };
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
