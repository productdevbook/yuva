import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, PlugIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { BotAvatar, ContactAvatar, ErrorLine, PersonAvatar } from "@/components/common"
import { Column, ColumnHeading } from "@/components/common/Column"
import { formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Item } from "@/components/ui/item"
import { Skeleton } from "@/components/ui/skeleton"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { useDrafts, useMentions } from "@/features/inbox/queries"
import { ASSISTANTS } from "@/features/settings/connected-apps/assistants"
import { useGrants } from "@/features/settings/connected-apps/queries"
import type { DraftAuthorKind, DraftItem, Member, Mention, MessageAuthor } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useMemberMap } from "@/lib/workspace"

export function messageLink(conversationId: string, messageId: string) {
  return `/conversations/${conversationId}?m=${messageId}`
}

function useAuthorLabel() {
  const { t } = useLingui()
  const members = useMemberMap()
  return (a: MessageAuthor) => {
    const member: Member | undefined = a.member_id ? members.get(a.member_id) : undefined
    const name = a.type === "bot" ? a.name || t`Bot` : a.type === "member" ? (member ? member.name || member.email : t`Deleted member`) : a.name || t`Contact`
    const via = a.via
    return via ? t`${name} via ${via}` : name
  }
}

function AuthorFace({ author, label }: { author: MessageAuthor; label: string }) {
  if (author.type === "bot") return <BotAvatar name={label} url={author.avatar_url} className="size-9" />
  return <PersonAvatar name={label} className="size-9 bg-mate text-white" />
}

function MoreButton({ query }: { query: { hasNextPage: boolean; isFetchingNextPage: boolean; fetchNextPage: () => unknown } }) {
  if (!query.hasNextPage) return null
  return (
    <div className="flex justify-center pt-2">
      <Button variant="ghost" size="sm" onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>
        <Trans>Show more</Trans>
      </Button>
    </div>
  )
}

function MentionRow({ item }: { item: Mention }) {
  const { i18n } = useLingui()
  const label = useAuthorLabel()
  const who = label(item.note.author)
  const contact = item.conversation.contact.name || item.conversation.contact.email || ""
  return (
    <li data-testid="mention-row" data-seen={item.seen}>
      <Item render={<Link to={messageLink(item.conversation.id, item.note.id)} />} size="xs" className="flex-nowrap items-start gap-3">
        <AuthorFace author={item.note.author} label={who} />
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className={cn("min-w-0 flex-1 truncate text-body", !item.seen && "font-medium")}>
              <Trans>
                {who} in {contact}
              </Trans>
            </span>
            <time className="shrink-0 text-caption text-faint" dateTime={item.note.created_at}>
              {formatRelative(item.note.created_at, i18n.locale)}
            </time>
            {!item.seen && <span className="size-2 shrink-0 rounded-full bg-brand" aria-hidden data-testid="mention-unseen" />}
          </span>
          {item.conversation.subject && <small className="block truncate text-caption text-faint">{item.conversation.subject}</small>}
          <span className="line-clamp-2 text-small text-muted-foreground">{item.note.text}</span>
        </span>
      </Item>
    </li>
  )
}

export function MentionsPanel() {
  const mentions = useMentions()
  const items = mentions.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <Column title={<Trans>Mentions</Trans>} testId="mentions-panel">
      {mentions.isPending && <Skeleton className="mx-2 h-14" />}
      <ErrorLine error={mentions.error} className="px-2.5" />
      {mentions.data && items.length === 0 && (
        <p className="px-2.5 pt-2 text-body text-faint">
          <Trans>No one has mentioned you in a note yet. When a teammate writes @ and your name, the note shows up here.</Trans>
        </p>
      )}
      <ul className="flex flex-col" data-testid="mention-list">
        {items.map((m) => (
          <MentionRow key={m.note.id} item={m} />
        ))}
      </ul>
      <MoreButton query={mentions} />
    </Column>
  )
}

function DraftRow({ item }: { item: DraftItem }) {
  const { i18n } = useLingui()
  const label = useAuthorLabel()
  const d = item.draft
  const author = label(d.author)
  const contact = item.conversation.contact.name || item.conversation.contact.email || ""
  return (
    <li data-testid="draft-row">
      <Item render={<Link to={messageLink(item.conversation.id, d.id)} />} size="xs" className="flex-nowrap items-start gap-3">
        <ContactAvatar id={item.conversation.contact.id} name={contact} className="size-9" />
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-body">{contact}</span>
            <time className="shrink-0 text-caption text-faint" dateTime={d.created_at}>
              {formatRelative(d.created_at, i18n.locale)}
            </time>
          </span>
          <small className="block truncate text-caption text-faint">
            <Trans>Draft by {author}</Trans>
            {item.conversation.subject && ` · ${item.conversation.subject}`}
          </small>
          <span className="line-clamp-2 text-small text-muted-foreground">{d.body}</span>
        </span>
      </Item>
    </li>
  )
}

