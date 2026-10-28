import { Trans, useLingui } from "@lingui/react/macro"
import { ChartLineIcon, ExternalLinkIcon, MessageSquareReplyIcon, SearchIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { Link, useSearchParams } from "react-router"

import { ErrorLine, PersonAvatar, Segmented } from "@/components/common"
import { DailyChart } from "@/components/common/DailyChart"
import {
  BROADCAST_DELIVERY_STATES,
  formatDateTime,
  formatShort,
  useEnumText,
  useLanguageName,
} from "@/components/common/text"
import { Button } from "@/components/ui/button"
import type { ChartConfig } from "@/components/ui/chart"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { todayIso } from "@/features/analytics/state"
import { useDeliveries, useReport } from "@/features/broadcasts/broadcastQueries"
import { useNumber, usePercent } from "@/features/broadcasts/BroadcastParts"
import { headClass } from "@/features/reports/parts"
import { Card, EmptyRow, Rows, Section, StatusTag } from "@/features/settings/ui"
import { useDebounced } from "@/hooks/use-debounced"
import type { Broadcast, BroadcastDelivery, BroadcastDeliveryState, BroadcastReport } from "@/lib/api"
import { cn } from "@/lib/utils"

function Tiles({ b, report }: { b: Broadcast; report: BroadcastReport }) {
  const { t } = useLingui()
  const num = useNumber()
  const pct = usePercent()
  const c = report.counts
  const r = report.rates
  const tiles: { key: string; value: number; label: string; rate?: number; tone?: string }[] = [
    { key: "sent", value: c.sent, label: t`sent` },
    {
      key: "bounced",
      value: c.bounced,
      label: t`bounced`,
      rate: r.bounced,
      tone: r.bounced >= 0.05 ? "text-destructive" : undefined,
    },
    {
      key: "complained",
      value: c.complained,
      label: t`reported as spam`,
      rate: r.complained,
      tone: r.complained >= 0.003 ? "text-destructive" : undefined,
    },
    { key: "unsubscribed", value: c.unsubscribed, label: t`unsubscribed`, rate: r.unsubscribed },
    {
      key: "replied",
      value: c.replied,
      label: t`replied`,
      rate: r.replied,
      tone: c.replied ? "text-success" : undefined,
    },
  ]
  if (b.click_tracking)
    tiles.push({ key: "clicked", value: c.clicked, label: t`clicked a link, roughly`, rate: r.clicked })
  const skipped = Object.values(c.skipped).reduce((n, v) => n + (v ?? 0), 0)
  const recipients = num(c.recipients)
  const failed = num(c.failed)
  const auto = num(c.auto_replied)
  const left = num(skipped)
  return (
    <div className="flex flex-col gap-2">
      <div className="grid grid-cols-3 gap-2 phone:grid-cols-2" data-testid="report-tiles">
        {tiles.map((x) => (
          <div key={x.key} className="min-w-0 rounded-2xl border bg-card px-4 py-3.5 phone:px-3" data-tile={x.key}>
            <b className={cn("block text-page tabular-nums", x.tone)}>{num(x.value)}</b>
            <span className="block text-small break-words text-faint">
              {x.label}
              {x.rate !== undefined && c.sent > 0 && <> · {pct(x.rate)}</>}
            </span>
          </div>
        ))}
      </div>
      <p className="mx-1 text-small text-faint" data-testid="report-other">
        <Trans>
          {recipients} recipients · {failed} refused by the mail server · {left} skipped · {auto} automatic replies,
          only counted
        </Trans>
      </p>
    </div>
  )
}

function Timeline({ b, report }: { b: Broadcast; report: BroadcastReport }) {
  const { t, i18n } = useLingui()
  const config: ChartConfig = {
    sent: { label: t`Sent`, theme: { light: "#0891b2", dark: "#22d3ee" } },
    replied: { label: t`Replied`, theme: { light: "#15803d", dark: "#0d9488" } },
    unsubscribed: { label: t`Unsubscribed`, theme: { light: "#a1a1aa", dark: "#71717a" } },
    bounced: { label: t`Bounced`, theme: { light: "#d97706", dark: "#f59e0b" } },
  }
  if (b.click_tracking) config.clicked = { label: t`Clicked`, theme: { light: "#7c3aed", dark: "#a78bfa" } }
  const start = b.started_at ? Date.parse(b.started_at) : 0
  const hourly = (iso: string) => start > 0 && Date.parse(iso) < start + 72 * 3600_000
  const hour = new Intl.DateTimeFormat(i18n.locale, {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  })
  const hourTick = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const day = new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium", timeZone: "UTC" })
  const dayTick = new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "short", timeZone: "UTC" })
  const rows = report.timeline.map((p) => ({
    day: p.at,
    sent: p.sent,
    replied: p.replied,
    unsubscribed: p.unsubscribed,
    bounced: p.bounced,
    ...(b.click_tracking ? { clicked: p.clicked } : {}),
  }))
  return (
    <Section
      title={<Trans>Over time</Trans>}
      description={
        <Trans>
          Per hour for the first three days, then per day. Replies count people who answered; automatic replies are left
          out.
        </Trans>
      }
    >
      {rows.length === 0 ? (
        <Card flush>
          <EmptyRow>
            <Trans>Nothing has happened yet.</Trans>
          </EmptyRow>
        </Card>
      ) : (
        <Card>
          <DailyChart
            rows={rows}
            config={config}
            testId="report-chart"
            tick={(d) => (hourly(d) ? hourTick.format(new Date(d)) : dayTick.format(new Date(d)))}
            label={(d) => (hourly(d) ? hour.format(new Date(d)) : day.format(new Date(d)))}
          />
        </Card>
      )}
    </Section>
  )
}

