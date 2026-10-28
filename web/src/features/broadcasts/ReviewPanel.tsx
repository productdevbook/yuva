import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon, TriangleAlertIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link } from "react-router"

import { ErrorLine, Notice, Segmented } from "@/components/common"
import { BROADCAST_KEYS } from "@/components/common/ShortcutSheet"
import { formatBytes, languageName } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { problemOf, useSchedule, useUpdateBroadcast } from "@/features/broadcasts/broadcastQueries"
import { paceGap, usePaceSummary } from "@/features/broadcasts/Pace"
import { keyHint, useBroadcastsStatus, useNumber } from "@/features/broadcasts/BroadcastParts"
import { ZonePicker } from "@/features/broadcasts/SendTime"
import { useTriggerSentence } from "@/features/broadcasts/Trigger"
import { footerInEnglish, missing, versions, type Doc } from "@/features/designs/doc"
import { designLink, useDesign } from "@/features/designs/queries"
import { channelLink, isChecking, useDeliverability } from "@/features/settings/channels/email/deliverability"
import type {
  Broadcast,
  BroadcastReview,
  BroadcastReviewItem,
  BroadcastReviewKey,
  BroadcastReviewStatus,
} from "@/lib/api"
import { ApiError } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"
import {
  addDays,
  browserZone,
  dayLabel,
  localParts,
  localToInstant,
  nextWindowOpening,
  zoneCity,
  zoneLabel,
  zoneOffset,
  zonedDateTime,
  wallClock,
  clock,
} from "@/lib/zones"
import { cn } from "@/lib/utils"

export type Spot = "trigger" | "sender" | "audience" | "content" | "options" | "pace" | "hours" | "test"

type Fix = { spot: Spot } | { to: string }

const ORDER: BroadcastReviewKey[] = [
  "approval",
  "trigger",
  "basis",
  "sender_identity",
  "sender_ready",
  "deliverability",
  "audience_nonempty",
  "design",
  "subject",
  "translations",
  "merge_fields",
  "html_size",
  "links",
  "images_alt",
  "click_tracking",
  "daily_cap",
  "pace",
  "send_window",
  "ses_tenant",
  "test_sent",
]

function useDeliverabilityText(b: Broadcast, review: BroadcastReview) {
  const { t } = useLingui()
  const item = review.items.find((i) => i.key === "deliverability")
  const wanted = !!item && item.status !== "ok"
  const check = useDeliverability(wanted ? b.channel_id : undefined).data
  if (item?.status === "blocking")
    return t`The last check found no passing DKIM signature. Set up DKIM with your SMTP provider, or turn on Yuva DKIM for the channel.`
  if (check === null)
    return t`The channel’s deliverability was never checked. Run the check before the first broadcast.`
  if (!check) return t`The last deliverability check found something to fix.`
  if (isChecking(check)) return t`The deliverability check has not finished yet.`
  if (check.state === "timeout") return t`The last check’s test mail did not come back within 10 minutes. Check again.`
  if (check.state === "failed")
    return t`The last check could not send its test mail. Look at the channel’s SMTP settings.`
  const found: string[] = []
  if (check.state === "unverifiable")
    found.push(t`Only DNS could be checked, not how receiving servers judge the mail.`)
  else if (!check.dkim.aligned) found.push(t`DKIM does not pass for the sending domain.`)
  else if (!check.dkim.covers_list_unsubscribe)
    found.push(t`The DKIM signature does not cover the unsubscribe headers.`)
  if (!check.dmarc.record) found.push(t`The sending domain has no DMARC record.`)
  return found.length ? found.join(" ") : t`The last deliverability check found something to fix.`
}

