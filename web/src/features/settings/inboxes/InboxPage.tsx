import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useMemo, useState } from "react"
import { createContext, useContext } from "react"
import { Link, useParams } from "react-router"

import { ChannelIcon, ErrorLine, Notice, toast } from "@/components/common"
import { AssignmentFields, capValid } from "@/components/common/AssignmentFields"
import { ChipGroup } from "@/components/common/ChipGroup"
import { SignatureEditor } from "@/components/common/SignatureEditor"
import { SaveBar } from "@/components/common/SaveBar"
import { formatRelative, useEnumText, useErrorText, WEEKDAYS } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { Toggle } from "@/components/ui/toggle"
import { channelDomain, channelTabLink } from "@/features/settings/channels/ChannelState"
import {
  categoriesInvalid,
  cleanCategories,
  FeedbackCategoriesBlock,
} from "@/features/settings/inboxes/FeedbackCategories"
import { ColorPicker } from "@/features/settings/inboxes/colors"
import { DeleteProduct, IdentitySecretRow } from "@/features/settings/inboxes/InboxSecurity"
import { useInbox, useSaveInbox } from "@/features/settings/inboxes/queries"
import { Block, BlockRow, PageHeader, PageTabs, StatusTag } from "@/features/settings/ui"
import { useTab } from "@/features/settings/useTab"
import {
  api,
  unwrap,
  type AssignmentMode,
  type BusinessHours,
  type Inbox,
  type InboxFeedbackCategory,
  type InboxPurpose,
  type InboxSender,
  type InboxUpdate,
  type Weekday,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn, timeZones } from "@/lib/utils"
import { useInboxMembers, useMembers, useTeams } from "@/lib/workspace"
import { InboxMark } from "@/components/common"

const InboxContext = createContext<{ inbox: Inbox } | null>(null)

export function useInboxOutlet() {
  const v = useContext(InboxContext)
  if (!v) throw new Error("useInboxOutlet outside an inbox page")
  return v
}

export const LANGS = ["en", "tr", "de", "fr", "es", "it", "nl", "pt", "ar", "ja"]
const PROMISES = [15, 60, 240, 1440]
const SLUG = /^[a-z0-9]+(-[a-z0-9]+)*$/

function senderComplete(s: InboxSender) {
  return !!(s.legal_name && s.postal_address && s.contact)
}

type SenderKey = keyof InboxSender
type Draft = {
  name: string
  purpose: InboxPurpose
  locale: string
  color: string
  greeting: string
  mode: Inbox["mode"]
  promise: number | null
  rating: boolean
  hoursOn: boolean
  days: Weekday[]
  start: string
  end: string
  zone: string
  slug: string
  sender: Record<SenderKey, string>
  categories: InboxFeedbackCategory[]
  assignMode: AssignmentMode
  assignCap: string
  signature: string
}

function uniform(h: BusinessHours) {
  const first = h.intervals[0]
  const days = new Set(h.intervals.map((i) => i.day))
  const same =
    h.intervals.every((i) => i.start === first?.start && i.end === first?.end) && days.size === h.intervals.length
  return { same, days, start: first?.start ?? "09:00", end: first?.end ?? "18:00" }
}

function draftOf(inbox: Inbox): Draft {
  const h = inbox.business_hours
  const u = uniform(h)
  const sender = inbox.sender ?? {}
  return {
    name: inbox.name,
    purpose: inbox.purpose,
    locale: inbox.default_locale,
    color: inbox.branding.color ?? "#d4431c",
    greeting: inbox.branding.greeting ?? "",
    mode: inbox.mode,
    promise: inbox.expected_reply_minutes ?? null,
    rating: inbox.ask_for_rating,
    hoursOn: h.enabled,
    days: WEEKDAYS.filter((d) =>
      h.intervals.length ? u.days.has(d) : ["mon", "tue", "wed", "thu", "fri"].includes(d)
    ),
    start: u.start,
    end: u.end === "24:00" ? "23:59" : u.end,
    zone: inbox.timezone,
    slug: inbox.slug,
    sender: {
      legal_name: sender.legal_name ?? "",
      postal_address: sender.postal_address ?? "",
      registration: sender.registration ?? "",
      contact: sender.contact ?? "",
    },
    categories: inbox.feedback_categories,
    assignMode: inbox.assignment_mode,
    assignCap: inbox.assignment_cap ? String(inbox.assignment_cap) : "",
    signature: inbox.signature,
  }
}

