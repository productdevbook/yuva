import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon } from "lucide-react"
import { useRef } from "react"
import { Link, useSearchParams } from "react-router"

import {
  ContactAvatar,
  InfoColumn,
  InfoSection,
  isVisitor,
  Kbd,
  MoreLink,
  PageHeading,
  PageLayout,
  PageTop,
  SectionHeading,
  toast,
  TopSep,
  useContactName,
  VisitorAvatar,
} from "@/components/common"
import { HOME_KEYS } from "@/components/common/ShortcutSheet"
import { Stat, Stats } from "@/components/common/Stats"
import { formatDuration, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useBroadcastsStatus } from "@/features/broadcasts/BroadcastParts"
import { cardClass, ComingUp, NeedsYou, OnlineNow, Publish, Status, TextLink } from "@/features/home/HomeSide"
import { SiteRow } from "@/features/home/SiteCard"
import { useHome } from "@/features/home/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useBulkUpdateConversations } from "@/features/inbox/queries"
import { share } from "@/features/reports/data"
import { hubTarget, useHubPlaces } from "@/features/setup/SetupHub"
import { orderByFocus, type HubId, type SetupState } from "@/features/setup/setup"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import type { Home } from "@/lib/api"
import { useRealtimeStatus } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes, useMembers } from "@/lib/workspace"

const PERIODS = [7, 14, 30] as const
type Period = (typeof PERIODS)[number]

const boxClass = "overflow-hidden rounded-[10px] border bg-background"

function usePeriod(): [Period, (p: Period) => void] {
  const [params, setParams] = useSearchParams()
  const raw = Number(params.get("days"))
  const days = (PERIODS as readonly number[]).includes(raw) ? (raw as Period) : 14
  const set = (p: Period) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (p === 14) next.delete("days")
        else next.set("days", String(p))
        return next
      },
      { replace: true }
    )
  return [days, set]
}

function isFresh(h: Home, mailOnly: boolean) {
  return (
    !mailOnly &&
    !!h.setup &&
    h.inbox.open === 0 &&
    h.activity.conversations === 0 &&
    h.activity.previous.conversations === 0
  )
}

function useAgo() {
  const { i18n } = useLingui()
  const now = useNow(60_000)
  return (iso: string) => formatDuration(Math.max(0, (now - Date.parse(iso)) / 1000), i18n.locale)
}

function LiveState({ failed, updatedAt }: { failed: boolean; updatedAt: number }) {
  const { i18n } = useLingui()
  const status = useRealtimeStatus()
  const time = updatedAt
    ? new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" }).format(new Date(updatedAt))
    : ""
  const offline = failed || status === "offline"
  return (
    <span role="status" className="inline-flex min-w-0 items-center gap-2" data-testid="home-live">
      <span
        aria-hidden
        className={cn(
          "size-[7px] shrink-0 rounded-full",
          offline ? "bg-warn" : status === "connecting" ? "bg-muted2/60" : "bg-ok"
        )}
      />
      <span className={cn("truncate", !offline && status !== "connecting" && "text-foreground")}>
        {offline ? (
          time ? (
            <Trans>Offline · last updated {time}</Trans>
          ) : (
            <Trans>Offline</Trans>
          )
        ) : status === "connecting" ? (
          <Trans>Connecting</Trans>
        ) : (
          <Trans>Live</Trans>
        )}
      </span>
    </span>
  )
}

function Tail({ h, mailOnly, fresh }: { h?: Home; mailOnly: boolean; fresh: boolean }) {
  const { i18n } = useLingui()
  const ago = useAgo()
  const s = useBroadcastsStatus()
  if (!h) return <Trans>Loading your day…</Trans>
  if (mailOnly) {
    if (!s || s.mode === "off") return null
    if (s.status === "suspended") return <Trans>Sending is suspended.</Trans>
    if (s.status !== "allowed") return <Trans>Broadcasts wait for the operator’s approval.</Trans>
    const next = h.upcoming.find((u) => u.kind === "broadcast")
    if (!next) return <Trans>No broadcast is scheduled.</Trans>
    const when = new Intl.DateTimeFormat(i18n.locale, { weekday: "long", day: "numeric", month: "long" }).format(
      new Date(next.at)
    )
    return <Trans>Next broadcast goes out {when}.</Trans>
  }
  if (fresh) {
    const left = (h.setup?.items ?? []).filter((i) => !i.done).length
    return <Plural value={left} one="# setup step left." other="# setup steps left." />
  }
  const oldest = h.inbox.oldest_waiting
  const waiting = h.inbox.waiting
  if (waiting === 0 || !oldest) return <Trans>Nobody is waiting.</Trans>
  const waited = ago(oldest.since)
  return (
    <>
      <Plural value={waiting} one="# conversation is waiting." other="# conversations are waiting." />{" "}
      <Trans>Oldest has waited {waited}.</Trans>
    </>
  )
}