export function useItemText(b: Broadcast, review: BroadcastReview) {
  const { t, i18n } = useLingui()
  const deliverability = useDeliverabilityText(b, review)
  const doc = useDesign(b.design_id ?? "").data?.document
  const status = useBroadcastsStatus()
  const tenantMissing = useInboxes().data?.find((i) => i.id === b.inbox_id)?.ses_tenant?.state === "missing"
  const size = review.html_bytes !== undefined ? formatBytes(review.html_bytes, i18n.locale) : ""
  const days = review.days ?? 0
  const paceDays = Math.ceil((paceGap(b.pace) * Math.max(0, review.recipients - 1)) / 86400)
  const titles: Record<BroadcastReviewKey, string> = {
    sender_identity: t`Sender details for the footer`,
    sender_ready: t`Sending channel`,
    deliverability: t`Deliverability`,
    approval: t`Permission to send`,
    design: t`Design`,
    audience_nonempty: t`Recipients`,
    merge_fields: t`Merge fields`,
    html_size: t`Mail size`,
    images_alt: t`Image descriptions`,
    links: t`Links`,
    subject: t`Subject`,
    daily_cap: t`Daily limit`,
    pace: t`Sending pace`,
    send_window: t`Sending hours`,
    test_sent: t`Test mail`,
    click_tracking: t`Click tracking`,
    ses_tenant: t`Amazon SES tenant`,
    translations: t`Languages`,
    message_block: t`Message block`,
    trigger: t`Trigger`,
    basis: t`Why they get this mail`,
  }
  const name = (tag: string) => languageName(tag, i18n.locale)
  const list = (xs: string[]) => new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(xs)
  const translationText = () => {
    const tags = doc ? versions(doc) : []
    const own = b.translations ?? {}
    const found: string[] = []
    if (tags.length > 1) {
      const base = name(tags[0])
      const noSubject = tags.slice(1).filter((tag) => !own[tag]?.subject)
      const gaps = tags
        .slice(1)
        .map((tag) => [tag, missing(doc!, tag).length] as const)
        .filter(([, n]) => n > 0)
      const english = tags.filter(footerInEnglish)
      if (noSubject.length) {
        const languages = list(noSubject.map(name))
        found.push(t`Without a subject of their own, these get the ${base} subject: ${languages}.`)
      }
      if (gaps.length) {
        const languages = list(gaps.map(([tag, n]) => `${name(tag)} (${n})`))
        found.push(t`Some texts are not translated, so the ${base} text shows: ${languages}.`)
      }
      if (english.length) {
        const languages = list(english.map(name))
        found.push(t`The required footer is in English for: ${languages}.`)
      }
    }
    const unused = Object.keys(own)
      .filter((tag) => !tags.includes(tag))
      .sort()
    if (unused.length) {
      const languages = list(unused.map(name))
      found.push(t`The design has no version for these subjects, so they are not used: ${languages}.`)
    }
    return found.length ? found.join(" ") : t`Some language versions are not complete.`
  }
  const problem = (it: BroadcastReviewItem): string | null => {
    const blocking = it.status === "blocking"
    switch (it.key) {
      case "sender_identity":
        return t`The inbox has no legal name or postal address yet. Every broadcast ends with them, as the law asks.`
      case "sender_ready":
        return t`The channel cannot send yet. It needs an SMTP account and an address to send from.`
      case "deliverability":
        return deliverability
      case "approval":
        return status?.status === "suspended"
          ? t`The workspace may not send broadcasts for now.`
          : status?.status === "approval_requested"
            ? t`The server’s operator has not answered the request yet.`
            : t`The server’s operator has to approve broadcasts for this workspace first.`
      case "design":
        return t`Pick a design that renders without errors.`
      case "audience_nonempty":
        return t`Nobody would get this mail. Pick lists with subscribers, or loosen the conditions.`
      case "merge_fields":
        return t`A merge field without a fallback would be empty for some recipients. Give it a fallback.`
      case "html_size":
        return blocking
          ? t`The mail is ${size}. Gmail cuts mail off above 102 KB, so make it shorter.`
          : t`The mail is ${size}, close to the 102 KB where Gmail cuts it off.`
      case "images_alt":
        return t`Some images have no description for people who cannot see them.`
      case "links":
        return blocking ? t`Some links are not valid.` : t`Some links use http: instead of https:.`
      case "subject":
        return blocking
          ? t`Write a subject of at most 300 characters.`
          : t`Inboxes cut off subjects longer than 80 characters.`
      case "daily_cap":
        return days > 1 ? t`Under the workspace’s daily limit, sending takes ${days} days.` : null
      case "pace":
        return blocking
          ? t`At this pace, sending would take about ${paceDays} days. A broadcast may take at most 365 days: raise the pace or send to fewer people.`
          : null
      case "send_window": {
        const opening = b.send_window
          ? nextWindowOpening(b.timezone, new Date(), b.send_window.start, b.send_window.end)
          : null
        if (!opening) return null
        const time = wallClock(opening.time, i18n.locale)
        const date = dayLabel(opening.date, i18n.locale)
        return t`The sending hours are closed right now; sending starts at ${time} on ${date}.`
      }
      case "test_sent":
        return t`No test went out since the last change.`
      case "ses_tenant":
        return tenantMissing
          ? t`The inbox’s Amazon SES tenant no longer exists. The server creates it again; until then the broadcast cannot go out.`
          : blocking
            ? t`Amazon SES paused this inbox’s tenant, or the channel lost its tenant isolation. The broadcast cannot go out until the server’s operator fixes it.`
            : t`The inbox’s Amazon SES tenant is still being set up. The broadcast can go out, but check this again before it starts.`
      case "click_tracking":
        return t`Links will point at this server’s link domain instead of your own. Spam filters look at link domains.`
      case "translations":
        return translationText()
      case "trigger":
        return t`Choose what starts the e-mail and how long after it the mail goes out.`
      case "basis":
        return b.trigger?.basis === "service"
          ? t`Service mail is only about using the product: onboarding, the person’s account, a request for feedback. Advertising, newsletters and offers need the marketing basis.`
          : t`Pick the lists whose subscribers get this mail.`
      case "message_block":
        return t`This design has a message block, which holds the text of a conversation message. A broadcast would show it empty. Pick another design or take the block out.`
    }
  }
  return { titles, problem }
}

