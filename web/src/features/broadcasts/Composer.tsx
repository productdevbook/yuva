import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, LoaderIcon, MailWarningIcon } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { ErrorLine, InboxMark, Notice, PersonAvatar, Segmented, useConfirm } from "@/components/common"
import { PageBar } from "@/components/common/PageBar"
import { BROADCAST_KEYS } from "@/components/common/ShortcutSheet"
import { formatDuration, useLanguageName } from "@/components/common/text"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { AudienceBuilder, AudienceCount, conditionValid, type Audience } from "@/features/broadcasts/AudienceBuilder"
import {
  useAudienceCount,
  useDeleteBroadcast,
  useReview,
  useUpdateBroadcast,
} from "@/features/broadcasts/broadcastQueries"
import { keyHint, senderState, StatusNotice, useNumber, type SenderState } from "@/features/broadcasts/BroadcastParts"
import { DesignPicker } from "@/features/broadcasts/DesignPicker"
import { useInboxChannels, useLists } from "@/features/broadcasts/queries"
import { hasPace, PaceFields, paceInvalid, paceOf, paceTextOf, type PaceText } from "@/features/broadcasts/Pace"
import { hoursInvalid, hoursOf, SendTimeFields, windowOf, type HoursText } from "@/features/broadcasts/SendTime"
import {
  CheckState,
  PART_OF,
  ReviewDialog,
  useItemText,
  worstOf,
  type Part,
  type Spot,
} from "@/features/broadcasts/ReviewPanel"
import { TestSendDialog, useTestSend } from "@/features/broadcasts/TestSend"
import { versions } from "@/features/designs/doc"
import { designLink, useDesign } from "@/features/designs/queries"
import { Card, Field, Section, ToggleRow } from "@/features/settings/ui"
import { FrameButton, framePad } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Broadcast, BroadcastReview, BroadcastTranslation, BroadcastUpdate, Channel } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const NONE = "none"
const SUBJECT_MAX = 300
const SUBJECT_SOFT = 80
const PREHEADER_MAX = 300
const CAMPAIGN = /^[a-z0-9][a-z0-9_.-]*$/

type Draft = {
  name: string
  subject: string
  preheader: string
  translations: Record<string, BroadcastTranslation>
  channel_id: string
  design_id: string
  audience: Audience
  click_tracking: boolean
  utm: boolean
  utm_campaign: string
  pace_on: boolean
  pace: PaceText
  timezone: string
  hours: HoursText
}

function draftOf(b: Broadcast): Draft {
  return {
    name: b.name,
    subject: b.subject,
    preheader: b.preheader,
    translations: b.translations ?? {},
    channel_id: b.channel_id ?? "",
    design_id: b.design_id ?? "",
    audience: b.audience,
    click_tracking: b.click_tracking,
    utm: b.utm,
    utm_campaign: b.utm_campaign,
    pace_on: hasPace(b.pace),
    pace: paceTextOf(b.pace),
    timezone: b.timezone,
    hours: hoursOf(b.send_window),
  }
}

function cleanTranslations(all: Record<string, BroadcastTranslation>) {
  const out: Record<string, BroadcastTranslation> = {}
  for (const [tag, x] of Object.entries(all)) {
    const entry: BroadcastTranslation = {}
    if (x.subject) entry.subject = x.subject
    if (x.preheader) entry.preheader = x.preheader
    if (entry.subject || entry.preheader) out[tag] = entry
  }
  return out
}

function audienceInput(a: Audience) {
  return { list_ids: a.list_ids, conditions: a.conditions.filter(conditionValid), locales: a.locales }
}

function useAutosave(id: string) {
  const update = useUpdateBroadcast(id)
  const pending = useRef<BroadcastUpdate>({})
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const [waiting, setWaiting] = useState(false)
  const { mutate } = update
  const flush = useCallback(() => {
    clearTimeout(timer.current)
    const body = pending.current
    pending.current = {}
    setWaiting(false)
    if (Object.keys(body).length > 0) mutate(body)
  }, [mutate])
  const save = useCallback(
    (patch: BroadcastUpdate, delay = 700) => {
      pending.current = { ...pending.current, ...patch }
      setWaiting(true)
      clearTimeout(timer.current)
      timer.current = setTimeout(flush, delay)
    },
    [flush]
  )
  useEffect(() => flush, [flush])
  return { save, flush, saving: waiting || update.isPending, error: update.error }
}

