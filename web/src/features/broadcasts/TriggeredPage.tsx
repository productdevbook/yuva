import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CircleAlertIcon, LoaderIcon, PauseIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ErrorLine, Notice, PersonAvatar, useConfirm } from "@/components/common"
import { PageBar } from "@/components/common/PageBar"
import { BROADCAST_KEYS, TRIGGERED_KEYS } from "@/components/common/ShortcutSheet"
import { formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  broadcastLink,
  problemOf,
  useBroadcastAction,
  useDeleteBroadcast,
  useEnrollments,
  useRemoveEnrollment,
} from "@/features/broadcasts/broadcastQueries"
import { Summary } from "@/features/broadcasts/BroadcastPage"
import { keyHint, PaceLine, StateBadge, useNumber, WaitingLine, WindowLine } from "@/features/broadcasts/BroadcastParts"
import { Composer } from "@/features/broadcasts/Composer"
import { useItemText } from "@/features/broadcasts/ReviewPanel"
import { Deliveries, Report, ReportStats } from "@/features/broadcasts/Report"
import { TestSendDialog, useTestSend } from "@/features/broadcasts/TestSend"
import { sentenceCase, useTriggerSentence } from "@/features/broadcasts/Trigger"
import { designLink } from "@/features/designs/queries"
import { Card, EmptyRow, Row, Rows, Section } from "@/features/settings/ui"
import { FrameButton, FrameTabs, framePad, ProductTag } from "@/features/status/PageFrame"
import { Skeleton } from "@/components/ui/skeleton"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import type { Broadcast } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"
import { utcLine, zoneLabel, zonedDateTime } from "@/lib/zones"

const TABS = ["report", "waiting", "sent", "settings"] as const
type Tab = (typeof TABS)[number]

function Banner({ b }: { b: Broadcast }) {
  const { i18n } = useLingui()
  const text = useEnumText()
  const now = useNow(60_000)
  const reason = b.pause_reason
  if (b.state === "scheduled" && b.scheduled_at) {
    const local = `${zonedDateTime(b.scheduled_at, b.timezone, i18n.locale)} ${zoneLabel(b.timezone, new Date(b.scheduled_at), i18n.locale)}`
    const utc = utcLine(b.scheduled_at, i18n.locale)
    return (
      <div className="flex flex-col gap-1 rounded-xl border bg-card p-4" data-testid="triggered-scheduled">
        <p className="text-reading font-semibold">
          <Trans>Starts on {local}</Trans>
        </p>
        <p className="text-small text-muted-foreground">
          <Trans>{utc} UTC. Nobody is counted until then; you can go back to the draft until it starts.</Trans>
        </p>
      </div>
    )
  }
  const lines = [
    b.window_opens_at && <WindowLine key="window" opens={b.window_opens_at} zone={b.timezone} />,
    b.waiting_until && <WaitingLine key="waiting" until={b.waiting_until} zone={b.timezone} />,
    b.pace_next_at && Date.parse(b.pace_next_at) > now && <PaceLine key="pace" at={b.pace_next_at} />,
  ].filter(Boolean)
  if (b.state !== "paused" && lines.length === 0) return null
  return (
    <div className="flex flex-col gap-3 rounded-xl border bg-card p-4" data-testid="triggered-banner">
      {b.state === "paused" && (
        <Notice
          tone={reason && reason !== "member" ? "warning" : "info"}
          icon={PauseIcon}
          data-testid="broadcast-pause-reason"
        >
          <p>{reason ? text.pauseReason[reason] : null}</p>
          {b.state_detail && (
            <pre
              className="overflow-x-auto rounded-lg bg-muted px-2.5 py-1.5 font-mono text-caption whitespace-pre-wrap"
              dir="ltr"
            >
              {b.state_detail}
            </pre>
          )}
          <p className="text-muted-foreground">
            <Trans>
              Nothing is sent while it is paused, but people who sign in are still counted, so nobody is missed.
            </Trans>
          </p>
        </Notice>
      )}
      {lines}
    </div>
  )
}