type AuthorChip = "all" | DraftAuthorKind

function Drafts() {
  const { t } = useLingui()
  const [inboxId] = useInboxFilter()
  const [author, setAuthor] = useState<AuthorChip>("all")
  const drafts = useDrafts({ inbox_id: inboxId || undefined, author: author === "all" ? undefined : author })
  const items = drafts.data?.pages.flatMap((p) => p.items) ?? []
  const total = Number(drafts.data?.pages[0]?.total ?? 0)
  const chips: [AuthorChip, string][] = [
    ["all", t`All`],
    ["bot", t`Bots`],
    ["assistant", t`Assistants`],
    ["member", t`Members`],
  ]
  return (
    <>
      <div className="flex items-center gap-2 pe-1">
        <ColumnHeading>
          <Trans>Drafts waiting</Trans>
          {total > 0 && <span className="ms-1.5 tabular-nums" data-testid="drafts-total">{total}</span>}
        </ColumnHeading>
        <span className="flex-1" />
        <InboxPicker className="max-w-40" />
      </div>
      <div className="flex flex-wrap gap-1.5 px-2.5 pb-2" role="toolbar" aria-label={t`Draft author`} data-testid="draft-chips">
        {chips.map(([k, label]) => (
          <button
            key={k}
            type="button"
            aria-pressed={author === k}
            onClick={() => setAuthor(k)}
            className={cn(
              "h-7 shrink-0 rounded-full px-3 text-small font-medium transition-colors",
              author === k ? "bg-brand-wash text-brand" : "bg-muted text-muted-foreground hover:text-foreground",
            )}
            data-testid={`draft-chip-${k}`}
          >
            {label}
          </button>
        ))}
      </div>
      {drafts.isPending && <Skeleton className="mx-2 h-14" />}
      <ErrorLine error={drafts.error} className="px-2.5" />
      {drafts.data && items.length === 0 && (
        <p className="px-2.5 text-body text-faint">
          <Trans>No drafts are waiting. Replies an assistant, a bot or a teammate drafts show up here until someone sends or discards them.</Trans>
        </p>
      )}
      <ul className="flex flex-col" data-testid="draft-list">
        {items.map((x) => (
          <DraftRow key={x.draft.id} item={x} />
        ))}
      </ul>
      <MoreButton query={drafts} />
    </>
  )
}

export function AssistantsPanel() {
  const { t, i18n } = useLingui()
  const grants = useGrants()
  const items = grants.data ?? []
  return (
    <Column
      title={<Trans>Assistants</Trans>}
      testId="assistants-panel"
      actions={
        <Button variant="ghost" size="sm" render={<Link to="/settings/connected-apps" />}>
          <Trans>Connect</Trans>
          <ArrowUpRightIcon />
        </Button>
      }
    >
      <Drafts />
      <ColumnHeading>
        <Trans>Connected apps</Trans>
      </ColumnHeading>
      {grants.isPending && <Skeleton className="mx-2 h-10" />}
      <ErrorLine error={grants.error} className="px-2.5" />
      {grants.data && items.length === 0 && (
        <p className="px-2.5 text-body text-faint">
          <Trans>No assistant is connected yet. Connect Claude, ChatGPT, Cursor or another MCP client to let it read and draft replies.</Trans>
        </p>
      )}
      <ul className="flex flex-col" data-testid="assistant-grants">
        {items.map((g) => {
          const a = ASSISTANTS.find((x) => x.matches(g))
          const Icon = a?.icon ?? PlugIcon
          const used = g.last_used_at ? formatRelative(g.last_used_at, i18n.locale) : null
          return (
            <li key={g.id}>
              <Item render={<Link to={a ? `/settings/connected-apps/${a.id}` : "/settings/connected-apps"} />} size="xs" className="flex-nowrap gap-3">
                <span className="grid size-9 shrink-0 place-items-center rounded-full border text-faint">
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-body">{g.client.name}</span>
                  <small className="block truncate text-caption text-faint">
                    {used ? t`Last used ${used}` : t`Not used yet`} · <Plural value={g.requests_this_month} one="# request this month" other="# requests this month" />
                  </small>
                </span>
              </Item>
            </li>
          )
        })}
      </ul>
    </Column>
  )
}
