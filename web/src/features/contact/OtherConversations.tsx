import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import { formatShort, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Section } from "@/features/contact/Section"
import { statusIcons } from "@/features/conversation/controls/shared"
import { useConversations } from "@/features/inbox/queries"
import { cn } from "@/lib/utils"

export function OtherConversations({
  contactId,
  conversationId,
  hrefFor,
}: {
  contactId: string
  conversationId: string
  hrefFor: (id: string) => string
}) {
  const { i18n } = useLingui()
  const text = useEnumText()
  const others = useConversations({ contact_id: contactId })
  const list = (others.data?.pages.flatMap((p) => p.items) ?? []).filter((x) => x.id !== conversationId)
  if (!others.isPending && list.length === 0) return null
  return (
    <Section title={<Trans>Other conversations</Trans>}>
      {others.isPending ? (
        <Skeleton className="h-9 w-full" />
      ) : (
        <ul className="-mx-2 flex flex-col">
          {list.map((o) => {
            const Icon = statusIcons[o.status]
            return (
              <li key={o.id}>
                <Link to={hrefFor(o.id)} className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-body transition-colors hover:bg-muted">
                  <Icon className="size-3.5 shrink-0 text-faint" aria-label={text.status[o.status]} />
                  <span className={cn("min-w-0 flex-1 truncate", !o.subject && "text-muted-foreground italic")}>
                    {o.subject || <Trans>No subject</Trans>}
                  </span>
                  <span className="shrink-0 text-caption text-faint">{formatShort(o.last_activity_at, i18n.locale)}</span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
      {others.hasNextPage && (
        <Button variant="ghost" size="sm" className="self-start" onClick={() => void others.fetchNextPage()} disabled={others.isFetchingNextPage}>
          <Trans>Load more</Trans>
        </Button>
      )}
    </Section>
  )
}