function Links({ report }: { report: BroadcastReport }) {
  const num = useNumber()
  const links = report.links ?? []
  return (
    <Section
      title={<Trans>Links</Trans>}
      description={
        <Trans>
          People who opened each link at least once. Scanner clicks are left out, so this is a close estimate.
        </Trans>
      }
    >
      <Card flush>
        {links.length === 0 ? (
          <EmptyRow>
            <Trans>Nobody opened a link yet.</Trans>
          </EmptyRow>
        ) : (
          <Rows data-testid="report-links">
            {links.map((l) => (
              <li key={l.url} className="flex min-h-[48px] items-center gap-3 px-4 py-2.5">
                <a
                  href={l.url}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="flex min-w-0 flex-1 items-center gap-1.5 text-body hover:underline"
                  dir="ltr"
                >
                  <span className="truncate">{l.url}</span>
                  <ExternalLinkIcon className="size-3.5 shrink-0 text-faint" />
                </a>
                <span className="shrink-0 text-body tabular-nums">{num(l.clicks)}</span>
              </li>
            ))}
          </Rows>
        )}
      </Card>
    </Section>
  )
}

function CampaignVisits({ b, report }: { b: Broadcast; report: BroadcastReport }) {
  const num = useNumber()
  const visits = report.campaign_visits
  if (!b.utm || !visits) return null
  const campaign = b.utm_campaign
  const filter = new URLSearchParams([["f", `utm_campaign:is:${campaign}`]])
  if (b.started_at) {
    const from = b.started_at.slice(0, 10)
    const to = todayIso()
    if (to >= from) filter.set("period", `custom:${from},${to}`)
  }
  return (
    <Section
      title={<Trans>Visits from this campaign</Trans>}
      description={
        <Trans>
          Visits to your sites with utm_campaign={campaign} since sending started, counted by Analytics without cookies.
        </Trans>
      }
    >
      <Card flush>
        {visits.length === 0 ? (
          <EmptyRow>
            <Trans>No visits from this campaign yet.</Trans>
          </EmptyRow>
        ) : (
          <Rows data-testid="report-campaign-visits">
            {visits.map((v) => (
              <li key={v.property_id}>
                <Link
                  to={`/analytics/${v.property_id}/sources?${filter}`}
                  className="flex min-h-[52px] flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5 transition-colors hover:bg-muted"
                  data-testid="report-campaign-property"
                >
                  <span className="flex min-w-0 flex-1 items-center gap-2">
                    <ChartLineIcon className="size-4 shrink-0 text-faint" />
                    <span className="truncate text-body font-medium">{v.property_name}</span>
                  </span>
                  <span className="flex shrink-0 items-baseline gap-4 text-small text-faint tabular-nums">
                    <span>
                      <b className="text-body font-medium text-foreground">{num(v.visitors)}</b> <Trans>visitors</Trans>
                    </span>
                    <span>
                      <b className="text-body font-medium text-foreground">{num(v.visits)}</b> <Trans>visits</Trans>
                    </span>
                    <span>
                      <b className="text-body font-medium text-foreground">{num(v.pageviews)}</b>{" "}
                      <Trans>pageviews</Trans>
                    </span>
                  </span>
                </Link>
              </li>
            ))}
          </Rows>
        )}
      </Card>
    </Section>
  )
}

