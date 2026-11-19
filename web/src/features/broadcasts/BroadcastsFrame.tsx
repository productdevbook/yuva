import { Trans, useLingui } from "@lingui/react/macro"
import { Link, useNavigate, type To } from "react-router"

import { Dot, InfoColumn, PageLayout, PageTop } from "@/components/common"
import { FINISHED, isActive, useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { senderState, StateBadge } from "@/features/broadcasts/BroadcastParts"
import { useInboxChannels, useLists } from "@/features/broadcasts/queries"
import { useDesigns } from "@/features/designs/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { channelLink, useDeliverability } from "@/features/settings/channels/email/deliverability"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Broadcast, DeliverabilityCheck } from "@/lib/api"
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
    all: t({ message: "All broadcasts", context: "broadcasts place" }),
    drafts: t({ message: "Drafts and approvals", context: "broadcasts place" }),
    scheduled: t({ message: "Scheduled", context: "broadcasts place" }),
    sent: t({ message: "Sent", context: "broadcasts place" }),
    triggered: t({ message: "Triggered", context: "broadcasts place" }),
    lists: t({ message: "Subscribers", context: "broadcasts place" }),
    designs: t({ message: "Designs", context: "broadcasts place" }),
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
    lists:
      lists.data && !lists.hasNextPage
        ? lists.data.pages.flatMap((p) => p.items).reduce((n, l) => n + l.counts.subscribed, 0)
        : undefined,
    designs: canManage ? sized(designs) : undefined,
  }
}

export function SenderLine({ inboxId, channelId }: { inboxId?: string; channelId?: string }) {
  const channels = useInboxChannels(inboxId ?? "").data
  const channel =
    channels?.find((c) => c.id === channelId) ?? channels?.find((c) => c.kind === "email" && senderState(c) === "ready")
  const check = useDeliverability(channel?.email?.smtp ? channel.id : undefined).data
  if (!inboxId || !channel?.email) return null
  const address = channel.email.from_address || channel.email.address
  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5" data-testid="broadcasts-sender">
      <span
        aria-hidden
        className={cn("size-[7px] shrink-0 rounded-full", check && senderOk(check) ? "bg-success" : "bg-warning")}
      />
      <Trans>
        Sending from{" "}
        <span className="font-medium text-foreground" dir="ltr">
          {address}
        </span>
      </Trans>
      <span aria-hidden>·</span>
      {check === undefined ? null : check && senderOk(check) ? (
        <span>
          <Trans>SPF, DKIM and DMARC pass</Trans>
        </span>
      ) : (
        <Link
          to={channelLink(inboxId, channel.id, "deliverability")}
          className="text-warning underline-offset-4 hover:underline"
          data-testid="broadcasts-sender-check"
        >
          {check ? <Trans>SPF, DKIM or DMARC needs a look</Trans> : <Trans>Check SPF, DKIM and DMARC</Trans>}
        </Link>
      )}
    </span>
  )
}

function senderOk(c: DeliverabilityCheck) {
  return c.state === "received" && c.spf.status === "pass" && c.dkim.status === "pass" && c.dmarc.status === "pass"
}

export function BroadcastsLayout({
  aside,
  testId,
  children,
}: {
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <PageLayout
      testId={testId}
      label={t`Broadcasts`}
      aside={aside && <InfoColumn label={t`Details`}>{aside}</InfoColumn>}
    >
      {children}
    </PageLayout>
  )
}

export function DetailFrame({
  testId,
  back,
  heading,
  actions,
  aside,
  wide,
  children,
}: {
  testId: string
  back: { to: To & string; label: string }
  heading: React.ReactNode
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
      />
      <div className={cn("@container flex flex-col gap-4", wide ? "max-w-[980px]" : "max-w-[760px]")}>
        <div className="mt-1">{heading}</div>
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
