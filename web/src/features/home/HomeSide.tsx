import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { Link, type To } from "react-router"

import { byPresence } from "@/app/TeamMenu"
import { useGo } from "@/app/Rail"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useBroadcastsStatus } from "@/features/broadcasts/BroadcastParts"
import { entryLink } from "@/features/changelog/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useDrafts, useMentions } from "@/features/inbox/queries"
import { MigrationBar } from "@/features/migrations/parts"
import { percentOf, RUNNING, useMigrations, usePresets } from "@/features/migrations/queries"
import { usePresetName } from "@/features/migrations/text"
import { Face } from "@/features/settings/ui"
import { incidentLink } from "@/features/status/queries"
import { useStatusText } from "@/features/status/text"
import type { Home, HomeUpcoming, StatusComponentStateValue } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMembers } from "@/lib/workspace"

export const cardClass = "rounded-2xl border bg-card"
export const eyebrowClass = "m-0 text-small font-semibold tracking-[0.04em] text-muted-foreground uppercase"
const titleClass = "m-0 text-body font-semibold"

export function Arrow() {
  return (
    <span aria-hidden className="inline-block rtl:-scale-x-100">
      →
    </span>
  )
}

export function TextLink({ to, children, testId }: { to: To; children: React.ReactNode; testId?: string }) {
  return (
    <Link
      to={to}
      className="shrink-0 text-small font-medium whitespace-nowrap text-muted-foreground transition-colors hover:text-foreground hover:underline"
      data-testid={testId}
    >
      {children} <Arrow />
    </Link>
  )
}

const rowClass =
  "-mx-2 flex min-h-9 w-[calc(100%+1rem)] items-center gap-2.5 rounded-lg px-2 text-start outline-none transition-colors hover:bg-surface focus-visible:bg-surface"

export function NeedsYou({ className }: { className?: string }) {
  const { t, i18n } = useLingui()
  const go = useGo()
  const [inboxId] = useInboxFilter()
  const mentions = (useMentions().data?.pages[0]?.items ?? []).filter(
    (m) => !m.seen && (!inboxId || m.conversation.inbox_id === inboxId)
  ).length
  const botDrafts = Number(useDrafts({ author: "bot" }).data?.pages[0]?.total ?? 0)
  const assistantDrafts = Number(useDrafts({ author: "assistant" }).data?.pages[0]?.total ?? 0)
  const drafts = botDrafts + assistantDrafts
  const total = mentions + drafts
  if (total === 0) return null
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <section
      aria-label={t`Needs you`}
      className={cn(cardClass, "flex flex-col gap-1 px-[18px] py-4", className)}
      data-testid="home-approvals"
    >
      <div className="flex items-baseline justify-between pb-1.5">
        <h2 className={titleClass}>
          <Trans>Needs you</Trans>
        </h2>
        <span className="text-caption text-faint">
          <Plural value={total} one="# item" other="# items" />
        </span>
      </div>
      {mentions > 0 && (
        <Link to="/mentions" className={rowClass} data-testid="home-mention">
          <span className="min-w-5 text-end font-mono font-medium">{fmt.format(mentions)}</span>
          <span className="min-w-0 flex-1">
            <Plural value={mentions} one="Mention of you" other="Mentions of you" />
          </span>
          <span className="text-faint">
            <Arrow />
          </span>
        </Link>
      )}
      {drafts > 0 && (
        <button type="button" onClick={() => go("assistants")} className={rowClass} data-testid="home-drafts">
          <span className="min-w-5 text-end font-mono font-medium">{fmt.format(drafts)}</span>
          <span className="min-w-0 flex-1">
            <Plural value={drafts} one="Assistant draft to review" other="Assistant drafts to review" />
          </span>
          <span className="text-faint">
            <Arrow />
          </span>
        </button>
      )}
    </section>
  )
}

const stateTone: Record<StatusComponentStateValue, string> = {
  operational: "var(--success)",
  degraded: "var(--warning)",
  partial_outage: "var(--warning)",
  major_outage: "var(--destructive)",
  maintenance: "#2563eb",
}

type Day = { date: string; state: StatusComponentStateValue }

