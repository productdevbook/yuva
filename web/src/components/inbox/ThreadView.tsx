import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, PanelRightIcon } from "lucide-react"
import { useEffect, useLayoutEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { Composer, type ComposerHandle } from "@/components/inbox/Composer"
import {
  AssigneeMenu,
  LabelsMenu,
  PriorityMenu,
  StatusMenu,
  type MenuName,
} from "@/components/inbox/ConversationControls"
import { MessageItem } from "@/components/inbox/MessageItem"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { ConversationUpdate } from "@/lib/api"
import {
  useContact,
  useConversation,
  useInboxes,
  useLabels,
  useMemberMap,
  useMessages,
  useUpdateConversation,
} from "@/lib/queries"
import { useSession } from "@/lib/session"

function dayLabel(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: "full" }).format(new Date(iso))
}

export function ThreadView({
  conversationId,
  backHref,
  onToggleContact,
  contactShown,
}: {
  conversationId: string
  backHref: string
  onToggleContact: () => void
  contactShown: boolean
}) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { membership } = useSession()
  const conversation = useConversation(conversationId)
  const messages = useMessages(conversationId)
  const contact = useContact(conversation.data?.contact_id)
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const members = useMemberMap()
  const updater = useUpdateConversation(conversationId)
  const [openMenu, setOpenMenu] = useState<MenuName | null>(null)
  const composer = useRef<ComposerHandle>(null)
  const scroller = useRef<HTMLDivElement>(null)

  const update = (body: ConversationUpdate) => updater.mutate(body)
  const c = conversation.data

  useHotkeys(
    {
      [SHORTCUTS.reply]: () => composer.current?.focus("message"),
      [SHORTCUTS.note]: () => composer.current?.focus("note"),
      [SHORTCUTS.attach]: () => composer.current?.attach(),
      [SHORTCUTS.assign]: () => setOpenMenu("assign"),
      [SHORTCUTS.assignMe]: () => update({ assignee_id: membership.member_id }),
      [SHORTCUTS.status]: () => setOpenMenu("status"),
      [SHORTCUTS.close]: () => update({ status: "closed" }),
      [SHORTCUTS.reopen]: () => update({ status: "open" }),
      [SHORTCUTS.priority]: () => setOpenMenu("priority"),
      [SHORTCUTS.labels]: () => setOpenMenu("labels"),
      [SHORTCUTS.contact]: onToggleContact,
      [SHORTCUTS.back]: () => navigate(backHref),
    },
    !!c,
  )

  const count = messages.data?.length ?? 0
  useLayoutEffect(() => {
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
  }, [count, conversationId])

  useEffect(() => setOpenMenu(null), [conversationId])

  if (conversation.isPending) {
    return (
      <div className="flex flex-1 flex-col gap-3 p-4">
        <Skeleton className="h-6 w-64" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }
  if (!c) return <ErrorLine error={conversation.error} className="p-4" />

  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const contactName = contact.data ? contact.data.name || contact.data.emails[0] || t`Unnamed contact` : "…"
  const controls = { conversation: c, update, openMenu, setOpenMenu }
  const ctx = { members, contact: contact.data, labels }
  let lastDay = ""

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="thread">
      <div className="flex shrink-0 flex-col gap-2 border-b px-3 py-2.5">
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon-sm"
            className="md:hidden"
            render={<Link to={backHref} />}
            aria-label={t`Back to the list`}
          >
            <ArrowLeftIcon />
          </Button>
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-base font-semibold" data-testid="thread-subject">
              {c.subject || <Trans>No subject</Trans>}
            </h2>
            <p className="truncate text-xs text-muted-foreground">
              {contactName}
              {inbox && <> · {inbox.name}</>}
            </p>
          </div>
          <Button
            variant={contactShown ? "secondary" : "ghost"}
            size="icon-sm"
            onClick={onToggleContact}
            aria-label={t`Show or hide the contact`}
            aria-pressed={contactShown}
          >
            <PanelRightIcon />
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <AssigneeMenu {...controls} />
          <StatusMenu {...controls} />
          <PriorityMenu {...controls} />
          <LabelsMenu {...controls} />
          <ErrorLine error={updater.error} className="text-xs" />
        </div>
      </div>
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto py-3" data-testid="messages">
        {messages.isPending ? (
          <div className="flex flex-col gap-3 px-4">
            <Skeleton className="h-14 w-2/3" />
            <Skeleton className="ml-auto h-14 w-1/2" />
          </div>
        ) : messages.error ? (
          <ErrorLine error={messages.error} className="px-4" />
        ) : (
          messages.data?.map((m) => {
            const day = dayLabel(m.created_at, i18n.locale)
            const sep = day !== lastDay
            lastDay = day
            return (
              <div key={m.id}>
                {sep && (
                  <div className="my-2 flex items-center gap-3 px-4 text-xs text-muted-foreground">
                    <span className="h-px flex-1 bg-border" />
                    {day}
                    <span className="h-px flex-1 bg-border" />
                  </div>
                )}
                <MessageItem m={m} ctx={ctx} />
              </div>
            )
          })
        )}
      </div>
      <Composer ref={composer} conversationId={conversationId} />
    </div>
  )
}
