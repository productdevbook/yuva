import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, XIcon } from "lucide-react"
import { useRef } from "react"
import { Link, useSearchParams } from "react-router"

import { ContactAvatar, isVisitor, Kbd, toast, useContactName, VisitorAvatar } from "@/components/common"
import { HOME_KEYS } from "@/components/common/ShortcutSheet"
import { formatDuration, useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useBroadcastsStatus } from "@/features/broadcasts/BroadcastParts"
import {
  cardClass,
  ComingUp,
  eyebrowClass,
  NeedsYou,
  OnlineNow,
  Publish,
  Status,
  TextLink,
} from "@/features/home/HomeSide"
import { SiteCard } from "@/features/home/SiteCard"
import { useHome } from "@/features/home/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useBulkUpdateConversations } from "@/features/inbox/queries"
import { share } from "@/features/reports/data"
import { Page } from "@/features/settings/ui"
import { hubTarget, useHubPlaces } from "@/features/setup/SetupHub"
import { orderByFocus, type HubId, type SetupState } from "@/features/setup/setup"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import type { Home } from "@/lib/api"
import { useRealtimeStatus } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMembers } from "@/lib/workspace"

const PERIODS = [7, 14, 30] as const
type Period = (typeof PERIODS)[number]

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
    <span role="status" className="inline-flex items-center gap-2 text-caption text-faint" data-testid="home-live">
      <span
        aria-hidden
        className={cn(
          "size-[7px] rounded-full",
          offline ? "bg-warning" : status === "connecting" ? "bg-faint/60" : "bg-success"
        )}
      />
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
  if (h.inbox.waiting === 0 || !oldest) return <Trans>Nobody is waiting.</Trans>
  const waited = ago(oldest.since)
  return <Trans>Oldest has waited {waited}.</Trans>
}