function Yesterday({ h }: { h: Home }) {
  const { t } = useLingui()
  const y = h.activity.days.at(-2)
  if (!y) return null
  const created = y.conversations
  const bad = y.bad
  return (
    <p className="m-0 mt-1 text-small text-muted-foreground" data-testid="home-yesterday">
      <span className="text-muted2">{t`Yesterday`}:</span>{" "}
      <Link to="/reports/today" className="whitespace-nowrap hover:text-foreground hover:underline">
        <Plural value={created} one="# new" other="# new" />
      </Link>
      {bad > 0 && (
        <>
          {" · "}
          <Link to="/reports/ratings" className="whitespace-nowrap text-warn hover:underline">
            <Plural value={bad} one="# bad rating" other="# bad ratings" />
          </Link>
        </>
      )}
    </p>
  )
}

function Scope({ mailOnly }: { mailOnly: boolean }) {
  const [inboxId, setInboxId] = useInboxFilter()
  const products = (useInboxes().data ?? []).filter(isSupport)
  const product = products.find((i) => i.id === inboxId)?.name
  if (mailOnly || products.length < 2) return null
  return (
    <div className="mt-3.5 inline-flex max-w-full flex-wrap items-center gap-x-2 gap-y-1 self-start rounded-lg border px-2.5 py-1.5 text-small text-fg2">
      {product ? (
        <>
          <span className="min-w-0 truncate">
            <Trans>Showing {product} only.</Trans>
          </span>
          <button
            type="button"
            onClick={() => setInboxId("")}
            className="rounded-sm px-1 text-foreground underline underline-offset-4 outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="home-scope"
          >
            <Trans>Show all products</Trans>
          </button>
        </>
      ) : (
        <>
          <span>
            <Trans>Showing every product.</Trans>
          </span>
          <span className="text-muted-foreground max-md:hidden">
            <Trans>Pick one at the top of the page.</Trans>
          </span>
        </>
      )}
    </div>
  )
}

function HomeHeader({
  h,
  mailOnly,
  fresh,
  failed,
  updatedAt,
}: {
  h?: Home
  mailOnly: boolean
  fresh: boolean
  failed: boolean
  updatedAt: number
}) {
  const { t, i18n } = useLingui()
  const { me } = useSession()
  const now = useNow(60_000)
  const first = (me.person.name || me.person.email).split(/[\s@]/)[0]
  const hour = new Date(now).getHours()
  const greeting =
    hour < 12 ? t`Good morning, ${first}.` : hour < 18 ? t`Good afternoon, ${first}.` : t`Good evening, ${first}.`
  const date = new Intl.DateTimeFormat(i18n.locale, { weekday: "long", day: "numeric", month: "long" }).format(now)
  return (
    <>
      <PageTop>
        <span className="max-md:hidden">
          <Trans>Home</Trans>
        </span>
        <span className="max-md:hidden">
          <TopSep />
        </span>
        <span className="truncate">{date}</span>
        <TopSep />
        <LiveState failed={failed} updatedAt={updatedAt} />
      </PageTop>
      <div className="mt-1 flex flex-col">
        <PageHeading
          title={greeting}
          lede={
            <span data-testid="home-summary">
              <Tail h={h} mailOnly={mailOnly} fresh={fresh} />
            </span>
          }
        />
        {h && !mailOnly && !fresh && <Yesterday h={h} />}
        <Scope mailOnly={mailOnly} />
      </div>
    </>
  )
}

function RefreshAlert({ h, updatedAt, retry }: { h?: Home; updatedAt: number; retry: () => void }) {
  const { i18n } = useLingui()
  const time = updatedAt
    ? new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" }).format(new Date(updatedAt))
    : ""
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-3 rounded-xl border border-note-border bg-note px-4 py-3.5 text-small text-note-ink"
      data-testid="home-error"
    >
      <span className="flex-[1_1_240px]">
        {h && time ? (
          <Trans>Couldn’t refresh since {time}. The figures below may be out of date; your inbox still works.</Trans>
        ) : (
          <Trans>Couldn’t load Home. Your inbox still works.</Trans>
        )}
      </span>
      <Button size="sm" variant="outline" className="h-[34px] rounded-lg border-input bg-card" onClick={retry}>
        <Trans>Try again</Trans>
      </Button>
      <TextLink to="/conversations">
        <Trans>Open inbox</Trans>
      </TextLink>
    </div>
  )
}