function problems(d: Draft) {
  return {
    name: !d.name.trim(),
    slug: !SLUG.test(d.slug),
    zone: !timeZones().includes(d.zone),
    categories: categoriesInvalid(d.categories, d.locale),
    cap: !capValid(d.assignCap),
  }
}

function changes(inbox: Inbox, d: Draft): InboxUpdate {
  const base = draftOf(inbox)
  const body: InboxUpdate = {}
  if (d.name.trim() !== base.name) body.name = d.name.trim()
  if (d.purpose !== base.purpose) body.purpose = d.purpose
  if (d.locale !== base.locale) body.default_locale = d.locale
  if (d.mode !== base.mode) body.mode = d.mode
  if (d.promise !== base.promise) body.expected_reply_minutes = d.promise
  if (d.rating !== base.rating) body.ask_for_rating = d.rating
  if (d.color !== base.color || d.greeting !== base.greeting)
    body.branding = { ...inbox.branding, color: d.color, greeting: d.greeting || undefined }
  if (d.hoursOn !== base.hoursOn || d.start !== base.start || d.end !== base.end || d.days.join() !== base.days.join())
    body.business_hours = {
      enabled: d.hoursOn,
      intervals: WEEKDAYS.filter((x) => d.days.includes(x)).map((day) => ({ day, start: d.start, end: d.end })),
    }
  if (d.zone !== base.zone) body.timezone = d.zone
  if (d.slug !== base.slug) body.slug = d.slug
  const sender: Partial<Record<SenderKey, string | null>> = {}
  for (const k of Object.keys(d.sender) as SenderKey[])
    if (d.sender[k].trim() !== base.sender[k]) sender[k] = d.sender[k].trim() || null
  if (Object.keys(sender).length) body.sender = sender
  if (d.purpose === "mail" && base.purpose !== "mail" && base.assignMode !== "off") body.assignment_mode = "off"
  else if (d.assignMode !== base.assignMode && d.purpose !== "mail") body.assignment_mode = d.assignMode
  if (d.assignCap !== base.assignCap && capValid(d.assignCap))
    body.assignment_cap = d.assignCap === "" ? null : Number(d.assignCap)
  if (d.signature !== base.signature) body.signature = d.signature
  const categories = cleanCategories(d.categories)
  if (JSON.stringify(categories) !== JSON.stringify(cleanCategories(base.categories)))
    body.feedback_categories = categories
  return body
}

type Form = { d: Draft; set: (p: Partial<Draft>) => void; inbox: Inbox; locked: boolean }

function languageName(tag: string, locale: string) {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(tag) ?? tag
  } catch {
    return tag
  }
}

function usePromiseText() {
  const { t } = useLingui()
  return (m: number) =>
    m === 15
      ? t`Within minutes`
      : m === 60
        ? t`Within an hour`
        : m === 240
          ? t`Within 4 hours`
          : m === 1440
            ? t`Within a day`
            : t`Within ${m} minutes`
}

function ChatPreview({ d, inbox }: { d: Draft; inbox: Inbox }) {
  const { t } = useLingui()
  const promiseText = usePromiseText()
  const status = d.mode === "live" ? t`Online now` : d.promise ? promiseText(d.promise) : t`Replies when available`
  return (
    <div className="flex flex-col overflow-hidden rounded-xl border bg-card" aria-hidden>
      <div className="flex items-center gap-2.5 px-4 py-3 text-white" style={{ backgroundColor: d.color }}>
        <InboxMark
          name={d.name || inbox.name}
          color="rgb(255 255 255 / 0.22)"
          className="size-8 rounded-full text-[14px]"
        />
        <span className="flex min-w-0 flex-col leading-[1.25]">
          <span className="truncate font-semibold">{d.name || inbox.name}</span>
          <span className="truncate text-caption opacity-90">{status}</span>
        </span>
      </div>
      <div className="flex min-h-36 flex-col gap-2 bg-surface p-4">
        <span className="max-w-[85%] rounded-2xl rounded-es-sm border bg-card px-3.5 py-2">
          {d.greeting || t`Hi! Ask us anything.`}
        </span>
      </div>
      <div className="border-t px-4 py-3 text-muted-foreground">
        <Trans>Write a message…</Trans>
      </div>
    </div>
  )
}

