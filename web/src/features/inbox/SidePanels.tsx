import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useNavigate } from "react-router"

import { BotAvatar, ContactAvatar, ErrorLine, PersonAvatar } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ListHead, UnderlineTabs } from "@/features/inbox/ListHead"
import { Ago, FrameTabs, look, PageBody, PageTop } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { useMarkAllMentionsSeen, useMarkMentionSeen, useMentions } from "@/features/inbox/queries"
import type { Member, Mention, MessageAuthor } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useMemberMap } from "@/lib/workspace"

export function messageLink(conversationId: string, messageId: string) {
  return `/conversations/${conversationId}?m=${messageId}`
}

export function useAuthorLabel() {
  const { t } = useLingui()
  const members = useMemberMap()
  return (a: MessageAuthor) => {
    const member: Member | undefined = a.member_id ? members.get(a.member_id) : undefined
    const name =
      a.type === "bot"
        ? a.name || t`Bot`
        : a.type === "member"
          ? member
            ? member.name || member.email
            : t`Deleted member`
          : a.name || t`Contact`
    const via = a.via
    return via ? t`${name} via ${via}` : name
  }
}

function AuthorFace({ author, label, large }: { author: MessageAuthor; label: string; large?: boolean }) {
  if (author.type === "bot") return <BotAvatar name={label} url={author.avatar_url} className="size-9" />
  if (large && author.member_id)
    return <ContactAvatar id={author.member_id} name={label} className="size-9 font-semibold" />
  return <PersonAvatar name={label} className="size-9 bg-mate text-white" />
}

export function MoreButton({
  query,
}: {
  query: { hasNextPage: boolean; isFetchingNextPage: boolean; fetchNextPage: () => unknown }
}) {
  if (!query.hasNextPage) return null
  return (
    <div className="flex justify-center pt-2">
      <Button variant="ghost" size="sm" onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>
        <Trans>Show more</Trans>
      </Button>
    </div>
  )
}

type MentionTab = "unread" | "all"

function useMentionsView() {
  const mentions = useMentions()
  const all = mentions.data?.pages.flatMap((p) => p.items) ?? []
  const unseen = Number(mentions.data?.pages[0]?.unseen ?? 0)
  const [picked, setPicked] = useState<MentionTab | null>(null)
  const tab: MentionTab = picked ?? (unseen > 0 ? "unread" : "all")
  const items = tab === "unread" ? all.filter((m) => !m.seen) : all
  return { mentions, all, unseen, tab, setPicked, items }
}

function MentionsList({ view }: { view: ReturnType<typeof useMentionsView> }) {
  const { t, i18n } = useLingui()
  const markAll = useMarkAllMentionsSeen()
  const { mentions, all, unseen, items, setPicked } = view
  const unseenText = new Intl.NumberFormat(i18n.locale).format(unseen)
  const totalText = new Intl.NumberFormat(i18n.locale).format(all.length)
  return (
    <>
      {mentions.isPending && <Skeleton className="h-32 rounded-xl" />}
      <ErrorLine error={mentions.error} />
      <ErrorLine error={markAll.error} />
      {all.length > 0 && (
        <div className="flex items-center gap-3">
          <span className="min-w-0 flex-1 text-caption text-faint" data-testid="mentions-summary">
            {unseen > 0 ? (
              <Trans>
                {unseenText} unread of {totalText}
              </Trans>
            ) : (
              <Trans>Everything is read</Trans>
            )}
          </span>
          <Button
            variant="outline"
            className={cn(look.btnSm, "shrink-0")}
            disabled={unseen === 0 || markAll.isPending}
            onClick={() => markAll.mutate()}
            data-testid="mentions-mark-all"
          >
            <Trans>Mark all as read</Trans>
          </Button>
        </div>
      )}
      {mentions.data && all.length === 0 && (
        <EmptyBox title={<Trans>Nobody has mentioned you yet</Trans>}>
          <Trans>When a teammate writes @ and your name in a note, the note shows up here.</Trans>
        </EmptyBox>
      )}
      {mentions.data && all.length > 0 && items.length === 0 && (
        <EmptyBox
          title={<Trans>You have read every mention.</Trans>}
          testId="mentions-read"
          actions={
            <Button
              variant="outline"
              className={look.btnSm}
              onClick={() => setPicked("all")}
              data-testid="mentions-see-all"
            >
              <Trans>See all mentions</Trans>
            </Button>
          }
        />
      )}
      <ul className="flex flex-col gap-2.5" aria-label={t`Mentions`} data-testid="mention-list">
        {items.map((m) => (
          <MentionCard key={m.note.id} item={m} />
        ))}
      </ul>
      <MoreButton query={mentions} />
    </>
  )
}

