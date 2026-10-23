import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { RefreshCwIcon, TriangleAlertIcon } from "lucide-react"
import { Link, Navigate, useLocation, useParams, useSearchParams } from "react-router"

import { ErrorLine, Notice } from "@/components/common"
import { SaveBarProvider } from "@/components/common/SaveBar"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useConversations } from "@/features/inbox/queries"
import { PushSection } from "@/features/settings/channels/app/PushSection"
import { AppAccessSection, AppInstallSection } from "@/features/settings/channels/app/AppSections"
import {
  ChatInstallSection,
  DiscussionsSection,
  LookSection,
  SitesSection,
  UpdatesSection,
  VisitorsSection,
} from "@/features/settings/channels/chat/ChatSections"
import {
  channelAction,
  channelState,
  channelTabLink,
  channelTabs,
  type ChannelTab,
} from "@/features/settings/channels/ChannelState"
import { AddressProofSection } from "@/features/settings/channels/email/AddressProof"
import { DeliverabilityCard, DkimSection } from "@/features/settings/channels/email/Deliverability"
import {
  AutoReplySection,
  BroadcastsSection,
  ReceivingSection,
  RepliesSection,
  SesSection,
} from "@/features/settings/channels/email/EmailSections"
import { AttachmentsSection } from "@/features/settings/channels/Attachments"
import { ApiSettingsSection, NameSection, RemoveSection } from "@/features/settings/channels/sections"
import { InboxProvider } from "@/features/settings/inboxes/InboxPage"
import { Card, Chip, EmptyRow, PageHeader, PageTabs, Pill, Row, Rows, Section } from "@/features/settings/ui"
import { api, unwrap, type Channel } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useChannel } from "@/lib/workspace"

const HASH_TAB: Record<string, ChannelTab> = {
  ses: "deliverability",
  broadcasts: "deliverability",
  deliverability: "deliverability",
  dkim: "deliverability",
  install: "install",
}

function address(ch: Channel) {
  if (ch.email) return ch.email.from_address || ch.email.address
  if (ch.chat) return ch.chat.allowed_origins[0]
  return undefined
}

function StateBanner({ ch, tab }: { ch: Channel; tab: ChannelTab }) {
  const state = channelState(ch)
  const action = channelAction(ch)
  if (state === "ok") return null
  const body =
    state === "receive" ? (
      <Trans>This channel only receives mail. Add an SMTP account under Replies so you can answer from Yuva.</Trans>
    ) : state === "identity_missing" ? (
      <Trans>Amazon SES cannot verify the sending domain yet. Publish the DNS records, then check again.</Trans>
    ) : state === "paused" ? (
      <Trans>Amazon SES paused sending. Replies and broadcasts are refused until the operator reinstates it.</Trans>
    ) : (
      <Trans>Amazon SES refused to send for this channel. Check the deliverability tab for the reason.</Trans>
    )
  return (
    <Notice
      tone={state === "receive" || state === "identity_missing" ? "warning" : "danger"}
      icon={TriangleAlertIcon}
      data-testid="channel-banner"
    >
      <p>{body}</p>
      {action.tab !== tab && (
        <div>
          <Button variant="outline" size="sm" render={<Link to={channelTabLink(ch, action.tab)} />}>
            {action.tab === "deliverability" ? <Trans>Deliverability</Trans> : <Trans>Settings</Trans>}
          </Button>
        </div>
      )}
    </Notice>
  )
}

function InstallStatus({ ch }: { ch: Channel }) {
  const list = useConversations({ inbox_id: ch.inbox_id, channel_kind: [ch.kind] })
  const first = list.data?.pages.flatMap((p) => p.items).find((c) => c.channel_id === ch.id)
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-3" data-testid="install-status">
        {first ? (
          <Pill tone="success">
            <Trans>Receiving</Trans>
          </Pill>
        ) : (
          <Pill tone="muted">
            <Trans>Waiting</Trans>
          </Pill>
        )}
        <p className="min-w-0 flex-1 text-body text-muted-foreground">
          {first ? (
            <Trans>The first message arrived. This channel works.</Trans>
          ) : (
            <Trans>Waiting for the first message. It shows up here as soon as it reaches Yuva.</Trans>
          )}
        </p>
        <Button variant="outline" size="sm" onClick={() => void list.refetch()} disabled={list.isFetching}>
          <RefreshCwIcon />
          <Trans>Check</Trans>
        </Button>
      </div>
    </Card>
  )
}