function ChatTab({ d, set, inbox, locked }: Form) {
  const { t, i18n } = useLingui()
  const promiseText = usePromiseText()
  const langs = LANGS.includes(d.locale) ? LANGS : [d.locale, ...LANGS]
  const promises =
    d.promise && !PROMISES.includes(d.promise) ? [...PROMISES, d.promise].sort((a, b) => a - b) : PROMISES
  const bad = problems(d)
  const mail = d.purpose === "mail"
  return (
    <div className={cn("grid items-start gap-4", !mail && "lg:grid-cols-[minmax(0,1fr)_320px]")}>
      <div className={cn("flex flex-col gap-4", mail && "max-w-[860px]")}>
        <PurposeBlock d={d} set={set} locked={locked} />
        <Block
          title={mail ? <Trans>Name and look</Trans> : <Trans>What the customer sees</Trans>}
          description={
            mail ? (
              <Trans>How this inbox appears in the panel</Trans>
            ) : (
              <Trans>Name, colour and the first thing the chat says</Trans>
            )
          }
        >
          <BlockRow label={<Trans>Name</Trans>} htmlFor="inbox-name">
            <Input
              id="inbox-name"
              value={d.name}
              maxLength={200}
              disabled={locked}
              aria-invalid={bad.name || undefined}
              onChange={(e) => set({ name: e.target.value })}
              className="h-9 max-w-80 rounded-[10px] bg-background"
            />
          </BlockRow>
          <BlockRow label={<Trans>Language</Trans>}>
            <ChipGroup
              label={t`Language`}
              value={d.locale}
              disabled={locked}
              onChange={(locale) => set({ locale })}
              options={langs.map((l) => [l, languageName(l, i18n.locale)] as const)}
            />
          </BlockRow>
          <BlockRow label={<Trans>Colour</Trans>}>
            <ColorPicker value={d.color} disabled={locked} onChange={(color) => set({ color })} />
          </BlockRow>
          {!mail && (
            <BlockRow
              label={<Trans>Greeting</Trans>}
              htmlFor="inbox-greeting"
              hint={<Trans>The first thing the chat says</Trans>}
            >
              <Textarea
                id="inbox-greeting"
                rows={3}
                maxLength={500}
                value={d.greeting}
                disabled={locked}
                placeholder={t`Hi! Ask us anything.`}
                onChange={(e) => set({ greeting: e.target.value })}
                className="rounded-[10px] bg-background"
              />
            </BlockRow>
          )}
        </Block>
        {!mail && (
          <Block
            title={<Trans>Replies and ratings</Trans>}
            description={<Trans>What the chat promises, and what it asks after closing</Trans>}
          >
            <BlockRow
              label={<Trans>Online status</Trans>}
              hint={
                d.mode === "live" ? (
                  <Trans>The chat shows who is available and typing</Trans>
                ) : (
                  <Trans>The chat shows when you usually reply</Trans>
                )
              }
            >
              <ChipGroup<Inbox["mode"]>
                label={t`Online status`}
                value={d.mode}
                disabled={locked}
                onChange={(mode) => set({ mode })}
                options={[
                  ["live", t`Show who is online`],
                  ["async", t`Show the reply time`],
                ]}
              />
            </BlockRow>
            <BlockRow label={<Trans>Reply time</Trans>} hint={<Trans>Shown in the chat window</Trans>}>
              <ChipGroup
                label={t`Reply time`}
                value={d.promise ? String(d.promise) : "none"}
                disabled={locked}
                onChange={(v) => set({ promise: v === "none" ? null : Number(v) })}
                options={[
                  ["none", t`Not shown`] as const,
                  ...promises.map((m) => [String(m), promiseText(m)] as const),
                ]}
              />
            </BlockRow>
            <BlockRow
              label={<Trans>Ask for a rating after closing</Trans>}
              hint={
                <Trans>
                  The customer taps thumbs up or down when a conversation is closed. E-mail contacts get a short e-mail
                  with two links; chat and app contacts are asked in the widget or app.
                </Trans>
              }
            >
              <Switch
                checked={d.rating}
                disabled={locked}
                onCheckedChange={(rating) => set({ rating })}
                aria-label={t`Ask for a rating after closing`}
                data-testid="ask-for-rating"
              />
            </BlockRow>
          </Block>
        )}
        {!mail && (
          <FeedbackCategoriesBlock
            list={d.categories}
            saved={inbox.feedback_categories}
            locale={d.locale}
            langs={LANGS}
            locked={locked}
            onChange={(categories) => set({ categories })}
          />
        )}
      </div>
      {!mail && (
        <div className="flex flex-col gap-2 lg:sticky lg:top-28">
          <div className="mx-1 flex flex-col">
            <span className="font-semibold">
              <Trans>Preview</Trans>
            </span>
            <span className="text-caption text-faint">
              <Trans>How the chat opens</Trans>
            </span>
          </div>
          <ChatPreview d={d} inbox={inbox} />
        </div>
      )}
    </div>
  )
}

