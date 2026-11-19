import { useLingui } from "@lingui/react/macro"
import { useNavigate, type To } from "react-router"

import { InfoColumn, NavGroup, NavList, PageLayout, PageTop, Dot, TopSep, type NavItem } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { PageBar } from "@/components/common/PageBar"
import { FINISHED, isActive, useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { StateBadge } from "@/features/broadcasts/BroadcastParts"
import { useLists, useSender } from "@/features/broadcasts/queries"
import { useDesigns } from "@/features/designs/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { framePad } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Broadcast } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export type BroadcastView = "all" | "drafts" | "scheduled" | "sent"
export type BroadcastsPlace = BroadcastView | "triggered" | "lists" | "designs"

export const VIEWS: BroadcastView[] = ["all", "drafts", "scheduled", "sent"]

export function inView(v: BroadcastView, b: Broadcast) {
  if (v === "sent") return FINISHED.includes(b.state)
  if (v === "scheduled") return b.state === "scheduled"
  if (v === "drafts") return b.state === "draft"
  return true
}

export function viewLink(v: BroadcastView) {
  return v === "all" ? "/broadcasts" : `/broadcasts?tab=${v}`
}

let remembered: { view: BroadcastView; y: number } = { view: "all", y: 0 }

export function rememberList(view: BroadcastView, y = window.scrollY) {
  remembered = { view, y }
}

export function rememberedList() {
  return remembered
}

export function useFlat() {
  return !useIsPhone()
}

export function useViewNames(): Record<BroadcastsPlace, string> {
  const { t } = useLingui()
  return {
    all: t`All`,
    drafts: t`Drafts`,
    scheduled: t`Scheduled`,
    sent: t`Sent`,
    triggered: t`Triggered`,
    lists: t`Mailing lists`,
    designs: t`Designs`,
  }
}

export function viewOf(b: Broadcast): BroadcastsPlace {
  if (b.kind === "triggered") return "triggered"
  const back = remembered.view
  if (back === "all" || inView(back, b)) return back
  if (b.state === "draft") return "drafts"
  if (b.state === "scheduled") return "scheduled"
  if (FINISHED.includes(b.state) || isActive(b)) return "sent"
  return "all"
}

export function backOf(place: BroadcastsPlace, views: Record<BroadcastsPlace, string>) {
  if (place === "triggered") return { to: "/broadcasts/triggered", label: views.triggered }
  if (place === "lists") return { to: "/broadcasts/lists", label: views.lists }
  if (place === "designs") return { to: "/broadcasts/designs", label: views.designs }
  return { to: viewLink(place), label: views[place] }
}

type Paged = { data?: { pages: { items: unknown[] }[] }; hasNextPage: boolean }

const sized = (q: Paged) =>
  q.data && !q.hasNextPage ? q.data.pages.reduce((n, p) => n + p.items.length, 0) : undefined

export function useBroadcastCounts(): Record<BroadcastsPlace, number | undefined> {
  const { canManage } = useSession()
  const [inboxId] = useInboxFilter()
  const once = useBroadcasts(inboxId, undefined, "once")
  const triggered = useBroadcasts(inboxId, undefined, "triggered")
  const lists = useLists(inboxId, false)
  const designs = useDesigns(inboxId, "", false, canManage)
  const all = once.data && !once.hasNextPage ? once.data.pages.flatMap((p) => p.items) : undefined
  const of = (v: BroadcastView) => all?.filter((b) => inView(v, b)).length
  return {
    all: of("all"),
    drafts: of("drafts"),
    scheduled: of("scheduled"),
    sent: of("sent"),
    triggered: sized(triggered),
    lists: sized(lists),
    designs: canManage ? sized(designs) : undefined,
  }
}

export function BroadcastsLayout({
  sub,
  aside,
  testId,
  children,
}: {
  sub?: NavItem[]
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <PageLayout
      testId={testId}
      label={t`Broadcasts`}
      nav={
        sub &&
        sub.length > 0 && (
          <NavList label={t`Broadcasts`}>
            <NavGroup items={sub} />
          </NavList>
        )
      }
      aside={<InfoColumn label={t`Details`}>{aside}</InfoColumn>}
    >
      {children}
    </PageLayout>
  )
}

export function TopTrail({ place }: { place: BroadcastsPlace }) {
  const { t } = useLingui()
  const names = useViewNames()
  const [inboxId] = useInboxFilter()
  const product = useInboxes().data?.find((i) => i.id === inboxId)?.name
  return (
    <>
      <span className="font-medium text-foreground">{t`Broadcasts`}</span>
      <TopSep />
      <span className="truncate">{names[place]}</span>
      <TopSep />
      <span className="truncate">{product ?? t`All products`}</span>
    </>
  )
}

export function DetailFrame({
  testId,
  back,
  bar,
  heading,
  eyebrow,
  actions,
  aside,
  wide,
  children,
}: {
  testId: string
  back: { to: To & string; label: string }
  bar: { back: string; label: string; content: React.ReactNode }
  heading: React.ReactNode
  eyebrow: React.ReactNode
  actions?: React.ReactNode
  aside?: React.ReactNode
  wide?: boolean
  children: React.ReactNode
}) {
  const flat = useFlat()
  if (!flat)
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid={testId}>
        <PageBar back={bar.back} label={bar.label}>
          {bar.content}
        </PageBar>
        <div className="@container min-h-0 flex-1 in-data-frame:overflow-y-auto">
          <div className={cn(framePad, "max-w-[1200px]")}>{children}</div>
        </div>
      </div>
    )
  return (
    <FlatDetail
      testId={testId}
      back={back}
      heading={heading}
      eyebrow={eyebrow}
      actions={actions}
      aside={aside}
      wide={wide}
    >
      {children}
    </FlatDetail>
  )
}

