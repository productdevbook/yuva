import type { ConversationQuery } from "@/lib/api"

export type ConversationFilters = Omit<ConversationQuery, "cursor" | "limit">

export const keys = {
  ws: (ws: string) => ["ws", ws] as const,
  members: (ws: string) => ["ws", ws, "members"] as const,
  invites: (ws: string) => ["ws", ws, "invites"] as const,
  apiKeys: (ws: string) => ["ws", ws, "api-keys"] as const,
  inboxes: (ws: string) => ["ws", ws, "inboxes"] as const,
  inboxMembers: (ws: string, inbox: string) => ["ws", ws, "inbox-members", inbox] as const,
  channels: (ws: string, inbox: string) => ["ws", ws, "channels", inbox] as const,
  channel: (ws: string, id: string) => ["ws", ws, "channel", id] as const,
  labels: (ws: string) => ["ws", ws, "labels"] as const,
  cannedReplies: (ws: string) => ["ws", ws, "canned-replies"] as const,
  usage: (ws: string) => ["ws", ws, "usage"] as const,
  contact: (ws: string, id: string) => ["ws", ws, "contact", id] as const,
  conversationLists: (ws: string) => ["ws", ws, "conversations"] as const,
  conversations: (ws: string, f: ConversationFilters) => ["ws", ws, "conversations", f] as const,
  counts: (ws: string) => ["ws", ws, "counts"] as const,
  conversation: (ws: string, id: string) => ["ws", ws, "conversation", id] as const,
  messages: (ws: string, id: string) => ["ws", ws, "messages", id] as const,
  messageEmail: (ws: string, id: string) => ["ws", ws, "message-email", id] as const,
  passkeys: ["me", "passkeys"] as const,
}
