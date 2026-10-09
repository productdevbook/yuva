import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useNavigate } from "react-router"

import { MemberAvatar } from "@/components/common"
import { formatDuration } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { popupClass } from "@/components/ui/dropdown-menu"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Separator } from "@/components/ui/separator"
import { useStats } from "@/features/inbox/queries"
import { useQueue } from "@/features/inbox/queue"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Member } from "@/lib/api"
import { useAllViewers } from "@/lib/presence"
import { useSession } from "@/lib/session"
import { useAllTyping } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useMembers } from "@/lib/workspace"
import { Item } from "@/components/ui/item"

export function useConversationName() {
  const { t } = useLingui()
  const { open } = useQueue()
  const items = open.data?.pages.flatMap((p) => p.items) ?? []
  return (id: string) => {
    const c = items.find((x) => x.id === id)
    return c ? c.contact.name || c.contact.email || t`Unnamed contact` : t`another conversation`
  }
}

export function useMemberActivity() {
  const viewers = useAllViewers()
  const typing = useAllTyping()
  return (m: Member) => {
    for (const [conversationId, authors] of typing) {
      if (authors.some((a) => a.type === "member" && a.member_id === m.id)) return { conversationId, typing: true }
    }
    for (const [conversationId, ids] of viewers) if (ids.includes(m.id)) return { conversationId, typing: false }
    return null
  }
}

export function TeamSummary({ className }: { className?: string }) {
  const { i18n } = useLingui()
  const { inboxId } = useQueue()
  const stats = useStats(inboxId).data
  if (!stats) return null
  const replies = stats.replies
  const closed = stats.closed
  const median = stats.median_first_reply_seconds
  const first = median !== undefined ? formatDuration(median, i18n.locale) : ""
  const rated = stats.ratings.good + stats.ratings.bad
  const satisfied = rated > 0 ? new Intl.NumberFormat(i18n.locale, { style: "percent" }).format(stats.ratings.good / rated) : ""
  return (
    <span className={className}>
      <Plural value={replies} one="# reply" other="# replies" />
      {median !== undefined && (
        <>
          {" · "}
          <Trans>first reply in {first}</Trans>
        </>
      )}
      {" · "}
      <Plural value={closed} one="# closed" other="# closed" />
      {rated > 0 && (
        <>
          {" · "}
          <Trans>{satisfied} satisfied</Trans>
        </>
      )}
    </span>
  )
}

export function TeamMenu() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { membership } = useSession()
  const phone = useIsPhone()
  const mates = (useMembers().data ?? []).filter((m) => m.id !== membership.member_id)
  const activity = useMemberActivity()
  const label = useConversationName()
  const [open, setOpen] = useState(false)
  if (mates.length === 0) return null
  const order = [...mates].sort((a, b) => Number(b.online && b.availability === "auto") - Number(a.online && a.availability === "auto"))
  const shown = order.slice(0, phone ? 1 : 3)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={<Button variant="ghost" size="sm" />}
        aria-label={t`Team`}
        title={t`Team`}
        data-testid="team-button"
      >
        {shown.map((m, i) => (
          <span key={m.id} className={cn(i > 0 && "-ms-1.5")}>
            <MemberAvatar name={m.name || m.email} online={m.online} away={m.availability === "away"} className="ring-2 ring-background" />
          </span>
        ))}
      </PopoverTrigger>
      <PopoverContent side="bottom" align="end" sideOffset={8} className={cn(popupClass, "w-[300px] max-w-[calc(100vw-24px)] gap-0 rounded-xl p-1.5 ring-0")} data-testid="team-popup">
            <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
              <Trans>Team right now</Trans>
            </p>
            {order.map((m) => {
              const name = m.name || m.email
              const away = m.availability === "away"
              const a = activity(m)
              const where = a ? label(a.conversationId) : ""
              const what = away
                ? t`Away`
                : !m.online
                  ? t`Offline`
                  : a
                    ? a.typing
                      ? t`Typing · ${where}`
                      : t`Looking at ${where}`
                    : t`Online`
              const row = (
                <>
                  <MemberAvatar name={name} online={m.online} away={away} className="size-[30px]" ring="ring-card" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-body">{name}</span>
                    <small className="block truncate text-caption text-faint">{what}</small>
                  </span>
                </>
              )
              return a && m.online && !away ? (
                <Item
                  key={m.id}
                  render={<button type="button" />}
                  size="xs"
                  className="flex-nowrap"
                  onClick={() => {
                    setOpen(false)
                    navigate(`/conversations/${a.conversationId}`)
                  }}
                >
                  {row}
                </Item>
              ) : (
                <div key={m.id} className="flex items-center gap-2.5 px-2.5 py-2">
                  {row}
                </div>
              )
            })}
            <Separator className="mx-1.5 my-1 w-auto" />
            <div className="px-2.5 pt-1 pb-1.5">
              <span className="block text-body">
                <Trans>The team today</Trans>
              </span>
              <TeamSummary className="block text-caption text-faint" />
            </div>
      </PopoverContent>
    </Popover>
  )
}