function Yesterday({ h }: { h: Home }) {
  const { t } = useLingui()
  const y = h.activity.days.at(-2)
  if (!y) return null
  const created = y.conversations
  const bad = y.bad
  return (
    <p className="text-small text-muted-foreground" data-testid="home-yesterday">
      <span className="text-faint">{t`Yesterday`}:</span>{" "}
      <Link to="/reports/today" className="whitespace-nowrap hover:text-foreground hover:underline">
        <Plural value={created} one="# new" other="# new" />
      </Link>
      {bad > 0 && (
        <>
          {" · "}
          <Link to="/reports/ratings" className="whitespace-nowrap text-warning hover:underline">
            <Plural value={bad} one="# bad rating" other="# bad ratings" />
          </Link>
        </>
      )}
    </p>
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
  const [inboxId, setInboxId] = useInboxFilter()
  const product = (useInboxes().data ?? []).find((i) => i.id === inboxId)?.name
  const now = useNow(60_000)
  const first = (me.person.name || me.person.email).split(/[\s@]/)[0]
  const hour = new Date(now).getHours()
  const greeting =
    hour < 12 ? t`Good morning, ${first}.` : hour < 18 ? t`Good afternoon, ${first}.` : t`Good evening, ${first}.`
  const date = new Intl.DateTimeFormat(i18n.locale, { weekday: "long", day: "numeric", month: "long" }).format(now)
  return (
    <header className="flex flex-wrap items-end justify-between gap-x-4 gap-y-3">
      <div className="flex min-w-0 flex-[1_1_260px] flex-col gap-1">
        <span className="font-mono text-caption tracking-[0.04em] text-faint uppercase">{date}</span>
        {product && !mailOnly && (
          <button
            type="button"
            onClick={() => setInboxId("")}
            aria-label={t`Showing ${product} only. Show all products`}
            className="inline-flex max-w-full items-center gap-2 self-start rounded-full bg-foreground py-1 ps-3 pe-1.5 text-small font-medium text-background outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="home-scope"
          >
            <span className="truncate">
              <Trans>{product} only</Trans>
            </span>
            <span aria-hidden className="grid size-5 shrink-0 place-items-center rounded-full bg-background/20">
              <XIcon className="size-3" />
            </span>
          </button>
        )}
        <h1 className="m-0 text-[30px] leading-9 font-semibold tracking-[-0.02em] phone:text-2xl phone:leading-[30px]">
          {greeting}{" "}
          <span className="text-faint">
            <Tail h={h} mailOnly={mailOnly} fresh={fresh} />
          </span>
        </h1>
        {h && !mailOnly && !fresh && <Yesterday h={h} />}
      </div>
      <LiveState failed={failed} updatedAt={updatedAt} />
    </header>
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

function HomeSkeleton() {
  return (
    <div aria-hidden className="flex flex-col gap-4" data-testid="home-loading">
      <Skeleton className="h-14 rounded-xl" />
      <Skeleton className="h-[92px] rounded-2xl" />
      <div className="flex flex-wrap gap-6">
        <div className="flex min-w-0 flex-[999_1_560px] flex-col gap-2.5">
          <Skeleton className="h-[120px] rounded-2xl" />
          <Skeleton className="h-[52px] rounded-lg" />
          <Skeleton className="h-[52px] rounded-lg" />
          <Skeleton className="h-[52px] rounded-lg" />
        </div>
        <div className="flex min-w-0 flex-[1_1_300px] flex-col gap-4">
          <Skeleton className="h-40 rounded-2xl" />
          <Skeleton className="h-[120px] rounded-2xl" />
          <Skeleton className="h-[120px] rounded-2xl" />
        </div>
      </div>
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

function NowStrip({ h, solo }: { h: Home; solo: boolean }) {
  const { t, i18n } = useLingui()
  const ago = useAgo()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const oldest = h.inbox.oldest_waiting
  const waited = oldest ? ago(oldest.since) : ""
  const tiles = [
    {
      key: "waiting",
      to: "/conversations",
      name: t`Waiting`,
      value: h.inbox.waiting,
      hint: oldest && h.inbox.waiting > 0 ? t`oldest ${waited}` : t`nobody is waiting`,
      testId: "home-inbox-waiting",
    },
    ...(solo
      ? []
      : [
          {
            key: "unassigned",
            to: "/conversations",
            name: t`Unassigned`,
            value: h.inbox.unassigned,
            hint: t`no owner yet`,
            testId: "home-unassigned",
          },
        ]),
    {
      key: "feedback",
      to: "/feedback",
      name: t`New feedback`,
      value: h.inbox.feedback,
      hint: t`to triage`,
      testId: "home-feedback",
    },
  ]
  return (
    <section
      aria-label={t`Right now`}
      className={cn(cardClass, "grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] overflow-hidden phone:grid-cols-2")}
      data-testid="home-inbox"
    >
      {tiles.map((k) => (
        <Link
          key={k.key}
          to={k.to}
          className="-me-px flex min-w-0 flex-col gap-1.5 border-e px-5 py-4 outline-none transition-colors hover:bg-surface focus-visible:bg-surface phone:-mb-px phone:border-b phone:last:odd:col-span-2"
          data-testid={k.testId}
        >
          <span className="truncate text-small text-muted-foreground">{k.name}</span>
          <b
            className={cn(
              "text-[28px] leading-[34px] font-semibold tracking-[-0.02em] tabular-nums",
              k.value === 0 && "text-faint"
            )}
          >
            {fmt.format(k.value)}
          </b>
          <span className="truncate text-caption text-faint">{k.hint}</span>
        </Link>
      ))}
    </section>
  )
}

function ProductMark({ color, name }: { color?: string; name: string }) {
  return (
    <>
      <span
        aria-hidden
        className="size-2 shrink-0 rounded-[2px]"
        style={{ backgroundColor: color || "var(--faint)" }}
      />
      <span className="truncate">{name}</span>
    </>
  )
}

function Waiting({ h, className }: { h: Home; className?: string }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const ago = useAgo()
  const errorText = useErrorText()
  const { membership } = useSession()
  const inboxes = useInboxes().data ?? []
  const byId = new Map(inboxes.map((i) => [i.id, i]))
  const many = inboxes.length > 1
  const bulk = useBulkUpdateConversations()
  const list = useRef<HTMLElement>(null)
  const rows = h.inbox.waiting_conversations
  const [top, ...queue] = rows
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
  const waiting = h.inbox.waiting
  const total = new Intl.NumberFormat(i18n.locale).format(waiting)
  const contactName = useContactName()
  const who = (c: (typeof rows)[number]) => contactName({}, undefined, c.contact.name, c.contact.email)
  const avatar = (c: (typeof rows)[number], className: string) =>
    isVisitor(c.contact.name, c.contact.email) ? (
      <VisitorAvatar className={className} />
    ) : (
      <ContactAvatar id={c.contact.id} name={who(c)} className={className} />
    )
  const product = (id: string) => {
    const inbox = many ? byId.get(id) : undefined
    return inbox ? <ProductMark color={inbox.branding.color} name={inbox.name} /> : null
  }
  if (!top) return null
  const topChannel = top.channel_kind ? text.channel[top.channel_kind] : ""
  const topMeta = [topChannel, top.contact.email].filter(Boolean).join(" · ")
  return (
    <section
      ref={list}
      aria-label={t`Waiting longest`}
      className={cn(cardClass, "overflow-hidden rounded-2xl", className)}
      data-testid="home-needs-card"
    >
      <div className="flex items-baseline justify-between gap-3 border-b px-[18px] py-3.5">
        <h2 className={eyebrowClass}>
          <Trans>Waiting longest</Trans>
        </h2>
        <span className="flex items-baseline gap-3.5">
          <span
            aria-hidden
            className="inline-flex items-center gap-1 text-caption text-faint phone:hidden [@media(hover:none)]:hidden"
          >
            <Kbd>{HOME_KEYS.next}</Kbd>
            <Kbd>{HOME_KEYS.previous}</Kbd>
            <Trans>move</Trans> · <Kbd>{HOME_KEYS.take}</Kbd>
            <Trans>take</Trans>
          </span>
          <TextLink to="/conversations" testId="home-inbox-link">
            <Plural value={waiting} one="Open in Inbox" other={`All ${total} in Inbox`} />
          </TextLink>
        </span>
      </div>
      <ul data-testid="home-needs">
        <li className="border-b bg-surface/40">
          <Link
            to={`/conversations/${top.id}`}
            data-waiting={top.id}
            className="flex flex-col gap-2.5 px-[18px] pt-5 pb-[18px] outline-none transition-colors hover:bg-surface focus-visible:bg-surface"
            data-testid="home-oldest-waiting"
          >
            <span className="flex items-center gap-3">
              {avatar(top, "size-10 text-caption font-semibold")}
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate font-semibold">{who(top)}</span>
                <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint">
                  {product(top.inbox_id)}
                  {topMeta && (
                    <span className="truncate">
                      {many && byId.has(top.inbox_id) ? " · " : ""}
                      {topMeta}
                    </span>
                  )}
                </span>
              </span>
              <span className="flex shrink-0 flex-col items-end">
                <span className="font-mono text-xl font-medium">{ago(top.since)}</span>
                <span className="text-caption text-faint">
                  <Trans>waiting</Trans>
                </span>
              </span>
            </span>
            {top.subject && (
              <span className="text-[17px] leading-6 font-semibold tracking-[-0.01em] break-words">{top.subject}</span>
            )}
            {top.preview && (
              <span className="line-clamp-3 max-w-[72ch] text-body leading-[22px] text-muted-foreground">
                {top.preview}
              </span>
            )}
          </Link>
        </li>
        {queue.map((c) => {
          const channel = c.channel_kind ? text.channel[c.channel_kind] : ""
          return (
            <li key={c.id} className="border-b last:border-b-0">
              <Link
                to={`/conversations/${c.id}`}
                data-waiting={c.id}
                className="grid grid-cols-[32px_minmax(0,1fr)_minmax(0,0.9fr)_64px] items-center gap-3.5 px-[18px] py-[11px] outline-none transition-colors hover:bg-surface focus-visible:bg-surface phone:grid-cols-[32px_minmax(0,1fr)_64px]"
                data-testid={`home-waiting-${c.id}`}
              >
                {avatar(c, "size-8 text-caption font-semibold")}
                <span className="flex min-w-0 flex-col">
                  <span className="truncate font-medium">{who(c)}</span>
                  <span className="flex min-w-0 items-center gap-1.5 text-caption whitespace-nowrap text-faint">
                    {product(c.inbox_id)}
                    {channel && (
                      <span className="truncate">
                        {many && byId.has(c.inbox_id) ? " · " : ""}
                        {channel}
                      </span>
                    )}
                  </span>
                </span>
                <span className="truncate text-small text-muted-foreground phone:hidden">{c.subject || c.preview}</span>
                <span className="text-end font-mono text-small text-faint">{ago(c.since)}</span>
              </Link>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

function NothingWaiting({ h, fresh, className }: { h: Home; fresh: boolean; className?: string }) {
  const { i18n } = useLingui()
  const median = h.activity.days.at(-1)?.median_first_reply_seconds
  const took = median !== undefined ? formatDuration(median, i18n.locale) : ""
  return (
    <section
      className={cn(cardClass, "flex flex-wrap items-center gap-3 px-6 py-[22px]", className)}
      data-testid="home-needs-empty"
    >
      <span className="flex min-w-0 flex-[1_1_260px] flex-col gap-0.5">
        <h2 className="m-0 text-base leading-[22px] font-semibold">
          <Trans>Nothing is waiting.</Trans>
        </h2>
        <span className="text-small text-faint">
          {fresh ? (
            <Trans>Conversations show up here as soon as the first customer writes.</Trans>
          ) : took ? (
            <Trans>First replies today took {took} at the median.</Trans>
          ) : (
            <Trans>New conversations and feedback show up here.</Trans>
          )}
        </span>
      </span>
      <TextLink to="/conversations" testId="home-inbox-link">
        <Trans>Open inbox</Trans>
      </TextLink>
    </section>
  )
}

function Delta({ children, good }: { children: React.ReactNode; good: boolean }) {
  return (
    <span
      className={cn(
        "rounded-full px-1.5 text-caption font-medium tabular-nums",
        good ? "bg-success/10 text-success" : "bg-warning/10 text-warning"
      )}
    >
      {children}
    </span>
  )
}

function KeepingUp({ h, className }: { h: Home; className?: string }) {
  const { t, i18n } = useLingui()
  const [days, setDays] = usePeriod()
  const slots = h.activity.days
  const n = slots.length
  const peak = Math.max(1, ...slots.map((d) => Math.max(d.conversations, d.first_replies)))
  const every = n <= 7 ? 1 : n <= 14 ? 2 : 5
  const dayNum = new Intl.DateTimeFormat(i18n.locale, { day: "numeric", timeZone: "UTC" })
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
  const ratedText = fmt.format(rated)
  const goodText = good !== undefined ? pct.format(good) : ""
  const created = fmt.format(h.activity.conversations)
  const replied = fmt.format(h.activity.first_replies)
  const labels: Record<Period, string> = { 7: t`Last 7 days`, 14: t`Last 14 days`, 30: t`Last 30 days` }
  return (
    <section
      aria-label={t`Keeping up`}
      className={cn(cardClass, "flex flex-col gap-3.5 p-[18px]", className)}
      data-testid="home-chart"
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <span className="flex min-w-0 flex-col">
          <h2 className="m-0 text-body font-semibold">
            <Trans>Keeping up</Trans>
          </h2>
          <span className="text-caption text-faint">
            <Trans>new conversations against first replies, per day</Trans>
          </span>
        </span>
        <span className="inline-flex items-center gap-2 text-small text-muted-foreground">
          <span aria-hidden>
            <Trans>Period</Trans>
          </span>
          <Select value={String(days)} items={labels} onValueChange={(v) => v && setDays(Number(v) as Period)}>
            <SelectTrigger size="sm" className="rounded-lg" aria-label={t`Period`} data-testid="home-period">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PERIODS.map((p) => (
                <SelectItem key={p} value={String(p)} data-testid={`home-period-${p}`}>
                  {labels[p]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </span>
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1.5 text-caption text-faint phone:hidden">
        <span className="inline-flex items-center gap-1.5">
          <span aria-hidden className="size-2.5 rounded-[2px] bg-faint/60" />
          <Trans>New conversations</Trans>
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span aria-hidden className="h-[3px] w-3 rounded-[2px] bg-foreground" />
          <Trans>First replies</Trans>
        </span>
      </div>
      <div
        className="grid h-[150px] items-end phone:hidden"
        style={{ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))`, gap: n > 14 ? 3 : 8 }}
        role="img"
        aria-label={t`${created} new conversations and ${replied} first replies in the period`}
      >
        {slots.map((d, i) => {
          const at = new Date(`${d.day}T00:00:00Z`)
          const date = dayFull.format(at)
          const newCount = fmt.format(d.conversations)
          const replyCount = fmt.format(d.first_replies)
          return (
            <span
              key={d.day}
              title={t`${date}: ${newCount} new, ${replyCount} first replies`}
              className="flex h-full flex-col justify-end gap-1.5"
            >
              <span className="relative block h-[126px]">
                <span
                  className="absolute inset-x-0 bottom-0 rounded-t-[4px] rounded-b-[1px] bg-faint/60"
                  style={{ height: Math.round((126 * d.conversations) / peak) }}
                />
                {d.first_replies > 0 && (
                  <span
                    className="absolute -inset-x-0.5 h-[3px] rounded-[2px] bg-foreground"
                    style={{ bottom: Math.max(0, Math.round((126 * d.first_replies) / peak) - 1) }}
                  />
                )}
              </span>
              <span
                className={cn(
                  "h-3.5 text-center font-mono text-[11px]",
                  i === n - 1 ? "text-foreground" : "text-faint"
                )}
              >
                {(n - 1 - i) % every === 0 ? dayNum.format(at) : ""}
              </span>
            </span>
          )
        })}
      </div>
      <div className="flex flex-wrap gap-x-6 gap-y-2 border-t pt-3 text-small text-muted-foreground">
        <span className="inline-flex items-center gap-2" data-testid="home-first-reply">
          <Trans>First reply, median</Trans>
          <b className="font-mono font-medium text-foreground">
            {median !== undefined ? formatDuration(median, i18n.locale) : "–"}
          </b>
          {change !== undefined && change !== 0 && <Delta good={change < 0}>{signed.format(change)}</Delta>}
        </span>
        <span className="inline-flex items-center gap-2" data-testid="home-rating">
          <Trans>Rating</Trans>
          <b className="font-mono font-medium text-foreground">
            {rated < 5 ? (
              <Plural value={rated} _0="no ratings" one="# rating" other="# ratings" />
            ) : (
              t`${goodText} good of ${ratedText}`
            )}
          </b>
          {pts !== 0 && <Delta good={pts > 0}>{t`${ptsText} pts`}</Delta>}
        </span>
      </div>
    </section>
  )
}

function Products({ h, className }: { h: Home; className?: string }) {
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
  return (
    <section
      aria-label={t`Products`}
      className={cn(cardClass, "overflow-hidden", className)}
      data-testid="home-products"
    >
      <div className="flex items-baseline justify-between gap-3 border-b px-[18px] py-3.5">
        <h2 className={eyebrowClass}>
          <Trans>Products</Trans>
        </h2>
        <span className="text-caption text-faint">
          <Trans>last {shown} days</Trans>
        </span>
      </div>
      <Table className="text-body">
        <TableHeader>
          <TableRow className="border-b-0 text-caption text-faint hover:bg-transparent">
            <TableHead className="h-auto px-[18px] py-2 font-medium text-faint">
              <Trans>Product</Trans>
            </TableHead>
            <TableHead className="h-auto px-2.5 py-2 text-end font-medium text-faint phone:hidden">
              <Trans>New</Trans>
            </TableHead>
            <TableHead className="h-auto px-2.5 py-2 text-end font-medium text-faint">
              <Trans>Rating</Trans>
            </TableHead>
            <TableHead className="h-auto py-2 ps-2.5 pe-[18px] text-end font-medium text-faint">
              <Trans>Open</Trans>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(({ p, inbox }) => {
            const name = inbox.name
            const rated = p.good + p.bad
            const good = share(p.good, p.bad)
            return (
              <TableRow
                key={inbox.id}
                className="border-t border-b-0 tabular-nums hover:bg-transparent"
                data-testid={`home-product-${inbox.id}`}
              >
                <TableHead scope="row" className="h-auto px-[18px] py-2.5 text-start font-medium text-foreground">
                  <button
                    type="button"
                    onClick={() => setInboxId(inbox.id)}
                    title={inbox.name}
                    aria-label={t`Show only ${name}`}
                    className="inline-flex max-w-[260px] items-center gap-2.5 rounded-md outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring phone:max-w-[150px]"
                  >
                    <span
                      aria-hidden
                      className="size-[9px] shrink-0 rounded-[3px]"
                      style={{ backgroundColor: inbox.branding.color || "var(--faint)" }}
                    />
                    <span className="min-w-0 truncate">{inbox.name}</span>
                  </button>
                </TableHead>
                <TableCell className="px-2.5 py-2.5 text-end font-mono phone:hidden">
                  {fmt.format(p.conversations)}
                </TableCell>
                <TableCell className="px-2.5 py-2.5 text-end">
                  <span
                    className={cn(
                      "text-small",
                      rated < 5 ? "text-faint" : good !== undefined && good < 0.85 ? "font-medium text-warning" : ""
                    )}
                  >
                    {rated < 5 ? (
                      <Plural value={rated} _0="no ratings" one="# rating" other="# ratings" />
                    ) : (
                      t`${pct.format(good ?? 0)} good`
                    )}
                  </span>
                </TableCell>
                <TableCell className="py-2.5 ps-2.5 pe-[18px] text-end font-mono text-faint">
                  {fmt.format(p.open)}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
      <p className="m-0 border-t px-[18px] pt-2.5 pb-3 text-caption text-faint">
        <Trans>
          Open includes conversations waiting on the customer. Rating turns amber below 85% good once a product has 5
          ratings.
        </Trans>
      </p>
    </section>
  )
}

function Sites({ h, className }: { h: Home; className?: string }) {
  const properties = h.analytics?.properties ?? []
  if (properties.length === 0) return null
  return (
    <section className={cn("flex flex-col gap-3", className)} data-testid="home-analytics">
      <div className="flex items-baseline justify-between gap-3">
        <h2 className={eyebrowClass}>
          <Trans>Sites</Trans>
        </h2>
        <TextLink to="/analytics">
          <Trans>Analytics</Trans>
        </TextLink>
      </div>
      <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-3">
        {properties.map((p) => (
          <SiteCard key={p.id} property={p} />
        ))}
      </div>
    </section>
  )
}

function HomeBlocks({ h, mailOnly, fresh }: { h: Home; mailOnly: boolean; fresh: boolean }) {
  const members = useMembers().data ?? []
  const [inboxId] = useInboxFilter()
  const solo = members.length <= 1
  const support = !mailOnly
  const showSetup = !!h.setup && (fresh || (mailOnly && h.setup.items.some((i) => !i.done)))
  return (
    <div className="flex flex-col gap-6 phone:gap-5">
      <BroadcastBanner h={h} mailOnly={mailOnly} />
      {showSetup && h.setup && <SetupCard setup={h.setup} />}
      {support && !fresh && <NowStrip h={h} solo={solo} />}
      <div className="flex flex-wrap items-start gap-6 phone:flex-col phone:flex-nowrap phone:items-stretch phone:gap-5">
        <div className="flex min-w-0 flex-[999_1_560px] flex-col gap-4 phone:contents">
          {support &&
            (h.inbox.waiting > 0 && h.inbox.waiting_conversations.length > 0 ? (
              <Waiting h={h} className="phone:order-1" />
            ) : (
              <NothingWaiting h={h} fresh={fresh} className="phone:order-1" />
            ))}
          {support && !fresh && <KeepingUp h={h} className="phone:order-5" />}
          {support && !fresh && !inboxId && h.products.length > 1 && <Products h={h} className="phone:order-6" />}
          {!fresh && <Sites h={h} className="phone:order-7" />}
        </div>
        <aside className="flex min-w-0 flex-[1_1_300px] flex-col gap-4 empty:hidden phone:contents">
          {support && !fresh && <NeedsYou className="phone:order-2" />}
          {support && !fresh && <Status h={h} className="phone:order-3" />}
          {!fresh && <ComingUp h={h} mailOnly={mailOnly} className="phone:order-4" />}
          {!fresh && <Publish h={h} mailOnly={mailOnly} className="phone:order-8" />}
          {support && !solo && <OnlineNow h={h} className="phone:order-9" />}
        </aside>
      </div>
    </div>
  )
}

export function HomePage() {
  const { membership } = useSession()
  const mailOnly = membership.workspace.focus === "mail"
  const [days] = usePeriod()
  const [inboxId] = useInboxFilter()
  const home = useHome(true, { days, inboxId })
  const h = home.data
  const fresh = h ? isFresh(h, mailOnly) : false
  const failed = !!home.error
  return (
    <Page testId="home">
      <div className="mx-auto flex w-full max-w-[1280px] flex-col gap-6 pt-8 phone:gap-5 phone:pt-4">
        <HomeHeader h={h} mailOnly={mailOnly} fresh={fresh} failed={failed} updatedAt={home.dataUpdatedAt} />
        {failed && <RefreshAlert h={h} updatedAt={home.dataUpdatedAt} retry={() => void home.refetch()} />}
        {!h ? !failed && <HomeSkeleton /> : <HomeBlocks h={h} mailOnly={mailOnly} fresh={fresh} />}
      </div>
    </Page>
  )
}
