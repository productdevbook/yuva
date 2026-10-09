import { Trans, useLingui } from "@lingui/react/macro"

import { formatDateTime, formatRelative } from "@/components/common/text"
import { useContactPresence } from "@/features/contact/queries"
import { cn } from "@/lib/utils"

export function Presence({ contactId }: { contactId: string }) {
  const { i18n } = useLingui()
  const presence = useContactPresence(contactId).data
  if (!presence || (!presence.online && !presence.last_seen_at)) return null
  const seen = presence.last_seen_at ? formatRelative(presence.last_seen_at, i18n.locale) : ""
  return (
    <p
      className="flex items-center gap-1.5 text-caption text-muted-foreground"
      data-testid="contact-presence"
      data-online={presence.online}
      title={presence.last_seen_at ? formatDateTime(presence.last_seen_at, i18n.locale) : undefined}
    >
      <span className={cn("size-1.5 shrink-0 rounded-full", presence.online ? "bg-success" : "bg-faint")} />
      {presence.online ? <Trans>Online now</Trans> : <Trans>Last seen {seen}</Trans>}
    </p>
  )
}
