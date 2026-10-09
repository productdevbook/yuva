import { Trans, useLingui } from "@lingui/react/macro"

import { ContactAvatar } from "@/components/common"
import type { ConversationListItem } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"
import { Item } from "@/components/ui/item"

export function UpNext({ c, onOpen }: { c: ConversationListItem; onOpen: () => void }) {
  const { t } = useLingui()
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const name = c.contact.name || c.contact.email || t`Unnamed contact`
  return (
    <Item render={<button type="button" />} variant="outline" size="sm" className="mt-10 flex-nowrap border-dashed text-muted-foreground hover:border-solid hover:border-faint [button]:hover:bg-card"
      onClick={onOpen}
      data-testid="up-next"
    >
      <ContactAvatar id={c.contact.id} name={name} className="size-[30px]" />
      <span className="min-w-0 flex-1">
        <small className="eyebrow block">
          <Trans>Up next</Trans>
        </small>
        <b className="font-medium text-foreground">{name}</b>
        {inbox && <span className="text-faint"> · {inbox.name}</span>}
        {(c.last_message?.text || c.subject) && <p className="truncate text-small">{c.last_message?.text || c.subject}</p>}
      </span>
      <span className="text-faint">J</span>
    </Item>
  )
}