function Strip({ days, length }: { days: Day[]; length: number }) {
  const slots = [...days].sort((a, b) => a.date.localeCompare(b.date)).slice(-length)
  return (
    <span aria-hidden className="grid gap-0.5" style={{ gridTemplateColumns: `repeat(${length}, minmax(0, 1fr))` }}>
      {Array.from({ length }, (_, i) => {
        const s = slots[i - (length - slots.length)]
        const last = i === length - 1
        return (
          <span
            key={i}
            className={cn("h-3.5 rounded-[2px]", !s && "bg-muted")}
            style={s ? { backgroundColor: stateTone[s.state], opacity: last ? 1 : 0.6 } : undefined}
          />
        )
      })}
    </span>
  )
}

export function Status({ h, className }: { h: Home; className?: string }) {
  const { t, i18n } = useLingui()
  const statusText = useStatusText()
  const [inboxId] = useInboxFilter()
  const pages = inboxId ? h.status_pages.filter((p) => p.inbox_id === inboxId) : h.status_pages
  const issues = pages.flatMap((p) => p.components.filter((c) => c.state !== "operational").map((c) => ({ p, c })))
  const scoped = inboxId ? pages[0] : undefined
  const count = issues.length
  if (issues.length === 0 && !scoped) return null
  const length = h.period.days
  const fine = pages.flatMap((p) => p.components.filter((c) => c.state === "operational").map((c) => c.name))
  const names = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(fine)
  const uptimes = pages.flatMap((p) => p.days.flatMap((d) => (d.uptime === undefined ? [] : [d.uptime])))
  const uptime =
    uptimes.length > 0
      ? new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 1 }).format(
          uptimes.reduce((a, b) => a + b, 0) / uptimes.length / 100
        )
      : ""
  const shown = new Intl.NumberFormat(i18n.locale).format(length)
  const rest = [fine.length > 0 ? t`${names} operational` : "", uptime ? t`uptime ${uptime} over ${shown} days` : ""]
    .filter(Boolean)
    .join(" · ")
  return (
    <section
      aria-label={t`Status`}
      className={cn(cardClass, "flex flex-col gap-2.5 px-[18px] py-4", className)}
      data-testid="home-status"
    >
      <div className="flex items-baseline justify-between gap-2">
        <h2 className={titleClass}>
          <Trans>Status</Trans>
        </h2>
        <span className={cn("text-caption font-semibold", issues.length > 0 ? "text-note-ink" : "text-success")}>
          {issues.length > 0 ? <Plural value={count} one="# issue" other="# issues" /> : <Trans>All operational</Trans>}
        </span>
      </div>
      {issues.map(({ p, c }, i) => (
        <div key={c.id} className="flex flex-col gap-2 rounded-[10px] bg-note px-3 py-2.5">
          <span className="flex justify-between gap-2 text-small">
            <span className="min-w-0 truncate font-semibold">
              {pages.length > 1 ? `${p.name} · ${c.name}` : c.name}
            </span>
            <span className="shrink-0 font-semibold text-note-ink">{statusText.component[c.state]}</span>
          </span>
          <Strip days={c.days} length={length} />
          <span className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              className="h-[34px] rounded-lg bg-brand-solid px-3 font-semibold text-white hover:bg-brand-solid/90"
              render={<Link to="/incidents" />}
              data-testid={i === 0 ? "home-status-update" : undefined}
            >
              <Trans>Post status update</Trans>
            </Button>
          </span>
        </div>
      ))}
      {rest && <span className="text-caption text-faint">{rest}</span>}
    </section>
  )
}

function upcomingLink(u: HomeUpcoming) {
  if (u.kind === "broadcast") return `/broadcasts/${u.id}`
  if (u.kind === "whats_new") return entryLink(u.changelog_id ?? "", u.id, u.locale)
  return incidentLink(u.status_page_id ?? "", u.id)
}