export function fixOf(b: Broadcast, key: BroadcastReviewKey, doc?: Doc): Fix | null {
  switch (key) {
    case "sender_identity":
      return { to: `/settings/inboxes/${b.inbox_id}` }
    case "trigger":
      return { spot: "trigger" }
    case "basis":
      return { spot: "audience" }
    case "sender_ready":
      return { spot: "sender" }
    case "deliverability":
      return b.channel_id ? { to: channelLink(b.inbox_id, b.channel_id, "deliverability") } : { spot: "sender" }
    case "approval":
      return { to: "/settings/workspace" }
    case "audience_nonempty":
      return { spot: "audience" }
    case "design":
    case "subject":
      return { spot: "content" }
    case "merge_fields":
    case "message_block":
    case "html_size":
    case "links":
    case "images_alt":
      return b.design_id ? { to: designLink(b.design_id) } : { spot: "content" }
    case "click_tracking":
      return { spot: "options" }
    case "ses_tenant":
      return { to: `/settings/inboxes/${b.inbox_id}` }
    case "test_sent":
      return { spot: "test" }
    case "pace":
      return { spot: "pace" }
    case "send_window":
      return { spot: "hours" }
    case "daily_cap":
      return null
    case "translations": {
      const gap = doc && b.design_id ? versions(doc).find((tag, i) => i > 0 && missing(doc, tag).length > 0) : undefined
      return gap && b.design_id
        ? { to: `${designLink(b.design_id)}?lang=${encodeURIComponent(gap)}` }
        : { spot: "content" }
    }
  }
}

export type Part = "sender" | "audience" | "content" | "subject"

export const PART_OF: Partial<Record<BroadcastReviewKey, Part>> = {
  approval: "sender",
  sender_identity: "sender",
  sender_ready: "sender",
  deliverability: "sender",
  ses_tenant: "sender",
  audience_nonempty: "audience",
  trigger: "audience",
  basis: "audience",
  design: "content",
  merge_fields: "content",
  html_size: "content",
  links: "content",
  images_alt: "content",
  translations: "content",
  subject: "subject",
}

