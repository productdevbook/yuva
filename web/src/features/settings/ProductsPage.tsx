import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { TriangleAlertIcon } from "lucide-react"
import { useState } from "react"
import { Link, useSearchParams } from "react-router"

import { ChannelIcon, InboxMark, Notice } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { SettingsStrip } from "@/components/common/SettingsStrip"
import { formatDuration, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { AddProductDialog } from "@/features/settings/AddProductDialog"
import { AddChannelDialog } from "@/features/settings/channels/AddChannelDialog"
import { ChannelSummary } from "@/features/settings/channels/ChannelSummary"
import {
  channelAction,
  channelDomain,
  ChannelPill,
  channelTabLink,
  needsAttention,
} from "@/features/settings/channels/ChannelState"
import { PersonalMailboxes } from "@/features/settings/inboxes/Mailbox"
import { useInboxChannels } from "@/features/settings/SettingsColumn"
import { Card, CardHead, headButton, HeadLink, Page, PageHeader, PageTabs } from "@/features/settings/ui"
import { useTab } from "@/features/settings/useTab"
import type { BusinessHours, Channel, Inbox, Weekday } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { managesInbox } from "@/lib/mailbox"
import { useInboxMembers, useLockedMailboxes } from "@/lib/workspace"

function language(tag: string, locale: string) {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(tag) ?? tag
  } catch {
    return tag
  }
}

const DAYS: Weekday[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]

function useHours() {
  const { t, i18n } = useLingui()
  const day = new Intl.DateTimeFormat(i18n.locale, { weekday: "short", timeZone: "UTC" })
  const name = (d: Weekday) => day.format(new Date(Date.UTC(2024, 0, 1 + DAYS.indexOf(d))))
  const clock = (v: string) => (v.endsWith(":00") ? v.slice(0, 2) : v)
  return (h: BusinessHours) => {
    if (!h.enabled) return t`Always`
    if (h.intervals.length === 0) return t`not set`
    const spans = [...new Set(h.intervals.map((i) => `${clock(i.start)}–${clock(i.end)}`))]
    const days = DAYS.filter((d) => h.intervals.some((i) => i.day === d))
    const run = days.every((d, i) => i === 0 || DAYS.indexOf(d) === DAYS.indexOf(days[i - 1]) + 1)
    const which =
      days.length === 1
        ? name(days[0])
        : run
          ? `${name(days[0])}–${name(days[days.length - 1])}`
          : days.map(name).join(", ")
    return spans.length === 1 ? `${which} ${spans[0]}` : which
  }
}

function Fact({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col">
      <dt className="text-caption text-faint">{label}</dt>
      <dd className="flex min-w-0 items-center gap-1.5 truncate">{children}</dd>
    </div>
  )
}

function openNow(h: BusinessHours, zone: string) {
  if (!h.enabled) return false
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      weekday: "short",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).formatToParts(new Date())
    const get = (type: string) => parts.find((p) => p.type === type)?.value ?? ""
    const day = get("weekday").slice(0, 3).toLowerCase()
    const clock = `${get("hour")}:${get("minute")}`
    return h.intervals.some((i) => i.day === day && i.start.slice(0, 5) <= clock && clock < i.end.slice(0, 5))
  } catch {
    return false
  }
}

function ChannelLine({ ch }: { ch: Channel }) {
  const { canManage } = useSession()
  const action = channelAction(ch)
  const address = ch.email?.address ?? ch.chat?.allowed_origins[0]
  const text = useEnumText()
  return (
    <li className="flex min-w-0 items-center gap-2.5" data-testid="product-channel">
      <ChannelIcon kind={ch.kind} className="size-4 shrink-0 text-faint" />
      <span className="min-w-0 flex-1 truncate">
        {ch.name}
        <span className="text-faint"> · {address ?? text.channel[ch.kind]}</span>
      </span>
      <ChannelPill ch={ch} />
      {canManage && needsAttention(ch) && (
        <Link
          to={channelTabLink(ch, action.tab)}
          className="shrink-0 text-caption font-medium underline-offset-4 hover:underline"
        >
          <Trans>Fix</Trans>
        </Link>
      )}
    </li>
  )
}