function PurposeBlock({ d, set, locked }: Pick<Form, "d" | "set" | "locked">) {
  const { t } = useLingui()
  return (
    <Block
      title={<Trans>Purpose</Trans>}
      description={
        d.purpose === "mail" ? (
          <Trans>Team mail: a shared mailbox, kept out of support views, stats and SLA.</Trans>
        ) : (
          <Trans>Support: conversations with customers, with chat, ratings, assignment and SLA.</Trans>
        )
      }
      testId="inbox-purpose"
    >
      <BlockRow
        label={<Trans>This inbox is for</Trans>}
        hint={
          locked ? (
            <Trans>Only owners and admins can change it.</Trans>
          ) : (
            <Trans>Team mail takes e-mail channels only. Automatic assignment is turned off.</Trans>
          )
        }
      >
        <ChipGroup<InboxPurpose>
          label={t`Purpose`}
          value={d.purpose}
          disabled={locked}
          onChange={(purpose) => set({ purpose })}
          options={[
            ["support", t`Support`],
            ["mail", t`Team mail`],
          ]}
        />
      </BlockRow>
    </Block>
  )
}

function WhoCanReply({ inbox }: { inbox: Inbox }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const members = useMembers().data ?? []
  const access = new Map((useInboxMembers(inbox.id).data ?? []).map((m) => [m.id, m]))
  const teams = new Map((useTeams().data ?? []).map((team) => [team.id, team.name]))
  const toggle = useMutation({
    mutationFn: ({ memberId, on }: { memberId: string; on: boolean }) => {
      const params = { params: { path: { inboxId: inbox.id, memberId } } }
      return on
        ? unwrap(api.PUT("/v1/inboxes/{inboxId}/members/{memberId}", params))
        : unwrap(api.DELETE("/v1/inboxes/{inboxId}/members/{memberId}", params))
    },
    onSuccess: () => {
      toast(t`Saved`)
      void qc.invalidateQueries({ queryKey: keys.inboxMembers(ws, inbox.id) })
    },
    onError: (e) => toast(errorText(e)),
  })
  return (
    <Block
      title={<Trans>Who can reply</Trans>}
      description={<Trans>Owners and admins see every product; agents see the products they are given.</Trans>}
    >
      <ul className="divide-y">
        {members.map((m) => {
          const name = m.name || m.email
          const always = m.role !== "agent"
          const entry = access.get(m.id)
          const through = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(
            (entry?.team_ids ?? []).map((id) => teams.get(id)).filter((x): x is string => !!x)
          )
          return (
            <li key={m.id} className="flex min-h-[52px] items-center gap-3 px-[18px] py-2.5">
              <span className="flex min-w-0 flex-1 flex-col leading-[1.3]">
                <span className="truncate">{name}</span>
                <span className="text-caption text-faint">{text.role[m.role]}</span>
                {!always && through && (
                  <span className="truncate text-caption text-faint" data-testid="access-teams">
                    <Trans>Through {through}</Trans>
                  </span>
                )}
              </span>
              {always ? (
                <span className="text-caption text-faint">
                  <Trans>Always</Trans>
                </span>
              ) : (
                <Switch
                  checked={entry?.via === "direct"}
                  disabled={!canManage || toggle.isPending}
                  onCheckedChange={(on) => toggle.mutate({ memberId: m.id, on })}
                  aria-label={t`${name} can reply`}
                />
              )}
            </li>
          )
        })}
      </ul>
    </Block>
  )
}