export function ComingUp({ h, mailOnly, className }: { h: Home; mailOnly: boolean; className?: string }) {
  const { t, i18n } = useLingui()
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const byId = new Map(inboxes.map((i) => [i.id, i.name]))
  const many = inboxes.length > 1 && !inboxId
  const s = useBroadcastsStatus()
  const presets = usePresets().data ?? []
  const presetName = usePresetName()
  const running = (useMigrations().data?.pages.flatMap((p) => p.items) ?? []).filter(
    (m) => RUNNING.includes(m.state) && (!inboxId || m.inbox_id === inboxId)
  )
  const upcoming = (mailOnly ? h.upcoming.filter((u) => u.kind === "broadcast") : h.upcoming).slice(0, 3)
  if (running.length === 0 && upcoming.length === 0) return null
  const mon = new Intl.DateTimeFormat(i18n.locale, { month: "short" })
  const day = new Intl.DateTimeFormat(i18n.locale, { day: "2-digit" })
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const fmt = new Intl.NumberFormat(i18n.locale)
  const held = !s || s.status === "allowed" ? "" : s.status === "suspended" ? t`paused` : t`waits for approval`
  const meta = (u: HomeUpcoming) => {
    const when = time.format(new Date(u.at))
    const product = many ? byId.get(u.inbox_id) : undefined
    if (u.kind === "broadcast") {
      const people = fmt.format(u.recipients ?? 0)
      return [product, t`${people} recipients`, when, held].filter(Boolean).join(" · ")
    }
    const label = u.kind === "maintenance" ? t`Maintenance` : u.kind === "incident" ? t`Incident` : t`What’s new`
    return [product, label, when].filter(Boolean).join(" · ")
  }
  return (
    <section
      aria-label={t`Coming up`}
      className={cn(cardClass, "flex flex-col gap-3 px-[18px] py-4", className)}
      data-testid="home-upcoming"
    >
      <h2 className={titleClass}>
        <Trans>Coming up</Trans>
      </h2>
      {running.map((m) => {
        const source = presetName(m.preset, presets.find((p) => p.id === m.preset)?.name ?? m.preset)
        const percent = percentOf(m)
        return (
          <Link
            key={m.id}
            to={`/migrations/${m.id}`}
            className="flex flex-col gap-1.5 border-b pb-3 outline-none hover:underline-offset-2 focus-visible:underline"
            data-testid={`home-migration-${m.id}`}
          >
            <span className="flex justify-between gap-2">
              <span className="min-w-0 truncate font-medium">{t`Importing ${source} history`}</span>
              {m.counts.total > 0 && (
                <span className="shrink-0 font-mono text-caption text-faint">{`${percent}%`}</span>
              )}
            </span>
            <MigrationBar m={m} thin />
            <span className="truncate text-caption text-faint" dir="auto">
              {m.source_address}
            </span>
          </Link>
        )
      })}
      {upcoming.map((u) => {
        const at = new Date(u.at)
        return (
          <Link
            key={u.id}
            to={upcomingLink(u)}
            className="grid grid-cols-[44px_minmax(0,1fr)] items-center gap-3 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid={`home-upcoming-${u.id}`}
          >
            <span className="flex flex-col items-center rounded-lg bg-muted py-[3px] leading-[1.15]">
              <span className="text-[10px] tracking-[0.06em] text-faint uppercase">{mon.format(at)}</span>
              <span className="font-semibold tabular-nums">{day.format(at)}</span>
            </span>
            <span className="flex min-w-0 flex-col">
              <span className="truncate font-medium">{u.title || "–"}</span>
              <span className="truncate text-caption text-faint">{meta(u)}</span>
            </span>
          </Link>
        )
      })}
    </section>
  )
}