function ProductCard({ inbox, channels }: { inbox: Inbox; channels?: Channel[] }) {
  const { i18n } = useLingui()
  const text = useEnumText()
  const { canManage } = useSession()
  const hours = useHours()
  const personal = !!inbox.mailbox
  const members = useInboxMembers(inbox.id).data
  const reply = inbox.expected_reply_minutes
  const within = reply !== undefined ? formatDuration(reply * 60, i18n.locale) : ""
  const domain = channelDomain(channels)
  const scheduled = openNow(inbox.business_hours, inbox.timezone)
  return (
    <li className="flex flex-col overflow-hidden rounded-xl border bg-card" data-testid="product-card">
      <div className="flex items-center gap-3 border-b px-[18px] py-4">
        <InboxMark name={inbox.name} color={inbox.branding.color} className="size-10 rounded-[10px] text-[18px]" />
        <div className="flex min-w-0 flex-1 flex-col leading-[1.3]">
          <span className="truncate text-[16px] font-semibold">{inbox.name}</span>
          <span className="truncate text-caption text-faint">
            {domain && <>{domain} · </>}
            {text.mode[inbox.mode]}
            {within && (
              <>
                {" · "}
                <Trans>replies within {within}</Trans>
              </>
            )}
          </span>
        </div>
        {managesInbox(inbox, true) && (
          <Button
            variant="outline"
            className={headButton.secondary}
            render={<Link to={`/settings/inboxes/${inbox.id}`} />}
            data-testid="product-settings"
          >
            <Trans>Settings</Trans>
          </Button>
        )}
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2.5 px-[18px] pt-3.5 pb-3">
        <Fact label={<Trans>Language</Trans>}>{language(inbox.default_locale, i18n.locale)}</Fact>
        <Fact label={<Trans>Hours</Trans>}>
          {scheduled && <span aria-hidden className="size-2 shrink-0 rounded-full bg-success" />}
          <span className="truncate">{hours(inbox.business_hours)}</span>
        </Fact>
        {personal ? (
          <Fact label={<Trans>Access</Trans>}>
            {inbox.mailbox?.access === "owner" ? (
              <Trans>Yours</Trans>
            ) : inbox.mailbox?.access === "send" ? (
              <Trans>Read and send</Trans>
            ) : (
              <Trans context="mailbox access">Read</Trans>
            )}
          </Fact>
        ) : (
          <Fact label={<Trans>Members</Trans>}>{members ? members.length : "–"}</Fact>
        )}
        {!personal && (
          <Fact label={<Trans>Ratings</Trans>}>
            {inbox.ask_for_rating ? <Trans>asked after closing</Trans> : <Trans>off</Trans>}
          </Fact>
        )}
      </dl>
      <div className="mt-auto flex flex-col gap-2 border-t bg-surface/50 px-[18px] py-3.5">
        {channels === undefined ? (
          <Skeleton className="h-5 w-24" />
        ) : channels.length === 0 ? (
          <p className="flex flex-wrap items-center gap-x-2 text-caption text-faint">
            <Trans>No channels yet</Trans>
            {canManage && (
              <Link
                to="/settings?view=channels"
                className="text-foreground underline-offset-4 hover:underline"
                data-testid="product-add-channel"
              >
                <Trans>Add channel</Trans>
              </Link>
            )}
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {channels.map((ch) => (
              <ChannelLine key={ch.id} ch={ch} />
            ))}
          </ul>
        )}
      </div>
    </li>
  )
}

