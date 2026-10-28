import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { CircleAlertIcon, ClockIcon, GaugeIcon, LoaderIcon, PauseIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router"

import { ErrorLine, Notice, useConfirm } from "@/components/common"
import { PageBar } from "@/components/common/PageBar"
import { BROADCAST_KEYS } from "@/components/common/ShortcutSheet"
import { formatDateTime, formatDuration, useEnumText } from "@/components/common/text"
import { Skeleton } from "@/components/ui/skeleton"
import {
  broadcastLink,
  FINISHED,
  isActive,
  useBroadcast,
  useBroadcastAction,
  useDeleteBroadcast,
  type BroadcastAction,
} from "@/features/broadcasts/broadcastQueries"
import {
  keyHint,
  progressOf,
  ProgressBar,
  StateBadge,
  useNumber,
  PaceLine,
  WaitingLine,
  WindowLine,
} from "@/features/broadcasts/BroadcastParts"
import { Composer } from "@/features/broadcasts/Composer"
import { DesignThumb } from "@/features/broadcasts/DesignPicker"
import { useInboxChannels, useLists } from "@/features/broadcasts/queries"
import { Deliveries, Report, ReportStats } from "@/features/broadcasts/Report"
import { paceGap, PaceDialog, usePaceSummary } from "@/features/broadcasts/Pace"
import { SendTimeDialog } from "@/features/broadcasts/SendTime"
import { useTriggerSentence } from "@/features/broadcasts/Trigger"
import { designLink } from "@/features/designs/queries"
import { Card, Section } from "@/features/settings/ui"
import { FrameButton, FrameTabs, framePad, ProductTag } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Broadcast } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMemberMap } from "@/lib/workspace"
import {
  addDays,
  localParts,
  utcLine,
  wallClock,
  windowInstants,
  zoneCity,
  zoneLabel,
  zonedDateTime,
} from "@/lib/zones"

export { BroadcastsOverview as BroadcastsHome } from "@/features/broadcasts/Overview"

function Actions({
  b,
  run,
  pending,
  onDelete,
  onPace,
  onHours,
}: {
  b: Broadcast
  run: (a: BroadcastAction) => void
  pending: boolean
  onDelete: () => void
  onPace: () => void
  onHours: () => void
}) {
  const { t } = useLingui()
  const active = b.state === "sending" || b.state === "preparing"
  const live = b.state === "scheduled" || isActive(b)
  return (
    <>
      {live && (
        <FrameButton
          disabled={pending}
          onClick={() => run("cancel")}
          title={keyHint(t`Cancel the broadcast`, BROADCAST_KEYS.cancel)}
          data-testid="broadcast-cancel"
        >
          <Trans>Cancel broadcast</Trans>
        </FrameButton>
      )}
      {live && (
        <FrameButton
          onClick={onPace}
          title={keyHint(t`Change pace`, BROADCAST_KEYS.pace)}
          data-testid="broadcast-pace-change"
        >
          <GaugeIcon />
          <Trans>Change pace</Trans>
        </FrameButton>
      )}
      {live && (
        <FrameButton
          onClick={onHours}
          title={keyHint(t`Change the sending hours`, BROADCAST_KEYS.hours)}
          data-testid="broadcast-hours-change"
        >
          <ClockIcon />
          <Trans>Sending hours</Trans>
        </FrameButton>
      )}
      {b.state === "scheduled" && (
        <FrameButton
          ink
          disabled={pending}
          onClick={() => run("unschedule")}
          title={keyHint(t`Back to draft`, BROADCAST_KEYS.unschedule)}
          data-testid="broadcast-unschedule"
        >
          <Trans>Back to draft</Trans>
        </FrameButton>
      )}
      {active && (
        <FrameButton
          ink
          disabled={pending}
          onClick={() => run("pause")}
          title={keyHint(t`Pause`, BROADCAST_KEYS.pause)}
          data-testid="broadcast-pause"
        >
          <Trans>Pause</Trans>
        </FrameButton>
      )}
      {b.state === "paused" && (
        <FrameButton
          ink
          disabled={pending}
          onClick={() => run("resume")}
          title={keyHint(t`Resume`, BROADCAST_KEYS.pause)}
          data-testid="broadcast-resume"
        >
          <Trans>Resume</Trans>
        </FrameButton>
      )}
      {FINISHED.includes(b.state) && (
        <>
          {b.design_id && (
            <FrameButton
              render={<Link to={designLink(b.design_id)} />}
              title={keyHint(t`Open the design`, BROADCAST_KEYS.design)}
            >
              <Trans>Open the design</Trans>
            </FrameButton>
          )}
          <FrameButton onClick={onDelete} data-testid="broadcast-delete">
            <Trans>Delete</Trans>
          </FrameButton>
        </>
      )}
    </>
  )
}

