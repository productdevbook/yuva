import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useState } from "react"

import { ChannelIcon, ContactAvatar, Dot, MemberAvatar } from "@/components/common"
import { formatDateTime, useEnumText } from "@/components/common/text"
import { useContactPresence } from "@/features/contact/queries"
import { CategoryChip } from "@/features/conversation/Feedback"
import { currentRating, RatingMark } from "@/features/conversation/rating"
import { RelatedLine } from "@/features/conversation/RelatedLine"
import type { Channel, Conversation, Message } from "@/lib/api"
import { useViewers } from "@/lib/presence"
import { useSession } from "@/lib/session"
import { useTyping } from "@/lib/typing"
import { useInboxes, useLabels, useMemberMap } from "@/lib/workspace"

function useNow(ms: number) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(id)
  }, [ms])
  return now
}

function WaitingFor({ since }: { since: string }) {
  const now = useNow(30_000)
  const minutes = Math.max(0, Math.floor((now - new Date(since).getTime()) / 60_000))
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)
  if (minutes < 1) return <Trans>Just wrote</Trans>
  if (hours < 1) return <Plural value={minutes} one="Waiting for # minute" other="Waiting for # minutes" />
  if (days < 1) return <Plural value={hours} one="Waiting for # hour" other="Waiting for # hours" />
  return <Plural value={days} one="Waiting for # day" other="Waiting for # days" />
}

function Status({ c, lastInbound }: { c: Conversation; lastInbound?: Message }) {
  const { i18n } = useLingui()
  const { membership } = useSession()
  const members = useMemberMap()
  const presence = useContactPresence(c.contact_id).data
  if (presence?.online) {
    return (
      <span className="inline-flex items-center gap-[5px] text-green-600 dark:text-green-500" data-testid="contact-online">
        <span className="size-1.5 rounded-full bg-current" />
        <Trans>Online now</Trans>
      </span>
    )
  }
  if (c.status === "open" && c.assignee_id && c.assignee_id !== membership.member_id) {
    const m = members.get(c.assignee_id)
    const name = m ? m.name || m.email : "…"
    return <Trans>With {name}</Trans>
  }
  if (c.status === "open") {
    const waiting = c.last_message_at && lastInbound && lastInbound.created_at >= c.last_message_at
    return waiting ? (
      <span className="text-brand">
        <WaitingFor since={lastInbound.created_at} />
      </span>
    ) : (
      <Trans>Open</Trans>
    )
  }
  if (c.status === "pending") return <Trans>Waiting for the customer</Trans>
  if (c.status === "snoozed" && c.snooze_until) {
    const until = formatDateTime(c.snooze_until, i18n.locale)
    return <Trans>Snoozed until {until}</Trans>
  }
  const rating = currentRating(c)
  return (
    <span className="inline-flex items-center gap-1.5">
      <Trans>Done</Trans>
      {rating && <RatingMark rating={rating.rating} comment={rating.comment} />}
    </span>
  )
}

function Watchers({ conversationId }: { conversationId: string }) {
  const { t } = useLingui()
  const { membership } = useSession()
  const members = useMemberMap()
  const viewers = useViewers(conversationId)
  const typing = useTyping(conversationId)
    .filter((a) => a.type === "member" && a.member_id && a.member_id !== membership.member_id)
    .map((a) => a.member_id!)
  const ids = [...new Set([...typing, ...viewers])]
  return ids.map((id) => {
    const m = members.get(id)
    const full = m ? m.name || m.email : t`A teammate`
    const name = full.trim().split(/\s+/)[0]
    return (
      <span
        key={id}
        className="ms-1 inline-flex items-center gap-[5px] rounded-full border border-mate/35 bg-mate/10 py-0.5 ps-0.5 pe-2 text-xs text-muted-foreground"
        data-testid="viewer-chip"
      >
        <MemberAvatar name={full} className="size-[18px] text-[8px]" />
        {typing.includes(id) ? <Trans>{name} is typing…</Trans> : <Trans>{name} is looking</Trans>}
      </span>
    )
  })
}

export function PersonHeader({
  c,
  name,
  channel,
  lastInbound,
  onContact,
  menu,
}: {
  c: Conversation
  name: string
  channel?: Channel
  lastInbound?: Message
  onContact: () => void
  menu: React.ReactNode
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const labels = (useLabels().data ?? []).filter((l) => c.labels.includes(l.id))
  const sep = <span aria-hidden>·</span>
  return (
    <div className="flex items-center gap-4" data-testid="person">
      <button type="button" onClick={onContact} className="relative shrink-0 rounded-full" aria-label={t`Contact details`}>
        <ContactAvatar id={c.contact_id} name={name} className="size-14 text-[19px] phone:size-12 phone:text-base" />
        {channel && (
          <span className="absolute -end-1 -bottom-1 grid size-6 place-items-center rounded-full border bg-card text-muted-foreground" title={text.channel[channel.kind]}>
            <ChannelIcon kind={channel.kind} className="size-[13px]" />
          </span>
        )}
      </button>
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-[26px] leading-[1.15] font-semibold tracking-[-0.025em] phone:text-[22px]" data-testid="person-name">
          <button type="button" onClick={onContact} className="max-w-full truncate text-start">
            {name}
          </button>
        </h1>
        <div className="mt-1 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-sm text-faint">
          {inbox && (
            <span className="inline-flex items-center gap-1.5 text-muted-foreground">
              <Dot color={inbox.branding.color} className="size-2 rounded-[3px]" />
              {inbox.name}
            </span>
          )}
          {channel && (
            <>
              {inbox && sep}
              <span>{text.channel[channel.kind]}</span>
            </>
          )}
          {(inbox || channel) && sep}
          <Status c={c} lastInbound={lastInbound} />
          <Watchers conversationId={c.id} />
          {c.kind === "feedback" && c.feedback && (
            <>
              {sep}
              <CategoryChip category={c.feedback.category} />
            </>
          )}
          {labels.map((l) => (
            <span key={l.id} className="inline-flex items-center gap-1 rounded-full border bg-card px-2 py-px text-xs text-muted-foreground">
              <Dot color={l.color} className="size-1.5" />
              {l.name}
            </span>
          ))}
        </div>
        {c.related_conversation_id && (
          <div className="mt-1">
            <RelatedLine id={c.related_conversation_id} hrefFor={(id) => `/conversations/${id}`} />
          </div>
        )}
      </div>
      {menu}
    </div>
  )
}
