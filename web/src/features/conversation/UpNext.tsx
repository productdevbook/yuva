import { Trans, useLingui } from "@lingui/react/macro"

import { ContactAvatar } from "@/components/common"
import type { ConversationListItem } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

export function UpNext({ c, onOpen }: { c: ConversationListItem; onOpen: () => void }) {
  const { t } = useLingui()
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const name = c.contact.name || c.contact.email || t`Unnamed contact`
  return (
    <button
      type="button"
      onClick={onOpen}
      className="mt-10 flex w-full items-center gap-3 rounded-2xl border border-dashed px-3.5 py-3 text-start text-muted-foreground transition-colors hover:border-solid hover:border-faint hover:bg-card"
      data-testid="up-next"
    >
      <ContactAvatar id={c.contact.id} name={name} className="size-[30px] text-[11px]" />
      <span className="min-w-0 flex-1">
        <small className="block text-[11px] tracking-[0.04em] text-faint uppercase">
          <Trans>Up next</Trans>
        </small>
        <b className="font-medium text-foreground">{name}</b>
        {inbox && <span className="text-faint"> · {inbox.name}</span>}
        {(c.last_message?.text || c.subject) && <p className="truncate text-[13px]">{c.last_message?.text || c.subject}</p>}
      </span>
      <span className="text-faint">J</span>
    </button>
  )
}