function Banner({ b, at }: { b: Broadcast; at: number }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const num = useNumber()
  const channel = useInboxChannels(b.inbox_id).data?.find((c) => c.id === b.channel_id)
  const { done, total } = progressOf(b)
  const rate = channel?.email?.broadcast_rate
  const left = Math.max(0, total - done)
  const sent = num(done)
  const all = num(total)
  const prepared = num(b.counts.prepared ?? 0)
  const gap = paceGap(b.pace)
  const per = rate ? Math.max(1 / rate, gap) : 0
  const next = b.pace_next_at ? Date.parse(b.pace_next_at) : 0
  const start = gap > 0 && next > at ? next : at
  const finish = start + (left - (start > at ? 1 : 0)) * per * 1000
  const today = localParts(b.timezone, new Date(at)).date
  const open =
    b.send_window &&
    [addDays(today, -1), today]
      .map((d) => windowInstants(b.timezone, d, b.send_window!.start, b.send_window!.end))
      .find((w) => w && w.from.getTime() <= at && at < w.to.getTime())
  const fits = !b.send_window || (!!open && finish <= open.to.getTime())
  const end =
    b.state === "sending" && rate && left > 0 && !b.waiting_until && !b.window_opens_at && fits
      ? formatDateTime(new Date(finish).toISOString(), i18n.locale)
      : null
  const every = gap > 0 ? formatDuration(gap, i18n.locale) : ""
  const reason = b.pause_reason
  const closedUntil =
    b.window_opens_at && Date.parse(b.window_opens_at) > Math.max(at, Date.parse(b.scheduled_at ?? "") || 0)
  if (b.state === "scheduled" && b.window_opens_at && closedUntil) {
    return (
      <div className="flex flex-col gap-1 rounded-xl border bg-card p-4" data-testid="broadcast-scheduled">
        <WindowLine opens={b.window_opens_at} zone={b.timezone} />
        <p className="text-small text-muted-foreground">
          <Trans>The recipients are taken then; you can go back to the draft until it starts.</Trans>
        </p>
      </div>
    )
  }
  if (b.state === "scheduled" && b.scheduled_at) {
    const local = `${zonedDateTime(b.scheduled_at, b.timezone, i18n.locale)} ${zoneLabel(b.timezone, new Date(b.scheduled_at), i18n.locale)}`
    const utc = utcLine(b.scheduled_at, i18n.locale)
    return (
      <div className="flex flex-col gap-1 rounded-xl border bg-card p-4" data-testid="broadcast-scheduled">
        <p className="text-reading font-semibold">
          <Trans>Sends on {local}</Trans>
        </p>
        <p className="text-small text-muted-foreground">
          <Trans>{utc} UTC. The recipients are taken then; you can go back to the draft until it starts.</Trans>
        </p>
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-3 rounded-xl border bg-card p-4" data-testid="broadcast-sending">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <p className="text-reading font-semibold tabular-nums" data-testid="broadcast-sent-count">
          {b.state === "preparing" ? (
            <Trans>Taking the recipients: {prepared} so far</Trans>
          ) : (
            <Trans>
              {sent} of {all} done
            </Trans>
          )}
        </p>
        <p className="text-small text-muted-foreground">
          {rate ? (every ? t`${rate} mails a second · one every ${every}` : t`${rate} mails a second`) : null}
          {end && (
            <>
              {" · "}
              <Trans>about done by {end}</Trans>
            </>
          )}
        </p>
      </div>
      <ProgressBar b={b} className="h-2" />
      {b.window_opens_at && <WindowLine opens={b.window_opens_at} zone={b.timezone} />}
      {b.waiting_until && <WaitingLine until={b.waiting_until} zone={b.timezone} />}
      {b.state === "sending" && b.pace_next_at && next > at && <PaceLine at={b.pace_next_at} />}
      {b.state === "paused" && reason && (
        <Notice tone={reason === "member" ? "info" : "warning"} icon={PauseIcon} data-testid="broadcast-pause-reason">
          <p>{text.pauseReason[reason]}</p>
          {b.state_detail && (
            <pre
              className="overflow-x-auto rounded-lg bg-muted px-2.5 py-1.5 font-mono text-caption whitespace-pre-wrap"
              dir="ltr"
            >
              {b.state_detail}
            </pre>
          )}
        </Notice>
      )}
    </div>
  )
}

export function Summary({ b }: { b: Broadcast }) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const channel = useInboxChannels(b.inbox_id).data?.find((c) => c.id === b.channel_id)
  const open = useLists(b.inbox_id, false)
  const archived = useLists(b.inbox_id, true)
  const lists = new Map(
    [...(open.data?.pages ?? []), ...(archived.data?.pages ?? [])].flatMap((p) => p.items).map((l) => [l.id, l.name])
  )
  const paceSummary = usePaceSummary()
  const members = useMemberMap()
  const by = b.scheduled_by ? members.get(b.scheduled_by) : undefined
  const triggered = b.kind === "triggered"
  const sentence = useTriggerSentence(b)
  const days = b.trigger?.backfill_days ?? 0
  const confirmer = b.service_confirmed?.by ? members.get(b.service_confirmed.by) : undefined
  const stamp = (iso: string) =>
    `${zonedDateTime(iso, b.timezone, i18n.locale)} ${zoneLabel(b.timezone, new Date(iso), i18n.locale)}`
  const rows: [string, React.ReactNode][] = triggered
    ? [
        [t`Starts when`, sentence],
        [
          t`Why they get it`,
          b.trigger?.basis === "service" ? t`Service mail, about using the product` : t`Marketing, to list subscribers`,
        ],
      ]
    : []
  if (triggered && days > 0 && !b.started_at)
    rows.push([
      t`Also sent to`,
      b.trigger?.event === "subscribed"
        ? plural(days, {
            one: "people who confirmed a subscription during the last # day",
            other: "people who confirmed a subscription during the last # days",
          })
        : plural(days, {
            one: "people who signed in during the last # day",
            other: "people who signed in during the last # days",
          }),
    ])
  if (triggered && b.service_confirmed)
    rows.push([
      t`Confirmed by`,
      `${confirmer ? confirmer.name || confirmer.email : t`An API key or assistant`} · ${stamp(b.service_confirmed.at)}`,
    ])
  rows.push(
    [t`Subject`, b.subject || "—"],
    [t`Preview text`, b.preheader || t`The design’s own`],
    [t`Sends from`, channel ? `${channel.name} · ${channel.email?.from_address || channel.email?.address}` : "—"]
  )
  if (!triggered || b.audience.list_ids.length)
    rows.push([t`Lists`, b.audience.list_ids.map((id) => lists.get(id) ?? "…").join(", ") || "—"])
  if (b.audience.conditions.length) rows.push([t`Conditions`, String(b.audience.conditions.length)])
  if (b.audience.locales.length) rows.push([t`Languages`, b.audience.locales.join(", ")])
  rows.push([t`Sending pace`, paceSummary(b.pace)])
  rows.push([
    t`Time zone`,
    zoneLabel(b.timezone, new Date(b.scheduled_at ?? b.started_at ?? b.updated_at), i18n.locale),
  ])
  rows.push([
    t`Sending hours`,
    b.send_window
      ? `${wallClock(b.send_window.start, i18n.locale)}–${wallClock(b.send_window.end, i18n.locale)} ${zoneCity(b.timezone)}`
      : t`Any time`,
  ])
  rows.push([t`Click counting`, b.click_tracking ? t`On` : t`Off`])
  rows.push([
    t`Campaign in links`,
    b.utm ? (
      <span dir="ltr" key="utm">
        {b.utm_campaign}
      </span>
    ) : (
      t`Off`
    ),
  ])
  if (by) rows.push([t`Scheduled by`, by.name || by.email])
  else if (b.scheduled_by_api_key) rows.push([t`Scheduled by`, t`An API key or assistant`])
  if (b.started_at) rows.push([t`Started`, stamp(b.started_at)])
  if (b.finished_at) rows.push([t`Finished`, stamp(b.finished_at)])
  return (
    <Section
      title={
        triggered ? (
          <Trans>What it sends</Trans>
        ) : b.started_at ? (
          <Trans>What went out</Trans>
        ) : (
          <Trans>What will go out</Trans>
        )
      }
    >
      <Card>
        <div className="flex flex-wrap items-start gap-5 phone:flex-col" data-testid="broadcast-summary">
          {b.design_id &&
            (canManage ? (
              <Link to={designLink(b.design_id)} title={t`Open the design`}>
                <DesignThumb designId={b.design_id} inboxId={b.inbox_id} />
              </Link>
            ) : (
              <DesignThumb designId={b.design_id} inboxId={b.inbox_id} />
            ))}
          <dl className="grid min-w-0 flex-1 grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-body">
            {rows.map(([label, value]) => (
              <div key={label} className="contents">
                <dt className="text-faint">{label}</dt>
                <dd className="min-w-0 break-words">{value}</dd>
              </div>
            ))}
          </dl>
        </div>
      </Card>
    </Section>
  )
}