export function SaveState({ saving, error }: { saving: boolean; error: unknown }) {
  if (error) return <ErrorLine error={error} className="text-small" />
  return (
    <span
      className="inline-flex items-center gap-1.5 text-small text-faint"
      role="status"
      data-testid="draft-save-state"
    >
      {saving ? (
        <>
          <LoaderIcon className="size-3.5 animate-spin" />
          <Trans>Saving…</Trans>
        </>
      ) : (
        <>
          <CheckIcon className="size-3.5" />
          <Trans>Draft saved</Trans>
        </>
      )}
    </span>
  )
}

function useSenderText(): Record<SenderState, string> {
  const { t } = useLingui()
  return {
    ready: t`Ready to send`,
    no_smtp: t`No SMTP account`,
    no_from: t`Catch-all without a From address`,
  }
}

function SenderPicker({
  b,
  value,
  onChange,
  disabled,
}: {
  b: Broadcast
  value: string
  onChange: (id: string | null) => void
  disabled: boolean
}) {
  const { t } = useLingui()
  const stateText = useSenderText()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const channels = (useInboxChannels(b.inbox_id).data ?? []).filter((c) => c.kind === "email")
  const current = channels.find((c) => c.id === value)
  const items: Record<string, string> = { [NONE]: t`No channel yet` }
  for (const c of channels) items[c.id] = c.email ? `${c.name} · ${c.email.from_address || c.email.address}` : c.name
  const state = current ? senderState(current) : null
  const none = channels.length === 0
  return (
    <div className="flex flex-col gap-4">
      {inbox && (
        <div className="flex items-center gap-2 text-body">
          <span className="text-small text-faint">
            <Trans>Inbox</Trans>
          </span>
          <span className="inline-flex items-center gap-1.5 font-medium">
            <InboxMark name={inbox.name} color={inbox.branding.color} className="size-4 text-[9px]" />
            {inbox.name}
          </span>
        </div>
      )}
      {none ? (
        <Notice tone="danger" icon={MailWarningIcon} data-testid="broadcast-sender-missing">
          <p>
            <b>
              <Trans>No channel yet</Trans>
            </b>{" "}
            <Trans>This inbox has no e-mail channel. Add one with an SMTP account to send broadcasts.</Trans>
          </p>
          <Link
            to={`/settings/inboxes/${b.inbox_id}`}
            className="w-fit text-brand underline-offset-4 hover:underline"
            data-testid="broadcast-inbox-settings"
          >
            <Trans>Open the inbox settings</Trans>
          </Link>
        </Notice>
      ) : (
        <>
          <Field label={<Trans>Sends from</Trans>}>
            <Select
              value={value || NONE}
              items={items}
              disabled={disabled}
              onValueChange={(v) => onChange(!v || v === NONE ? null : String(v))}
            >
              <SelectTrigger className="w-full min-w-0" aria-label={t`Sends from`} data-testid="broadcast-channel">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>{t`No channel yet`}</SelectItem>
                {channels.map((c) => {
                  const s = senderState(c)
                  return (
                    <SelectItem key={c.id} value={c.id} disabled={s !== "ready"}>
                      <span className="flex min-w-0 flex-col">
                        <span className="truncate">{c.name}</span>
                        <small className="truncate text-caption text-faint">
                          {c.email?.from_address || c.email?.address}
                          {s !== "ready" && <> · {stateText[s]}</>}
                        </small>
                      </span>
                    </SelectItem>
                  )
                })}
              </SelectContent>
            </Select>
          </Field>
          {current && state === "ready" && <SenderLine ch={current} />}
          {((current && state !== "ready") || channels.every((c) => senderState(c) !== "ready")) && (
            <Notice tone="warning" icon={MailWarningIcon} data-testid="broadcast-sender-missing">
              <p>
                <Trans>
                  A broadcast goes out through an e-mail channel with an SMTP account, ideally a separate one for
                  broadcasts so complaints never reach your support mail.
                </Trans>
              </p>
              <Link
                to={`/settings/inboxes/${b.inbox_id}`}
                className="w-fit text-brand underline-offset-4 hover:underline"
                data-testid="broadcast-inbox-settings"
              >
                <Trans>Open the inbox settings</Trans>
              </Link>
            </Notice>
          )}
        </>
      )}
    </div>
  )
}