function HoursTab({ d, set, locked, inbox }: Form) {
  const { t } = useLingui()
  const text = useEnumText()
  const zones = useMemo(() => timeZones(), [])
  const bad = problems(d)
  return (
    <div className="flex max-w-[860px] flex-col gap-4">
      <Block title={<Trans>Business hours</Trans>}>
        <BlockRow
          label={<Trans>Appear online only in business hours</Trans>}
          hint={<Trans>Outside them the product counts as closed.</Trans>}
        >
          <Switch
            checked={d.hoursOn}
            disabled={locked}
            onCheckedChange={(hoursOn) => set({ hoursOn })}
            aria-label={t`Appear online only in business hours`}
          />
        </BlockRow>
        {d.hoursOn ? (
          <>
            <BlockRow label={<Trans>Days</Trans>}>
              <span className="flex flex-wrap gap-1" role="group" aria-label={t`Days`}>
                {WEEKDAYS.map((x) => {
                  const on = d.days.includes(x)
                  return (
                    <Toggle
                      key={x}
                      pressed={on}
                      disabled={locked}
                      aria-label={text.weekday[x]}
                      title={text.weekday[x]}
                      onPressedChange={() => set({ days: on ? d.days.filter((y) => y !== x) : [...d.days, x] })}
                      className="size-8 min-w-8 rounded-full border border-border px-0 hover:border-faint hover:bg-transparent aria-pressed:border-primary aria-pressed:bg-primary aria-pressed:text-primary-foreground"
                    >
                      {text.weekday[x].slice(0, 2)}
                    </Toggle>
                  )
                })}
              </span>
              {d.days.length === 0 && (
                <p className="text-caption text-warning">
                  <Trans>No days picked: the product counts as closed all week.</Trans>
                </p>
              )}
            </BlockRow>
            <BlockRow label={<Trans>Opens</Trans>} htmlFor="inbox-open">
              <Input
                id="inbox-open"
                type="time"
                value={d.start}
                disabled={locked}
                onChange={(e) => set({ start: e.target.value })}
                className="h-9 w-auto max-w-40 rounded-[10px] bg-background"
              />
            </BlockRow>
            <BlockRow label={<Trans>Closes</Trans>} htmlFor="inbox-close">
              <Input
                id="inbox-close"
                type="time"
                value={d.end}
                disabled={locked}
                onChange={(e) => set({ end: e.target.value })}
                className="h-9 w-auto max-w-40 rounded-[10px] bg-background"
              />
            </BlockRow>
          </>
        ) : (
          <p className="border-b px-[18px] py-3 text-caption text-faint">
            <Trans>The product always counts as open.</Trans>
          </p>
        )}
        <BlockRow label={<Trans>Time zone</Trans>} htmlFor="inbox-zone">
          <Input
            id="inbox-zone"
            list="inbox-zones"
            value={d.zone}
            disabled={locked}
            aria-invalid={bad.zone || undefined}
            onChange={(e) => set({ zone: e.target.value })}
            className="h-9 max-w-80 rounded-[10px] bg-background"
          />
          <datalist id="inbox-zones">
            {zones.map((z) => (
              <option key={z} value={z} />
            ))}
          </datalist>
        </BlockRow>
      </Block>
      {d.purpose !== "mail" && (
        <Block title={<Trans>Automatic assignment</Trans>}>
          <div className="px-[18px] py-4">
            <AssignmentFields
              scope="inbox"
              mode={d.assignMode}
              cap={d.assignCap}
              disabled={locked}
              onMode={(assignMode) => set({ assignMode })}
              onCap={(assignCap) => set({ assignCap })}
            />
          </div>
        </Block>
      )}
      <WhoCanReply inbox={inbox} />
    </div>
  )
}

