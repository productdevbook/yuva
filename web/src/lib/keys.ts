import type { ConversationQuery } from "@/lib/api"

export type ConversationFilters = Omit<ConversationQuery, "cursor" | "limit">

export const keys = {
  members: (ws: string) => ["ws", ws, "members"] as const,
  invites: (ws: string) => ["ws", ws, "invites"] as const,
  apiKeys: (ws: string) => ["ws", ws, "api-keys"] as const,
  inboxes: (ws: string) => ["ws", ws, "inboxes"] as const,
  inboxMembers: (ws: string, inbox: string) => ["ws", ws, "inbox-members", inbox] as const,
  channels: (ws: string, inbox: string) => ["ws", ws, "channels", inbox] as const,
  channel: (ws: string, id: string) => ["ws", ws, "channel", id] as const,
  labels: (ws: string) => ["ws", ws, "labels"] as const,
  cannedReplies: (ws: string) => ["ws", ws, "canned-replies"] as const,
  contact: (ws: string, id: string) => ["ws", ws, "contact", id] as const,
  contactSearch: (ws: string) => ["ws", ws, "contact-search"] as const,
  contactConversationCount: (ws: string, id: string) => ["ws", ws, "contact-conversation-count", id] as const,
  contactPresence: (ws: string, id: string) => ["ws", ws, "contact-presence", id] as const,
  webhooks: (ws: string) => ["ws", ws, "webhooks"] as const,
  webhookList: (ws: string, inbox: string) => ["ws", ws, "webhooks", "list", inbox] as const,
  webhook: (ws: string, id: string) => ["ws", ws, "webhooks", "one", id] as const,
  webhookDeliveries: (ws: string, id: string, state: string) => ["ws", ws, "webhooks", "deliveries", id, state] as const,
  webhookDelivery: (ws: string, id: string, delivery: string) => ["ws", ws, "webhooks", "delivery", id, delivery] as const,
  webhookAttempts: (ws: string, id: string) => ["ws", ws, "webhooks", "attempts", id] as const,
  conversationLists: (ws: string) => ["ws", ws, "conversations"] as const,
  conversations: (ws: string, f: ConversationFilters) => ["ws", ws, "conversations", f] as const,
  counts: (ws: string) => ["ws", ws, "counts"] as const,
  conversation: (ws: string, id: string) => ["ws", ws, "conversation", id] as const,
  messages: (ws: string, id: string) => ["ws", ws, "messages", id] as const,
  messageEmail: (ws: string, id: string) => ["ws", ws, "message-email", id] as const,
  passkeys: ["me", "passkeys"] as const,
  notifications: (ws: string) => ["ws", ws, "notifications"] as const,
}
