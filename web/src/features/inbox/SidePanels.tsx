import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import {
  BotAvatar,
  ContactAvatar,
  dayOf,
  ErrorLine,
  FlatEmpty,
  FlatRow,
  FlatTag,
  FlatWho,
  groupRuns,
  InfoColumn,
  InfoSection,
  KV,
  ListGroup,
  NavGroup,
  NavList,
  PageHeading,
  PageLayout,
  PageSidebar,
  PageTop,
  PersonAvatar,
  ProductsGroup,
  SectionNav,
  TopSep,
  UnreadBadge,
  useDayLabel,
} from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useSession } from "@/lib/session"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ListHead, UnderlineTabs } from "@/features/inbox/ListHead"
import { Ago, look } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { useMarkAllMentionsSeen, useMarkMentionSeen, useMentions } from "@/features/inbox/queries"
import type { Member, Mention, MessageAuthor } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes, useMemberMap } from "@/lib/workspace"

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

function FlatMentionRow({ item }: { item: Mention }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const seen = useMarkMentionSeen()
  const label = useAuthorLabel()
  const text = useEnumText()
  const inbox = useInboxes().data?.find((i) => i.id === item.conversation.inbox_id)
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
    <li data-testid="mention-row" data-seen={item.seen}>
      <FlatRow
        color={inbox?.branding.color}
        button={{ onClick: open, "aria-label": aria, "data-testid": "mention-open", title: inbox?.name }}
        who={<FlatWho strong={!item.seen}>{who}</FlatWho>}
        end={
          <>
            <FlatTag className="@max-2xl/list:hidden">{text.status[item.conversation.status]}</FlatTag>
            {!item.seen && <UnreadBadge label={t`Unread`} testId="mention-unseen" />}
            <Ago iso={item.note.created_at} className="font-mono text-xs whitespace-nowrap text-muted2" />
          </>
        }
        after={
          !item.seen && (
            <Button
              variant="ghost"
              size="icon-sm"
              className="me-0.5 shrink-0 text-muted2"
              onClick={() => seen.mutate(item.note.id)}
              disabled={seen.isPending}
              aria-label={t`Mark as read`}
              title={t`Mark as read`}
              data-testid="mention-read"
            >
              <CheckIcon />
            </Button>
          )
        }
      >
        <span className={cn("text-foreground", !item.seen && "font-semibold")}>{subject || contact}</span>
        <span className="text-muted-foreground">
          {"  "}
          <Highlighted text={item.note.text} />
        </span>
      </FlatRow>
    </li>
  )
}

export function MentionsPage() {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const view = useMentionsView()
  const markAll = useMarkAllMentionsSeen()
  const dayLabel = useDayLabel()
  const [inboxId] = useInboxFilter()
  const product = useInboxes().data?.find((i) => i.id === inboxId)?.name
  const { mentions, all, tab, setPicked } = view
  const inScope = all.filter((m) => !inboxId || m.conversation.inbox_id === inboxId)
  const unseen = inboxId ? inScope.filter((m) => !m.seen).length : view.unseen
  const items = tab === "unread" ? inScope.filter((m) => !m.seen) : inScope
  const viewName = tab === "unread" ? t`Unread mentions` : t`All mentions`
  const fmt = new Intl.NumberFormat(i18n.locale)
  const unreadText = fmt.format(unseen)
  const where = product ?? t`every product`
  return (
    <PageLayout
      testId="mentions-panel"
      label={t`Mentions`}
      sidebar={
        <PageSidebar title={t`Mentions`}>
          <NavList label={t`Mentions`}>
            <NavGroup
              items={[
                {
                  key: "unread",
                  label: t`Unread`,
                  count: unseen,
                  hot: true,
                  current: tab === "unread",
                  onSelect: () => setPicked("unread"),
                  testId: "mentions-tab-unread",
                },
                {
                  key: "all",
                  label: t`All mentions`,
                  count: inScope.length,
                  current: tab === "all",
                  onSelect: () => setPicked("all"),
                  testId: "mentions-tab-all",
                },
              ]}
            />
            {canManage && <ProductsGroup />}
          </NavList>
          {!canManage && <SectionNav />}
        </PageSidebar>
      }
      aside={
        <InfoColumn label={t`Details`}>
          <InfoSection title={viewName} testId="mentions-info">
            <KV label={t`Unread`} value={unreadText} tone={unseen > 0 ? "brand" : undefined} />
            <KV label={t`All mentions`} value={fmt.format(inScope.length)} />
            <KV label={t`Product`} value={product ?? t`All products`} />
          </InfoSection>
        </InfoColumn>
      }
    >
      <PageTop
        actions={
          <Button
            variant="ghost"
            className="h-8 rounded-[9px] px-3 font-normal"
            disabled={unseen === 0 || markAll.isPending}
            onClick={() => markAll.mutate()}
            data-testid="mentions-mark-all"
          >
            <Trans>Mark all as read</Trans>
          </Button>
        }
      >
        <span className="font-medium text-foreground">
          <Trans>Mentions</Trans>
        </span>
        <TopSep />
        <span className="truncate">{viewName}</span>
        <TopSep />
        <span className="truncate">{product ?? t`All products`}</span>
      </PageTop>
      <section aria-label={viewName} className="@container/list max-w-[860px]">
        <PageHeading
          title={viewName}
          lede={
            <span data-testid="mentions-summary">
              <Plural value={items.length} one="# mention" other="# mentions" />
              {" · "}
              <Trans>{unreadText} unread</Trans>
              {" · "}
              {where}
            </span>
          }
        />
        <ErrorLine error={mentions.error} className="mt-4" />
        <ErrorLine error={markAll.error} className="mt-4" />
        {mentions.isPending && (
          <div className="mt-7 flex flex-col gap-3">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-5 w-full" />
            ))}
          </div>
        )}
        {mentions.data && inScope.length === 0 && (
          <FlatEmpty>
            <p className="m-0 font-medium text-foreground">
              <Trans>Nobody has mentioned you yet</Trans>
            </p>
            <p className="m-0">
              <Trans>When a teammate writes @ and your name in a note, the note shows up here.</Trans>
            </p>
          </FlatEmpty>
        )}
        {mentions.data && inScope.length > 0 && items.length === 0 && (
          <FlatEmpty testId="mentions-read">
            <p className="m-0">
              <Trans>You have read every mention.</Trans>
            </p>
            <Button
              variant="outline"
              className={look.btnSm}
              onClick={() => setPicked("all")}
              data-testid="mentions-see-all"
            >
              <Trans>See all mentions</Trans>
            </Button>
          </FlatEmpty>
        )}
        {items.length > 0 && (
          <div className="mt-7" data-testid="mention-list">
            {groupRuns(items, (m) => dayOf(m.note.created_at)).map((g) => (
              <ListGroup key={g.key} label={dayLabel(g.items[0].note.created_at)}>
                <ul className="m-0 list-none p-0" aria-label={dayLabel(g.items[0].note.created_at)}>
                  {g.items.map((m) => (
                    <FlatMentionRow key={m.note.id} item={m} />
                  ))}
                </ul>
              </ListGroup>
            ))}
          </div>
        )}
        <MoreButton query={mentions} />
      </section>
    </PageLayout>
  )
}