function SesTenant({ inbox }: { inbox: Inbox }) {
  const { t, i18n } = useLingui()
  const name = inbox.name
  const { canManage } = useSession()
  const ses = inbox.ses_tenant
  if (!ses)
    return (
      <Block
        title={<Trans>Amazon SES tenant</Trans>}
        description={<Trans>Sending reputation of this product, kept apart from the others</Trans>}
      >
        <div className="flex flex-wrap items-center gap-3 px-[18px] py-3.5">
          <p className="min-w-0 flex-1 text-muted-foreground">
            <Trans>
              {name} sends through its own SMTP account, so there is no Amazon SES tenant. Checks for SPF, DKIM and
              DMARC are on its e-mail channel.
            </Trans>
          </p>
          <Button variant="outline" size="sm" render={<Link to="/settings?view=channels" />}>
            <Trans>Open channels</Trans>
          </Button>
        </div>
      </Block>
    )
  const paused = ses.sending_status === "disabled"
  const status = {
    enabled: t({ message: "Sending", context: "SES tenant status" }),
    reinstated: t`Reinstated`,
    disabled: t`Paused`,
  }[ses.sending_status]
  const impact = { none: t`None`, low: t`Low`, high: t`High` }[ses.reputation_impact]
  return (
    <div className="flex max-w-[860px] flex-col gap-4">
      {paused && canManage && (
        <Notice tone="danger" data-testid="ses-paused">
          <p>
            <Trans>
              Amazon SES paused this inbox’s sending. Its replies and broadcasts through the server’s SES account are
              refused until the operator reinstates it. Other inboxes keep sending.
            </Trans>
          </p>
          {ses.paused_by === "aws" && (
            <p>
              <Trans>Amazon SES itself paused it; the operator has to open a case with AWS.</Trans>
            </p>
          )}
        </Notice>
      )}
      <Block
        title={<Trans>Amazon SES tenant</Trans>}
        description={<Trans>Sending reputation of this product, kept apart from the others</Trans>}
      >
        <BlockRow
          label={<Trans>Sending</Trans>}
          hint={
            ses.state === "pending" ? (
              <Trans>Setting up the tenant…</Trans>
            ) : ses.state === "missing" ? (
              <Trans>
                The tenant is gone from Amazon SES. Broadcasts are blocked until the server sets it up again.
              </Trans>
            ) : ses.state === "error" ? (
              <Trans>Amazon SES refused to set up the tenant.</Trans>
            ) : undefined
          }
        >
          <StatusTag tone={paused ? "danger" : ses.sending_status === "reinstated" ? "warning" : "success"}>
            {status}
          </StatusTag>
        </BlockRow>
        <BlockRow label={<Trans>Reputation impact</Trans>} hint={ses.cause || undefined}>
          <StatusTag
            tone={ses.reputation_impact === "high" ? "danger" : ses.reputation_impact === "low" ? "warning" : "muted"}
          >
            {impact}
          </StatusTag>
        </BlockRow>
        {ses.checked_at && (
          <BlockRow label={<Trans>Last checked</Trans>}>
            <span className="text-faint">{formatRelative(ses.checked_at, i18n.locale)}</span>
          </BlockRow>
        )}
      </Block>
    </div>
  )
}