function KeysTab({ ch }: { ch: Channel }) {
  const { workspaceId: ws } = useSession()
  const list = useQuery({
    queryKey: keys.apiKeys(ws),
    queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items),
  })
  const mine = (list.data ?? []).filter(
    (k) => !k.revoked_at && (!k.inbox_ids || k.inbox_ids.length === 0 || k.inbox_ids.includes(ch.inbox_id))
  )
  return (
    <Section
      title={<Trans>Keys</Trans>}
      description={<Trans>Keys that can reach this product. Create and revoke them under API & MCP.</Trans>}
      action={
        <Link
          to="/settings/api-keys"
          className="text-body text-muted-foreground transition-colors hover:text-foreground"
        >
          <Trans>API keys</Trans>
        </Link>
      }
    >
      <Card flush>
        {list.data && mine.length === 0 ? (
          <EmptyRow>
            <Trans>No API keys can reach this product yet.</Trans>
          </EmptyRow>
        ) : (
          <Rows>
            {mine.map((k) => (
              <Row key={k.id}>
                <span className="min-w-0 flex-1 truncate font-medium">{k.name}</span>
                <Chip className="font-mono">{k.prefix}…</Chip>
              </Row>
            ))}
          </Rows>
        )}
        <ErrorLine error={list.error} className="px-4 pb-3" />
      </Card>
    </Section>
  )
}

function Body({ ch, tab }: { ch: Channel; tab: ChannelTab }) {
  if (ch.kind === "email")
    return tab === "settings" ? (
      <>
        <NameSection key={`name:${ch.id}`} channel={ch} />
        <AddressProofSection channel={ch} />
        <ReceivingSection channel={ch} />
        <RepliesSection channel={ch} />
        <AutoReplySection channel={ch} />
        <AttachmentsSection channel={ch} />
        <RemoveSection channel={ch} />
      </>
    ) : (
      <>
        <SesSection channel={ch} />
        <DeliverabilityCard channel={ch} />
        <DkimSection key={ch.email?.dkim?.selector ?? "none"} channel={ch} />
        <BroadcastsSection channel={ch} />
      </>
    )
  if (ch.kind === "chat")
    return tab === "settings" ? (
      <>
        <NameSection key={`name:${ch.id}`} channel={ch} />
        <SitesSection channel={ch} />
        <VisitorsSection channel={ch} />
        <LookSection channel={ch} />
        <DiscussionsSection channel={ch} />
        <UpdatesSection channel={ch} />
        <AttachmentsSection channel={ch} />
        <RemoveSection channel={ch} />
      </>
    ) : (
      <>
        <InstallStatus ch={ch} />
        <ChatInstallSection channel={ch} />
      </>
    )
  if (ch.kind === "app")
    return tab === "settings" ? (
      <>
        <NameSection key={`name:${ch.id}`} channel={ch} />
        <AppAccessSection channel={ch} />
        <PushSection channel={ch} />
        <AttachmentsSection channel={ch} />
        <RemoveSection channel={ch} />
      </>
    ) : (
      <>
        <InstallStatus ch={ch} />
        <AppInstallSection channel={ch} />
      </>
    )
  return tab === "settings" ? (
    <>
      <NameSection key={`name:${ch.id}`} channel={ch} />
      <ApiSettingsSection channel={ch} />
      <AttachmentsSection channel={ch} />
      <RemoveSection channel={ch} />
    </>
  ) : (
    <KeysTab ch={ch} />
  )
}

export function ChannelPage() {
  const { t } = useLingui()
  const { inboxId = "", channelId = "" } = useParams()
  const text = useEnumText()
  const { canManage } = useSession()
  const channel = useChannel(channelId)
  const [params] = useSearchParams()
  const { hash } = useLocation()
  const ch = channel.data
  if (!canManage || (ch && ch.inbox_id !== inboxId)) return <Navigate to={`/settings/inboxes/${inboxId}`} replace />
  const tabs = ch ? channelTabs(ch) : (["settings"] as ChannelTab[])
  const asked = params.get("tab") ?? HASH_TAB[hash.slice(1)] ?? "settings"
  const tab = (tabs as string[]).includes(asked) ? (asked as ChannelTab) : "settings"
  const label: Record<ChannelTab, string> = {
    settings: t`Settings`,
    deliverability: t`Deliverability`,
    install: t`Install`,
    keys: t`Keys`,
  }
  return (
    <InboxProvider>
      {() =>
        ch ? (
          <>
            <PageHeader
              title={ch.name}
              eyebrow={text.channel[ch.kind]}
              description={<span dir="ltr">{address(ch)}</span>}
              back={{ to: "/settings?view=channels", label: t`Channels` }}
              tabs={
                <PageTabs
                  label={t`Channel`}
                  current={tab}
                  items={tabs.map((x) => ({
                    key: x,
                    to: channelTabLink(ch, x),
                    label: label[x],
                    testId: `channel-tab-${x}`,
                  }))}
                />
              }
            />
            <StateBanner ch={ch} tab={tab} />
            <SaveBarProvider what={ch.name}>
              <div className="flex flex-col gap-4" key={`${ch.id}:${tab}`}>
                <Body ch={ch} tab={tab} />
              </div>
            </SaveBarProvider>
          </>
        ) : channel.isPending ? (
          <Skeleton className="h-64 w-full rounded-2xl" />
        ) : (
          <ErrorLine error={channel.error} />
        )
      }
    </InboxProvider>
  )
}