function Replies({ report }: { report: BroadcastReport }) {
  const { t, i18n } = useLingui()
  const unnamed = t`Unnamed contact`
  return (
    <Section
      title={<Trans>Replies</Trans>}
      description={<Trans>Each reply is a conversation in the inbox. The first 50 are listed here.</Trans>}
    >
      <Card flush>
        {report.replies.length === 0 ? (
          <EmptyRow>
            <Trans>No replies yet.</Trans>
          </EmptyRow>
        ) : (
          <Rows data-testid="report-replies">
            {report.replies.map((r) => (
              <li key={r.conversation_id}>
                <Link
                  to={`/conversations/${r.conversation_id}`}
                  className="flex min-h-[52px] items-center gap-3 px-4 py-2.5 transition-colors hover:bg-muted"
                  data-testid="report-reply"
                >
                  <PersonAvatar name={r.contact.name || unnamed} className="size-8" />
                  <span className="min-w-0 flex-1 truncate text-body">{r.contact.name || unnamed}</span>
                  <time
                    className="shrink-0 text-caption text-faint"
                    dateTime={r.at}
                    title={formatDateTime(r.at, i18n.locale)}
                  >
                    {formatShort(r.at, i18n.locale)}
                  </time>
                  <MessageSquareReplyIcon className="size-4 shrink-0 text-faint" />
                </Link>
              </li>
            ))}
          </Rows>
        )}
      </Card>
    </Section>
  )
}

const ALL = "all"

function useLanguageReport(b: Broadcast) {
  const [params, setParams] = useSearchParams()
  const whole = useReport(b.id)
  const tags = (whole.data?.locales ?? []).map((l) => l.locale)
  const raw = params.get("lang") ?? ""
  const locale = tags.length > 1 && tags.includes(raw) ? raw : ""
  const narrowed = useReport(b.id, !!locale, locale)
  const setLocale = (tag: string) =>
    setParams(
      (p) => {
        const next = new URLSearchParams(p)
        if (tag) next.set("lang", tag)
        else next.delete("lang")
        return next
      },
      { replace: true }
    )
  return { whole, report: locale ? narrowed : whole, tags: tags.length > 1 ? tags : [], locale, setLocale }
}

