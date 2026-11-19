import { plural } from "@lingui/core/macro"
import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { Link, type To } from "react-router"

import { inboxRoot } from "@/app/shell"
import { ContactAvatar, isVisitor, useContactName, VisitorAvatar } from "@/components/common"
import { formatDuration } from "@/components/common/text"
import { Empty, EmptyDescription } from "@/components/ui/empty"
import { docsPageLink, useDocsPages } from "@/features/docs/queries"
import { Block, BlockLink, Row, Tag, useClock } from "@/features/home/HomeBlock"
import { useMigrationsNeedingAttention } from "@/features/home/queries"
import { rememberChip, rememberInbox, type Chip } from "@/features/inbox/ChatList"
import { useConversations, useCounts } from "@/features/inbox/queries"
import { setRailView } from "@/features/inbox/railView"
import { usePresets } from "@/features/migrations/queries"
import { usePresetName } from "@/features/migrations/text"
import { dayKey, share, useReport, useWindowStats } from "@/features/reports/data"
import { incidentLink } from "@/features/status/queries"
import type { ConversationListItem, Home } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { slaState } from "@/lib/sla"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes, useMemberMap } from "@/lib/workspace"

const SHOWN = 6

export function mineFilters(inboxId: string): ConversationFilters {
  return {
    assignee: "me",
    status: ["open", "pending", "snoozed"],
    sort: "sla_due",
    purpose: "support",
    inbox_id: inboxId || undefined,
  }
}

function lateFilters(inboxId: string): ConversationFilters {
  return {
    status: ["open"],
    sla_state: ["breached"],
    sort: "sla_due",
    purpose: "support",
    inbox_id: inboxId || undefined,
  }
}

export function useMine(inboxId: string) {
  return useConversations(mineFilters(inboxId)).data?.pages.flatMap((p) => p.items)
}

export function useLate(inboxId: string) {
  const q = useConversations(lateFilters(inboxId))
  return q.data?.pages.flatMap((p) => p.items)
}

export function useOpenView() {
  const { apart } = useSession()
  return (chip: Chip, inboxId = "") => ({
    to: inboxRoot(apart) as To,
    onClick: () => {
      setRailView("conversations")
      rememberChip(chip)
      rememberInbox(inboxId)
    },
  })
}

function useConversationText() {
  const contactName = useContactName()
  const byInbox = new Map((useInboxes().data ?? []).map((i) => [i.id, i.name]))
  return {
    who: (c: ConversationListItem) => contactName(c, undefined, c.contact.name, c.contact.email),
    inbox: (c: ConversationListItem) => byInbox.get(c.inbox_id) ?? "",
    face: (c: ConversationListItem, name: string) =>
      isVisitor(c.contact.name, c.contact.email) ? (
        <VisitorAvatar className="size-7 text-[11px]" />
      ) : (
        <ContactAvatar id={c.contact.id} name={name} className="size-7 text-[11px] font-semibold" />
      ),
  }
}

function headline(name: string, subject: string) {
  return subject ? `${name}: ${subject}` : name
}

function isWaitingOnUs(c: ConversationListItem) {
  return c.status === "open" && c.last_message?.author_type === "contact"
}