function ChannelGroup({ inbox, channels, only }: { inbox: Inbox; channels?: Channel[]; only: boolean }) {
  const { canManage } = useSession()
  const shown = only ? (channels ?? []).filter(needsAttention) : channels
  if (only && shown?.length === 0) return null
  return (
    <Card flush data-testid="channel-group">
      <CardHead
        action={
          managesInbox(inbox, canManage) && (
            <HeadLink to={`/settings/inboxes/${inbox.id}`} testId="channel-group-settings">
              <Trans>Product settings</Trans>
            </HeadLink>
          )
        }
      >
        <InboxMark name={inbox.name} color={inbox.branding.color} className="size-[22px]" />
        {inbox.name}
      </CardHead>
      {shown === undefined ? (
        <div className="p-4">
          <Skeleton className="h-9 w-full" />
        </div>
      ) : shown.length === 0 ? (
        <p className="px-4 py-5 text-body text-faint">
          <Trans>No channels yet. Add one so customers can write to {inbox.name}.</Trans>
        </p>
      ) : (
        <ul>
          {shown.map((ch) => {
            const action = channelAction(ch)
            return (
              <li
                key={ch.id}
                className="flex items-center gap-3 border-b px-4 py-[11px] last:border-b-0"
                data-testid="channel-row"
              >
                <span className="grid size-8 shrink-0 place-items-center rounded-lg border bg-surface text-muted-foreground">
                  <ChannelIcon kind={ch.kind} className="size-4" />
                </span>
                <div className="flex min-w-0 flex-1 flex-col leading-[1.3]">
                  <span className="truncate font-medium">{ch.name}</span>
                  <span className="truncate text-caption text-faint">
                    <ChannelSummary ch={ch} />
                  </span>
                </div>
                <ChannelPill ch={ch} />
                {managesInbox(inbox, canManage) && (
                  <Button
                    variant="outline"
                    className="h-[30px] rounded-lg border-input px-2.5 font-normal"
                    render={<Link to={channelTabLink(ch, action.tab)} />}
                    data-testid="channel-open"
                  >
                    {action.label}
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

export function ProductsHome() {
  const { t } = useLingui()
  const { canManage } = useSession()
  const [params] = useSearchParams()
  const [inboxId] = useInboxFilter()
  const [adding, setAdding] = useState<"product" | "channel" | null>(null)
  const view = params.get("view") === "channels" ? "channels" : "products"
  const { tab, to } = useTab(["all", "attention"] as const, "all")
  const all = useInboxChannels()
  const locked = useLockedMailboxes()
  const inboxes = all.filter(({ inbox }) => !inboxId || inbox.id === inboxId)
  const loaded = inboxes.every((x) => x.channels !== undefined)
  const everyChannel = inboxes.flatMap((x) => x.channels ?? [])
  const attention = everyChannel.filter(needsAttention).length
  return (
    <Page testId="settings-home">
      <PageHeader
        back={false}
        title={view === "channels" ? <Trans>Channels</Trans> : <Trans>Products</Trans>}
        description={
          view === "channels" ? (
            <Trans>How messages reach each product</Trans>
          ) : (
            <Trans>Each product has its own name, branding, hours, team and channels</Trans>
          )
        }
        action={
          canManage &&
          (view === "channels" ? (
            inboxes.length > 0 && (
              <Button className={headButton.primary} onClick={() => setAdding("channel")} data-testid="channels-add">
                <Trans>Add channel</Trans>
              </Button>
            )
          ) : (
            <Button className={headButton.primary} onClick={() => setAdding("product")} data-testid="products-add">
              <Trans>Add product</Trans>
            </Button>
          ))
        }
        tabs={
          view === "channels" && (
            <PageTabs
              label={t`Show`}
              current={tab}
              items={[
                {
                  key: "all",
                  to: to("all"),
                  label: t`All channels`,
                  count: loaded ? everyChannel.length : undefined,
                  testId: "channels-tab-all",
                },
                {
                  key: "attention",
                  to: to("attention"),
                  label: t`Needs attention`,
                  count: loaded ? attention : undefined,
                  testId: "channels-tab-attention",
                },
              ]}
            />
          )
        }
      />
      <SettingsStrip current={view} />
      {inboxes.length === 0 && locked.length === 0 ? (
        <EmptyBox
          title={<Trans>No products yet</Trans>}
          testId="products-empty"
          actions={
            canManage && (
              <Button className={headButton.primary} onClick={() => setAdding("product")}>
                <Trans>Add product</Trans>
              </Button>
            )
          }
        >
          <Trans>
            Add one for each product or brand you answer for. Each gets its own branding, hours, team and channels.
          </Trans>
        </EmptyBox>
      ) : view === "channels" ? (
        tab === "attention" && loaded && attention === 0 ? (
          <EmptyBox icon={TriangleAlertIcon} title={<Trans>All channels work</Trans>} testId="channels-all-work">
            <Trans>Nothing needs attention right now.</Trans>
          </EmptyBox>
        ) : (
          <div className="flex flex-col gap-4" data-testid="settings-channels">
            {inboxes.map(({ inbox, channels }) => (
              <ChannelGroup key={inbox.id} inbox={inbox} channels={channels} only={tab === "attention"} />
            ))}
          </div>
        )
      ) : (
        <>
          {attention > 0 && (
            <Notice tone="warning" icon={TriangleAlertIcon} data-testid="products-attention">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                <p className="min-w-0 flex-1">
                  <Plural value={attention} one="# channel needs attention" other="# channels need attention" />
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  className={cn("bg-background")}
                  render={<Link to="/settings?view=channels&tab=attention" />}
                >
                  <Trans>Open channels</Trans>
                </Button>
              </div>
            </Notice>
          )}
          <ul className="grid gap-4 lg:grid-cols-2" data-testid="settings-products">
            {inboxes.map(({ inbox, channels }) => (
              <ProductCard key={inbox.id} inbox={inbox} channels={channels} />
            ))}
          </ul>
          {!inboxId && <PersonalMailboxes items={locked} />}
        </>
      )}
      {adding === "product" && <AddProductDialog onClose={() => setAdding(null)} />}
      {adding === "channel" && (
        <AddChannelDialog
          inboxes={inboxes.map((x) => x.inbox)}
          initial={inboxId || undefined}
          onClose={() => setAdding(null)}
        />
      )}
    </Page>
  )
}
