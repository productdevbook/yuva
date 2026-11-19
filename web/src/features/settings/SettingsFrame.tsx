import { useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { PageLayout, TopSlotProvider } from "@/components/common"
import { useIssueIntegrations } from "@/features/issues/queries"
import { useMigrations } from "@/features/migrations/queries"
import { useAutomationRules } from "@/features/settings/automation/queries"
import { needsAttention } from "@/features/settings/channels/ChannelState"
import { useGrants } from "@/features/settings/connected-apps/queries"
import { useInboxChannels } from "@/features/settings/SettingsColumn"
import { TitleSlotProvider } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import {
  useCannedReplies,
  useCustomFields,
  useLabels,
  useMacros,
  useMembers,
  useSlaPolicies,
  useTeams,
} from "@/lib/workspace"

export type SettingsPlace =
  | "profile"
  | "signature"
  | "addresses"
  | "notifications"
  | "appearance"
  | "workspace"
  | "members"
  | "teams"
  | "migrations"
  | "products"
  | "channels"
  | "labels"
  | "canned-replies"
  | "fields"
  | "automation"
  | "macros"
  | "sla"
  | "api-keys"
  | "webhooks"
  | "connected-apps"
  | "issue-trackers"
  | "slack"
  | "sso"
  | "audit-log"

const PLACES: SettingsPlace[] = [
  "profile",
  "signature",
  "addresses",
  "notifications",
  "appearance",
  "workspace",
  "members",
  "teams",
  "migrations",
  "products",
  "channels",
  "labels",
  "canned-replies",
  "fields",
  "automation",
  "macros",
  "sla",
  "api-keys",
  "webhooks",
  "connected-apps",
  "issue-trackers",
  "slack",
  "sso",
  "audit-log",
]

export function placeOf(pathname: string, search: string): SettingsPlace {
  if (pathname.startsWith("/migrations")) return "migrations"
  const seg = pathname.split("/")[2] ?? ""
  if (seg === "inboxes") return pathname.includes("/channels/") ? "channels" : "products"
  if ((PLACES as string[]).includes(seg)) return seg as SettingsPlace
  return new URLSearchParams(search).get("view") === "channels" ? "channels" : "products"
}

export function useSettingsNames() {
  const { t } = useLingui()
  const group = {
    workspace: t`Workspace`,
    channels: t`Channels`,
    conversations: t`Conversations`,
    developer: t`Developer`,
    security: t`Security`,
    you: t`You`,
  }
  const place: Record<SettingsPlace, string> = {
    profile: t`Profile`,
    signature: t`Signature`,
    addresses: t`Addresses`,
    notifications: t`Notifications`,
    appearance: t`Appearance`,
    workspace: t`General`,
    members: t`Members`,
    teams: t`Teams`,
    migrations: t`Imports`,
    products: t`Products`,
    channels: t`Inboxes and channels`,
    labels: t`Labels`,
    "canned-replies": t`Saved replies`,
    fields: t`Fields`,
    automation: t`Automation`,
    macros: t`Macros`,
    sla: t`Reply times`,
    "api-keys": t`API keys`,
    webhooks: t`Webhooks`,
    "connected-apps": t`Connected apps`,
    "issue-trackers": t`Issue trackers`,
    slack: t`Slack`,
    sso: t`SSO`,
    "audit-log": t`Audit log`,
  }
  return { group, place }
}

export function useSettingsAbout(): Partial<Record<SettingsPlace, string>> {
  const { t } = useLingui()
  return {
    profile: t`Your name, photo, language and sign-in`,
    signature: t`What your e-mailed replies end with`,
    addresses: t`Send mail from your own address`,
    notifications: t`When we let you know`,
    appearance: t`Theme and e-mail view on this device`,
    workspace: t`Name, closed conversations and broadcast sending`,
    products: t`Who you support: name, hours, team, sender`,
    channels: t`How messages arrive: chat, e-mail, apps`,
    labels: t`Sort conversations your way`,
    "canned-replies": t`What you write often`,
    fields: t`Typed values on conversations and contacts`,
    automation: t`Rules that label, assign, reply and call webhooks`,
    macros: t`A reply and its actions in one step`,
    sla: t`Reply and resolution targets with due times`,
    "api-keys": t`Connect your servers and bots to Yuva`,
    webhooks: t`Send events to your own server`,
    "connected-apps": t`Connect an assistant such as Claude or ChatGPT`,
    "issue-trackers": t`Turn feedback into GitHub or Linear issues`,
    slack: t`Post conversations to a channel and answer from it`,
    sso: t`Let your team sign in through your identity provider`,
    "audit-log": t`Who changed access, keys and data`,
  }
}

export function useSettingsCounts(): Partial<Record<SettingsPlace, number>> & { attention?: number } {
  const { canManage, workspaceId: ws } = useSession()
  const inboxes = useInboxChannels()
  const members = useMembers().data
  const teams = useTeams().data
  const labels = useLabels().data
  const canned = useCannedReplies().data
  const fields = useCustomFields().data
  const macros = useMacros().data
  const sla = useSlaPolicies().data
  const rules = useAutomationRules().data
  const moves = useMigrations().data?.pages[0]?.items
  const grants = useGrants().data
  const trackers = useIssueIntegrations(canManage).data
  const apiKeys = useQuery({
    queryKey: keys.apiKeys(ws),
    queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items),
    enabled: canManage,
  }).data
  const hooks = useQuery({
    queryKey: keys.webhooks(ws),
    queryFn: () => unwrap(api.GET("/v1/webhooks")).then((r) => r.items),
    enabled: canManage,
  }).data
  const loaded = inboxes.every((x) => x.channels !== undefined)
  const channels = inboxes.flatMap((x) => x.channels ?? [])
  return {
    members: members?.length,
    teams: teams?.length,
    migrations: moves?.length,
    products: inboxes.length,
    channels: loaded ? channels.length : undefined,
    attention: loaded ? channels.filter(needsAttention).length : undefined,
    labels: labels?.length,
    "canned-replies": canned?.length,
    fields: fields?.length,
    automation: rules?.length,
    macros: macros?.length,
    sla: sla?.length,
    "api-keys": apiKeys?.filter((k) => !k.revoked_at).length,
    webhooks: hooks?.length,
    "connected-apps": grants?.length,
    "issue-trackers": trackers?.length,
  }
}

