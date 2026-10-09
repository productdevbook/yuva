import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ContactAvatar, ErrorLine, Segmented } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Attributes,
  ConversationRows,
  Identities,
  NewConversationDialog,
  Satisfaction,
  SectionTitle,
  sinceText,
  useContactConversations,
} from "@/features/contact/ContactDetails"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { Presence } from "@/features/contact/Presence"
import { ContactNotes, useNoteCount } from "@/features/contact/ContactNotes"
import { useContact, useContactSummary } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useSession } from "@/lib/session"

type Tab = "info" | "conversations" | "notes"

function Body({ contactId, conversationId, onClose }: { contactId: string; conversationId?: string; onClose: () => void }) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const contact = useContact(contactId)
  const conversations = useContactConversations(contactId)
  const summary = useContactSummary(contactId).data
  const noteCount = useNoteCount(contactId)
  const [tab, setTab] = useState<Tab>("info")
  const [merging, setMerging] = useState(false)
  const [starting, setStarting] = useState(false)
  const c = contact.data
  const fmt = new Intl.NumberFormat(i18n.locale)

  const top = (
    <div className="flex items-center justify-between px-4 pt-3.5">
      <span className="text-small text-faint">
        <Trans>Contact</Trans>
      </span>
      <span className="flex gap-1">
        <Button variant="outline" render={<Link to={`/contacts/${contactId}`} />} data-testid="open-contact-page">
          <Trans>Open page</Trans>
          <ArrowUpRightIcon />
        </Button>
        <Button variant="ghost" size="icon" onClick={onClose} aria-label={t`Close`}>
          <XIcon />
        </Button>
      </span>
    </div>
  )

  if (contact.isPending) {
    return (
      <>
        {top}
        <div className="flex flex-col items-center gap-3 p-6">
          <Skeleton className="size-16 rounded-full" />
          <Skeleton className="h-5 w-40" />
        </div>
      </>
    )
  }
  if (!c) {
    return (
      <>
        {top}
        <ErrorLine error={contact.error} className="p-5" />
      </>
    )
  }

  const name = contactName(c, t`Unnamed contact`)
  const since = sinceText(c, i18n.locale)
  const count = c.activity ? fmt.format(c.activity.conversations) : undefined
  return (
    <>
      {top}
      <div className="flex flex-col items-center gap-1.5 border-b px-5 pt-3 pb-4 text-center">
        <ContactAvatar id={c.id} name={name} className="size-16" />
        <SheetTitle className="mt-1 max-w-full truncate text-title" data-testid="contact-sheet-name">
          {name}
        </SheetTitle>
        <SheetDescription className="text-small text-muted-foreground">
          <Trans>Contact since {since}</Trans>
        </SheetDescription>
        <Presence contactId={c.id} />
        {c.blocked && (
          <span className="mt-1 rounded-md bg-destructive/10 px-2 py-0.5 text-caption text-destructive">
            <Trans>Blocked</Trans>
          </span>
        )}
      </div>
      <Segmented<Tab>
        label={t`Contact`}
        value={tab}
        onChange={setTab}
        className="mx-4 mt-3 w-auto"
        itemClassName="flex-auto shrink-0 px-2"
        items={[
          { value: "info", label: t`Info`, testId: "contact-tab-info" },
          {
            value: "conversations",
            testId: "contact-tab-conversations",
            label: (
              <>
                {t`Conversations`}
                {count !== undefined && <span className="ms-1.5 text-faint tabular-nums">{count}</span>}
              </>
            ),
          },
          {
            value: "notes",
            testId: "contact-tab-notes",
            label: (
              <>
                {t`Notes`}
                {noteCount !== undefined && noteCount !== "0" && <span className="ms-1.5 text-faint tabular-nums">{noteCount}</span>}
              </>
            ),
          },
        ]}
      />
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {tab === "info" ? (
          <div className="flex flex-col gap-5">
            <section>
              <SectionTitle>
                <Trans>Contact</Trans>
              </SectionTitle>
              <Identities contact={c} editable />
            </section>
            {(c.locale || Object.keys(c.attributes ?? {}).length > 0) && (
              <section>
                <SectionTitle>
                  <Trans>Details</Trans>
                </SectionTitle>
                <Attributes contact={c} />
              </section>
            )}
            {summary && summary.ratings.good + summary.ratings.bad > 0 && (
              <section>
                <SectionTitle>
                  <Trans>Satisfaction</Trans>
                </SectionTitle>
                <Satisfaction summary={summary} />
              </section>
            )}
          </div>
        ) : tab === "notes" ? (
          <ContactNotes contactId={c.id} />
        ) : (
          <div className="flex flex-col gap-2">
            <ConversationRows items={conversations.items} currentId={conversationId} pending={conversations.isPending} />
            {conversations.hasNextPage && (
              <Button variant="ghost" size="sm" className="self-center" onClick={() => void conversations.fetchNextPage()} disabled={conversations.isFetchingNextPage}>
                <Trans>Load more</Trans>
              </Button>
            )}
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-2 border-t px-4 py-3">
        {canManage && (
          <Button variant="outline" onClick={() => setMerging(true)} data-testid="merge-contact">
            <Trans>Merge with another contact</Trans>
          </Button>
        )}
        <Button variant="outline" onClick={() => setStarting(true)} data-testid="new-conversation-open">
          <Trans>New conversation</Trans>
        </Button>
      </div>
      {canManage && <MergeContactDialog target={c} open={merging} onOpenChange={setMerging} />}
      <NewConversationDialog contact={c} open={starting} onOpenChange={setStarting} />
    </>
  )
}

export function ContactSheet({
  contactId,
  conversationId,
  open,
  onOpenChange,
}: {
  contactId: string
  conversationId?: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" showCloseButton={false} className="flex w-[420px] max-w-full flex-col gap-0 bg-background p-0 phone:w-full" data-testid="contact-sheet">
        {open && <Body contactId={contactId} conversationId={conversationId} onClose={() => onOpenChange(false)} />}
      </SheetContent>
    </Sheet>
  )
}