function Counts({ b }: { b: Broadcast }) {
  const { t } = useLingui()
  const num = useNumber()
  const text = useEnumText()
  const c = b.counts
  const labels: Record<string, string> = { ...text.skipReason, ...text.triggeredSkip }
  const skipped = Object.entries(c.skipped).filter(([, n]) => (n ?? 0) > 0)
  const tiles = [
    { key: "enrolled", value: c.enrolled ?? 0, label: t`counted so far` },
    { key: "waiting", value: c.waiting ?? 0, label: t`waiting for their time` },
    { key: "mailed", value: c.recipients, label: t`mail made` },
  ]
  return (
    <Section
      title={<Trans>People</Trans>}
      description={<Trans>Everyone who signed in while it was running, once each.</Trans>}
    >
      <div className="grid grid-cols-3 gap-2 phone:grid-cols-1" data-testid="triggered-counts">
        {tiles.map((x) => (
          <div key={x.key} className="min-w-0 rounded-2xl border bg-card px-4 py-3.5" data-tile={x.key}>
            <b className="block text-page tabular-nums">{num(x.value)}</b>
            <span className="block text-small break-words text-faint">{x.label}</span>
          </div>
        ))}
      </div>
      <Card flush>
        {skipped.length === 0 ? (
          <EmptyRow>
            <Trans>Nobody was left out.</Trans>
          </EmptyRow>
        ) : (
          <Rows data-testid="triggered-skipped">
            {skipped.map(([reason, n]) => (
              <Row key={reason} className="min-h-10 py-2" data-reason={reason}>
                <span className="min-w-0 flex-1 text-body text-muted-foreground">{labels[reason] ?? reason}</span>
                <span className="tabular-nums">{num(n ?? 0)}</span>
              </Row>
            ))}
          </Rows>
        )}
      </Card>
    </Section>
  )
}

