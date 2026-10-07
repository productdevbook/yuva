import type { Messages } from "@lingui/core";

export type Layout = "launcher" | "embedded";

export interface PanelContext {
  locale: string;
  messages: Messages;
  layout: Layout;
  inbox: string | null;
  onClose: () => void;
}

export interface PanelModule {
  renderPanel(target: HTMLElement, context: PanelContext): void;
}