export function MyConversations({ mine }: { mine: ConversationListItem[] }) {
  const { t, i18n } = useLingui()
  const clock = useClock()
  const text = useConversationText()
  const view = useOpenView()
  const open = new Intl.NumberFormat(i18n.locale).format(mine.length)
  const state = (c: ConversationListItem) =>
    c.status === "snoozed"
      ? t`snoozed`
      : c.status === "pending"
        ? t`waiting on the customer`
        : isWaitingOnUs(c)
          ? t`waiting on you`
          : t`open`
  const end = (c: ConversationListItem): { text: string; late?: boolean } => {
    if (c.status === "snoozed" && c.snooze_until) {
      const at = clock.at(c.snooze_until)
      return { text: t`Back ${at}` }
    }
    const since = clock.at(c.last_message_at ?? c.last_activity_at)
    if (c.status === "open" && c.sla?.due_at) {
      const left = (Date.parse(c.sla.due_at) - clock.now) / 1000
      const time = formatDuration(Math.abs(left), i18n.locale)
      if (slaState(c.sla, undefined, clock.now) === "breached") return { text: t`Late by ${time}`, late: true }
      if (left > 0) return { text: t`Reply in ${time}`, late: left <= 600 }
    }
    return { text: t`Since ${since}` }
  }
  return (
    <Block
      id="home-mine-title"
      title={<Trans>My conversations</Trans>}
      count={<Trans>{open} open</Trans>}
      action={
        <BlockLink {...view("mine")}>
          <Trans>Open in Inbox</Trans>
        </BlockLink>
      }
      testId="home-mine"
    >
      {mine.slice(0, SHOWN).map((c) => {
        const name = text.who(c)
        const e = end(c)
        return (
          <Row
            key={c.id}
            to={`/conversations/${c.id}`}
            media={text.face(c, name)}
            title={headline(name, c.subject)}
            description={[text.inbox(c), state(c)].filter(Boolean).join(" · ")}
            end={e.text}
            endTone={e.late ? "late" : undefined}
            conversation={c.id}
            testId={`home-mine-${c.id}`}
          />
        )
      })}
    </Block>
  )
}

type Waiting = { key: string; node: React.ReactNode }

export function useWaitingRows(h: Home, inboxId: string, late: ConversationListItem[] | undefined): Waiting[] {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const clock = useClock()
  const text = useConversationText()
  const view = useOpenView()
  const members = useMemberMap()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const counts = useCounts().data
  const docs = useDocsPages(inboxId, "", 7, "activity").data
  const attention = (useMigrationsNeedingAttention().data ?? []).filter((m) => !inboxId || m.inbox_id === inboxId)
  const presets = usePresets().data ?? []
  const presetName = usePresetName()
  const rows: Waiting[] = []
  for (const c of (late ?? []).slice(0, SHOWN)) {
    const name = text.who(c)
    const due = c.sla?.due_at
    const by = due ? formatDuration(Math.max(0, (clock.now - Date.parse(due)) / 1000), i18n.locale) : ""
    const owner = c.assignee_id ? (members.get(c.assignee_id)?.name ?? "") : t`Unassigned`
    rows.push({
      key: c.id,
      node: (
        <Row
          key={c.id}
          to={`/conversations/${c.id}`}
          media={
            <Tag tone="late">
              <Trans>Late</Trans>
            </Tag>
          }
          title={headline(name, c.subject)}
          description={[text.inbox(c), owner].filter(Boolean).join(" · ")}
          end={by}
          endTone="late"
          conversation={c.id}
          late={c.assignee_id ? undefined : c.id}
          testId={`home-late-${c.id}`}
        />
      ),
    })
  }
  const unassigned = (counts?.unassigned_inboxes ?? []).filter((u) => u.count > 0 && (!inboxId || u.id === inboxId))
  if (unassigned.length > 0) {
    const total = unassigned.reduce((n, u) => n + u.count, 0)
    const byInbox = new Map(inboxes.map((i) => [i.id, i.name]))
    const parts = unassigned
      .filter((u) => byInbox.has(u.id))
      .map((u) => {
        const name = byInbox.get(u.id)
        return plural(u.count, { one: `${name} · # waiting`, other: `${name} · # waiting` })
      })
      .join(", ")
    const one = unassigned.length === 1 ? unassigned[0]!.id : ""
    rows.push({
      key: "unassigned",
      node: (
        <Row
          key="unassigned"
          {...view("unassigned", one)}
          media={
            <Tag tone="plain">
              <Trans>Unassigned</Trans>
            </Tag>
          }
          title={plural(total, { one: "# conversation nobody has taken", other: "# conversations nobody has taken" })}
          description={parts}
          testId="home-unassigned"
        />
      ),
    })
  }
  const pages = new Map(h.status_pages.map((p) => [p.id, p.name]))
  for (const u of h.upcoming.filter((u) => u.kind === "incident" && u.status_page_id)) {
    rows.push({
      key: u.id,
      node: (
        <Row
          key={u.id}
          to={incidentLink(u.status_page_id!, u.id)}
          media={
            <Tag tone="warn">
              <Trans>Incident</Trans>
            </Tag>
          }
          title={u.title || "–"}
          description={pages.get(u.status_page_id!) ?? ""}
          end={clock.at(u.at)}
          testId={`home-incident-${u.id}`}
        />
      ),
    })
  }
  const questions = (docs?.pages.flatMap((p) => p.items) ?? []).filter((p) => p.unanswered_threads > 0)
  for (const p of questions.slice(0, 3)) {
    const path = (() => {
      try {
        const u = new URL(p.page)
        return `${u.host}${u.pathname}`
      } catch {
        return p.page
      }
    })()
    rows.push({
      key: `${p.inbox_id}:${p.page}`,
      node: (
        <Row
          key={`${p.inbox_id}:${p.page}`}
          to={docsPageLink(p.inbox_id, p.page)}
          media={
            <Tag tone="plain">
              <Trans>Discussion</Trans>
            </Tag>
          }
          title={p.title || path}
          description={[
            path,
            plural(p.unanswered_threads, {
              one: "# question needs a reply",
              other: "# questions need a reply",
            }),
          ].join(" · ")}
          testId="home-discussion"
        />
      ),
    })
  }
  if (canManage)
    for (const m of attention) {
      const source = presetName(m.preset, presets.find((p) => p.id === m.preset)?.name ?? m.preset)
      rows.push({
        key: m.id,
        node: (
          <Row
            key={m.id}
            to={`/migrations/${m.id}`}
            media={
              <Tag tone="plain">
                <Trans>Import</Trans>
              </Tag>
            }
            title={t`${source} import needs attention`}
            description={m.state_reason || m.source_address}
            testId={`home-import-${m.id}`}
          />
        ),
      })
    }
  return rows
}

