import { useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { createContext, useContext, useState } from "react"
import { createPortal } from "react-dom"
import { useLocation } from "react-router"

import { InfoSection, KV, NavGroup, SectionPage, TopSep, TopSlotProvider, type NavItem } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { useIssueIntegrations } from "@/features/issues/queries"
import { useMigrations } from "@/features/migrations/queries"
import { useAutomationRules } from "@/features/settings/automation/queries"
import { needsAttention } from "@/features/settings/channels/ChannelState"
import { useGrants } from "@/features/settings/connected-apps/queries"
import { useInboxChannels } from "@/features/settings/SettingsColumn"
import { useSlack } from "@/features/settings/slack/queries"
import { FlatSettingsProvider } from "@/features/settings/ui"
import { api, unwrap, useVersion } from "@/lib/api"
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

type GroupKey = "you" | "workspace" | "products" | "content" | "automation" | "developers" | "security"

const GROUPS: [GroupKey, SettingsPlace[]][] = [
  ["you", ["profile", "signature", "addresses", "notifications", "appearance"]],
  ["workspace", ["workspace", "members", "teams", "migrations"]],
  ["products", ["products", "channels"]],
  ["content", ["labels", "canned-replies", "fields"]],
  ["automation", ["automation", "macros", "sla"]],
  ["developers", ["api-keys", "webhooks", "connected-apps", "issue-trackers", "slack"]],
  ["security", ["sso", "audit-log"]],
]

const PLACES = GROUPS.flatMap(([, items]) => items)

export function placeOf(pathname: string, search: string): SettingsPlace {
  if (pathname.startsWith("/migrations")) return "migrations"
  const seg = pathname.split("/")[2] ?? ""
  if (seg === "inboxes") return pathname.includes("/channels/") ? "channels" : "products"
  if ((PLACES as string[]).includes(seg)) return seg as SettingsPlace
  return new URLSearchParams(search).get("view") === "channels" ? "channels" : "products"
}

function groupOf(place: SettingsPlace): GroupKey {
  return GROUPS.find(([, items]) => items.includes(place))![0]
}

export function useSettingsNames() {
  const { t } = useLingui()
  const { canManage } = useSession()
  const group: Record<GroupKey, string> = {
    you: t`You`,
    workspace: t`Workspace`,
    products: t`Products`,
    content: t`Content`,
    automation: t`Automation`,
    developers: canManage ? t`Developers` : t`API & MCP`,
    security: t`Security`,
  }
  const place: Record<SettingsPlace, string> = {
    profile: t`Profile`,
    signature: t`Signature`,
    addresses: t`Your addresses`,
    notifications: t`Notifications`,
    appearance: t`Appearance`,
    workspace: t`General`,
    members: t`Members`,
    teams: t`Teams`,
    migrations: t`Move to Yuva`,
    products: t`Products`,
    channels: t`Channels`,
    labels: t`Labels`,
    "canned-replies": t`Canned replies`,
    fields: t`Custom fields`,
    automation: t`Rules`,
    macros: t`Macros`,
    sla: t`SLA policies`,
    "api-keys": t`API keys`,
    webhooks: t`Webhooks`,
    "connected-apps": t`Connected apps`,
    "issue-trackers": t`Issue trackers`,
    slack: t`Slack`,
    sso: t`Single sign-on`,
    "audit-log": t`Audit log`,
  }
  return { group, place }
}

function useAbout(): Partial<Record<SettingsPlace, string>> {
  const { t } = useLingui()
  return {
    signature: t`What your e-mailed replies end with`,
    addresses: t`Send mail from your own address`,
    notifications: t`When we let you know`,
    appearance: t`Theme and e-mail view on this device`,
    workspace: t`The workspace: labels, replies, data`,
    teams: t`Groups of members and the inboxes they cover`,
    migrations: t`Bring a mailbox's history from Gmail, Yandex or any IMAP server`,
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

function useCounts(): Partial<Record<SettingsPlace, number>> & { attention?: number } {
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

const TO: Record<SettingsPlace, string> = {
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

const TEST_IDS: Partial<Record<SettingsPlace, string>> = {
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

const MANAGERS: SettingsPlace[] = [
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
const OWNERS: SettingsPlace[] = ["sso", "audit-log"]

function SettingsNav({ place }: { place: SettingsPlace }) {
  const { t } = useLingui()
  const { canManage, isOwner } = useSession()
  const names = useSettingsNames()
  const counts = useCounts()
  const slack = useSlack(canManage).data
  const allowed = (p: SettingsPlace) => (OWNERS.includes(p) ? isOwner : MANAGERS.includes(p) ? canManage : true)
  const attention = counts.attention ?? 0
  return (
    <>
      {GROUPS.map(([g, items]) => {
        const rows: NavItem[] = items.filter(allowed).map((p) => ({
          key: p,
          label: names.place[p],
          to: TO[p],
          count: p === "channels" && attention > 0 ? attention : counts[p],
          hot: p === "channels" && attention > 0,
          ariaLabel: p === "channels" && attention > 0 ? t`Channels needing attention: ${attention}` : undefined,
          title: p === "slack" && slack?.available ? (slack.connected ? t`On` : t`Off`) : undefined,
          current: p === place,
          testId: TEST_IDS[p] ?? `settings-nav-${p}`,
        }))
        return rows.length > 0 && <NavGroup key={g} title={names.group[g]} items={rows} />
      })}
    </>
  )
}

const AsideSlot = createContext<HTMLElement | null>(null)

export function SettingsAside({ children }: { children: React.ReactNode }) {
  const slot = useContext(AsideSlot)
  return slot ? createPortal(children, slot) : null
}

function Aside({ place, slot }: { place: SettingsPlace; slot: (el: HTMLElement | null) => void }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const { membership } = useSession()
  const names = useSettingsNames()
  const about = useAbout()[place]
  const members = useMembers().data
  const version = useVersion().data?.version
  const num = new Intl.NumberFormat(i18n.locale)
  return (
    <>
      {about && (
        <InfoSection title={names.place[place]} testId="settings-about">
          <p className="m-0 text-small leading-5 text-muted-foreground">{about}</p>
        </InfoSection>
      )}
      <div ref={slot} className="contents" />
      <InfoSection title={t`Workspace`} testId="settings-workspace-info">
        <KV label={t`Name`} value={membership.workspace.name} />
        <KV label={t`Role`} value={text.role[membership.role]} />
        {members && <KV label={t`Members`} value={num.format(members.length)} to="/team" />}
        {version && <KV label={t`Version`} value={<span className="font-mono">{version}</span>} />}
      </InfoSection>
    </>
  )
}

export function SettingsTrail({ place, name }: { place: SettingsPlace; name?: string }) {
  const { t } = useLingui()
  const names = useSettingsNames()
  return (
    <>
      <span className="font-medium text-foreground">{t`Settings`}</span>
      <TopSep />
      <span className="truncate">{names.group[groupOf(place)]}</span>
      <TopSep />
      <span className="truncate">{names.place[place]}</span>
      {name && (
        <>
          <TopSep />
          <span className="truncate normal-case">{name}</span>
        </>
      )}
    </>
  )
}

export function SettingsLayout({ children }: { children: React.ReactNode }) {
  const { t } = useLingui()
  const { pathname, search } = useLocation()
  const place = placeOf(pathname, search)
  const [slot, setSlot] = useState<HTMLElement | null>(null)
  const [top, setTop] = useState<HTMLElement | null>(null)
  return (
    <SectionPage
      title={t`Settings`}
      searchLabel={t`Search`}
      nav={<SettingsNav place={place} />}
      aside={<Aside place={place} slot={setSlot} />}
      testId="settings-page"
    >
      <AsideSlot.Provider value={slot}>
        <TopSlotProvider value={top}>
          <FlatSettingsProvider value={{ trail: <SettingsTrail place={place} />, slot: setTop }}>
            {children}
          </FlatSettingsProvider>
        </TopSlotProvider>
      </AsideSlot.Provider>
    </SectionPage>
  )
}