const RANK = { blocking: 0, warning: 1, ok: 2 } as const

export function worstOf(items: BroadcastReviewItem[]): BroadcastReviewStatus {
  return items.reduce<BroadcastReviewStatus>((w, it) => (RANK[it.status] < RANK[w] ? it.status : w), "ok")
}

const stateTone = {
  ok: "border-success/35 text-success",
  warning: "border-warning/40 text-warning",
  blocking: "border-destructive/35 text-destructive",
}

export function CheckState({ status, className }: { status: BroadcastReviewStatus; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex min-w-14 shrink-0 items-center justify-center whitespace-nowrap rounded-full border px-2 py-px text-caption font-medium",
        stateTone[status],
        className
      )}
      data-status={status}
    >
      {status === "ok" ? <Trans>Ready</Trans> : status === "warning" ? <Trans>Check</Trans> : <Trans>Fix</Trans>}
    </span>
  )
}

export function Checklist({
  b,
  review,
  onFix,
  readOnly,
}: {
  b: Broadcast
  review: BroadcastReview
  onFix: (spot: Spot) => void
  readOnly: boolean
}) {
  const { t } = useLingui()
  const { titles, problem } = useItemText(b, review)
  const doc = useDesign(b.design_id ?? "").data?.document
  const items = [...review.items].sort(
    (x, y) => RANK[x.status] - RANK[y.status] || ORDER.indexOf(x.key) - ORDER.indexOf(y.key)
  )
  return (
    <ul className="flex flex-col divide-y rounded-xl border" data-testid="review-items">
      {items.map((it) => {
        const text = it.status === "ok" ? null : problem(it)
        const fix = it.status === "ok" || readOnly ? null : fixOf(b, it.key, doc)
        return (
          <li
            key={it.key}
            className="flex items-start gap-3 px-3.5 py-2.5"
            data-testid="review-item"
            data-key={it.key}
            data-status={it.status}
          >
            <CheckState status={it.status} className="mt-0.5" />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className={cn("text-body", it.status === "ok" ? "text-muted-foreground" : "font-medium")}>
                {titles[it.key]}
              </span>
              {text && <span className="text-small text-muted-foreground">{text}</span>}
              {fix &&
                ("to" in fix ? (
                  <Link to={fix.to} className="w-fit text-small text-brand underline-offset-4 hover:underline">
                    {t`Fix it`}
                  </Link>
                ) : (
                  <button
                    type="button"
                    onClick={() => onFix(fix.spot)}
                    className="w-fit text-small text-brand underline-offset-4 hover:underline"
                  >
                    {t`Fix it`}
                  </button>
                ))}
            </div>
          </li>
        )
      })}
    </ul>
  )
}

