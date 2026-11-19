import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { MegaphoneIcon, MessageSquareIcon, StarIcon } from "lucide-react"

import { PersonAvatar, useContactName } from "@/components/common"
import { Block, Row, useClock } from "@/features/home/HomeBlock"
import { useOpenView } from "@/features/home/HomeWork"
import { useActivity } from "@/features/home/queries"
import { articleLink } from "@/features/help/queries"
import { useMigrations, usePresets } from "@/features/migrations/queries"
import { usePresetName } from "@/features/migrations/text"
import { incidentLink } from "@/features/status/queries"
import { useStatusText } from "@/features/status/text"
import type { ActivityItem, ConversationListItem, Home, StatusComponentStateValue } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

type Later = { key: string; at: number; node: React.ReactNode }

function Time({ children }: { children: React.ReactNode }) {
  return <span className="min-w-12 shrink-0 text-small font-semibold whitespace-nowrap tabular-nums">{children}</span>
}

export function useLaterRows(h: Home, mine: ConversationListItem[], inboxId: string): Later[] {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const clock = useClock()
  const contactName = useContactName()
  const byInbox = new Map((useInboxes().data ?? []).map((i) => [i.id, i.name]))
  const migrations = useMigrations().data?.pages.flatMap((p) => p.items) ?? []
  const presets = usePresets().data ?? []
  const presetName = usePresetName()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "numeric", minute: "2-digit" })
  const soon = (at: number) => at >= clock.now - 60_000 && at <= clock.endOfDay
  const rows: Later[] = []
  for (const c of mine) {
    if (c.status !== "snoozed" || !c.snooze_until) continue
    const at = Date.parse(c.snooze_until)
    if (!soon(at)) continue
    const name = contactName(c, undefined, c.contact.name, c.contact.email)
    rows.push({
      key: c.id,
      at,
      node: (
        <Row
          key={c.id}
          to={`/conversations/${c.id}`}
          media={<Time>{time.format(at)}</Time>}
          title={t`${name} comes back`}
          description={[t`Snoozed conversation`, byInbox.get(c.inbox_id)].filter(Boolean).join(" · ")}
          testId={`home-later-${c.id}`}
        />
      ),
    })
  }
  if (canManage) {
    for (const u of h.upcoming) {
      if (u.kind !== "broadcast") continue
      const at = Date.parse(u.at)
      if (!soon(at)) continue
      const name = u.title || "–"
      const n = u.recipients ?? 0
      rows.push({
        key: u.id,
        at,
        node: (
          <Row
            key={u.id}
            to={`/broadcasts/${u.id}`}
            media={<Time>{time.format(at)}</Time>}
            title={t`${name} goes out`}
            description={[t`Scheduled broadcast`, plural(n, { one: "# person", other: "# people" })].join(" · ")}
            testId={`home-later-${u.id}`}
          />
        ),
      })
    }
    for (const m of migrations) {
      if (inboxId && m.inbox_id !== inboxId) continue
      const at =
        m.state === "copying" && m.seconds_left !== undefined
          ? clock.now + m.seconds_left * 1000
          : m.state === "syncing" && m.sync_until
            ? Date.parse(m.sync_until)
            : undefined
      if (at === undefined || !soon(at)) continue
      const source = presetName(m.preset, presets.find((p) => p.id === m.preset)?.name ?? m.preset)
      const done = fmt.format(m.counts.imported)
      const total = fmt.format(m.counts.total)
      const when = time.format(at)
      rows.push({
        key: m.id,
        at,
        node: (
          <Row
            key={m.id}
            to={`/migrations/${m.id}`}
            media={<Time>{`~${when}`}</Time>}
            title={t`${source} import should finish`}
            description={m.counts.total > 0 ? t`${done} of ${total} copied` : m.source_address}
            testId={`home-later-${m.id}`}
          />
        ),
      })
    }
  }
  return rows.sort((a, b) => a.at - b.at)
}

export function LaterToday({ rows }: { rows: Later[] }) {
  return (
    <Block id="home-later-title" title={<Trans>Later today</Trans>} testId="home-later">
      {rows.map((r) => r.node)}
    </Block>
  )
}

const stateTone: Record<StatusComponentStateValue, string> = {
  operational: "text-ok",
  degraded: "text-note-ink",
  partial_outage: "text-note-ink",
  major_outage: "text-destructive",
  maintenance: "text-muted-foreground",
}