function BroadcastBanner({ h, mailOnly }: { h: Home; mailOnly: boolean }) {
  const { i18n } = useLingui()
  const s = useBroadcastsStatus()
  const { isOwner } = useSession()
  if (!s || s.mode === "off" || s.status === "allowed") return null
  const relevant =
    mailOnly || s.status === "suspended" || h.upcoming.some((u) => u.kind === "broadcast") || !!h.broadcasts?.last_sent
  if (!relevant) return null
  const asked = s.status_at
    ? new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "long" }).format(new Date(s.status_at))
    : ""
  return (
    <div
      role="alert"
      className={cn(
        "flex flex-wrap items-center gap-3 rounded-xl border px-4 py-3.5 text-small",
        s.status === "suspended"
          ? "border-destructive/25 bg-destructive/8 text-destructive"
          : s.status === "approval_requested"
            ? "bg-card text-muted-foreground"
            : "border-note-border bg-note text-note-ink"
      )}
      data-testid="home-broadcasts-status"
    >
      <span className="flex-[1_1_280px]">
        {s.status === "suspended" ? (
          <>
            <b className="font-semibold">
              <Trans>Broadcasts are suspended.</Trans>
            </b>{" "}
            <Trans>
              The server’s operator suspended sending for this workspace. Scheduled broadcasts are paused and new
              schedules are refused.
            </Trans>
            {s.reason && <> {s.reason}</>}
          </>
        ) : s.status === "approval_requested" ? (
          <>
            <b className="font-semibold">
              <Trans>Waiting for the operator.</Trans>
            </b>{" "}
            {asked ? (
              <Trans>
                You asked on {asked} to send broadcasts. Drafts and schedules wait until the operator answers.
              </Trans>
            ) : (
              <Trans>You asked to send broadcasts. Drafts and schedules wait until the operator answers.</Trans>
            )}
          </>
        ) : (
          <>
            <b className="font-semibold">
              <Trans>Broadcasts can’t go out yet.</Trans>
            </b>{" "}
            <Trans>This server sends broadcasts only after its operator approves the workspace. Drafts are kept.</Trans>
          </>
        )}
      </span>
      {s.status === "approval_required" && isOwner && (
        <Button
          size="sm"
          className="h-[34px] rounded-lg bg-foreground px-3 text-background hover:bg-foreground/85"
          render={<Link to="/settings/workspace" />}
        >
          <Trans>Ask the operator</Trans>
        </Button>
      )}
      {s.status === "suspended" && (
        <Link to="/broadcasts" className="text-small text-inherit underline">
          <Trans>See paused broadcasts</Trans>
        </Link>
      )}
    </div>
  )
}

function useStepText() {
  const { t } = useLingui()
  return {
    title: {
      chat: t`Add live chat to your site`,
      email: t`Connect a support address`,
      mail: t`Give the team a shared mailbox`,
      app: t`Connect your iOS or Android app`,
      docs: t`Collect ratings on your docs pages`,
      analytics: t`Count visitors on your site`,
      newsletter: t`Start a mailing list`,
    } satisfies Record<HubId, string>,
    action: {
      chat: t`Get the snippet`,
      email: t`Connect`,
      mail: t`Set up`,
      app: t`Connect`,
      docs: t`Get the snippet`,
      analytics: t`Add analytics`,
      newsletter: t`Create`,
    } satisfies Record<HubId, string>,
  }
}