export function WaitingForYou({ rows }: { rows: Waiting[] }) {
  const { i18n } = useLingui()
  return (
    <Block
      id="home-waiting-title"
      title={<Trans>Waiting for you</Trans>}
      count={rows.length > 0 ? new Intl.NumberFormat(i18n.locale).format(rows.length) : undefined}
      testId="home-waiting"
    >
      {rows.length > 0 ? (
        rows.map((r) => r.node)
      ) : (
        <Empty className="items-start p-4 text-start">
          <EmptyDescription>
            <Trans>Nothing yet. Once mail or chat is on, new conversations and incidents show up here.</Trans>
          </EmptyDescription>
        </Empty>
      )}
    </Block>
  )
}

function Figure({
  to,
  onClick,
  label,
  value,
  sub,
  subTone,
  testId,
}: {
  to: To
  onClick?: () => void
  label: React.ReactNode
  value: string
  sub?: React.ReactNode
  subTone?: "late"
  testId?: string
}) {
  return (
    <Link
      to={to}
      onClick={onClick}
      className="flex min-w-0 flex-col gap-0.5 border-e border-b border-line-soft px-4 py-3 outline-none transition-colors hover:bg-muted focus-visible:bg-muted"
      data-testid={testId}
    >
      <span className="truncate text-small text-muted-foreground">{label}</span>
      <b className="text-[20px] leading-7 font-semibold tracking-[-0.01em] tabular-nums">{value}</b>
      {sub && (
        <span
          className={cn("truncate text-caption", subTone === "late" ? "text-destructive" : "text-muted-foreground")}
        >
          {sub}
        </span>
      )}
    </Link>
  )
}

function lateLabel(lateText: string) {
  return <Trans>{lateText} late</Trans>
}

function Figures({ children }: { children: React.ReactNode }) {
  return (
    <div className="-me-px -mb-px grid grid-cols-3 max-md:grid-cols-2" data-testid="home-today-figures">
      {children}
    </div>
  )
}