const TABS = ["report", "recipients", "broadcast"] as const
type Tab = (typeof TABS)[number]

export function BroadcastPage() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { broadcastId = "" } = useParams()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const broadcast = useBroadcast(broadcastId)
  const b = broadcast.data
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b?.inbox_id)
  const action = useBroadcastAction(broadcastId)
  const remove = useDeleteBroadcast()
  const [confirm, confirmDialog] = useConfirm()
  const [pacing, setPacing] = useState(false)
  const [hoursOpen, setHoursOpen] = useState(false)
  const rate = useInboxChannels(b?.inbox_id ?? "").data?.find((c) => c.id === b?.channel_id)?.email?.broadcast_rate
  const draft = b?.state === "draft"
  const scheduled = b?.state === "scheduled"
  const raw = params.get("tab") as Tab | null
  const tab: Tab = scheduled
    ? "broadcast"
    : raw && TABS.includes(raw) && (raw !== "recipients" || canManage)
      ? raw
      : "report"
  const name = b?.name ?? ""

  const run = (a: BroadcastAction) => {
    if (a === "cancel")
      confirm({
        title: <Trans>Cancel “{name}”?</Trans>,
        description: (
          <Trans>
            Sending stops within a few seconds and cannot start again. Mail that already went out stays sent.
          </Trans>
        ),
        confirm: <Trans>Cancel broadcast</Trans>,
        run: () => action.mutate("cancel"),
      })
    else action.mutate(a)
  }
  const onDelete = () =>
    confirm({
      title: <Trans>Delete “{name}”?</Trans>,
      description: (
        <Trans>Its report and the list of recipients are deleted. Conversations opened by replies stay.</Trans>
      ),
      confirm: <Trans>Delete</Trans>,
      run: () => remove.mutate(broadcastId, { onSuccess: () => navigate("/broadcasts") }),
    })

  useHotkeys(
    b && !draft
      ? {
          ...(b.design_id && FINISHED.includes(b.state)
            ? { [BROADCAST_KEYS.design]: () => navigate(designLink(b.design_id!)) }
            : {}),
          ...(b.state === "scheduled" ? { [BROADCAST_KEYS.unschedule]: () => run("unschedule") } : {}),
          ...(b.state === "sending" || b.state === "preparing" ? { [BROADCAST_KEYS.pause]: () => run("pause") } : {}),
          ...(b.state === "paused" ? { [BROADCAST_KEYS.pause]: () => run("resume") } : {}),
          ...(b.state === "scheduled" || isActive(b)
            ? {
                [BROADCAST_KEYS.cancel]: () => run("cancel"),
                [BROADCAST_KEYS.pace]: () => setPacing(true),
                [BROADCAST_KEYS.hours]: () => setHoursOpen(true),
              }
            : {}),
          ...(!scheduled
            ? {
                [BROADCAST_KEYS.report]: () => navigate(broadcastLink(b.id)),
                [BROADCAST_KEYS.summary]: () => navigate(broadcastLink(b.id, "broadcast")),
                ...(canManage
                  ? { [BROADCAST_KEYS.deliveries]: () => navigate(broadcastLink(b.id, "recipients")) }
                  : {}),
              }
            : {}),
        }
      : {},
    canManage && !!b && !pacing && !hoursOpen
  )

  if (broadcast.isPending || !b) {
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background">
        <PageBar back="/broadcasts" label={t`Broadcasts`}>
          {broadcast.isPending ? <Skeleton className="h-7 w-60" /> : <ErrorLine error={broadcast.error} />}
        </PageBar>
        <div className={framePad}>{broadcast.isPending && <Skeleton className="h-40 w-full rounded-2xl" />}</div>
      </div>
    )
  }
  if (draft) return <Composer key={b.id} b={b} readOnly={!canManage} />
  const lang = params.get("lang")
  const withLang = (to: string) => (lang ? `${to}${to.includes("?") ? "&" : "?"}lang=${encodeURIComponent(lang)}` : to)
  const names: Record<Tab, string> = { report: t`Report`, recipients: t`Recipients`, broadcast: t`Broadcast` }
  const tabs = TABS.filter((v) => (v === "recipients" ? canManage : true))
    .filter((v) => !scheduled || v === "broadcast")
    .map((v) => ({
      key: v,
      to: withLang(broadcastLink(b.id, v === "report" ? undefined : v)),
      label: names[v],
      testId: `broadcast-tab-${v}`,
    }))
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="broadcast-page" key={b.id}>
      <PageBar back="/broadcasts" label={t`Broadcasts`}>
        <div className="flex min-w-48 flex-1 basis-64 flex-col leading-snug">
          <span className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1">
            <h1 className="min-w-0 truncate text-[17px] font-semibold" data-testid="broadcast-name-heading">
              {b.name}
            </h1>
            <StateBadge state={b.state} />
            {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
          </span>
          {b.subject && <span className="truncate text-small text-muted-foreground">{b.subject}</span>}
        </div>
        {canManage && (
          <div className="flex min-w-0 max-w-full flex-wrap items-center gap-2">
            <Actions
              b={b}
              run={run}
              pending={action.isPending}
              onDelete={onDelete}
              onPace={() => setPacing(true)}
              onHours={() => setHoursOpen(true)}
            />
          </div>
        )}
      </PageBar>
      <div className="@container min-h-0 flex-1 in-data-frame:overflow-y-auto">
        <div className={cn(framePad, "max-w-[1200px]")}>
          <ErrorLine error={action.error ?? remove.error} />
          {b.state === "failed" && (
            <Notice tone="danger" icon={CircleAlertIcon} data-testid="broadcast-failed">
              <p>
                <Trans>Sending stopped and cannot go on, for example because its channel was removed.</Trans>
              </p>
              {b.state_detail && <p className="text-muted-foreground">{b.state_detail}</p>}
            </Notice>
          )}
          {(scheduled || isActive(b)) && <Banner b={b} at={broadcast.dataUpdatedAt} />}
          {!scheduled && <ReportStats b={b} />}
          {tabs.length > 1 && (
            <div className="border-b">
              <FrameTabs label={t`Broadcast`} items={tabs} current={tab} />
            </div>
          )}
          {tab === "report" && <Report b={b} />}
          {tab === "recipients" && <Deliveries b={b} />}
          {tab === "broadcast" && <Summary b={b} />}
        </div>
      </div>
      {pacing && <PaceDialog b={b} rate={rate} onClose={() => setPacing(false)} />}
      {hoursOpen && <SendTimeDialog b={b} inboxZone={inbox?.timezone} onClose={() => setHoursOpen(false)} />}
      {action.isPending && (
        <span className="fixed bottom-6 left-1/2 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full bg-foreground px-3 py-1.5 text-caption text-background shadow-lg">
          <LoaderIcon className="size-3 animate-spin" />
          <Trans>Working…</Trans>
        </span>
      )}
      {confirmDialog}
    </div>
  )
}
