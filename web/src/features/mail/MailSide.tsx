import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import {
  ContactAvatar,
  InfoSection,
  KV,
  NavGroup,
  NavList,
  PageSidebar,
  SectionNav,
  useContactName,
  type NavItem,
} from "@/components/common"
import { formatDateTime, formatShort, useEnumText } from "@/components/common/text"
import { useContact } from "@/features/contact/queries"
import { useConversation, useMessages } from "@/features/conversation/queries"
import { useAuthorLabel } from "@/features/inbox/SidePanels"
import { useConversations } from "@/features/inbox/queries"
import {
  mailPath,
  useFilterAccess,
  useFolderLabels,
  useFolders,
  useMailInboxes,
  type Folder,
} from "@/features/mail/folders"
import { useMailCounts } from "@/features/mail/queries"
import { useSession } from "@/lib/session"
import { useChannelMap } from "@/lib/workspace"

export function useFolderCounts(box: string): Partial<Record<Folder, number>> {
  const counts = useMailCounts()
  if (!counts) return {}
  if (box === "all") return { inbox: counts.all, starred: counts.starred }
  return { inbox: counts.inboxes.find((c) => c.id === box)?.count ?? 0 }
}

export function MailSidebar({ box, folder }: { box: string; folder: Folder }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const folders = useFolders()
  const labels = useFolderLabels()
  const counts = useFolderCounts(box)
  const all = useMailCounts()
  const inboxes = useMailInboxes()
  const filters = useFilterAccess(box)
  const folderItems: NavItem[] = folders.map((f) => ({
    key: f,
    label: labels[f],
    count: counts[f],
    hot: f === "inbox",
    current: folder === f,
    to: mailPath(box, f),
    testId: `mail-folder-${f}`,
  }))
  const boxItems: NavItem[] = [
    {
      key: "all",
      label: t`All mail`,
      color: "",
      count: all?.all,
      current: box === "all",
      to: mailPath("all", folder),
      testId: "mail-box-all",
    },
    ...inboxes.map((i) => ({
      key: i.id,
      label: i.name,
      color: i.branding.color ?? "",
      count: all?.inboxes.find((c) => c.id === i.id)?.count,
      current: box === i.id,
      to: mailPath(i.id, folder),
      title: i.mailbox?.addresses[0],
      testId: "mail-box-option",
    })),
    {
      key: "new",
      label: t`New mailbox`,
      sub: true,
      to: "/setup/mail",
      state: { back: "/mail" },
      testId: "mail-box-new",
    },
    ...(filters
      ? [{ key: "filters", label: t`Filters`, sub: true, to: `/mail/${box}/filters`, testId: "mail-filters-link" }]
      : []),
  ]
  return (
    <PageSidebar title={t`Mail`} searchLabel={t`Search or jump to…`}>
      <NavList label={t`Mail`}>
        <NavGroup items={folderItems} />
        <NavGroup title={t`Mailboxes`} items={boxItems} />
      </NavList>
      {!canManage && <SectionNav />}
    </PageSidebar>
  )
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

export function MailThreadInfo({ id, box, folder }: { id: string; box: string; folder: Folder }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const authorLabel = useAuthorLabel()
  const contactName = useContactName()
  const c = useConversation(id).data
  const contact = useContact(c?.contact_id).data
  const channel = useChannelMap().get(c?.channel_id ?? "")
  const inbox = useMailInboxes().find((i) => i.id === c?.inbox_id)
  const messages = useMessages(id).data
  const history = useConversations({ contact_id: c?.contact_id }, !!c)
  if (!c) return null
  const name = contact ? contactName(c, channel, contact.name, contact.emails[0]) : "…"
  const mails = (messages?.pages ?? [])
    .toReversed()
    .flatMap((p) => p.items.toReversed())
    .filter((m) => !m.draft && (m.kind === "message" || m.kind === "forward"))
  const before = (history.data?.pages.flatMap((p) => p.items) ?? []).filter((o) => o.id !== c.id).slice(0, 4)
  const jump = (messageId: string) =>
    document
      .querySelector<HTMLElement>(`[data-testid=mail-messages] [data-message-id="${messageId}"]`)
      ?.scrollIntoView({ block: "start", behavior: "smooth" })
  return (
    <>
      <section className="flex min-w-0 items-center gap-2.5 pb-3.5" data-testid="mail-person">
        <ContactAvatar id={c.contact_id} name={name} className="size-9 shrink-0" />
        <div className="min-w-0">
          <div className="truncate text-[15px] font-semibold">{name}</div>
          {contact?.emails[0] && (
            <div className="truncate font-mono text-caption text-muted-foreground">{contact.emails[0]}</div>
          )}
        </div>
      </section>
      {mails.length > 1 && (
        <InfoSection title={t`In this thread`} testId="mail-toc">
          {mails.map((m) => (
            <button
              key={m.id}
              type="button"
              onClick={() => jump(m.id)}
              className="flex w-full min-w-0 items-center gap-2 rounded-sm py-[3px] text-start text-small leading-5 text-fg2 outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              <span
                aria-hidden
                className={
                  m.direction === "out"
                    ? "size-1.5 shrink-0 rounded-full bg-brand"
                    : "size-1.5 shrink-0 rounded-full bg-muted2"
                }
              />
              <span className="min-w-0 flex-1 truncate">{m.direction === "out" ? authorLabel(m.author) : name}</span>
              <time className="shrink-0 font-mono text-[11px] text-muted2" dateTime={m.created_at}>
                {formatShort(m.created_at, i18n.locale)}
              </time>
            </button>
          ))}
        </InfoSection>
      )}
      <InfoSection title={t`Details`} testId="mail-details">
        {inbox && <KV label={t`Mailbox`} value={inbox.name} />}
        <KV label={t`Status`} value={text.statusOf(c.status, c.kind)} />
        <KV label={t`Started`} value={formatDateTime(c.created_at, i18n.locale)} />
        {mails.length > 0 && <KV label={t`Messages`} value={new Intl.NumberFormat(i18n.locale).format(mails.length)} />}
      </InfoSection>
      {before.length > 0 && (
        <InfoSection title={t`Before`} testId="mail-before">
          {before.map((o) => (
            <Link
              key={o.id}
              to={o.inbox_purpose === "mail" ? mailPath(box, folder, o.id) : `/conversations/${o.id}`}
              className="block truncate rounded-sm py-[3px] text-small leading-5 text-fg2 underline-offset-4 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring"
            >
              {o.subject.trim() || <Trans>(no subject)</Trans>}
            </Link>
          ))}
        </InfoSection>
      )}
    </>
  )
}