export function ReviewDialog({
  b,
  review,
  reviewError,
  fetching = false,
  from,
  onFix,
  onClose,
}: {
  b: Broadcast
  review: BroadcastReview | undefined
  reviewError: unknown
  fetching?: boolean
  from: string
  onFix: (spot: Spot) => void
  onClose: () => void
}) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const paceSummary = usePaceSummary()
  const [step, setStep] = useState<1 | 2>(1)
  const [mode, setMode] = useState<"now" | "later">("now")
  const [now] = useState(() => Date.now())
  const zone = b.timezone
  const [date, setDate] = useState(() => localParts(zone, new Date(now + 60 * 60_000)).date)
  const [time, setTime] = useState(() => localParts(zone, new Date(now + 60 * 60_000)).time)
  const [confirmed, setConfirmed] = useState<number | null>(null)
  const schedule = useSchedule(b.id)
  const update = useUpdateBroadcast(b.id)
  const triggered = b.kind === "triggered"
  const service = triggered && b.trigger?.basis === "service"
  const stored = b.trigger?.backfill_days ?? 0
  const [days, setDays] = useState(String(stored))
  const [serviceOk, setServiceOk] = useState(false)
  const daysValid = /^\d{1,2}$/.test(days) && Number(days) <= 30
  const synced = daysValid && Number(days) === stored && !update.isPending && !fetching
  const sentence = useTriggerSentence(b)
  const inboxName = useInboxes().data?.find((i) => i.id === b.inbox_id)?.name ?? ""
  const { mutate: save } = update
  useEffect(() => {
    if (!triggered || !daysValid || Number(days) === stored) return
    const timer = setTimeout(() => save({ trigger: { backfill_days: Number(days) } }), 500)
    return () => clearTimeout(timer)
  }, [triggered, daysValid, days, stored, save])
  const inboxZone = useInboxes().data?.find((i) => i.id === b.inbox_id)?.timezone
  const local = `${date}T${time}`
  const sendAt = mode === "later" && date && time ? localToInstant(local, zone) : null
  const atValid =
    mode === "now" ||
    (!!sendAt && !Number.isNaN(sendAt.getTime()) && sendAt.getTime() > now && sendAt.getTime() < now + 90 * 86400_000)
  const ready = !!review?.ready
  const count = confirmed ?? review?.recipients ?? 0
  const recipients = num(count)
  const name = b.name
  const shown = sendAt && atValid ? sendAt : null
  const when = shown
    ? `${zonedDateTime(shown.toISOString(), zone, i18n.locale)} ${zoneLabel(zone, shown, i18n.locale)}`
    : ""
  const city = zoneCity(zone)
  const offset = shown ? zoneOffset(zone, shown, i18n.locale) : ""
  const sendDate = shown ? dayLabel(localParts(zone, shown).date, i18n.locale) : ""
  const sendTime = shown ? clock(shown, zone, i18n.locale) : ""
  const utcTime = shown ? clock(shown, "UTC", i18n.locale) : ""
  const mine = browserZone()
  const mineTime = shown ? clock(shown, mine, i18n.locale) : ""
  const mineName = shown ? zoneLabel(mine, shown, i18n.locale) : ""
  const showMine = !!shown && mine !== zone && zoneOffset(mine, shown, "en") !== zoneOffset(zone, shown, "en")
  const closed = shown && b.send_window ? nextWindowOpening(zone, shown, b.send_window.start, b.send_window.end) : null
  const openTime = closed ? wallClock(closed.time, i18n.locale) : ""
  const openDate = closed ? dayLabel(closed.date, i18n.locale) : ""
  const pace = paceSummary(b.pace)
  const problem = problemOf(schedule.error)
  const changed = schedule.error instanceof ApiError && problem?.code === "audience_changed"
  const sendTitle = keyHint(
    triggered ? t`Review, then start or schedule` : t`Review, then send or schedule`,
    BROADCAST_KEYS.send
  )
  const submit = () => {
    schedule.mutate(
      {
        send_at_local: mode === "later" && sendAt ? local : undefined,
        confirm_recipients: count,
        confirm_service: service ? true : undefined,
      },
      {
        onSuccess: onClose,
        onError: (err) => {
          const p = problemOf(err)
          if (p?.code === "audience_changed" && p.recipients !== undefined) setConfirmed(p.recipients)
        },
      }
    )
  }
  return (
    <Dialog open onOpenChange={(o) => !o && !schedule.isPending && onClose()}>
      <DialogContent className="max-h-[90svh] gap-4 overflow-y-auto sm:max-w-xl" data-testid="review-dialog">
        {step === 1 ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-3 pe-8">
                <span className="min-w-0 flex-1">
                  {triggered ? <Trans>Review and start</Trans> : <Trans>Review and send</Trans>}
                </span>
                {review && (
                  <span
                    className={cn("text-caption font-medium", ready ? "text-success" : "text-destructive")}
                    data-testid="review-ready"
                  >
                    {ready ? <Trans>Ready</Trans> : <Trans>Not ready</Trans>}
                  </span>
                )}
              </DialogTitle>
              <DialogDescription className="sr-only">
                {triggered ? <Trans>Review and start</Trans> : <Trans>Review and send</Trans>}
              </DialogDescription>
            </DialogHeader>
            {review ? (
              <Checklist
                b={b}
                review={review}
                onFix={(spot) => {
                  onClose()
                  onFix(spot)
                }}
                readOnly={false}
              />
            ) : reviewError ? (
              <ErrorLine error={reviewError} />
            ) : (
              <div className="flex flex-col gap-2">
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-full" />
              </div>
            )}
            {triggered && (
              <div className="flex flex-col gap-3 border-t pt-4" data-testid="start-backfill">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="start-backfill-days">
                    <Trans>Also send to people who first signed in during the last</Trans>
                  </Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="start-backfill-days"
                      inputMode="numeric"
                      value={days}
                      maxLength={2}
                      aria-invalid={!daysValid || undefined}
                      className="w-20"
                      onChange={(e) => setDays(e.target.value.trim())}
                      data-testid="start-backfill-days"
                    />
                    <span className="text-body text-muted-foreground">
                      <Trans>days (0 to 30)</Trans>
                    </span>
                  </div>
                  <p className="text-small text-faint" data-testid="start-backfill-count" aria-live="polite">
                    {!daysValid ? (
                      <Trans>Use a whole number from 0 to 30.</Trans>
                    ) : !synced ? (
                      <Trans>Counting…</Trans>
                    ) : count === 0 ? (
                      <Trans>Only people who sign in from now on get it.</Trans>
                    ) : (
                      <Trans>
                        <Plural value={count} one="# person" other="# people" /> who signed in before would get it too,
                        before anyone who bounced, left or does not match is left out.
                      </Trans>
                    )}
                  </p>
                </div>
                {service && (
                  <Label className="items-start gap-3 font-normal" data-testid="start-service">
                    <Checkbox checked={serviceOk} onCheckedChange={(on) => setServiceOk(on)} className="mt-0.5" />
                    <span className="flex min-w-0 flex-col">
                      <span className="text-body font-medium">
                        <Trans>This mail is about using {inboxName}, not advertising.</Trans>
                      </span>
                      <span className="text-small text-faint">
                        <Trans>Who confirmed it and when is saved in the audit log.</Trans>
                      </span>
                    </span>
                  </Label>
                )}
              </div>
            )}
            <div className="flex flex-col gap-3 border-t pt-4">
              <Segmented
                value={mode}
                onChange={setMode}
                label={t`When`}
                className="w-full"
                itemClassName="flex-1"
                items={[
                  { value: "now", label: triggered ? t`Start now` : t`Send now`, testId: "send-mode-now" },
                  { value: "later", label: t`Schedule`, testId: "send-mode-later" },
                ]}
              />
              {mode === "later" && (
                <div className="flex flex-col gap-2">
                  <div className="grid grid-cols-2 gap-2">
                    <Input
                      type="date"
                      value={date}
                      min={localParts(zone, new Date(now)).date}
                      max={addDays(localParts(zone, new Date(now)).date, 90)}
                      onChange={(e) => setDate(e.target.value)}
                      aria-label={t`Send on`}
                      aria-invalid={!atValid || undefined}
                      data-testid="send-at"
                    />
                    <Input
                      type="time"
                      value={time}
                      onChange={(e) => setTime(e.target.value)}
                      aria-label={t`At`}
                      aria-invalid={!atValid || undefined}
                      data-testid="send-at-time"
                    />
                  </div>
                  <div className="flex items-center gap-2 text-small text-faint">
                    <Trans>Time zone</Trans>
                    <ZonePicker
                      value={zone}
                      inboxZone={inboxZone}
                      disabled={update.isPending}
                      onChange={(timezone) => {
                        update.mutate({ timezone })
                      }}
                      className="h-8 w-auto max-w-full flex-1"
                    />
                  </div>
                  <div className="flex flex-col gap-0.5 text-small text-faint" data-testid="send-zone">
                    {shown ? (
                      <>
                        <p>
                          <Trans>
                            Sends on {sendDate} at {sendTime} {city} time ({offset})
                          </Trans>
                        </p>
                        <p>
                          <Trans>= {utcTime} UTC</Trans>
                        </p>
                        {showMine && (
                          <p>
                            <Trans>
                              = {mineTime} your time ({mineName})
                            </Trans>
                          </p>
                        )}
                        {closed && (
                          <p className="text-warning" data-testid="send-window-closed">
                            <Trans>
                              The sending hours are closed then; sending starts at {openTime} on {openDate}.
                            </Trans>
                          </p>
                        )}
                      </>
                    ) : (
                      <p>
                        <Trans>Pick a time in the next 90 days.</Trans>
                      </p>
                    )}
                  </div>
                </div>
              )}
            </div>
            <DialogFooter className="sticky -bottom-6 -mx-6 -mb-6 border-t bg-card px-6 py-4">
              <Button variant="ghost" onClick={onClose}>
                <Trans>Cancel</Trans>
              </Button>
              <Button
                onClick={() => {
                  schedule.reset()
                  setConfirmed(null)
                  setStep(2)
                }}
                disabled={!ready || !atValid || (triggered ? !synced || (service && !serviceOk) : count === 0)}
                title={sendTitle}
                data-testid="open-send"
              >
                <Trans>Continue</Trans>
              </Button>
            </DialogFooter>
          </>
        ) : (
          <div data-testid="send-confirm" className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>
                {triggered ? (
                  mode === "now" ? (
                    <Trans>Start “{name}” now?</Trans>
                  ) : (
                    <Trans>
                      Start “{name}” on {when}?
                    </Trans>
                  )
                ) : mode === "now" ? (
                  <Trans>Send “{name}” now?</Trans>
                ) : (
                  <Trans>
                    Send “{name}” on {when}?
                  </Trans>
                )}
              </DialogTitle>
              <DialogDescription className="flex flex-col gap-2">
                <span className="font-medium text-foreground" data-testid="confirm-recipients">
                  {triggered ? (
                    count === 0 ? (
                      <Trans>Nobody who signed in before gets it.</Trans>
                    ) : (
                      <Plural
                        value={count}
                        one="# person who signed in before gets it too."
                        other="# people who signed in before get it too."
                      />
                    )
                  ) : (
                    <Plural value={count} one="It goes to # recipient." other="It goes to # recipients." />
                  )}
                </span>
                <span>
                  {from ? <Trans>From {from}</Trans> : null}
                  {from ? " · " : null}
                  {pace}
                </span>
                <span>
                  {triggered ? (
                    <Trans>
                      Mails go out {sentence}, once to each person. Pausing stops sending but keeps counting people who
                      sign in, so nobody is missed. You can stop it for good at any time.
                    </Trans>
                  ) : (
                    <Trans>
                      The recipients are taken when sending starts: people who subscribe later do not get it, and anyone
                      who unsubscribes before their mail goes out is skipped. You can pause or cancel while it sends.
                    </Trans>
                  )}
                </span>
                {mode === "later" && (
                  <span>
                    <Trans>Until it starts you can go back to a draft.</Trans>
                  </span>
                )}
              </DialogDescription>
            </DialogHeader>
            {changed && (
              <Notice tone="warning" icon={TriangleAlertIcon} data-testid="audience-changed">
                <p>
                  <Trans>
                    The number of recipients changed while you looked. It is {recipients} now. Confirm again to send to
                    them.
                  </Trans>
                </p>
              </Notice>
            )}
            {schedule.error && !changed && <ErrorLine error={schedule.error} />}
            <DialogFooter>
              <Button variant="ghost" onClick={() => setStep(1)} disabled={schedule.isPending}>
                <Trans>Back</Trans>
              </Button>
              <Button onClick={submit} disabled={schedule.isPending} data-testid="confirm-send">
                {schedule.isPending && <LoaderIcon className="animate-spin" />}
                {mode === "now" ? (
                  triggered ? (
                    <Trans>Start now</Trans>
                  ) : (
                    <Trans>Send now</Trans>
                  )
                ) : (
                  <Trans>Schedule</Trans>
                )}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