export function Publish({ h, mailOnly, className }: { h: Home; mailOnly: boolean; className?: string }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const byId = new Map(inboxes.map((i) => [i.id, i.name]))
  const b = h.broadcasts?.last_sent
  const ideas = mailOnly ? [] : (h.ideas ?? []).slice(0, 3)
  if (!b && ideas.length === 0) return null
  const fmt = new Intl.NumberFormat(i18n.locale)
  const sentOn = b
    ? new Intl.DateTimeFormat(i18n.locale, { weekday: "short", day: "numeric", month: "short" }).format(
        new Date(b.sent_at)
      )
    : ""
  const name = b ? b.name || b.subject : ""
  return (
    <section
      aria-label={t`Publish`}
      className={cn(cardClass, "flex flex-col gap-3 px-[18px] py-4", className)}
      data-testid="home-publish"
    >
      <h2 className={titleClass}>
        <Trans>Publish</Trans>
      </h2>
      {b && (
        <Link
          to={`/broadcasts/${b.id}`}
          className="flex flex-col gap-2 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="home-broadcast"
        >
          <span className="flex items-baseline justify-between gap-2 text-small">
            <span className="min-w-0 truncate text-muted-foreground">
              <Trans>Last broadcast · {name}</Trans>
            </span>
            <span className="shrink-0 text-caption text-faint">{sentOn}</span>
          </span>
          <span className="grid grid-cols-3 gap-2">
            {(
              [
                [fmt.format(b.sent), t`sent`],
                [b.click_tracking ? fmt.format(b.clicked) : "–", b.click_tracking ? t`clicked` : t`clicks not tracked`],
                [fmt.format(b.replied), t`replied`],
              ] as const
            ).map(([value, label]) => (
              <span key={label} className="flex min-w-0 flex-col">
                <b className="font-mono font-medium">{value}</b>
                <span className="truncate text-caption text-faint">{label}</span>
              </span>
            ))}
          </span>
        </Link>
      )}
      {ideas.length > 0 && (
        <div className={cn("flex flex-col gap-2.5", b && "border-t pt-3")} data-testid="home-asks">
          <span className="flex items-baseline justify-between text-small">
            <span className="text-muted-foreground">
              <Trans>Most asked for</Trans>
            </span>
            <Link
              to="/board"
              className="text-caption font-medium text-muted-foreground hover:text-foreground hover:underline"
            >
              <Trans>Roadmap</Trans> <Arrow />
            </Link>
          </span>
          {ideas.map((idea) => {
            const meta = [inboxes.length > 1 ? byId.get(idea.inbox_id) : undefined, text.stage[idea.stage]]
              .filter(Boolean)
              .join(" · ")
            const votes = fmt.format(idea.votes)
            return (
              <div key={idea.id} className="grid grid-cols-[44px_minmax(0,1fr)] items-center gap-3">
                <span className="rounded-lg bg-muted py-[3px] text-center font-mono text-small">{`▲${votes}`}</span>
                <span className="flex min-w-0 flex-col">
                  <span className="truncate font-medium">{idea.title}</span>
                  <span className="truncate text-caption text-faint">{meta}</span>
                </span>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

export function OnlineNow({ h, className }: { h: Home; className?: string }) {
  const { t, i18n } = useLingui()
  const { membership } = useSession()
  const members = useMembers().data ?? []
  const load = new Map(h.team.members.map((m) => [m.member_id, m.open]))
  const online = members.filter((m) => m.online).sort(byPresence)
  const away = members.filter((m) => !m.online)
  const awayOpen = away.reduce((n, m) => n + (load.get(m.id) ?? 0), 0)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const off = fmt.format(away.length)
  const offOpen = fmt.format(awayOpen)
  return (
    <section
      aria-label={t`Team`}
      className={cn(cardClass, "flex flex-col gap-2.5 px-[18px] py-4", className)}
      data-testid="home-team"
    >
      <div className="flex items-baseline justify-between gap-2">
        <h2 className={titleClass}>
          <Trans>Online now</Trans>
        </h2>
        <Link
          to="/team"
          className="text-caption font-medium text-muted-foreground hover:text-foreground hover:underline"
        >
          {away.length === 0 ? t`Team` : awayOpen > 0 ? t`+${off} offline · ${offOpen} open` : t`+${off} offline`}
        </Link>
      </div>
      {online.length === 0 ? (
        <p className="m-0 text-small text-faint">
          <Trans>Nobody is online right now.</Trans>
        </p>
      ) : (
        online.slice(0, 6).map((m) => {
          const name = m.name || m.email
          const open = load.get(m.id) ?? 0
          return (
            <Link
              key={m.id}
              to={`/team/${m.id}`}
              className="flex items-center gap-2.5 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring"
              data-testid={`home-member-${m.id}`}
            >
              <Face id={m.id} name={name} online={m.online} away={m.availability === "away"} className="size-7" />
              <span className="min-w-0 flex-1 truncate">{m.id === membership.member_id ? t`${name} (you)` : name}</span>
              <span className="shrink-0 font-mono text-caption text-faint">
                <Plural value={open} one="# open" other="# open" />
              </span>
            </Link>
          )
        })
      )}
    </section>
  )
}