function MentionsTabs({ view }: { view: ReturnType<typeof useMentionsView> }) {
  const { t } = useLingui()
  return (
    <FrameTabs
      label={t`Show`}
      value={view.tab}
      onChange={view.setPicked}
      items={[
        { key: "unread", label: <Trans>Unread</Trans>, count: view.unseen, testId: "mentions-tab-unread" },
        { key: "all", label: <Trans>All</Trans>, testId: "mentions-tab-all" },
      ]}
    />
  )
}

export function MentionsPanel() {
  const { t } = useLingui()
  const view = useMentionsView()
  return (
    <section className="flex h-full min-h-0 flex-col bg-background" data-testid="mentions-panel">
      <ListHead title={<Trans>Mentions</Trans>} sub={<Trans>Notes where a teammate mentioned you</Trans>}>
        <UnderlineTabs
          value={view.tab}
          onChange={view.setPicked}
          label={t`Show`}
          items={[
            { value: "unread", label: <Trans>Unread</Trans>, count: view.unseen, testId: "mentions-tab-unread" },
            { value: "all", label: <Trans>All</Trans>, testId: "mentions-tab-all" },
          ]}
        />
      </ListHead>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-3 pb-4">
        <MentionsList view={view} />
      </div>
    </section>
  )
}

function Highlighted({ text }: { text: string }) {
  return text.split(/(@[^\s@,.?!]+)/).map((part, i) =>
    part.startsWith("@") ? (
      <span key={i} className="font-medium text-brand">
        {part}
      </span>
    ) : (
      part
    )
  )
}

function MentionCard({ item }: { item: Mention }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const seen = useMarkMentionSeen()
  const label = useAuthorLabel()
  const who = label(item.note.author)
  const contact = item.conversation.contact.name || item.conversation.contact.email || ""
  const subject = item.conversation.subject
  const to = messageLink(item.conversation.id, item.note.id)
  const aria = subject
    ? t`${who} mentioned you in ${contact}, ${subject}. Open the conversation`
    : t`${who} mentioned you in ${contact}. Open the conversation`
  const open = () => {
    if (!item.seen) seen.mutate(item.note.id)
    navigate(to)
  }
  return (
    <li
      className={cn(
        "relative flex gap-2 rounded-xl border bg-background p-3.5 transition-colors focus-within:border-faint hover:border-faint",
        !item.seen && "border-input"
      )}
      data-testid="mention-row"
      data-seen={item.seen}
    >
      <button
        type="button"
        onClick={open}
        aria-label={aria}
        className={cn(
          "flex min-w-0 flex-1 gap-3 rounded-lg text-start outline-none focus-visible:ring-2 focus-visible:ring-ring",
          !item.seen && "pb-8 phone:pb-9"
        )}
        data-testid="mention-open"
      >
        <AuthorFace author={item.note.author} label={who} large />
        <span className="flex min-w-0 flex-1 flex-col gap-2">
          <span className="flex min-w-0 flex-col leading-[1.35]">
            <span className="flex min-w-0 items-center gap-2">
              <span className={cn("min-w-0 truncate", item.seen ? "font-medium" : "font-semibold")}>{who}</span>
              <span className="shrink-0 text-muted-foreground">
                <Trans>mentioned you</Trans>
              </span>
              <span className="flex-1" />
              {!item.seen && (
                <span
                  className="size-2 shrink-0 rounded-full bg-brand"
                  role="img"
                  aria-label={t`Unread`}
                  data-testid="mention-unseen"
                />
              )}
              <Ago iso={item.note.created_at} className={cn(look.faint, "shrink-0")} />
            </span>
            <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint">
              <ProductTag id={item.conversation.inbox_id} />
              <span className="min-w-0 truncate">
                {subject ? (
                  <Trans>
                    in {contact} · {subject}
                  </Trans>
                ) : (
                  <Trans>in {contact}</Trans>
                )}
              </span>
            </span>
          </span>
          <span className="block rounded-[10px] border border-note-border bg-note px-3 py-2.5 whitespace-pre-line">
            <Highlighted text={item.note.text} />
          </span>
        </span>
      </button>
      {!item.seen && (
        <Button
          variant="outline"

          className="absolute end-3.5 bottom-3 h-[26px] rounded-md border-input px-2.5 font-normal text-muted-foreground"
          onClick={() => seen.mutate(item.note.id)}
          disabled={seen.isPending}
          data-testid="mention-read"
        >
          <Trans>Mark as read</Trans>
        </Button>
      )}
    </li>
  )
}

export function MentionsPage() {
  const view = useMentionsView()
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="mentions-panel">
      <PageTop
        title={<Trans>Mentions</Trans>}
        sub={<Trans>Notes where a teammate mentioned you</Trans>}
        tabs={<MentionsTabs view={view} />}
      />
      <PageBody>
        <div className="flex max-w-[908px] flex-col gap-4 p-6">
          <MentionsList view={view} />
        </div>
      </PageBody>
    </main>
  )
}