function LanguageSelect({
  tags,
  locale,
  onChange,
}: {
  tags: string[]
  locale: string
  onChange: (tag: string) => void
}) {
  const { t } = useLingui()
  const languageName = useLanguageName()
  const items: Record<string, string> = {
    [ALL]: t`All languages`,
    ...Object.fromEntries(tags.map((tag) => [tag, languageName(tag)])),
  }
  return (
    <Select value={locale || ALL} items={items} onValueChange={(v) => onChange(!v || v === ALL ? "" : String(v))}>
      <SelectTrigger className="w-56 max-w-full min-w-0" aria-label={t`Language`} data-testid="report-language">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {Object.entries(items).map(([value, label]) => (
          <SelectItem key={value} value={value}>
            {label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function ByLanguage({ b, report }: { b: Broadcast; report: BroadcastReport }) {
  const { t } = useLingui()
  const num = useNumber()
  const pct = usePercent()
  const languageName = useLanguageName()
  const rows = report.locales ?? []
  if (rows.length < 2) return null
  const cell = (value: number, rate?: number, sent = 0) => (
    <td className="px-3 py-2.5 text-end tabular-nums">
      {num(value)}
      {rate !== undefined && sent > 0 && <span className="block text-caption text-faint">{pct(rate)}</span>}
    </td>
  )
  const th = "px-3 py-2 text-end font-semibold"
  return (
    <Section
      title={<Trans>By language</Trans>}
      description={<Trans>Each recipient counts under the version of the design they got.</Trans>}
    >
      <div className="overflow-x-auto rounded-[14px] border bg-card" data-testid="report-by-language">
        <table className="w-full min-w-[560px] text-body">
          <caption className="sr-only">{t`Figures by language`}</caption>
          <thead className={headClass}>
            <tr className="border-b">
              <th scope="col" className="px-3 py-2 text-start font-semibold">
                <Trans>Language</Trans>
              </th>
              <th scope="col" className={th}>
                <Trans>Recipients</Trans>
              </th>
              <th scope="col" className={th}>
                <Trans>Sent</Trans>
              </th>
              <th scope="col" className={th}>
                <Trans>Bounced</Trans>
              </th>
              <th scope="col" className={th}>
                <Trans>Unsubscribed</Trans>
              </th>
              <th scope="col" className={th}>
                <Trans>Replied</Trans>
              </th>
              {b.click_tracking && (
                <th scope="col" className={th}>
                  <Trans>Clicked</Trans>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {rows.map((l) => (
              <tr key={l.locale} className="border-b last:border-b-0" data-testid="report-language-row">
                <th scope="row" className="px-3 py-2.5 text-start font-medium">
                  {languageName(l.locale)}
                </th>
                {cell(l.counts.recipients)}
                {cell(l.counts.sent)}
                {cell(l.counts.bounced, l.rates.bounced, l.counts.sent)}
                {cell(l.counts.unsubscribed, l.rates.unsubscribed, l.counts.sent)}
                {cell(l.counts.replied, l.rates.replied, l.counts.sent)}
                {b.click_tracking && cell(l.counts.clicked, l.rates.clicked, l.counts.sent)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Section>
  )
}

export function ReportStats({ b }: { b: Broadcast }) {
  const { report, tags, locale, setLocale } = useLanguageReport(b)
  if (report.isPending) return <Skeleton className="h-28 rounded-2xl" />
  if (!report.data) return <ErrorLine error={report.error} />
  return (
    <div className="flex flex-col gap-2">
      {tags.length > 0 && <LanguageSelect tags={tags} locale={locale} onChange={setLocale} />}
      <Tiles b={b} report={report.data} />
    </div>
  )
}

export function Report({ b }: { b: Broadcast }) {
  const { whole, report, locale } = useLanguageReport(b)
  if (report.isPending) return <Skeleton className="h-64 rounded-2xl" />
  if (!report.data) return null
  return (
    <>
      {!locale && whole.data && <ByLanguage b={b} report={whole.data} />}
      <Timeline b={b} report={report.data} />
      {b.click_tracking && <Links report={report.data} />}
      <CampaignVisits b={b} report={report.data} />
      <Replies report={report.data} />
    </>
  )
}

function deliveryTone(s: BroadcastDeliveryState) {
  return s === "sent" ? "success" : s === "queued" || s === "skipped" ? "muted" : s === "bounced" ? "warning" : "danger"
}

function DeliveryRow({ d, languages }: { d: BroadcastDelivery; languages: boolean }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const languageName = useLanguageName()
  const at = d.sent_at ?? d.created_at
  const note = d.skip_reason ? text.skipReason[d.skip_reason] : d.error
  return (
    <li className="flex items-center gap-3 px-4 py-2.5" data-testid="delivery-row">
      <span
        className={cn(
          "grid min-w-0 flex-1 items-center gap-x-4 gap-y-0.5",
          languages ? "sm:grid-cols-[minmax(0,1fr)_7rem_9rem_6.5rem]" : "sm:grid-cols-[minmax(0,1fr)_9rem_6.5rem]"
        )}
      >
        <span className="flex min-w-0 flex-col">
          <span className="truncate text-body font-medium" dir="ltr">
            {d.email}
          </span>
          {(d.contact.name || note) && (
            <span className="truncate text-small text-faint" title={note}>
              {[d.contact.name, note].filter(Boolean).join(" · ")}
            </span>
          )}
        </span>
        <span className="flex flex-wrap items-center gap-x-2 sm:contents">
          {languages && (
            <span className="truncate text-small text-muted-foreground" data-testid="delivery-language">
              {d.locale ? languageName(d.locale) : ""}
            </span>
          )}
          <span className="flex min-w-0 items-center gap-2">
            <StatusTag tone={deliveryTone(d.state)} data-testid="delivery-state">
              {text.deliveryState[d.state]}
            </StatusTag>
            {d.conversation_id && (
              <Link
                to={`/conversations/${d.conversation_id}`}
                className="inline-flex items-center gap-1 text-caption text-brand hover:underline"
                title={t`Open the reply`}
              >
                <MessageSquareReplyIcon className="size-3.5" />
                <Trans>Replied</Trans>
              </Link>
            )}
          </span>
          <time
            className="text-caption text-faint tabular-nums sm:text-end"
            dateTime={at}
            title={formatDateTime(at, i18n.locale)}
          >
            {formatShort(at, i18n.locale)}
          </time>
        </span>
      </span>
    </li>
  )
}

export function Deliveries({ b }: { b: Broadcast }) {
  const { t } = useLingui()
  const text = useEnumText()
  const [state, setState] = useState<BroadcastDeliveryState | "">("")
  const [replied, setReplied] = useState(false)
  const [query, setQuery] = useState("")
  const q = useDebounced(query.trim(), 300)
  const { tags, locale } = useLanguageReport(b)
  const deliveries = useDeliveries(b.id, state, q, replied, locale)
  const items = deliveries.data?.pages.flatMap((p) => p.items) ?? []
  const filtered = !!state || !!q || replied || !!locale
  return (
    <Section title={<Trans>Recipients</Trans>}>
      <div className="flex flex-wrap items-center gap-2">
        <div className="max-w-full overflow-x-auto">
          <Segmented
            label={t`Show`}
            value={state || "all"}
            onChange={(v) => setState(v === "all" ? "" : (v as BroadcastDeliveryState))}
            items={[
              { value: "all", label: t`All`, testId: "deliveries-all" },
              ...BROADCAST_DELIVERY_STATES.map((s) => ({
                value: s,
                label: text.deliveryState[s],
                testId: `deliveries-${s}`,
              })),
            ]}
          />
        </div>
        <InputGroup className="h-9 min-w-48 flex-1 rounded-[10px] bg-card">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                if (query) setQuery("")
                else e.currentTarget.blur()
              }
            }}
            placeholder={t`Address or name`}
            aria-label={t`Search recipients`}
            data-testid="deliveries-search"
          />
          {query && (
            <InputGroupAddon align="inline-end">
              <InputGroupButton size="icon-xs" onClick={() => setQuery("")} aria-label={t`Clear the search`}>
                <XIcon />
              </InputGroupButton>
            </InputGroupAddon>
          )}
        </InputGroup>
        <Label className="gap-2 font-normal text-muted-foreground">
          <Switch checked={replied} onCheckedChange={setReplied} data-testid="deliveries-replied" />
          <Trans>Only those who replied</Trans>
        </Label>
      </div>
      <Card flush>
        {deliveries.isPending ? (
          <div className="flex flex-col gap-2 p-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : items.length === 0 ? (
          <EmptyRow>
            {filtered ? <Trans>Nobody matches.</Trans> : <Trans>Nobody is on the list of recipients yet.</Trans>}
          </EmptyRow>
        ) : (
          <Rows data-testid="deliveries">
            {items.map((d) => (
              <DeliveryRow key={d.id} d={d} languages={tags.length > 0} />
            ))}
          </Rows>
        )}
        {deliveries.hasNextPage && (
          <div className="flex justify-center border-t p-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void deliveries.fetchNextPage()}
              disabled={deliveries.isFetchingNextPage}
            >
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
        <ErrorLine error={deliveries.error} className="px-4 pb-3" />
      </Card>
    </Section>
  )
}