export function Today({ h, late, mine }: { h: Home; late: number; mine: ConversationListItem[] }) {
  const { t, i18n } = useLingui()
  const { canManage, membership } = useSession()
  const view = useOpenView()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  const clock = useClock()
  const today = h.activity.days.at(-1)
  const key = dayKey(new Date(clock.now))
  const own = useReport({ from: key, to: key, granularity: "day", member_id: membership.member_id }, !canManage).data
  const stats = useWindowStats("", undefined).data
  const counts = useCounts().data
  const lateText = fmt.format(late)
  const lateSub = late > 0 ? lateLabel(lateText) : undefined
  const rating = (good: number, bad: number) => {
    const s = share(good, bad)
    return s === undefined ? "–" : pct.format(s)
  }
  const ratings = (rated: number) => <Plural value={rated} _0="no ratings" one="# rating" other="# ratings" />
  const firstReplies = (n: number) => (
    <Plural value={n} _0="no first replies" one="# first reply" other="# first replies" />
  )
  const reports = (
    <BlockLink to="/reports/today">
      <Trans>Reports</Trans>
    </BlockLink>
  )
  if (!canManage) {
    const v = own?.totals
    const median = v?.first_reply_median_seconds
    const good = v?.ratings?.good ?? 0
    const bad = v?.ratings?.bad ?? 0
    const myLate = mine.filter((c) => c.sla && slaState(c.sla, undefined, clock.now) === "breached").length
    const myLateText = fmt.format(myLate)
    return (
      <Block id="home-today-title" title={<Trans>Today</Trans>} action={reports} testId="home-today">
        <Figures>
          <Figure
            {...view("mine")}
            label={<Trans>Your open conversations</Trans>}
            value={fmt.format(counts?.mine ?? mine.filter((c) => c.status === "open").length)}
            sub={myLate > 0 ? lateLabel(myLateText) : undefined}
            subTone="late"
            testId="home-today-open"
          />
          <Figure
            to="/reports/today"
            label={<Trans>Your first reply</Trans>}
            value={median !== undefined ? formatDuration(median, i18n.locale) : "–"}
            sub={firstReplies(v?.first_replies ?? 0)}
            testId="home-today-first-reply"
          />
          <Figure
            to="/reports/ratings"
            label={<Trans>Your rating</Trans>}
            value={rating(good, bad)}
            sub={ratings(good + bad)}
            testId="home-today-rating"
          />
        </Figures>
      </Block>
    )
  }
  const opened = today?.conversations ?? 0
  const median = today?.median_first_reply_seconds
  const properties = h.analytics?.properties ?? []
  const visitors = properties.reduce((n, p) => n + p.realtime_visitors, 0)
  const b = h.broadcasts?.last_sent
  const name = b ? b.name || b.subject : ""
  return (
    <Block id="home-today-title" title={<Trans>Today</Trans>} action={reports} testId="home-today">
      <Figures>
        <Figure
          {...view("all")}
          label={<Trans>Open conversations</Trans>}
          value={fmt.format(h.inbox.open)}
          sub={lateSub}
          subTone="late"
          testId="home-today-open"
        />
        <Figure
          to="/reports/today"
          label={<Trans>Opened · closed</Trans>}
          value={`${fmt.format(opened)} · ${stats ? fmt.format(stats.closed) : "–"}`}
          testId="home-today-opened"
        />
        <Figure
          to="/reports/today"
          label={<Trans>First reply</Trans>}
          value={median !== undefined ? formatDuration(median, i18n.locale) : "–"}
          sub={firstReplies(today?.first_replies ?? 0)}
          testId="home-today-first-reply"
        />
        {properties.length > 0 && (
          <Figure
            to={properties.length === 1 ? `/analytics/${properties[0]!.id}/realtime` : "/analytics"}
            label={<Trans>Visitors now</Trans>}
            value={fmt.format(visitors)}
            sub={properties.length === 1 ? properties[0]!.name : undefined}
            testId="home-today-visitors"
          />
        )}
        <Figure
          to="/reports/ratings"
          label={<Trans>Rating</Trans>}
          value={rating(today?.good ?? 0, today?.bad ?? 0)}
          sub={ratings((today?.good ?? 0) + (today?.bad ?? 0))}
          testId="home-today-rating"
        />
        {b && (
          <Figure
            to={`/broadcasts/${b.id}`}
            label={<Trans>Last broadcast</Trans>}
            value={`${b.click_tracking ? fmt.format(b.clicked) : "–"} · ${fmt.format(b.replied)}`}
            sub={t`clicks · replies · ${name}`}
            testId="home-today-broadcast"
          />
        )}
      </Figures>
    </Block>
  )
}