function SetupCard({ setup }: { setup: NonNullable<Home["setup"]> }) {
  const { t, i18n } = useLingui()
  const { homePath, membership } = useSession()
  const text = useStepText()
  const places = useHubPlaces()
  const count = setup.items.length
  const done = setup.items.filter((i) => i.done).length
  const fmt = new Intl.NumberFormat(i18n.locale)
  const doneText = fmt.format(done)
  const total = fmt.format(count)
  const percent = count ? Math.round((done / count) * 100) : 0
  return (
    <section
      aria-label={t`Finish setup`}
      className={cn(cardClass, "flex flex-col gap-3 px-5 py-[18px]")}
      data-testid="home-setup-card"
    >
      <div className="flex flex-wrap items-center gap-x-3.5 gap-y-1">
        <h2 className="m-0 min-w-0 flex-1 text-body font-semibold">
          <Trans>Finish setting up</Trans>
        </h2>
        <Link to="/setup" className="text-caption text-faint hover:text-foreground" data-testid="home-setup-open">
          {t`${doneText} of ${total} done`}
        </Link>
        <span
          className="flex h-1.5 w-[140px] overflow-hidden rounded-full bg-muted"
          role="progressbar"
          aria-label={t`Setup`}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={percent}
          data-testid="home-setup-progress"
        >
          <span className="rounded-full bg-success" style={{ width: `${percent}%` }} />
        </span>
      </div>
      <ul data-testid="home-setup">
        {orderByFocus(setup.items, membership.workspace.focus).map((item) => (
          <li
            key={item.id}
            className="flex min-h-11 items-center gap-3 border-t pt-2.5 pb-0.5 first:border-t"
            data-testid={`home-setup-${item.id}`}
          >
            <span
              aria-hidden
              className={cn(
                "grid size-[22px] shrink-0 place-items-center rounded-full",
                item.done ? "bg-success text-white dark:text-background" : "border-2 border-input"
              )}
            >
              {item.done && <CheckIcon className="size-3" strokeWidth={3} />}
            </span>
            <span className={cn("min-w-0 flex-1", item.done ? "text-faint line-through" : "font-medium")}>
              {text.title[item.id]}
            </span>
            {!item.done && (
              <Button
                size="sm"
                variant="outline"
                className="h-[34px] rounded-lg border-input px-3 font-normal"
                render={<Link to={hubTarget(item, places)} state={{ back: homePath } satisfies SetupState} />}
              >
                {text.action[item.id]}
              </Button>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}

function HomeSkeleton() {
  return (
    <div aria-hidden className="mt-9 flex flex-col gap-4" data-testid="home-loading">
      <Skeleton className="h-[92px] rounded-[10px]" />
      <Skeleton className="h-[52px] rounded-lg" />
      <Skeleton className="h-[52px] rounded-lg" />
      <Skeleton className="h-[260px] rounded-[10px]" />
    </div>
  )
}

function Tag({ children }: { children: React.ReactNode }) {
  return (
    <span className="max-w-[140px] shrink-0 truncate rounded border border-input px-1.5 font-mono text-[10px] leading-4 tracking-[0.03em] text-muted-foreground uppercase phone:hidden">
      {children}
    </span>
  )
}

function NeedsYouMain({ h, fresh, solo }: { h: Home; fresh: boolean; solo: boolean }) {
  const { t, i18n } = useLingui()
  const ago = useAgo()
  const errorText = useErrorText()
  const { membership } = useSession()
  const inboxes = useInboxes().data ?? []
  const byId = new Map(inboxes.map((i) => [i.id, i]))
  const many = inboxes.length > 1
  const bulk = useBulkUpdateConversations()
  const list = useRef<HTMLDivElement>(null)
  const rows = h.inbox.waiting_conversations
  const links = () => Array.from(list.current?.querySelectorAll<HTMLAnchorElement>("a[data-waiting]") ?? [])
  const move = (step: number) => {
    const all = links()
    if (all.length === 0) return
    const i = all.indexOf(document.activeElement as HTMLAnchorElement)
    const next = i < 0 ? (step > 0 ? 0 : all.length - 1) : Math.min(all.length - 1, Math.max(0, i + step))
    all[next]?.focus()
    all[next]?.scrollIntoView({ block: "nearest" })
  }
  const take = () => {
    const all = links()
    const id = (all.find((a) => a === document.activeElement) ?? all[0])?.dataset.waiting
    if (!id) return
    bulk.mutate(
      { conversation_ids: [id], assignee_id: membership.member_id },
      { onSuccess: () => toast(t`Assigned to you`), onError: (e) => toast(errorText(e)) }
    )
  }
  useHotkeys({ [HOME_KEYS.next]: () => move(1), [HOME_KEYS.previous]: () => move(-1), [HOME_KEYS.take]: take })
  const contactName = useContactName()
  const who = (c: (typeof rows)[number]) => contactName({}, undefined, c.contact.name, c.contact.email)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const waiting = h.inbox.waiting
  const total = fmt.format(waiting)
  const oldest = h.inbox.oldest_waiting
  const median = h.activity.days.at(-1)?.median_first_reply_seconds
  const took = median !== undefined ? formatDuration(median, i18n.locale) : ""
  const any = waiting > 0 && rows.length > 0
  return (
    <section aria-labelledby="home-needs-title" data-testid="home-needs-card">
      <SectionHeading
        id="home-needs-title"
        title={<Trans>Needs you</Trans>}
        lead={
          <span className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
            <span>
              <Trans>Conversations waiting for an answer, oldest first.</Trans>
            </span>
            {any && (
              <span
                aria-hidden
                className="inline-flex items-center gap-1 text-caption text-muted2 phone:hidden [@media(hover:none)]:hidden"
              >
                <Kbd>{HOME_KEYS.next}</Kbd>
                <Kbd>{HOME_KEYS.previous}</Kbd>
                <Trans>move</Trans> · <Kbd>{HOME_KEYS.take}</Kbd>
                <Trans>take</Trans>
              </span>
            )}
          </span>
        }
        more={
          <MoreLink to="/conversations" testId="home-inbox-link">
            {any ? (
              <Plural value={waiting} one="Open in Inbox" other={`All ${total} in Inbox`} />
            ) : (
              <Trans>Open inbox</Trans>
            )}
          </MoreLink>
        }
      />
      <div ref={list} className={boxClass}>
        {!fresh && (
          <Stats label={t`Right now`} testId="home-inbox">
            <Stat
              value={fmt.format(waiting)}
              label={t`Waiting`}
              to="/conversations"
              quiet={waiting === 0}
              testId="home-inbox-waiting"
            />
            {!solo && (
              <Stat
                value={fmt.format(h.inbox.unassigned)}
                label={t`Unassigned`}
                to="/conversations"
                quiet={h.inbox.unassigned === 0}
                testId="home-unassigned"
              />
            )}
            <Stat value={oldest && waiting > 0 ? ago(oldest.since) : "–"} label={t`Oldest wait`} quiet={!any} />
            <Stat
              value={fmt.format(h.inbox.feedback)}
              label={t`New feedback`}
              to="/feedback"
              quiet={h.inbox.feedback === 0}
              tone={h.inbox.feedback > 0 ? "brand" : undefined}
              testId="home-feedback"
            />
          </Stats>
        )}
        {any ? (
          <ul className="m-0 list-none p-0" data-testid="home-needs">
            {rows.map((c, i) => {
              const inbox = many ? byId.get(c.inbox_id) : undefined
              const name = who(c)
              return (
                <li key={c.id} className="border-t border-line-soft first:border-t-0">
                  <Link
                    to={`/conversations/${c.id}`}
                    data-waiting={c.id}
                    className="flex items-center gap-3 px-4 py-3 text-body outline-none transition-colors hover:bg-subtle focus-visible:bg-subtle"
                    data-testid={i === 0 ? "home-oldest-waiting" : `home-waiting-${c.id}`}
                  >
                    {isVisitor(c.contact.name, c.contact.email) ? (
                      <VisitorAvatar className="size-7 text-[11px] font-semibold" />
                    ) : (
                      <ContactAvatar id={c.contact.id} name={name} className="size-7 text-[11px] font-semibold" />
                    )}
                    <span className="min-w-0 flex-1 truncate">
                      <b className="font-medium">{name}</b>
                      {(c.subject || c.preview) && (
                        <span className="text-muted-foreground"> · {c.subject || c.preview}</span>
                      )}
                    </span>
                    {inbox && <Tag>{inbox.name}</Tag>}
                    <span className="w-16 shrink-0 text-end font-mono text-caption text-warn">{ago(c.since)}</span>
                  </Link>
                </li>
              )
            })}
          </ul>
        ) : (
          <div className="flex flex-col gap-0.5 border-t border-line-soft px-4 py-4" data-testid="home-needs-empty">
            <span className="font-medium">
              <Trans>Nothing is waiting.</Trans>
            </span>
            <span className="text-small text-muted-foreground">
              {fresh ? (
                <Trans>Conversations show up here as soon as the first customer writes.</Trans>
              ) : took ? (
                <Trans>First replies today took {took} at the median.</Trans>
              ) : (
                <Trans>New conversations and feedback show up here.</Trans>
              )}
            </span>
          </div>
        )}
      </div>
    </section>
  )
}

function Delta({ children, good }: { children: React.ReactNode; good: boolean }) {
  return <span className={cn("font-mono text-[11px] tabular-nums", good ? "text-ok" : "text-warn")}>{children}</span>
}

function KeepingUp({ h }: { h: Home }) {
  const { t, i18n } = useLingui()
  const [days, setDays] = usePeriod()
  const slots = h.activity.days
  const n = slots.length
  const peak = Math.max(1, ...slots.map((d) => Math.max(d.conversations, d.first_replies)))
  const dayFull = new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "short", timeZone: "UTC" })
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  const signed = new Intl.NumberFormat(i18n.locale, { style: "percent", signDisplay: "exceptZero" })
  const points = new Intl.NumberFormat(i18n.locale, { signDisplay: "exceptZero" })
  const median = h.activity.median_first_reply_seconds
  const before = h.activity.previous.median_first_reply_seconds
  const change = median !== undefined && before ? (median - before) / before : undefined
  const rated = h.activity.good + h.activity.bad
  const good = share(h.activity.good, h.activity.bad)
  const ratedBefore = h.activity.previous.good + h.activity.previous.bad
  const goodBefore = share(h.activity.previous.good, h.activity.previous.bad)
  const pts =
    rated >= 5 && ratedBefore >= 5 && good !== undefined && goodBefore !== undefined
      ? Math.round(good * 100) - Math.round(goodBefore * 100)
      : 0
  const ptsText = points.format(pts)
  const created = fmt.format(h.activity.conversations)
  const replied = fmt.format(h.activity.first_replies)
  const shown = fmt.format(n)
  const labels: Record<Period, string> = { 7: t`Last 7 days`, 14: t`Last 14 days`, 30: t`Last 30 days` }
  return (
    <section aria-labelledby="home-keeping-title" data-testid="home-chart">
      <SectionHeading
        id="home-keeping-title"
        title={<Trans>Keeping up</Trans>}
        lead={<Trans>New conversations against first replies, per day.</Trans>}
        more={
          <MoreLink to="/reports">
            <Trans>Reports</Trans>
          </MoreLink>
        }
      />
      <div className={boxClass}>
        <div className="flex h-12 items-stretch gap-[22px] overflow-x-auto border-b px-4">
          <div
            role="tablist"
            aria-label={t`Period`}
            className="flex items-stretch gap-[22px]"
            data-testid="home-period"
          >
            {PERIODS.map((p) => (
              <button
                key={p}
                type="button"
                role="tab"
                aria-selected={p === days}
                onClick={() => setDays(p)}
                className="-mb-px flex shrink-0 items-center border-b-2 border-transparent px-0.5 text-body font-medium whitespace-nowrap text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-selected:border-brand aria-selected:font-semibold aria-selected:text-foreground"
                data-testid={`home-period-${p}`}
              >
                {labels[p]}
              </button>
            ))}
          </div>
          <span className="flex-1" />
          <span className="inline-flex shrink-0 items-center gap-1.5 text-caption text-muted-foreground phone:hidden">
            <span aria-hidden className="size-2.5 rounded-[2px] bg-brand" />
            <Trans>New</Trans>
          </span>
          <span className="inline-flex shrink-0 items-center gap-1.5 text-caption text-muted-foreground phone:hidden">
            <span aria-hidden className="size-2.5 rounded-[2px] bg-bar2" />
            <Trans>First replies</Trans>
          </span>
        </div>
        <div className="bg-subtle bg-[radial-gradient(var(--input)_1px,transparent_1px)] bg-size-[18px_18px] px-6 pt-7 pb-4 phone:px-4">
          <div
            className="flex h-[180px] items-end border-b border-input"
            style={{ gap: n > 14 ? 4 : 8 }}
            role="img"
            aria-label={t`${created} new conversations and ${replied} first replies in the period`}
          >
            {slots.map((d) => {
              const date = dayFull.format(new Date(`${d.day}T00:00:00Z`))
              const newCount = fmt.format(d.conversations)
              const replyCount = fmt.format(d.first_replies)
              return (
                <span
                  key={d.day}
                  title={t`${date}: ${newCount} new, ${replyCount} first replies`}
                  className="flex h-full min-w-0 flex-1 items-end gap-0.5"
                >
                  <span
                    className="min-w-px flex-1 rounded-t-[2px] bg-brand"
                    style={{ height: `${(100 * d.conversations) / peak}%` }}
                  />
                  <span
                    className="min-w-px flex-1 rounded-t-[2px] bg-bar2"
                    style={{ height: `${(100 * d.first_replies) / peak}%` }}
                  />
                </span>
              )
            })}
          </div>
          <div className="flex justify-between pt-1.5 font-mono text-[11px] text-muted2">
            <span>
              <Plural value={n} one="# day ago" other="# days ago" />
            </span>
            <span>
              <Trans>Today</Trans>
            </span>
          </div>
        </div>
        <div className="border-t">
          <Stats>
            <Stat value={created} label={t`New conversations`} hint={t`last ${shown} days`} />
            <Stat value={replied} label={t`First replies`} />
            <div
              className="flex min-w-0 flex-col gap-1 border-e border-b border-line-soft px-[18px] py-4"
              data-testid="home-first-reply"
            >
              <b className="text-[26px] leading-8 font-semibold tracking-[-0.02em] tabular-nums">
                {median !== undefined ? formatDuration(median, i18n.locale) : "–"}
              </b>
              <span className="truncate text-caption text-muted-foreground">
                <Trans>Median first reply</Trans>
              </span>
              {change !== undefined && change !== 0 && <Delta good={change < 0}>{signed.format(change)}</Delta>}
            </div>
            <div
              className="flex min-w-0 flex-col gap-1 border-e border-b border-line-soft px-[18px] py-4"
              data-testid="home-rating"
            >
              <b className="text-[26px] leading-8 font-semibold tracking-[-0.02em] tabular-nums">
                {rated < 5 || good === undefined ? "–" : pct.format(good)}
              </b>
              <span className="truncate text-caption text-muted-foreground">
                <Trans>Rated good</Trans>
              </span>
              <span className="truncate font-mono text-[11px] text-muted2">
                <Plural value={rated} _0="no ratings" one="# rating" other="# ratings" />
              </span>
              {pts !== 0 && <Delta good={pts > 0}>{t`${ptsText} pts`}</Delta>}
            </div>
          </Stats>
        </div>
      </div>
    </section>
  )
}

function Products({ h }: { h: Home }) {
  const { t, i18n } = useLingui()
  const [, setInboxId] = useInboxFilter()
  const byInbox = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  const rows = h.products
    .flatMap((p) => {
      const inbox = byInbox.get(p.inbox_id)
      return inbox ? [{ p, inbox }] : []
    })
    .sort((a, b) => b.p.open - a.p.open)
  const shown = fmt.format(h.period.days)
  const th = "border-b px-4 py-2.5 text-caption font-medium text-muted2"
  const td = "border-b border-line-soft px-4 py-[11px] text-fg2"
  return (
    <section aria-labelledby="home-products-title" data-testid="home-products">
      <SectionHeading
        id="home-products-title"
        title={<Trans>Products</Trans>}
        lead={<Trans>Open conversations now, new ones and ratings over the last {shown} days.</Trans>}
      />
      <div className={cn(boxClass, "overflow-x-auto")}>
        <table className="w-full border-collapse text-body [&_tr:last-child>*]:border-b-0">
          <thead>
            <tr>
              <th scope="col" className={cn(th, "text-start")}>
                <Trans>Product</Trans>
              </th>
              <th scope="col" className={cn(th, "text-end")}>
                <Trans>Open</Trans>
              </th>
              <th scope="col" className={cn(th, "text-end phone:hidden")}>
                <Trans>New</Trans>
              </th>
              <th scope="col" className={cn(th, "text-end")}>
                <Trans>Rating</Trans>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ p, inbox }) => {
              const name = inbox.name
              const rated = p.good + p.bad
              const good = share(p.good, p.bad)
              return (
                <tr key={inbox.id} className="tabular-nums" data-testid={`home-product-${inbox.id}`}>
                  <th scope="row" className={cn(td, "text-start font-medium")}>
                    <button
                      type="button"
                      onClick={() => setInboxId(inbox.id)}
                      title={inbox.name}
                      aria-label={t`Show only ${name}`}
                      className="inline-flex max-w-[260px] items-center gap-2 rounded-sm text-foreground underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring phone:max-w-[150px]"
                    >
                      <span
                        aria-hidden
                        className="size-[7px] shrink-0 rounded-full bg-muted2"
                        style={inbox.branding.color ? { backgroundColor: inbox.branding.color } : undefined}
                      />
                      <span className="min-w-0 truncate">{inbox.name}</span>
                    </button>
                  </th>
                  <td className={cn(td, "text-end font-mono")}>{fmt.format(p.open)}</td>
                  <td className={cn(td, "text-end font-mono phone:hidden")}>{fmt.format(p.conversations)}</td>
                  <td
                    className={cn(
                      td,
                      "text-end text-small",
                      rated < 5 ? "text-muted2" : good !== undefined && good < 0.85 && "font-medium text-warn"
                    )}
                  >
                    {rated < 5 ? (
                      <Plural value={rated} _0="no ratings" one="# rating" other="# ratings" />
                    ) : (
                      t`${pct.format(good ?? 0)} good`
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <p className="m-0 mt-2 text-caption text-muted2">
        <Trans>
          Open includes conversations waiting on the customer. Rating turns amber below 85% good once a product has 5
          ratings.
        </Trans>
      </p>
    </section>
  )
}

function Later({ h, mailOnly, support }: { h: Home; mailOnly: boolean; support: boolean }) {
  return (
    <section aria-labelledby="home-later-title" className="hidden has-[[data-home-box]]:block">
      <SectionHeading id="home-later-title" title={<Trans>Status and coming up</Trans>} />
      <div className="grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] items-start gap-4">
        {support && <Status h={h} />}
        <ComingUp h={h} mailOnly={mailOnly} />
        <Publish h={h} mailOnly={mailOnly} />
      </div>
    </section>
  )
}

function Sites({ h }: { h: Home }) {
  const { t } = useLingui()
  const properties = h.analytics?.properties ?? []
  if (properties.length === 0) return null
  return (
    <InfoSection
      title={t`Visitors · 7 days`}
      testId="home-analytics"
      action={
        <Link
          to="/analytics"
          className="shrink-0 rounded-sm font-normal text-fg2 underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Trans>Analytics</Trans>
        </Link>
      }
    >
      {properties.map((p) => (
        <SiteRow key={p.id} property={p} />
      ))}
    </InfoSection>
  )
}

type Anchor = { id: string; label: string }

function OnThisPage({ anchors }: { anchors: Anchor[] }) {
  const { t } = useLingui()
  if (anchors.length < 2) return null
  return (
    <InfoSection title={t`On this page`} testId="home-toc">
      {anchors.map((a) => (
        <a
          key={a.id}
          href={`#${a.id}`}
          onClick={(e) => {
            e.preventDefault()
            document.getElementById(a.id)?.scrollIntoView({ block: "start" })
          }}
          className="block rounded-sm py-[3px] text-small leading-5 text-fg2 underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        >
          {a.label}
        </a>
      ))}
    </InfoSection>
  )
}

function HomeInfo({ h, anchors, support, solo }: { h?: Home; anchors: Anchor[]; support: boolean; solo: boolean }) {
  return (
    <>
      <OnThisPage anchors={anchors} />
      {h && support && <NeedsYou />}
      {h && <Sites h={h} />}
      {h && support && !solo && <OnlineNow h={h} />}
    </>
  )
}

export function HomePage() {
  const { t } = useLingui()
  const { membership } = useSession()
  const mailOnly = membership.workspace.focus === "mail"
  const [days] = usePeriod()
  const [inboxId] = useInboxFilter()
  const members = useMembers().data ?? []
  const solo = members.length <= 1
  const home = useHome(true, { days, inboxId })
  const h = home.data
  const fresh = h ? isFresh(h, mailOnly) : false
  const failed = !!home.error
  const support = !mailOnly
  const showSetup = !!h?.setup && (fresh || (mailOnly && h.setup.items.some((i) => !i.done)))
  const showChart = !!h && support && !fresh
  const showProducts = !!h && support && !fresh && !inboxId && h.products.length > 1
  const anchors: Anchor[] = h
    ? [
        ...(support ? [{ id: "home-needs-title", label: t`Needs you` }] : []),
        ...(showChart ? [{ id: "home-keeping-title", label: t`Keeping up` }] : []),
        ...(showProducts ? [{ id: "home-products-title", label: t`Products` }] : []),
      ]
    : []
  return (
    <PageLayout
      testId="home"
      label={t`Home`}
      aside={
        <InfoColumn label={t`Details`}>
          <HomeInfo h={h} anchors={anchors} support={support} solo={solo} />
        </InfoColumn>
      }
    >
      <div className="max-w-[920px]">
        <HomeHeader h={h} mailOnly={mailOnly} fresh={fresh} failed={failed} updatedAt={home.dataUpdatedAt} />
        {failed && (
          <div className="mt-6">
            <RefreshAlert h={h} updatedAt={home.dataUpdatedAt} retry={() => void home.refetch()} />
          </div>
        )}
        {!h ? (
          !failed && <HomeSkeleton />
        ) : (
          <>
            <div className="mt-6 flex flex-col gap-4 empty:hidden">
              <BroadcastBanner h={h} mailOnly={mailOnly} />
              {showSetup && h.setup && <SetupCard setup={h.setup} />}
            </div>
            {support && <NeedsYouMain h={h} fresh={fresh} solo={solo} />}
            {showChart && <KeepingUp h={h} />}
            {showProducts && <Products h={h} />}
            {!fresh && <Later h={h} mailOnly={mailOnly} support={support} />}
            <div className="mt-9 grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-x-8 min-[1180px]:hidden [&>section]:border-t-0 [&>section]:pt-0">
              <HomeInfo h={h} anchors={[]} support={support} solo={solo} />
            </div>
          </>
        )}
      </div>
    </PageLayout>
  )
}