function FlatDetail({
  testId,
  back,
  heading,
  eyebrow,
  actions,
  aside,
  wide,
  children,
}: {
  testId: string
  back: { to: string; label: string }
  heading: React.ReactNode
  eyebrow: React.ReactNode
  actions?: React.ReactNode
  aside?: React.ReactNode
  wide?: boolean
  children: React.ReactNode
}) {
  const navigate = useNavigate()
  useHotkeys({ Escape: () => navigate(back.to) })
  return (
    <BroadcastsLayout aside={aside} testId={testId}>
      <PageTop
        back={{ label: back.label, to: back.to }}
        actions={actions && <div className="flex flex-wrap items-center gap-1.5">{actions}</div>}
      >
        {eyebrow}
      </PageTop>
      <div className={cn("@container flex flex-col gap-4", wide ? "max-w-[980px]" : "max-w-[760px]")}>
        <div className="mt-3">{heading}</div>
        {children}
      </div>
    </BroadcastsLayout>
  )
}

export function DetailHeading({ b, sub }: { b: Broadcast; sub?: React.ReactNode }) {
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  return (
    <header className="flex flex-col">
      <div className="flex min-w-0 flex-wrap items-center gap-2 text-small text-muted-foreground">
        <StateBadge state={b.state} kind={b.kind} />
        {inbox && (
          <>
            <Dot color={inbox.branding.color} className="size-[7px] rounded-full" />
            <span className="truncate">{inbox.name}</span>
          </>
        )}
      </div>
      <h1
        className="m-0 mt-2.5 text-page leading-[34px] text-balance break-words"
        dir="auto"
        data-testid="broadcast-name-heading"
      >
        {b.name}
      </h1>
      {(sub ?? b.subject) && (
        <p className="m-0 mt-1.5 text-reading text-muted-foreground" dir="auto">
          {sub ?? b.subject}
        </p>
      )}
    </header>
  )
}

export function DetailTrail({ b }: { b: Broadcast }) {
  const { t } = useLingui()
  const text = useEnumText()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const from = useSender(b.inbox_id, b.channel_id)
  return (
    <>
      <span className="font-medium text-foreground">{inbox?.name ?? t`Broadcasts`}</span>
      {from && (
        <>
          <TopSep />
          <span className="truncate normal-case" dir="ltr">
            {from}
          </span>
        </>
      )}
      <TopSep />
      <span className="truncate">
        {b.kind === "triggered" && b.state === "cancelled" ? t`Stopped` : text.broadcastState[b.state]}
      </span>
    </>
  )
}
