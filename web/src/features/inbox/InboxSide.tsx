import { Trans, useLingui } from "@lingui/react/macro"
import { useLocation, useSearchParams } from "react-router"

import { inboxRoot, isChats } from "@/app/shell"
import {
  Dot,
  InfoSection,
  KV,
  NavGroup,
  NavList,
  PageSidebar,
  ProductsGroup,
  SectionNav,
  sourceOf,
  TopSep,
  useSourceText,
  type NavItem,
} from "@/components/common"
import { formatDateTime, formatDuration, useEnumText } from "@/components/common/text"
import { ContactAside } from "@/features/contact/ContactCard"
import { useConversation } from "@/features/conversation/queries"
import { isWaiting, useChipLabels, useChips, type Chip } from "@/features/inbox/ChatList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useCounts, useLoadedCount, useStats } from "@/features/inbox/queries"
import { resetSourceFilter } from "@/features/inbox/SourceFilter"
import { viewSearch } from "@/features/inbox/views/filters"
import { byPosition, useViews } from "@/features/inbox/views/queries"
import { useSession } from "@/lib/session"
import { useChannelMap, useInboxes, useMemberMap } from "@/lib/workspace"

export function useChipCounts(inboxId: string) {
  const counts = useCounts(["conversation"]).data
  const scope = { kind: ["conversation" as const], purpose: "support" as const, inbox_id: inboxId || undefined }
  const waiting = useLoadedCount({ ...scope, status: ["open"] }, isWaiting)
  const unassigned = useLoadedCount({ ...scope, status: ["open"], assignee: "unassigned" })
  return (k: Chip): number | undefined =>
    k === "waiting"
      ? waiting
      : k === "unassigned"
        ? inboxId
          ? unassigned
          : counts?.unassigned
        : k === "mine"
          ? inboxId
            ? undefined
            : counts?.mine
          : k === "teams"
            ? counts?.mine_teams
            : undefined
}

export function ConversationsSidebar({
  current,
  onChip,
  inboxId,
}: {
  current: Chip | null
  onChip: (k: Chip) => void
  inboxId: string
}) {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const { apart, canManage } = useSession()
  const labels = useChipLabels()
  const chips = useChips()
  const count = useChipCounts(inboxId)
  const all = useCounts().data
  const views = [...(useViews().data ?? [])].sort(byPosition)
  const chats = isChats(pathname, apart)
  const activeView = chats ? params.get("view") : null
  const outboundCount = inboxId
    ? (all?.outbound_inboxes.find((c) => c.id === inboxId)?.count ?? 0)
    : (all?.outbound ?? 0)
  const outbound = chats && !activeView && params.get("origin") === "outbound"
  const items: NavItem[] = chips.map((k) => ({
    key: k,
    label: labels[k],
    count: count(k),
    hot: k === "waiting" || k === "unassigned",
    current: chats && !activeView && !outbound && current === k,
    onSelect: () => onChip(k),
    testId: `page-view-${k}`,
  }))
  if (outboundCount || outbound)
    items.push({
      key: "outbound",
      label: t`Replies to mail we sent`,
      to: `${inboxRoot(apart)}?origin=outbound&status=open`,
      onSelect: resetSourceFilter,
      count: outboundCount,
      current: outbound,
      testId: "page-view-outbound",
    })
  const saved: NavItem[] = views.map((v) => ({
    key: v.id,
    label: v.name,
    to: { pathname: inboxRoot(apart), search: viewSearch(v.id, v.filters) },
    count: v.count_unavailable ? undefined : v.count,
    current: v.id === activeView,
    testId: "view-link",
  }))
  return (
    <PageSidebar title={t`Conversations`} searchLabel={t`Search or jump to…`}>
      <NavList label={t`Conversations`}>
        <NavGroup items={items} />
        {saved.length > 0 && <NavGroup title={t`Saved views`} items={saved} />}
        {canManage && <ProductsGroup />}
      </NavList>
      {!canManage && <SectionNav />}
    </PageSidebar>
  )
}