export const SETTINGS_TO: Record<SettingsPlace, string> = {
  profile: "/settings/profile",
  signature: "/settings/signature",
  addresses: "/settings/addresses",
  notifications: "/settings/notifications",
  appearance: "/settings/appearance",
  workspace: "/settings/workspace",
  members: "/team",
  teams: "/settings/teams",
  migrations: "/settings/migrations",
  products: "/settings",
  channels: "/settings?view=channels",
  labels: "/settings/labels",
  "canned-replies": "/settings/canned-replies",
  fields: "/settings/fields",
  automation: "/settings/automation",
  macros: "/settings/macros",
  sla: "/settings/sla",
  "api-keys": "/settings/api-keys",
  webhooks: "/settings/webhooks",
  "connected-apps": "/settings/connected-apps",
  "issue-trackers": "/settings/issue-trackers",
  slack: "/settings/slack",
  sso: "/settings/sso",
  "audit-log": "/settings/audit-log",
}

export const SETTINGS_TEST_IDS: Partial<Record<SettingsPlace, string>> = {
  profile: "settings-profile",
  appearance: "settings-appearance",
  workspace: "general-tab-workspace",
  teams: "general-tab-teams",
  migrations: "settings-migrations",
  products: "settings-products-nav",
  channels: "settings-channels-nav",
  labels: "general-tab-labels",
  "canned-replies": "general-tab-canned-replies",
  fields: "general-tab-fields",
  automation: "general-tab-automation",
  macros: "general-tab-macros",
  sla: "general-tab-sla",
  "api-keys": "developer-tab-api-keys",
  webhooks: "developer-tab-webhooks",
  "connected-apps": "developer-tab-connected-apps",
  sso: "settings-sso",
  "audit-log": "settings-audit-log",
}

export const SETTINGS_MANAGERS: SettingsPlace[] = [
  "teams",
  "migrations",
  "fields",
  "automation",
  "sla",
  "api-keys",
  "webhooks",
  "issue-trackers",
  "slack",
]
export const SETTINGS_OWNERS: SettingsPlace[] = ["sso", "audit-log"]

export function SettingsLayout({ children }: { children: React.ReactNode }) {
  const { t } = useLingui()
  const [slot, setSlot] = useState<HTMLElement | null>(null)
  return (
    <PageLayout testId="settings-page" label={t`Settings`}>
      <TitleSlotProvider value={setSlot}>
        <TopSlotProvider value={slot}>{children}</TopSlotProvider>
      </TitleSlotProvider>
    </PageLayout>
  )
}