export function useProductRows(h: Home) {
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const byId = new Map(inboxes.map((i) => [i.id, i]))
  return h.products
    .flatMap((p) => {
      const inbox = byId.get(p.inbox_id)
      return inbox ? [{ p, inbox }] : []
    })
    .sort((a, b) => b.p.open - a.p.open)
}

export function Products({ h, rows }: { h: Home; rows: ReturnType<typeof useProductRows> }) {
  const { t, i18n } = useLingui()
  const statusText = useStatusText()
  const view = useOpenView()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pages = new Map(h.status_pages.map((p) => [p.inbox_id, p]))
  return (
    <Block id="home-products-title" title={<Trans>Products</Trans>} testId="home-products">
      {rows.map(({ p, inbox }) => {
        const page = pages.get(inbox.id)
        const incidents = h.upcoming.filter((u) => u.kind === "incident" && u.inbox_id === inbox.id).length
        const open = fmt.format(p.open)
        return (
          <Row
            key={inbox.id}
            {...view("waiting", inbox.id)}
            media={
              <span
                aria-hidden
                className="grid size-7 place-items-center rounded-md bg-muted2 text-small font-semibold text-white"
                style={inbox.branding.color ? { backgroundColor: inbox.branding.color } : undefined}
              >
                {inbox.name.trim().charAt(0).toUpperCase()}
              </span>
            }
            title={inbox.name}
            description={[
              t`${open} open`,
              incidents > 0 ? plural(incidents, { one: "# incident", other: "# incidents" }) : "",
            ]
              .filter(Boolean)
              .join(" · ")}
            end={
              page ? (
                <span
                  className={cn(
                    "block max-w-28 text-end leading-4 font-medium whitespace-normal",
                    stateTone[page.state]
                  )}
                >
                  {statusText.component[page.state]}
                </span>
              ) : undefined
            }
            testId={`home-product-${inbox.id}`}
          />
        )
      })}
    </Block>
  )
}

function activityLink(a: ActivityItem) {
  const { type, id, parent_id } = a.target
  if (type === "conversation") return `/conversations/${id}`
  if (type === "incident") return incidentLink(parent_id ?? "", id)
  if (type === "article") return articleLink(parent_id ?? "", id, a.detail?.locale)
  return `/broadcasts/${id}`
}

function useActivityText() {
  const { t } = useLingui()
  return (a: ActivityItem) => {
    const title = a.target.title || t`(no subject)`
    const who = a.actor?.name ?? ""
    switch (a.kind) {
      case "conversation.closed":
        return who ? t`${who} closed “${title}”` : t`“${title}” was closed`
      case "conversation.rated":
        return a.detail?.rating === "bad" ? t`“${title}” was rated bad` : t`“${title}” was rated good`
      case "incident.updated":
        return who ? t`${who} posted an update on “${title}”` : t`An update was posted on “${title}”`
      case "article.published":
        return who ? t`${who} published “${title}”` : t`“${title}” was published`
      default:
        return who ? t`${who} sent “${title}”` : t`“${title}” was sent`
    }
  }
}

function ActivityMedia({ a }: { a: ActivityItem }) {
  if (a.actor) return <PersonAvatar name={a.actor.name} className="size-7 text-[11px] font-semibold" />
  const Icon =
    a.kind === "conversation.rated" ? StarIcon : a.kind === "broadcast.sent" ? MegaphoneIcon : MessageSquareIcon
  return (
    <span aria-hidden className="grid size-7 place-items-center rounded-full bg-muted text-muted-foreground">
      <Icon className="size-3.5" />
    </span>
  )
}

export function useRecent(inboxId: string) {
  return useActivity(inboxId).data?.items ?? []
}

export function Recent({ items }: { items: ActivityItem[] }) {
  const clock = useClock()
  const text = useActivityText()
  return (
    <Block id="home-recent-title" title={<Trans>Recent</Trans>} testId="home-recent">
      {items.map((a) => (
        <Row
          key={a.id}
          to={activityLink(a)}
          media={<ActivityMedia a={a} />}
          title={text(a)}
          wrap
          description={clock.ago(a.at)}
          testId="home-recent-item"
        />
      ))}
    </Block>
  )
}
