import { useQueries } from "@tanstack/react-query"
import { useMemo } from "react"

import { contactPresenceQuery, useContactPresence } from "@/features/contact/queries"
import type { Channel, Conversation } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useChannel, useChannelMap } from "@/lib/workspace"

function talksLive(ch: Channel | undefined) {
  return ch?.kind === "chat" || ch?.kind === "app"
}

export function useIsChat(c: Conversation) {
  const channels = useChannelMap()
  const known = c.channel_id ? channels.get(c.channel_id) : undefined
  const fetched = useChannel(c.channel_id && !known ? c.channel_id : undefined).data
  return talksLive(known ?? fetched)
}

export function useIsLive(c: Conversation) {
  const chat = useIsChat(c)
  const presence = useContactPresence(chat ? c.contact_id : undefined).data
  return chat && presence?.contact_id === c.contact_id && presence.online
}

export function useLiveIds(items: Conversation[]) {
  const { workspaceId: ws } = useSession()
  const channels = useChannelMap()
  const chats = items.filter((c) => c.channel_id && talksLive(channels.get(c.channel_id)))
  const contacts = [...new Set(chats.map((c) => c.contact_id))]
  const online = useQueries({
    queries: contacts.map((id) => contactPresenceQuery(ws, id)),
    combine: (rs) => rs.flatMap((r) => (r.data?.online ? [r.data.contact_id] : [])).join(","),
  })
  const ids = chats.filter((c) => online.split(",").includes(c.contact_id)).map((c) => c.id).join(",")
  return useMemo(() => new Set(ids ? ids.split(",") : []), [ids])
}
