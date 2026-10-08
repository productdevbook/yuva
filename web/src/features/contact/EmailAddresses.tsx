import { Trans, useLingui } from "@lingui/react/macro"

import { ErrorLine } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useClearUndeliverable } from "@/features/contact/queries"
import type { Contact, UndeliverableEmail } from "@/lib/api"

function Undeliverable({ contactId, u }: { contactId: string; u: UndeliverableEmail }) {
  const { i18n } = useLingui()
  const clear = useClearUndeliverable(contactId)
  const since = formatDateTime(u.created_at, i18n.locale)
  return (
    <li className="flex flex-col gap-1 rounded-xl bg-destructive/6 px-3 py-2.5 text-sm" data-testid="undeliverable">
      <span className="flex items-center gap-2">
        <span className="min-w-0 flex-1 truncate">{u.email}</span>
        <Button variant="outline" size="xs" onClick={() => clear.mutate(u.email)} disabled={clear.isPending} data-testid="clear-undeliverable">
          <Trans>Clear</Trans>
        </Button>
      </span>
      <span className="text-xs text-destructive">
        {u.reason === "complaint" ? (
          <Trans>Undeliverable: marked as spam by the recipient, {since}</Trans>
        ) : (
          <Trans>Undeliverable: bounced, {since}</Trans>
        )}
      </span>
      {u.detail && <span className="text-xs break-words text-muted-foreground">{u.detail}</span>}
      <span className="text-xs text-muted-foreground">
        <Trans>Replies to this address are refused until you clear it.</Trans>
      </span>
      <ErrorLine error={clear.error} className="text-xs" />
    </li>
  )
}

export function EmailAddresses({ contact }: { contact: Contact }) {
  return (
    <ul className="flex flex-col gap-1.5">
      {contact.emails.map((e) => {
        const u = contact.undeliverable.find((x) => x.email === e)
        return u ? (
          <Undeliverable key={e} contactId={contact.id} u={u} />
        ) : (
          <li key={e} className="truncate text-sm">
            {e}
          </li>
        )
      })}
    </ul>
  )
}