function Waiting({ b }: { b: Broadcast }) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const waiting = useEnrollments(b.id, "waiting")
  const remove = useRemoveEnrollment(b.id)
  const [confirm, dialog] = useConfirm()
  const items = waiting.data?.pages.flatMap((p) => p.items) ?? []
  const live = b.state === "active" || b.state === "paused"
  return (
    <Section
      title={<Trans>Waiting</Trans>}
      description={<Trans>People who signed in and whose mail is not due yet, the one due first on top.</Trans>}
    >
      <ErrorLine error={remove.error} />
      <Card flush>
        {waiting.isPending ? (
          <div className="flex flex-col gap-2 p-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : waiting.error && items.length === 0 ? (
          <ErrorLine error={waiting.error} className="p-4" />
        ) : items.length === 0 ? (
          <EmptyRow>
            <Trans>Nobody is waiting.</Trans>
          </EmptyRow>
        ) : (
          <Rows data-testid="triggered-waiting">
            {items.map((e) => {
              const name = e.contact.name || e.contact.email || t`Unnamed contact`
              const triggered = formatDateTime(e.triggered_at, i18n.locale)
              const due = e.sends_at
                ? `${zonedDateTime(e.sends_at, b.timezone, i18n.locale)} ${zoneLabel(b.timezone, new Date(e.sends_at), i18n.locale)}`
                : "–"
              return (
                <li
                  key={e.contact.id}
                  className="flex items-center gap-3 px-4 py-2.5"
                  data-testid="triggered-waiting-row"
                >
                  <PersonAvatar name={name} className="size-8 phone:hidden" />
                  <span className="grid min-w-0 flex-1 items-center gap-x-4 gap-y-0.5 sm:grid-cols-[minmax(0,1fr)_10rem_14rem]">
                    <span className="flex min-w-0 flex-col">
                      <Link to={`/contacts/${e.contact.id}`} className="truncate text-body font-medium hover:underline">
                        {name}
                      </Link>
                      {e.contact.name && e.contact.email && (
                        <span className="truncate text-small text-faint" dir="ltr">
                          {e.contact.email}
                        </span>
                      )}
                    </span>
                    <span className="text-small text-muted-foreground">
                      <span className="text-faint sm:hidden">
                        <Trans>Signed in</Trans>{" "}
                      </span>
                      {triggered}
                    </span>
                    <span className="text-small text-muted-foreground">
                      <span className="text-faint sm:hidden">
                        <Trans>Sends</Trans>{" "}
                      </span>
                      {due}
                    </span>
                  </span>
                  {canManage && live && (
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={remove.isPending}
                      onClick={() =>
                        confirm({
                          title: <Trans>Take {name} off the waiting list?</Trans>,
                          description: (
                            <Trans>They get no mail from this triggered e-mail, and it never counts them again.</Trans>
                          ),
                          confirm: <Trans>Remove</Trans>,
                          run: () => remove.mutate(e.contact.id),
                        })
                      }
                      aria-label={t`Take ${name} off the waiting list`}
                      data-testid="triggered-remove"
                    >
                      <XIcon />
                      <Trans>Remove</Trans>
                    </Button>
                  )}
                </li>
              )
            })}
          </Rows>
        )}
        {waiting.hasNextPage && (
          <div className="flex justify-center border-t p-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void waiting.fetchNextPage()}
              disabled={waiting.isFetchingNextPage}
            >
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </Card>
      {dialog}
    </Section>
  )
}

function ResumeProblem({ b, error }: { b: Broadcast; error: unknown }) {
  const items = problemOf(error)?.items
  const { titles, problem } = useItemText(b, { ready: false, recipients: 0, items: items ?? [] })
  if (!items?.length) return <ErrorLine error={error} />
  return (
    <Notice tone="danger" icon={CircleAlertIcon} data-testid="triggered-resume-failed">
      <p>
        <Trans>It cannot resume yet:</Trans>
      </p>
      <ul className="flex flex-col gap-1">
        {items.map((it) => (
          <li key={it.key}>
            <b>{titles[it.key]}</b> {problem(it)}
          </li>
        ))}
      </ul>
    </Notice>
  )
}

export function TriggeredPage({ b }: { b: Broadcast }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const { canManage } = useSession()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const action = useBroadcastAction(b.id)
  const remove = useDeleteBroadcast()
  const [confirm, confirmDialog] = useConfirm()
  const test = useTestSend(b.id)
  const [testing, setTesting] = useState(false)
  const sentence = sentenceCase(useTriggerSentence(b), i18n.locale)
  const waiting = b.counts.waiting ?? 0
  const name = b.name
  const draft = b.state === "draft"
  const scheduled = b.state === "scheduled"
  const active = b.state === "active"
  const paused = b.state === "paused"
  const live = scheduled || active || paused
  const raw = params.get("tab") as Tab | null
  const tab: Tab = scheduled
    ? "settings"
    : raw && TABS.includes(raw) && (raw === "report" || raw === "settings" || canManage)
      ? raw
      : "report"
  const editing = params.get("edit") === "1"
  const edit = () => setParams({ edit: "1" })

  const stop = () =>
    confirm({
      title: <Trans>Stop “{name}”?</Trans>,
      description: (
        <>
          <Plural
            value={waiting}
            _0="Nobody is waiting."
            one="# person is waiting and will not get it."
            other="# people are waiting and will not get it."
          />{" "}
          <Trans>
            Nobody new is counted, mail not sent yet is skipped, and it cannot start again. Mail that already went out
            stays sent.
          </Trans>
        </>
      ),
      confirm: <Trans>Stop</Trans>,
      run: () => action.mutate("cancel"),
    })
  const onDelete = () =>
    confirm({
      title: <Trans>Delete “{name}”?</Trans>,
      description: (
        <Trans>Its report and the list of recipients are deleted. Conversations opened by replies stay.</Trans>
      ),
      confirm: <Trans>Delete</Trans>,
      run: () => remove.mutate(b.id, { onSuccess: () => navigate("/broadcasts/triggered") }),
    })

  useHotkeys(
    {
      ...(b.design_id ? { [BROADCAST_KEYS.design]: () => navigate(designLink(b.design_id!)) } : {}),
      ...(scheduled ? { [BROADCAST_KEYS.unschedule]: () => action.mutate("unschedule") } : {}),
      ...(active ? { [BROADCAST_KEYS.pause]: () => action.mutate("pause") } : {}),
      ...(paused ? { [BROADCAST_KEYS.pause]: () => action.mutate("resume") } : {}),
      ...(live ? { [TRIGGERED_KEYS.stop]: stop, [TRIGGERED_KEYS.edit]: edit } : {}),
      ...(b.design_id && !draft ? { [BROADCAST_KEYS.test]: () => setTesting(true) } : {}),
      ...(!scheduled
        ? {
            [BROADCAST_KEYS.report]: () => navigate(broadcastLink(b.id)),
            [BROADCAST_KEYS.summary]: () => navigate(broadcastLink(b.id, "settings")),
            ...(canManage
              ? {
                  [TRIGGERED_KEYS.waiting]: () => navigate(broadcastLink(b.id, "waiting")),
                  [BROADCAST_KEYS.deliveries]: () => navigate(broadcastLink(b.id, "sent")),
                }
              : {}),
          }
        : {}),
    },
    canManage && !testing && !editing
  )

  if (draft || (editing && live)) return <Composer key={`${b.id}-${editing}`} b={b} readOnly={!canManage} />

  const lang = params.get("lang")
  const withLang = (to: string) => (lang ? `${to}${to.includes("?") ? "&" : "?"}lang=${encodeURIComponent(lang)}` : to)
  const names: Record<Tab, string> = {
    report: t`Report`,
    waiting: t`Waiting`,
    sent: t`Sent`,
    settings: t`Settings`,
  }
  const tabs = TABS.filter((v) => (v === "report" || v === "settings" ? true : canManage))
    .filter((v) => !scheduled || v === "settings")
    .map((v) => ({
      key: v,
      to: withLang(broadcastLink(b.id, v === "report" ? undefined : v)),
      label: names[v],
      testId: `triggered-tab-${v}`,
    }))
  const pending = action.isPending
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="triggered-page">
      <PageBar back="/broadcasts/triggered" label={t`Triggered`}>
        <div className="flex min-w-48 flex-1 basis-64 flex-col leading-snug">
          <span className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1">
            <h1 className="min-w-0 truncate text-[17px] font-semibold" data-testid="broadcast-name-heading">
              {b.name}
            </h1>
            <StateBadge state={b.state} kind={b.kind} />
            {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
          </span>
          <span className="truncate text-small text-muted-foreground" data-testid="triggered-sentence">
            {sentence}
          </span>
        </div>
        {canManage && (
          <div className="flex min-w-0 max-w-full flex-wrap items-center gap-2">
            {scheduled && (
              <FrameButton
                disabled={pending}
                onClick={() => action.mutate("unschedule")}
                title={keyHint(t`Back to draft`, BROADCAST_KEYS.unschedule)}
                data-testid="triggered-unschedule"
              >
                <Trans>Back to draft</Trans>
              </FrameButton>
            )}
            {live && (
              <FrameButton onClick={edit} title={keyHint(t`Edit`, TRIGGERED_KEYS.edit)} data-testid="triggered-edit">
                <Trans>Edit</Trans>
              </FrameButton>
            )}
            {b.design_id && (
              <FrameButton
                onClick={() => setTesting(true)}
                title={keyHint(t`Send a test`, BROADCAST_KEYS.test)}
                data-testid="triggered-test"
              >
                <Trans>Send a test</Trans>
              </FrameButton>
            )}
            {active && (
              <FrameButton
                ink
                disabled={pending}
                onClick={() => action.mutate("pause")}
                title={keyHint(t`Pause`, BROADCAST_KEYS.pause)}
                data-testid="triggered-pause"
              >
                <Trans>Pause</Trans>
              </FrameButton>
            )}
            {paused && (
              <FrameButton
                ink
                disabled={pending}
                onClick={() => action.mutate("resume")}
                title={keyHint(t`Resume`, BROADCAST_KEYS.pause)}
                data-testid="triggered-resume"
              >
                <Trans>Resume</Trans>
              </FrameButton>
            )}
            {live && (
              <FrameButton
                disabled={pending}
                onClick={stop}
                title={keyHint(t`Stop the triggered e-mail`, TRIGGERED_KEYS.stop)}
                data-testid="triggered-stop"
              >
                <Trans>Stop</Trans>
              </FrameButton>
            )}
            {!live && (
              <FrameButton onClick={onDelete} data-testid="broadcast-delete">
                <Trans>Delete</Trans>
              </FrameButton>
            )}
          </div>
        )}
      </PageBar>
      <div className="@container min-h-0 flex-1 in-data-frame:overflow-y-auto">
        <div className={cn(framePad, "max-w-[1200px]")}>
          {action.error && action.variables === "resume" ? (
            <ResumeProblem b={b} error={action.error} />
          ) : (
            <ErrorLine error={action.error ?? remove.error} />
          )}
          {b.state === "failed" && (
            <Notice tone="danger" icon={CircleAlertIcon} data-testid="broadcast-failed">
              <p>
                <Trans>Sending stopped and cannot go on, for example because its channel was removed.</Trans>
              </p>
              {b.state_detail && <p className="text-muted-foreground">{b.state_detail}</p>}
            </Notice>
          )}
          <Banner b={b} />
          {!scheduled && <ReportStats b={b} />}
          {tabs.length > 1 && (
            <div className="border-b">
              <FrameTabs label={t`Triggered e-mail`} items={tabs} current={tab} />
            </div>
          )}
          {tab === "report" && (
            <>
              <Counts b={b} />
              <Report b={b} />
            </>
          )}
          {tab === "waiting" && <Waiting b={b} />}
          {tab === "sent" && <Deliveries b={b} />}
          {tab === "settings" && <Summary b={b} />}
        </div>
      </div>
      {testing && <TestSendDialog b={b} state={test} disabled={false} onClose={() => setTesting(false)} />}
      {pending && (
        <span className="fixed bottom-6 left-1/2 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full bg-foreground px-3 py-1.5 text-caption text-background shadow-lg">
          <LoaderIcon className="size-3 animate-spin" />
          <Trans>Working…</Trans>
        </span>
      )}
      {confirmDialog}
    </div>
  )
}