function MailTab({ d, set, inbox, locked }: Form) {
  const { t } = useLingui()
  const complete = senderComplete({
    legal_name: d.sender.legal_name.trim(),
    postal_address: d.sender.postal_address.trim(),
    contact: d.sender.contact.trim(),
  })
  const field = (k: SenderKey) => ({
    value: d.sender[k],
    disabled: locked,
    onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      set({ sender: { ...d.sender, [k]: e.target.value } }),
  })
  const footer = Object.values(d.sender)
    .map((v) => v.trim())
    .filter(Boolean)
  return (
    <div className="flex max-w-[860px] flex-col gap-4">
      <Block
        title={<Trans>Sender identity</Trans>}
        description={
          <Trans>
            The law asks every marketing mail to say who sends it and how to reach them (CAN-SPAM, CASL, Türkiye's
            Regulation on Commercial Communication). Yuva puts this in the footer of every broadcast from this inbox, so
            fill it in before the first one.
          </Trans>
        }
      >
        <div className="flex items-center justify-end border-b px-[18px] py-2">
          {complete ? (
            <StatusTag tone="success">
              <Trans>Complete</Trans>
            </StatusTag>
          ) : (
            <StatusTag tone="warning" data-testid="sender-missing">
              <Trans>Needed for broadcasts</Trans>
            </StatusTag>
          )}
        </div>
        <BlockRow
          label={<Trans>Legal name</Trans>}
          htmlFor="sender-name"
          hint={<Trans>Your company or your own name</Trans>}
        >
          <Input
            id="sender-name"
            maxLength={200}
            placeholder={t`Example Ltd.`}
            className="h-9 max-w-96 rounded-[10px] bg-background"
            {...field("legal_name")}
          />
        </BlockRow>
        <BlockRow
          label={<Trans>Postal address</Trans>}
          htmlFor="sender-address"
          hint={<Trans>A real address where you receive post</Trans>}
        >
          <Textarea
            id="sender-address"
            rows={2}
            maxLength={500}
            className="rounded-[10px] bg-background"
            {...field("postal_address")}
          />
        </BlockRow>
        <BlockRow
          label={<Trans>Registration</Trans>}
          htmlFor="sender-registration"
          hint={<Trans>Such as a MERSİS number, trade register or VAT id, if you have one</Trans>}
        >
          <Input
            id="sender-registration"
            maxLength={200}
            className="h-9 max-w-96 rounded-[10px] bg-background"
            {...field("registration")}
          />
        </BlockRow>
        <BlockRow
          label={<Trans>Contact</Trans>}
          htmlFor="sender-contact"
          hint={<Trans>An e-mail address, phone number or web address</Trans>}
        >
          <Input
            id="sender-contact"
            maxLength={320}
            placeholder="hello@example.com"
            className="h-9 max-w-96 rounded-[10px] bg-background"
            {...field("contact")}
          />
        </BlockRow>
        {footer.length > 0 && (
          <div className="px-[18px] py-3" data-testid="sender-footer-preview">
            <p className="mb-1.5 text-caption text-faint">
              <Trans>In the footer</Trans>
            </p>
            <p className="text-center text-caption whitespace-pre-line text-muted-foreground">{footer.join("\n")}</p>
          </div>
        )}
      </Block>
      <Block
        title={<Trans>Signature</Trans>}
        description={
          <Trans>
            Replies by e-mail from this inbox end with it, unless the member has their own. Chat replies and notes carry
            none.
          </Trans>
        }
      >
        <BlockRow label={<Trans>Default signature</Trans>} htmlFor="inbox-signature">
          <SignatureEditor
            id="inbox-signature"
            value={d.signature}
            disabled={locked}
            placeholder={t`Best regards,`}
            onChange={(signature) => set({ signature })}
          />
        </BlockRow>
      </Block>
      <SesTenant inbox={inbox} />
    </div>
  )
}

function DeveloperTab({ d, set }: Form) {
  const bad = problems(d)
  return (
    <div className="flex max-w-[860px] flex-col gap-4">
      <Block
        title={<Trans>For developers</Trans>}
        description={<Trans>Used in addresses, the widget and the SDKs</Trans>}
      >
        <BlockRow
          label={<Trans>Slug</Trans>}
          htmlFor="inbox-slug"
          hint={<Trans>Used in addresses and the widget</Trans>}
        >
          <Input
            id="inbox-slug"
            value={d.slug}
            maxLength={64}
            aria-invalid={bad.slug || undefined}
            onChange={(e) => set({ slug: e.target.value })}
            className="h-9 max-w-80 rounded-[10px] bg-background"
          />
          {bad.slug && (
            <p role="alert" className="text-caption text-destructive">
              <Trans>Use lowercase letters, digits and single dashes.</Trans>
            </p>
          )}
        </BlockRow>
        <BlockRow label={<Trans>Webhooks for this product</Trans>}>
          <div>
            <Button variant="outline" size="sm" render={<Link to="webhooks" />}>
              <Trans>Manage</Trans>
            </Button>
          </div>
        </BlockRow>
        <IdentitySecretRow />
      </Block>
      <DeleteProduct />
    </div>
  )
}

