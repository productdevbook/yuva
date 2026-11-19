import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import { ContactAvatar, InfoSection, KV } from "@/components/common"
import { Fold, FoldRow, FoldSub } from "@/components/common/Fold"
import { formatDateTime, formatShort, useEnumText } from "@/components/common/text"
import { LabelEditor } from "@/features/contact/ConversationPanel"
import { useAuthorLabel } from "@/features/inbox/SidePanels"
import { useConversations } from "@/features/inbox/queries"
import { mailPath, useFolderLabels, useMailInboxes, type Folder } from "@/features/mail/folders"
import { useMailCounts } from "@/features/mail/queries"
import type { Conversation, ConversationUpdate, Message } from "@/lib/api"

export type MailFold = "details" | "thread" | "before"

export function useFolderCounts(box: string): Partial<Record<Folder, number>> {
  const counts = useMailCounts()
  if (!counts) return {}
  if (box === "all") return { inbox: counts.all, starred: counts.starred }
  return { inbox: counts.inboxes.find((c) => c.id === box)?.count ?? 0 }
}

export function MailListInfo({ box, folder }: { box: string; folder: Folder }) {
  const { t } = useLingui()
  const labels = useFolderLabels()
  const counts = useMailCounts()
  const inbox = useMailInboxes().find((i) => i.id === box)
  const open = box === "all" ? counts?.all : counts?.inboxes.find((c) => c.id === box)?.count
  return (
    <InfoSection title={labels[folder]} testId="mail-summary">
      <KV label={t`Mailbox`} value={inbox?.name ?? t`All mail`} />
      {inbox?.mailbox?.addresses[0] && (
        <KV label={t`Address`} value={<span className="font-mono">{inbox.mailbox.addresses[0]}</span>} />
      )}
      <KV label={t`In the inbox`} value={open ?? "–"} tone={open ? "brand" : undefined} />
      {box === "all" && counts && <KV label={t`Starred`} value={counts.starred} />}
    </InfoSection>
  )
}

export function MailPanel({
  c,
  name,
  mails,
  update,
  box,
  folder,
  folds,
  onFold,
}: {
  c: Conversation
  name: string
  mails: Message[]
  update: (body: ConversationUpdate) => void
  box?: string
  folder?: Folder
  folds: Record<MailFold, boolean>
  onFold: (part: MailFold, open: boolean) => void
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const authorLabel = useAuthorLabel()
  const inbox = useMailInboxes().find((i) => i.id === c.inbox_id)
  const history = useConversations({ contact_id: c.contact_id })
  const before = (history.data?.pages.flatMap((p) => p.items) ?? []).filter((o) => o.id !== c.id).slice(0, 6)
  const files = mails.reduce((n, m) => n + m.attachments.length, 0)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const first = name.split(/\s+/)[0]
  const status = text.statusOf(c.status, c.kind)
  const jump = (messageId: string) =>
    document
      .querySelector<HTMLElement>(`[data-testid=mail-messages] [data-message-id="${messageId}"]`)
      ?.scrollIntoView({ block: "start", behavior: "smooth" })
  const linkOf = (o: { id: string; inbox_id: string; inbox_purpose: string }) =>
    o.inbox_purpose === "mail" ? mailPath(box ?? o.inbox_id, folder ?? "inbox", o.id) : `/conversations/${o.id}`
  return (
    <>
      <Fold
        title={<Trans>This mail</Trans>}
        summary={status}
        open={folds.details}
        onOpenChange={(o) => onFold("details", o)}
        testId="mail-details"
      >
        {inbox && <FoldRow label={<Trans>Mailbox</Trans>} value={inbox.mailbox?.addresses[0] ?? inbox.name} />}
        <FoldRow label={<Trans>Status</Trans>} value={status} />
        <FoldRow label={<Trans>Started</Trans>} value={formatDateTime(c.created_at, i18n.locale)} />
        {mails.length > 0 && <FoldRow label={<Trans>Messages</Trans>} value={fmt.format(mails.length)} />}
        {files > 0 && <FoldRow label={<Trans>Attachments</Trans>} value={fmt.format(files)} />}
        <FoldSub>
          <Trans>Labels</Trans>
        </FoldSub>
        <LabelEditor c={c} update={update} />
      </Fold>
      <Fold
        title={<Trans>In this thread</Trans>}
        summary={fmt.format(mails.length)}
        open={folds.thread}
        onOpenChange={(o) => onFold("thread", o)}
        testId="mail-toc"
      >
        {mails.map((m) => (
          <button
            key={m.id}
            type="button"
            onClick={() => jump(m.id)}
            className="flex w-full min-w-0 items-center gap-2 border-t border-line-soft py-[7px] text-start text-small outline-none first:border-t-0 hover:[&_b]:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ContactAvatar
              id={m.direction === "out" ? (m.author.member_id ?? c.id) : c.contact_id}
              name={m.direction === "out" ? authorLabel(m.author) : name}
              className="size-6 text-[10px]"
            />
            <span className="min-w-0 flex-1">
              <b className="block truncate font-medium underline-offset-[3px]">
                {m.direction === "out" ? authorLabel(m.author) : name}
              </b>
              <small className="block truncate text-caption text-muted-foreground">{m.email?.from ?? ""}</small>
            </span>
            <time className="shrink-0 text-caption text-muted-foreground" dateTime={m.created_at}>
              {formatShort(m.created_at, i18n.locale)}
            </time>
          </button>
        ))}
      </Fold>
      <Fold
        title={<Trans>Earlier with {first}</Trans>}
        summary={history.isPending ? "" : before.length > 0 ? fmt.format(before.length) : t`None`}
        open={folds.before}
        onOpenChange={(o) => onFold("before", o)}
        testId="mail-before"
      >
        {before.length === 0 ? (
          <p className="text-muted-foreground">
            <Trans>Nothing earlier.</Trans>
          </p>
        ) : (
          before.map((o) => (
            <Link
              key={o.id}
              to={linkOf(o)}
              className="block border-t border-line-soft py-[7px] text-small outline-none first:border-t-0 hover:[&_b]:underline focus-visible:ring-2 focus-visible:ring-ring"
            >
              <b className="block truncate font-medium underline-offset-[3px]">
                {o.subject.trim() || <Trans>(no subject)</Trans>}
              </b>
              <small className="block truncate text-caption text-muted-foreground">
                {formatDateTime(o.last_message_at ?? o.created_at, i18n.locale)}
              </small>
            </Link>
          ))
        )}
      </Fold>
    </>
  )
}
