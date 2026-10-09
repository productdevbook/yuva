import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { EmailAddresses } from "@/features/contact/EmailAddresses"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { OtherConversations } from "@/features/contact/OtherConversations"
import { Presence } from "@/features/contact/Presence"
import { useContact } from "@/features/contact/queries"
import { attrValue, Section } from "@/features/contact/Section"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

function languageName(tag: string, locale: string) {
  try {
    const name = new Intl.DisplayNames([locale], { type: "language" }).of(tag)
    return name && name !== tag ? `${name} (${tag})` : tag
  } catch {
    return tag
  }
}

export function ContactPanel({ contactId, conversationId, hrefFor }: { contactId: string; conversationId: string; hrefFor: (id: string) => string }) {
  const { t, i18n } = useLingui()
  const contact = useContact(contactId)
  const inboxes = useInboxes().data ?? []
  const { canManage } = useSession()
  const [merging, setMerging] = useState(false)
  const c = contact.data

  if (contact.isPending) {
    return (
      <div className="flex flex-col gap-3 p-5">
        <Skeleton className="size-12 rounded-full" />
        <Skeleton className="h-4 w-40" />
      </div>
    )
  }
  if (!c) return <ErrorLine error={contact.error} className="p-5" />

  const name = c.name || c.emails[0] || t`Unnamed contact`
  const since = formatDateTime(c.created_at, i18n.locale)
  const attrs = Object.entries(c.attributes ?? {})
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto" data-testid="contact-panel">
      <div className="flex flex-col items-start gap-3 px-5 pt-5 pb-4">
        <PersonAvatar name={name} className="size-12" />
        <div className="flex min-w-0 flex-col gap-1">
          <p className="truncate text-title">{name}</p>
          <Presence contactId={c.id} />
          <p className="text-caption text-faint">
            <Trans>Since {since}</Trans>
          </p>
          {c.locale && (
            <p className="text-caption text-faint" data-testid="contact-locale">
              {languageName(c.locale, i18n.locale)}
            </p>
          )}
          {c.blocked && (
            <span className="mt-1 w-fit rounded-full bg-destructive/8 px-2 py-0.5 text-caption font-medium text-destructive">
              <Trans>Blocked</Trans>
            </span>
          )}
        </div>
      </div>
      {c.emails.length > 0 && (
        <Section title={<Trans>E-mail addresses</Trans>}>
          <EmailAddresses contact={c} />
        </Section>
      )}
      {c.external_ids.length > 0 && (
        <Section title={<Trans>External ids</Trans>}>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-body">
            {c.external_ids.map((x) => (
              <div key={`${x.inbox_id}:${x.external_id}`} className="contents">
                <dt className="truncate text-muted-foreground">{inboxes.find((i) => i.id === x.inbox_id)?.name ?? "?"}</dt>
                <dd className="truncate font-mono text-caption">{x.external_id}</dd>
              </div>
            ))}
          </dl>
        </Section>
      )}
      {attrs.length > 0 && (
        <Section title={<Trans>Attributes</Trans>}>
          <dl className="grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-body">
            {attrs.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="truncate text-muted-foreground">{k}</dt>
                <dd className="break-words">{attrValue(v)}</dd>
              </div>
            ))}
          </dl>
        </Section>
      )}
      <OtherConversations contactId={c.id} conversationId={conversationId} hrefFor={hrefFor} />
      {canManage && (
        <div className="mt-auto border-t px-3 py-3">
          <Button variant="ghost" size="sm" onClick={() => setMerging(true)} data-testid="merge-contact">
            <Trans>Merge another contact into this one</Trans>
          </Button>
          <MergeContactDialog target={c} open={merging} onOpenChange={setMerging} />
        </div>
      )}
    </div>
  )
}