export function InboxProvider({ children }: { children: (inbox: Inbox) => React.ReactNode }) {
  const { inboxId = "" } = useParams()
  const inbox = useInbox(inboxId)
  if (inbox.isPending) return <Skeleton className="h-64 w-full rounded-2xl" />
  if (!inbox.data) return <ErrorLine error={inbox.error} />
  return <InboxContext.Provider value={{ inbox: inbox.data }}>{children(inbox.data)}</InboxContext.Provider>
}

function Editor({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const { workspaceId: ws, canManage } = useSession()
  const { tab, to } = useTab(["chat", "hours", "mail", "developer"] as const, "chat")
  const [d, setD] = useState(() => draftOf(inbox))
  const save = useSaveInbox(inbox)
  const channels = useQuery({
    queryKey: keys.channels(ws, inbox.id),
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: inbox.id } } })).then(
        (r) => r.items
      ),
  })
  const body = changes(inbox, d)
  const dirty = Object.keys(body).length > 0
  const bad = problems(d)
  const invalid = bad.name || bad.slug || bad.zone || bad.categories || bad.cap
  const form: Form = { d, set: (p) => setD((x) => ({ ...x, ...p })), inbox, locked: !canManage }
  const domain = channelDomain(channels.data)
  const tabs = [
    {
      key: "chat",
      to: to("chat"),
      label: d.purpose === "mail" ? t({ message: "General", context: "inbox settings tab" }) : t`Chat`,
      testId: "product-tab-chat",
    },
    { key: "hours", to: to("hours"), label: t`Hours & team`, testId: "product-tab-hours" },
    { key: "mail", to: to("mail"), label: t`E-mail`, testId: "product-tab-mail" },
    ...(canManage
      ? [{ key: "developer", to: to("developer"), label: t`Developers`, testId: "product-tab-developer" }]
      : []),
  ]
  return (
    <>
      <PageHeader
        back={{ to: "/settings", label: t`Products` }}
        title={
          <span className="flex min-w-0 items-center gap-3">
            <InboxMark name={inbox.name} color={inbox.branding.color} className="size-9 rounded-[10px] text-[16px]" />
            <span className="truncate">{inbox.name}</span>
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1.5">
            <span>
              {domain && <>{domain} · </>}
              {inbox.purpose === "mail" ? t`Team mail` : text.mode[inbox.mode]} · {inbox.timezone}
            </span>
            {(channels.data ?? []).map((ch) => (
              <Link
                key={ch.id}
                to={channelTabLink(ch, "settings")}
                title={t`Open channels`}
                className="inline-flex h-6 items-center gap-1.5 rounded-md border bg-background px-2 text-caption text-muted-foreground transition-colors hover:border-faint hover:text-foreground"
              >
                <ChannelIcon kind={ch.kind} className="size-3" />
                {ch.name}
              </Link>
            ))}
          </span>
        }
        tabs={<PageTabs label={t`Product settings`} current={tab} items={tabs} />}
      />
      {tab === "chat" && <ChatTab {...form} />}
      {tab === "hours" && <HoursTab {...form} />}
      {tab === "mail" && <MailTab {...form} />}
      {tab === "developer" && canManage && <DeveloperTab {...form} />}
      {dirty && canManage && (
        <SaveBar
          what={inbox.name}
          pending={save.isPending}
          blocked={invalid}
          error={save.error}
          onDiscard={() => {
            save.reset()
            setD(draftOf(inbox))
          }}
          onSave={() =>
            save.mutate(body, {
              onSuccess: (saved) => {
                setD(draftOf(saved))
                toast(t`Saved`)
              },
              onError: (e) => toast(errorText(e)),
            })
          }
        />
      )}
    </>
  )
}

export function InboxPage() {
  return <InboxProvider>{(inbox) => <Editor key={inbox.id} inbox={inbox} />}</InboxProvider>
}

export function InboxWebhooks({ children }: { children: React.ReactNode }) {
  return (
    <InboxProvider>
      {(inbox) => (
        <>
          <PageHeader
            title={<Trans>Webhooks for {inbox.name}</Trans>}
            back={{ to: `/settings/inboxes/${inbox.id}`, label: inbox.name }}
          />
          {children}
        </>
      )}
    </InboxProvider>
  )
}