function SenderLine({ ch }: { ch: Channel }) {
  const { t } = useLingui()
  const e = ch.email
  if (!e) return null
  const rate = e.broadcast_rate
  return (
    <p
      className="flex flex-wrap items-center gap-x-2 gap-y-1 text-small text-muted-foreground"
      data-testid="sender-line"
    >
      <span className="inline-flex items-center gap-1 text-success">
        <CheckIcon className="size-3.5" />
        <Trans>Ready to send</Trans>
      </span>
      <span aria-hidden>·</span>
      <span>
        {e.broadcast_smtp ? (
          <Trans>through its own broadcast account</Trans>
        ) : (
          <Trans>through the support account</Trans>
        )}
      </span>
      <span aria-hidden>·</span>
      <span>{t`${rate} mails a second`}</span>
    </p>
  )
}

const asSeen = (s: string) =>
  s
    .replace(/\{\{\s*[^|}]*?\s*(?:\|\s*"([^"]*)"\s*)?\}\}/g, (_, fallback?: string) => fallback ?? "")
    .replace(/\s{2,}/g, " ")

function InboxPreview({
  sender,
  address,
  subject,
  preheader,
}: {
  sender: string
  address: string
  subject: string
  preheader: string
}) {
  const { t } = useLingui()
  return (
    <Card className="gap-3" data-testid="inbox-preview">
      <span className="text-small font-medium text-faint">
        <Trans>In the inbox</Trans>
      </span>
      <div className="flex items-start gap-3">
        <PersonAvatar name={sender || address || "?"} className="size-9" />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className="min-w-0 flex-1 truncate text-body font-semibold">{sender || address || t`Your name`}</span>
            <span className="shrink-0 text-caption text-faint">{t`now`}</span>
          </div>
          <p className="truncate text-body font-medium">{asSeen(subject) || t`The subject`}</p>
          <p className="truncate text-small text-faint">
            {asSeen(preheader) || t`The first line of the mail shows here when there is no preview text.`}
          </p>
        </div>
      </div>
    </Card>
  )
}

function Counter({ value, max, soft }: { value: string; max: number; soft?: number }) {
  const n = [...value].length
  return (
    <span
      className={cn(
        "text-caption tabular-nums",
        n > max ? "text-destructive" : soft && n > soft ? "text-warning" : "text-faint"
      )}
    >
      {n}/{max}
    </span>
  )
}

function Rail({
  b,
  review,
  preview,
  sender,
  design,
  onFix,
  action,
}: {
  b: Broadcast
  review: BroadcastReview | undefined
  preview: React.ReactNode
  sender: string
  design: string
  onFix: (spot: Spot) => void
  action: React.ReactNode
}) {
  const { t } = useLingui()
  const num = useNumber()
  const { titles, problem } = useItemText(b, review ?? { ready: false, recipients: 0, items: [] })
  const people = num(review?.recipients ?? 0)
  const parts: { part: Part; name: React.ReactNode }[] = [
    { part: "sender", name: <Trans>Sender</Trans> },
    { part: "audience", name: <Trans>Audience</Trans> },
    { part: "content", name: <Trans>Content</Trans> },
    { part: "subject", name: <Trans>Subject</Trans> },
  ]
  const spot: Record<Part, Spot> = { sender: "sender", audience: "audience", content: "content", subject: "content" }
  return (
    <aside className="flex flex-col gap-4" data-testid="review-panel">
      {preview}
      <Card className="gap-3" data-testid="before-you-send">
        <span className="text-small font-medium text-faint">
          <Trans>Before you send</Trans>
        </span>
        {review ? (
          <ul className="flex flex-col gap-3">
            {parts.map(({ part, name }) => {
              const items = review.items.filter((it) => PART_OF[it.key] === part)
              const status = worstOf(items)
              const worst = items.find((it) => it.status === status && status !== "ok")
              const text = worst
                ? (problem(worst) ?? titles[worst.key])
                : part === "sender"
                  ? sender
                  : part === "audience"
                    ? review.recipients > 0
                      ? t`${people} people`
                      : ""
                    : part === "content"
                      ? design
                      : ""
              return (
                <li key={part} className="flex items-start gap-3" data-testid="before-row" data-part={part}>
                  <CheckState status={status} className="mt-0.5" />
                  <div className="flex min-w-0 flex-1 flex-col">
                    <span className="text-body font-medium">{name}</span>
                    {text && <span className="text-small break-words text-muted-foreground">{text}</span>}
                    {worst && (
                      <button
                        type="button"
                        onClick={() => onFix(spot[part])}
                        className="w-fit text-small text-brand underline-offset-4 hover:underline"
                      >
                        <Trans>Fix it</Trans>
                      </button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        ) : (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        )}
        {action}
      </Card>
    </aside>
  )
}

export function Composer({ b, readOnly }: { b: Broadcast; readOnly: boolean }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const [d, setD] = useState<Draft>(() => draftOf(b))
  const inboxZone = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)?.timezone
  const auto = useAutosave(b.id)
  const count = useAudienceCount(b, true)
  const review = useReview(b, true)
  const test = useTestSend(b.id)
  const remove = useDeleteBroadcast()
  const [confirm, confirmDialog] = useConfirm()
  const [testing, setTesting] = useState(false)
  const [reviewing, setReviewing] = useState(false)
  const lists = useLists(b.inbox_id, false)
  const listNames = new Map((lists.data?.pages.flatMap((p) => p.items) ?? []).map((l) => [l.id, l.name]))
  const channel = useInboxChannels(b.inbox_id).data?.find((c) => c.id === d.channel_id)
  const design = useDesign(d.design_id)
  const designDoc = d.design_id ? design.data?.document : undefined
  const tags = designDoc ? versions(designDoc) : []
  const [tab, setTab] = useState<string | null>(null)
  const version = tab && tags.indexOf(tab) > 0 ? tab : null
  const languageName = useLanguageName()
  const baseLanguage = tags.length ? languageName(tags[0]) : ""
  const own = version ? (d.translations[version] ?? {}) : null
  const designPreheader = designDoc?.preheader ?? ""
  const inherited =
    (version ? designDoc?.translations?.[version]?.preheader : undefined) || (version && d.preheader) || designPreheader
  const subject = own ? (own.subject ?? "") : d.subject
  const preheader = own ? (own.preheader ?? "") : d.preheader
  const shownSubject = own ? own.subject || d.subject : d.subject
  const shownPreheader = own ? own.preheader || inherited : d.preheader || designPreheader
  const set = (patch: Partial<Draft>, save: BroadcastUpdate, delay?: number) => {
    setD((x) => ({ ...x, ...patch }))
    if (!readOnly) auto.save(save, delay)
  }
  const setText = (field: "subject" | "preheader", value: string) => {
    if (!version) {
      set({ [field]: value }, { [field]: value })
      return
    }
    const translations = { ...d.translations, [version]: { ...d.translations[version], [field]: value } }
    set({ translations }, { translations: cleanTranslations(translations) })
  }
  const fix = (spot: Spot) => {
    if (spot === "test") {
      setTesting(true)
      return
    }
    const el = document.getElementById(`broadcast-${spot}`)
    el?.scrollIntoView({ block: "center", behavior: "smooth" })
    el?.querySelector<HTMLElement>("input, button, [role=combobox]")?.focus({ preventScroll: true })
  }
  const openReview = () => {
    auto.flush()
    setReviewing(true)
  }
  const paceSecs = d.pace_on && hasPace(paceOf(d.pace)) ? review.data?.duration_seconds : undefined
  const paceDuration = paceSecs === undefined ? "" : formatDuration(paceSecs, i18n.locale)
  const merge = '{{ contact.first_name | "there" }}'
  const campaignBad = !CAMPAIGN.test(d.utm_campaign)
  const from = channel?.email?.from_address || channel?.email?.address || ""
  const testTitle = keyHint(t`Send a test`, BROADCAST_KEYS.test)
  const sendTitle = keyHint(t`Review, then send or schedule`, BROADCAST_KEYS.send)
  const name = b.name
  useHotkeys(
    {
      ...(d.design_id ? { [BROADCAST_KEYS.design]: () => navigate(designLink(d.design_id)) } : {}),
      ...(readOnly ? {} : { [BROADCAST_KEYS.test]: () => setTesting(true), [BROADCAST_KEYS.send]: openReview }),
    },
    !testing && !reviewing
  )
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="broadcast-page">
      <PageBar back="/broadcasts" label={t`Broadcasts`}>
        <Input
          id="broadcast-name"
          aria-label={t`Name`}
          value={d.name}
          maxLength={200}
          disabled={readOnly}
          onChange={(e) => set({ name: e.target.value }, e.target.value.trim() ? { name: e.target.value.trim() } : {})}
          className="h-[34px] w-80 max-w-full border-transparent bg-transparent text-[15px] font-semibold hover:border-input focus-visible:border-ring"
          data-testid="broadcast-name"
        />
        {!readOnly && <SaveState saving={auto.saving} error={auto.error} />}
        <span className="flex-1" />
        {!readOnly && (
          <>
            <FrameButton
              onClick={() =>
                confirm({
                  title: <Trans>Delete “{name}”?</Trans>,
                  description: (
                    <Trans>
                      Its report and the list of recipients are deleted. Conversations opened by replies stay.
                    </Trans>
                  ),
                  confirm: <Trans>Delete</Trans>,
                  run: () => remove.mutate(b.id, { onSuccess: () => navigate("/broadcasts") }),
                })
              }
              data-testid="broadcast-delete"
            >
              <Trans>Delete draft</Trans>
            </FrameButton>
            <FrameButton onClick={() => setTesting(true)} title={testTitle} data-testid="open-test">
              <Trans>Send a test</Trans>
            </FrameButton>
            <FrameButton ink onClick={openReview} title={sendTitle} data-testid="open-review">
              <Trans>Review and send</Trans>
            </FrameButton>
          </>
        )}
      </PageBar>
      <div className="@container min-h-0 flex-1 in-data-frame:overflow-y-auto">
        <div className={cn(framePad, "max-w-[1200px]")}>
          <StatusNotice />
          <ErrorLine error={remove.error} />
          <div
            className="grid grid-cols-[minmax(0,1fr)] items-start gap-6 @4xl:grid-cols-[minmax(0,1fr)_21rem]"
            data-testid="broadcast-composer"
          >
            <div className="flex min-w-0 flex-col gap-6">
              <Section title={<Trans>Sender</Trans>}>
                <Card id="broadcast-sender">
                  <SenderPicker
                    b={b}
                    value={d.channel_id}
                    disabled={readOnly}
                    onChange={(id) => set({ channel_id: id ?? "" }, { channel_id: id }, 0)}
                  />
                </Card>
              </Section>
              <Section title={<Trans>Audience</Trans>}>
                <Card id="broadcast-audience">
                  <AudienceBuilder
                    inboxId={b.inbox_id}
                    value={d.audience}
                    disabled={readOnly}
                    onChange={(audience) => set({ audience }, { audience: audienceInput(audience) })}
                  />
                  <AudienceCount
                    count={d.audience.list_ids.length ? count.data : undefined}
                    loading={count.isPending && d.audience.list_ids.length > 0}
                    stale={auto.saving || count.isFetching}
                    error={count.error ? <ErrorLine error={count.error} /> : null}
                    listNames={listNames}
                  />
                </Card>
              </Section>
              <Section title={<Trans>Content</Trans>}>
                <Card id="broadcast-content">
                  <DesignPicker
                    inboxId={b.inbox_id}
                    value={d.design_id}
                    disabled={readOnly}
                    onChange={(id) => set({ design_id: id ?? "" }, { design_id: id }, 0)}
                  />
                  {tags.length > 1 && (
                    <div className="-mx-1 overflow-x-auto px-1">
                      <Segmented
                        value={version ?? tags[0]}
                        onChange={(v) => setTab(v === tags[0] ? null : v)}
                        label={t`Language`}
                        items={tags.map((tag) => ({
                          value: tag,
                          label: languageName(tag),
                          testId: "subject-language",
                        }))}
                        itemClassName="whitespace-nowrap"
                      />
                    </div>
                  )}
                  <Field
                    label={
                      <span className="flex w-full items-center justify-between gap-2">
                        <Trans>Subject</Trans>
                        <Counter value={subject} max={SUBJECT_MAX} soft={SUBJECT_SOFT} />
                      </span>
                    }
                    htmlFor="broadcast-subject"
                    hint={
                      version ? (
                        <Trans>Empty uses the {baseLanguage} subject. Merge fields work here too.</Trans>
                      ) : (
                        <Trans>Merge fields work here too, for example {merge}.</Trans>
                      )
                    }
                  >
                    <Input
                      id="broadcast-subject"
                      value={subject}
                      maxLength={SUBJECT_MAX}
                      disabled={readOnly}
                      placeholder={version ? d.subject : t`What is new in October`}
                      onChange={(e) => setText("subject", e.target.value)}
                      data-testid="broadcast-subject"
                    />
                  </Field>
                  <Field
                    label={
                      <span className="flex w-full items-center justify-between gap-2">
                        <Trans>Preview text</Trans>
                        <Counter value={preheader} max={PREHEADER_MAX} />
                      </span>
                    }
                    htmlFor="broadcast-preheader"
                    hint={<Trans>Shown after the subject in most inboxes. Empty uses the design’s own.</Trans>}
                  >
                    <Input
                      id="broadcast-preheader"
                      value={preheader}
                      maxLength={PREHEADER_MAX}
                      disabled={readOnly}
                      placeholder={inherited || t`A line that makes people open it`}
                      onChange={(e) => setText("preheader", e.target.value)}
                      data-testid="broadcast-preheader"
                    />
                  </Field>
                </Card>
              </Section>
              <Section title={<Trans>Options</Trans>}>
                <Card id="broadcast-options">
                  <ToggleRow
                    title={<Trans>Count clicks</Trans>}
                    hint={
                      <Trans>
                        Off by default. When on, every link goes through this server’s link domain first, so the report
                        can count who opened which link. Clicks are approximate: security scanners that open every link
                        are left out. Yuva never counts opens.
                      </Trans>
                    }
                  >
                    <Switch
                      checked={d.click_tracking}
                      disabled={readOnly}
                      onCheckedChange={(on) => set({ click_tracking: on }, { click_tracking: on }, 0)}
                      aria-label={t`Count clicks`}
                      data-testid="broadcast-click-tracking"
                    />
                  </ToggleRow>
                  <ToggleRow
                    title={<Trans>Campaign in links</Trans>}
                    hint={
                      <Trans>
                        On by default. Links to your sites get utm_source=newsletter, utm_medium=email and the campaign
                        name, so Analytics shows the visits this broadcast brings. Links with their own utm_ parameters
                        keep them.
                      </Trans>
                    }
                  >
                    <Switch
                      checked={d.utm}
                      disabled={readOnly}
                      onCheckedChange={(on) => set({ utm: on }, { utm: on }, 0)}
                      aria-label={t`Campaign in links`}
                      data-testid="broadcast-utm"
                    />
                  </ToggleRow>
                  {d.utm && (
                    <Field
                      label={<Trans>Campaign name</Trans>}
                      htmlFor="broadcast-utm-campaign"
                      hint={
                        campaignBad ? undefined : (
                          <Trans>
                            Lowercase letters, digits, dashes, underscores and dots. Fixed once the broadcast is sent.
                          </Trans>
                        )
                      }
                      error={
                        campaignBad ? (
                          <Trans>
                            Use lowercase letters, digits, dashes, underscores and dots, starting with a letter or
                            digit.
                          </Trans>
                        ) : undefined
                      }
                    >
                      <Input
                        id="broadcast-utm-campaign"
                        dir="ltr"
                        value={d.utm_campaign}
                        maxLength={100}
                        disabled={readOnly}
                        aria-invalid={campaignBad || undefined}
                        onChange={(e) => {
                          const v = e.target.value.toLowerCase().replace(/\s+/g, "-")
                          set({ utm_campaign: v }, CAMPAIGN.test(v) ? { utm_campaign: v } : {})
                        }}
                        data-testid="broadcast-utm-campaign"
                      />
                    </Field>
                  )}
                  <div id="broadcast-pace" className="flex flex-col gap-4">
                    <ToggleRow
                      title={<Trans>Limit the pace</Trans>}
                      hint={
                        <Trans>
                          Off sends as fast as the channel and the workspace’s daily limit allow. On spreads the mails
                          evenly, so no 60 minutes, 24 hours or 30 days get more than you set.
                        </Trans>
                      }
                    >
                      <Switch
                        checked={d.pace_on}
                        disabled={readOnly}
                        onCheckedChange={(on) =>
                          on
                            ? setD((x) => ({ ...x, pace_on: true }))
                            : set({ pace_on: false, pace: paceTextOf(undefined) }, { pace: {} }, 0)
                        }
                        aria-label={t`Limit the pace`}
                        data-testid="broadcast-pace-on"
                      />
                    </ToggleRow>
                    {d.pace_on && (
                      <>
                        <PaceFields
                          value={d.pace}
                          disabled={readOnly}
                          windowed={d.hours.on}
                          rate={channel?.email?.broadcast_rate}
                          onChange={(pace) =>
                            set({ pace }, paceInvalid(pace) || !hasPace(paceOf(pace)) ? {} : { pace: paceOf(pace) })
                          }
                        />
                        {paceSecs !== undefined && (
                          <p className="text-small text-muted-foreground" data-testid="broadcast-pace-duration">
                            <Trans>At this pace, sending takes about {paceDuration}.</Trans>
                          </p>
                        )}
                      </>
                    )}
                  </div>
                  <div id="broadcast-hours" className="flex flex-col gap-4 border-t pt-4">
                    <SendTimeFields
                      zone={d.timezone}
                      onZone={(timezone) => set({ timezone }, { timezone })}
                      hours={d.hours}
                      onHours={(hours) =>
                        set({ hours }, hoursInvalid(hours) ? {} : { send_window: hours.on ? windowOf(hours) : null })
                      }
                      disabled={readOnly}
                      inboxZone={inboxZone}
                      days={Math.ceil((review.data?.days ?? 1) + 7)}
                    />
                  </div>
                </Card>
              </Section>
            </div>
            <div className="@4xl:sticky @4xl:top-28 @4xl:self-start">
              <Rail
                b={b}
                review={review.data}
                sender={from}
                design={design.data?.name ?? ""}
                onFix={fix}
                preview={
                  <InboxPreview
                    sender={channel?.email?.display_name || channel?.name || ""}
                    address={from}
                    subject={shownSubject}
                    preheader={shownPreheader}
                  />
                }
                action={
                  readOnly ? null : (
                    <FrameButton
                      ink
                      className="w-full justify-center"
                      onClick={openReview}
                      title={sendTitle}
                      data-testid="open-send"
                    >
                      <Trans>Review and send</Trans>
                    </FrameButton>
                  )
                }
              />
            </div>
          </div>
        </div>
      </div>
      {testing && <TestSendDialog b={b} state={test} disabled={readOnly} onClose={() => setTesting(false)} />}
      {reviewing && (
        <ReviewDialog
          b={b}
          review={review.data}
          reviewError={review.error}
          from={from}
          onFix={fix}
          onClose={() => setReviewing(false)}
        />
      )}
      {confirmDialog}
    </div>
  )
}
