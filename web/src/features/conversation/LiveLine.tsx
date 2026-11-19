import { Trans, useLingui } from "@lingui/react/macro"

import { hostPath, pageOf } from "@/components/common"
import { formatRelative } from "@/components/common/text"
import { useContactPresence } from "@/features/contact/queries"
import { timeOf } from "@/features/conversation/messages/context"
import { useNow } from "@/hooks/use-now"
import type { Channel, Contact, Conversation, Message } from "@/lib/api"
import { useChannelMap } from "@/lib/workspace"
import { cn } from "@/lib/utils"

export type ChatFallback = { to?: string; left: string; reason: "email" | "none" | "held" }

export function useChatFallback(
  c: Conversation,
  contact: Contact | undefined,
  chat: boolean,
  inbound: Message[]
): ChatFallback | undefined {
  const presence = useContactPresence(chat ? c.contact_id : undefined).data
  const channels = useChannelMap()
  if (!chat || !contact || !presence || presence.contact_id !== c.contact_id) return undefined
  if (presence.online || !presence.last_seen_at) return undefined
  const emails = contact.emails.map((e) => e.toLowerCase())
  const replied = inbound.findLast((m) => !!m.email?.from && emails.includes(m.email.from.toLowerCase()))
  const address = replied?.email?.from ?? contact.emails[0]
  if (!address) return { left: presence.last_seen_at, reason: "none" }
  const smtp = [...channels.values()].some(
    (ch) => ch.inbox_id === c.inbox_id && ch.kind === "email" && !!ch.email?.smtp
  )
  const suppressed = contact.undeliverable.some((u) => u.email.toLowerCase() === address.toLowerCase())
  if (!smtp || suppressed) return { left: presence.last_seen_at, reason: "held" }
  return { to: address, left: presence.last_seen_at, reason: "email" }
}

export function FallbackLine({ name, fallback }: { name: string; fallback: ChatFallback }) {
  const { i18n } = useLingui()
  const time = timeOf(fallback.left, i18n.locale)
  const to = fallback.to ?? ""
  return (
    <p
      role="status"
      className="mx-auto max-w-[560px] py-2 text-center text-caption text-muted-foreground"
      data-testid="chat-fallback"
      data-reason={fallback.reason}
    >
      {fallback.reason === "email" ? (
        <Trans>
          {name} left the chat · {time} · replies now go by e-mail to {to}
        </Trans>
      ) : fallback.reason === "none" ? (
        <Trans>
          {name} left the chat · {time} · no e-mail address known, replies wait until they come back
        </Trans>
      ) : (
        <Trans>
          {name} left the chat · {time} · replies wait until they come back
        </Trans>
      )}
    </p>
  )
}

function countryOf(contact: Contact, locale: string) {
  const own = contact.attributes?.country
  if (typeof own === "string" && own) return own
  const region = contact.locale?.split(/[-_]/)[1]
  if (!region) return undefined
  try {
    return new Intl.DisplayNames([locale], { type: "region" }).of(region.toUpperCase())
  } catch {
    return undefined
  }
}

function deviceOf(c: Conversation, contact?: Contact) {
  const info = c.client_info ?? contact?.client_info
  if (!info) return ""
  const system = [info.os, info.os_version].filter(Boolean).join(" ")
  return [info.device_model, system].filter(Boolean).join(", ")
}

function localTime(zone: string | undefined, locale: string, now: number) {
  if (!zone) return ""
  try {
    return new Intl.DateTimeFormat(locale, { timeStyle: "short", timeZone: zone }).format(now)
  } catch {
    return ""
  }
}

export function LiveLine({
  c,
  contact,
  channel,
  typing,
}: {
  c: Conversation
  contact?: Contact
  channel?: Channel
  typing: boolean
}) {
  const { t, i18n } = useLingui()
  const now = useNow(60_000)
  const presence = useContactPresence(c.contact_id).data
  const online = !!presence?.online
  const page = pageOf(c)
  const path = page ? hostPath(page) : channel?.name
  const device = deviceOf(c, contact)
  const country = contact ? countryOf(contact, i18n.locale) : undefined
  const zone = c.client_info?.timezone ?? contact?.client_info?.timezone
  const place = [country, localTime(zone, i18n.locale, now)].filter(Boolean).join(" · ")
  const seen = presence?.last_seen_at ? formatRelative(presence.last_seen_at, i18n.locale, now) : ""
  const state = online ? (typing ? t`Online now · typing` : t`Online now`) : seen ? t`Last seen ${seen}` : t`Offline`
  return (
    <div
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-line-soft px-4 py-2 text-small text-muted-foreground phone:px-3"
      data-testid="live-line"
      data-online={online}
    >
      <span className="inline-flex items-center gap-2">
        <span
          aria-hidden
          className={cn(
            "size-2 shrink-0 rounded-full",
            online ? "bg-success ring-3 ring-success/20" : "bg-muted-foreground/40"
          )}
        />
        <b className="font-semibold text-foreground" data-testid={online ? "contact-online" : undefined}>
          {state}
        </b>
      </span>
      {path && (
        <span className="min-w-0 truncate" data-testid="live-page">
          {online ? <Trans>On {path}</Trans> : <Trans>Last on {path}</Trans>}
        </span>
      )}
      {device && <span data-testid="live-device">{device}</span>}
      {place && <span data-testid="live-place">{place}</span>}
    </div>
  )
}
