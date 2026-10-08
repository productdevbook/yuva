import { Trans, useLingui } from "@lingui/react/macro"
import { SearchXIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { EmptyState, useConfirm } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Composer, type ComposerHandle } from "@/features/conversation/composer/Composer"
import type { MenuName } from "@/features/conversation/controls/shared"
import { MessageList } from "@/features/conversation/MessageList"
import type { DraftControls } from "@/features/conversation/messages/context"
import {
  useConversation,
  useDraftActions,
  useMessages,
  useMoveConversation,
  useUpdateConversation,
} from "@/features/conversation/queries"
import { ThreadHeader } from "@/features/conversation/ThreadHeader"
import { TypingLine } from "@/features/conversation/TypingLine"
import { useContact } from "@/features/contact/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { ApiError, isGone, type ConversationUpdate, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useChannel, useInboxes, useLabels, useMemberMap } from "@/lib/workspace"

export function ThreadView({
  conversationId,
  backHref,
  hrefFor,
  onToggleContact,
  contactShown,
}: {
  conversationId: string
  backHref: string
  hrefFor: (id: string) => string
  onToggleContact: () => void
  contactShown: boolean
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { membership } = useSession()
  const errorText = useErrorText()
  const conversation = useConversation(conversationId)
  const messages = useMessages(conversationId)
  const c = conversation.data
  const contact = useContact(c?.contact_id)
  const channel = useChannel(c?.channel_id)
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const members = useMemberMap()
  const updater = useUpdateConversation(conversationId)
  const mover = useMoveConversation(conversationId)
  const [openMenu, setOpenMenu] = useState<MenuName | null>(null)
  const [expandQuoted, setExpandQuoted] = useState(false)
  const composer = useRef<ComposerHandle>(null)
  const draftActions = useDraftActions()
  const [editingDraft, setEditingDraft] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()

  const update = (body: ConversationUpdate) => {
    mover.reset()
    updater.mutate(body)
  }
  const move = (inboxId: string) => {
    updater.reset()
    mover.mutate(inboxId)
  }

  const items = useMemo(() => (messages.data?.pages ?? []).toReversed().flatMap((p) => p.items.toReversed()), [messages.data])
  const latestDraft = items.findLast((m) => m.draft)
  const resetDrafts = () => {
    draftActions.send.reset()
    draftActions.save.reset()
    draftActions.discard.reset()
  }
  const drafts: DraftControls = {
    editing: editingDraft,
    setEditing: setEditingDraft,
    latest: latestDraft?.id,
    pending: draftActions.send.isPending || draftActions.save.isPending || draftActions.discard.isPending,
    error: draftActions.send.error ?? draftActions.save.error ?? draftActions.discard.error,
    send: (m: Message) => {
      resetDrafts()
      setEditingDraft(null)
      draftActions.send.mutate(m)
    },
    save: (m: Message, body: string) => {
      resetDrafts()
      draftActions.save.mutate({ m, body }, { onSuccess: () => setEditingDraft(null) })
    },
    discard: (m: Message) =>
      confirm({
        title: <Trans>Discard this draft?</Trans>,
        description: <Trans>It is deleted and never sent.</Trans>,
        confirm: <Trans>Discard</Trans>,
        run: () => {
          resetDrafts()
          setEditingDraft(null)
          draftActions.discard.mutate(m)
        },
      }),
  }

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
      [SHORTCUTS.spam]: () => c && update({ spam: !c.spam }),
      [SHORTCUTS.move]: () => setOpenMenu("move"),
      [SHORTCUTS.quoted]: () => setExpandQuoted((x) => !x),
      [SHORTCUTS.contact]: onToggleContact,
      [SHORTCUTS.back]: () => navigate(backHref),
      [SHORTCUTS.editDraft]: () => latestDraft && setEditingDraft(latestDraft.id),
      [SHORTCUTS.sendDraft]: () => latestDraft && !drafts.pending && drafts.send(latestDraft),
      [SHORTCUTS.discardDraft]: () => latestDraft && drafts.discard(latestDraft),
    },
    !!c && !isGone(conversation.error),
  )

  useEffect(() => {
    setOpenMenu(null)
    setEditingDraft(null)
  }, [conversationId])

  if (conversation.isPending) {
    return (
      <div className="flex flex-1 flex-col gap-3 p-6">
        <Skeleton className="h-6 w-64" />
        <Skeleton className="h-4 w-40" />
      </div>
    )
  }
  if (!c || isGone(conversation.error)) {
    return (
      <EmptyState icon={SearchXIcon} title={<Trans>This conversation is not available</Trans>}>
        <p>{errorText(conversation.error ?? new ApiError(404))}</p>
        <Button variant="outline" size="sm" className="mt-4" render={<Link to={backHref} />}>
          <Trans>Back to the list</Trans>
        </Button>
      </EmptyState>
    )
  }

  const contactName = contact.data ? contact.data.name || contact.data.emails[0] || t`Unnamed contact` : "…"
  const ctx = { members, contact: contact.data, labels, inboxes, subject: c.subject, expandQuoted, drafts }
  const isEmail = channel.data?.kind === "email"
  const lastInbound = items.findLast((m) => m.kind === "message" && m.direction === "in" && m.email)
  const emailTo = isEmail ? (lastInbound?.email?.from ?? contact.data?.emails[0]) : undefined
  const undeliverable = emailTo ? contact.data?.undeliverable.some((u) => u.email === emailTo) : false

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="thread">
      <ThreadHeader
        controls={{ conversation: c, update, openMenu, setOpenMenu }}
        channel={channel.data}
        contactName={contactName}
        backHref={backHref}
        hrefFor={hrefFor}
        move={move}
        moving={mover.isPending}
        error={updater.error ?? mover.error}
        contactShown={contactShown}
        onToggleContact={onToggleContact}
      />
      {c.spam && (
        <p className="shrink-0 border-b bg-destructive/5 px-6 py-2 text-xs" role="status" data-testid="spam-banner">
          <Trans>Marked as spam: left out of lists and counts, and never answered automatically.</Trans>
        </p>
      )}
      <MessageList conversationId={conversationId} messages={messages} items={items} ctx={ctx} />
      <TypingLine conversationId={conversationId} contactName={contactName} />
      <Composer ref={composer} conversationId={conversationId} emailTo={emailTo} undeliverable={undeliverable} />
      {confirmDialog}
    </div>
  )
}