export function ListInfo({ title, inboxId }: { title: string; inboxId: string }) {
  const { t } = useLingui()
  const labels = useChipLabels()
  const count = useChipCounts(inboxId)
  const n = (k: Chip) => count(k) ?? "–"
  return (
    <>
      <InfoSection title={title} testId="list-summary">
        <KV label={labels.waiting} value={n("waiting")} tone="brand" />
        {!inboxId && <KV label={t`Assigned to you`} value={n("mine")} />}
        <KV label={t`Unassigned`} value={n("unassigned")} />
        <KV label={labels.teams} value={n("teams")} />
      </InfoSection>
      <TodayInfo inboxId={inboxId} />
    </>
  )
}

function TodayInfo({ inboxId }: { inboxId: string }) {
  const { t, i18n } = useLingui()
  const stats = useStats(inboxId).data
  if (!stats) return null
  const fmt = new Intl.NumberFormat(i18n.locale)
  const median = stats.median_first_reply_seconds
  const good = stats.ratings.good
  const bad = stats.ratings.bad
  return (
    <InfoSection title={t`Today`} testId="day-stats">
      <KV label={t`Replies`} value={fmt.format(stats.replies)} />
      <KV label={t`First reply, median`} value={median !== undefined ? formatDuration(median, i18n.locale) : "–"} />
      <KV label={t`Closed`} value={fmt.format(stats.closed)} />
      {good + bad > 0 && (
        <KV
          label={t`Satisfied`}
          value={new Intl.NumberFormat(i18n.locale, { style: "percent" }).format(good / (good + bad))}
          tone="ok"
        />
      )}
    </InfoSection>
  )
}

export function ConversationEyebrow({ id }: { id: string }) {
  const text = useEnumText()
  const sourceText = useSourceText()
  const c = useConversation(id).data
  const inbox = useInboxes().data?.find((i) => i.id === c?.inbox_id)
  const channel = useChannelMap().get(c?.channel_id ?? "")
  if (!c) return null
  const source = sourceOf(c, channel)
  return (
    <>
      {inbox && (
        <>
          <Dot color={inbox.branding.color} className="size-2 rounded-[2px]" />
          <span className="truncate font-medium text-foreground">{inbox.name}</span>
          <TopSep />
        </>
      )}
      {source && (
        <>
          <span className="shrink-0">{sourceText[source]}</span>
          <TopSep />
        </>
      )}
      <span className="truncate">{text.statusOf(c.status, c.kind)}</span>
    </>
  )
}

export function ConversationInfo({
  id,
  panel,
  onPanel,
}: {
  id: string
  panel: boolean
  onPanel: (open: boolean) => void
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const sourceText = useSourceText()
  const members = useMemberMap()
  const [inboxId] = useInboxFilter()
  const c = useConversation(id).data
  const inbox = useInboxes().data?.find((i) => i.id === c?.inbox_id)
  const channel = useChannelMap().get(c?.channel_id ?? "")
  if (!c) return null
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const source = sourceOf(c, channel)
  return (
    <>
      {panel && (
        <div
          className="mb-3 border-b border-line-soft [&>div]:overflow-visible [&>div>div]:px-0 [&>div>header]:ps-0 [&>div>header]:pt-0 [&>div>section]:px-0 [&>div>section:last-of-type]:border-b-0"
          data-testid="contact-panel"
        >
          <ContactAside
            key={c.contact_id}
            contactId={c.contact_id}
            conversationId={c.id}
            onClose={() => onPanel(false)}
          />
        </div>
      )}
      <InfoSection
        title={t`Conversation`}
        action={
          !panel && (
            <button
              type="button"
              onClick={() => onPanel(true)}
              className="rounded-sm text-caption font-normal text-fg2 underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
              data-testid="show-contact"
            >
              <Trans>Contact</Trans>
            </button>
          )
        }
        testId="conversation-info"
      >
        <KV label={t`Status`} value={text.statusOf(c.status, c.kind)} />
        <KV
          label={t`Assignee`}
          value={assignee ? assignee.name || assignee.email : t`Unassigned`}
          tone={assignee ? undefined : "warn"}
        />
        {inbox && !inboxId && <KV label={t`Product`} value={inbox.name} />}
        {source && <KV label={t`Channel`} value={sourceText[source]} />}
        <KV label={t`Started`} value={formatDateTime(c.created_at, i18n.locale)} />
      </InfoSection>
    </>
  )
}
