import { Trans, useLingui } from "@lingui/react/macro"
import { useQueries, useQuery } from "@tanstack/react-query"
import {
  ArrowRightLeftIcon,
  ArrowUpRightIcon,
  BellIcon,
  BuildingIcon,
  CircleDotIcon,
  KeyRoundIcon,
  MessageSquareIcon,
  PaletteIcon,
  PlugIcon,
  PlusIcon,
  QuoteIcon,
  TextCursorInputIcon,
  TimerIcon,
  WorkflowIcon,
  UsersRoundIcon,
  ZapIcon,
  ScrollTextIcon,
  ShieldCheckIcon,
  TagIcon,
  UsersIcon,
} from "lucide-react"
import { Outlet, useLocation } from "react-router"

import { Column, ColumnHeading, ColumnLink } from "@/components/common/Column"
import { initials, useEnumText } from "@/components/common/text"
import { useAutomationRules } from "@/features/settings/automation/queries"
import { useSlack } from "@/features/settings/slack/queries"
import { useIssueIntegrations } from "@/features/issues/queries"
import { ProductsHome } from "@/features/settings/ProductsPage"
import { Page, RowIcon, RowText } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import {
  useCannedReplies,
  useCustomFields,
  useInboxes,
  useLabels,
  useMacros,
  useMembers,
  useSlaPolicies,
  useTeams,
} from "@/lib/workspace"

const WIDE = [
  "/settings/api-keys",
  "/settings/issue-trackers",
  "/settings/slack",
  "/settings/webhooks",
  "/settings/connected-apps",
]

function isWide(pathname: string) {
  if (WIDE.includes(pathname)) return true
  if (pathname.startsWith("/settings/connected-apps/")) return true
  return /^\/settings\/inboxes\/[^/]+$/.test(pathname)
}

export function SettingsPane() {
  const { pathname } = useLocation()
  return (
    <Page testId="settings" width={isWide(pathname) ? "full" : "form"}>
      <Outlet />
    </Page>
  )
}

export function SettingsHome() {
  return <ProductsHome />
}

function Group({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <>
      <ColumnHeading>{title}</ColumnHeading>
      <ul className="flex flex-col">{children}</ul>
    </>
  )
}

function NavRow({
  to,
  state,
  children,
  value,
  testId,
}: {
  to: string
  state?: unknown
  children: React.ReactNode
  value?: React.ReactNode
  testId?: string
}) {
  return (
    <li>
      <ColumnLink to={to} state={state} testId={testId}>
        {children}
        {value !== undefined && value !== "" && (
          <span className="shrink-0 text-small whitespace-nowrap text-faint">{value}</span>
        )}
      </ColumnLink>
    </li>
  )
}

export function useInboxChannels() {
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data ?? []
  const results = useQueries({
    queries: inboxes.map((i) => ({
      queryKey: keys.channels(ws, i.id),
      queryFn: () =>
        unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: i.id } } })).then((r) => r.items),
      staleTime: 5 * 60_000,
    })),
  })
  return inboxes.map((inbox, i) => ({ inbox, channels: results[i]?.data }))
}

export function SettingsColumn() {
  const { t } = useLingui()
  const text = useEnumText()
  const { me, membership, canManage, isOwner, workspaceId: ws } = useSession()
  const inboxes = useInboxChannels()
  const members = useMembers().data
  const canned = useCannedReplies().data
  const labels = useLabels().data
  const macros = useMacros().data
  const teams = useTeams().data
  const fields = useCustomFields().data
  const sla = useSlaPolicies().data
  const rules = useAutomationRules().data
  const apiKeys = useQuery({
    queryKey: keys.apiKeys(ws),
    queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items),
    enabled: canManage,
  })
  const hooks = useQuery({
    queryKey: keys.webhooks(ws),
    queryFn: () => unwrap(api.GET("/v1/webhooks")).then((r) => r.items),
    enabled: canManage,
  })
  const trackers = useIssueIntegrations(canManage)
  const slack = useSlack(canManage)
  const name = me.person.name || me.person.email
  const list = (n?: number) => (n === undefined ? "" : n)
  return (
    <Column title={<Trans>Settings</Trans>} testId="settings-nav">
      <Group title={<Trans>You</Trans>}>
        <NavRow to="/settings/profile" testId="settings-profile">
          <RowIcon>{initials(name)}</RowIcon>
          <RowText title={name} detail={me.person.email} />
        </NavRow>
        <NavRow to="/settings/notifications">
          <RowIcon>
            <BellIcon />
          </RowIcon>
          <RowText title={<Trans>Notifications</Trans>} detail={<Trans>When we let you know</Trans>} />
        </NavRow>
        <NavRow to="/settings/appearance" testId="settings-appearance">
          <RowIcon>
            <PaletteIcon />
          </RowIcon>
          <RowText title={<Trans>Appearance</Trans>} detail={<Trans>Theme and e-mail view on this device</Trans>} />
        </NavRow>
      </Group>
      <Group title={<Trans>Products</Trans>}>
        {inboxes.map(({ inbox, channels }) => (
          <NavRow key={inbox.id} to={`/settings/inboxes/${inbox.id}`} testId="inbox-row">
            <RowIcon>
              <span style={{ color: inbox.branding.color }}>{inbox.name.charAt(0).toLocaleUpperCase()}</span>
            </RowIcon>
            <RowText
              title={inbox.name}
              detail={
                channels
                  ? [...new Set(channels.map((c) => text.channel[c.kind]))].join(", ") || t`No channels yet`
                  : " "
              }
            />
          </NavRow>
        ))}
        {canManage && (
          <NavRow to="/setup/new" state={{ back: "/settings" }} testId="add-inbox">
            <RowIcon>
              <PlusIcon />
            </RowIcon>
            <RowText title={<Trans>Add an inbox</Trans>} detail={<Trans>One per product or brand</Trans>} />
          </NavRow>
        )}
      </Group>
      <Group title={<Trans>Team</Trans>}>
        <NavRow to="/settings/members" value={list(members?.length)}>
          <RowIcon>
            <UsersIcon />
          </RowIcon>
          <RowText
            title={<Trans>Members</Trans>}
            detail={(members ?? []).map((m) => (m.name || m.email).split(" ")[0]).join(", ")}
          />
        </NavRow>
        <NavRow to="/settings/canned-replies" value={list(canned?.length)}>
          <RowIcon>
            <QuoteIcon />
          </RowIcon>
          <RowText title={<Trans>Canned replies</Trans>} detail={<Trans>What you write often</Trans>} />
        </NavRow>
        <NavRow to="/settings/macros" value={list(macros?.length)}>
          <RowIcon>
            <ZapIcon />
          </RowIcon>
          <RowText title={<Trans>Macros</Trans>} detail={<Trans>A reply and its actions in one step</Trans>} />
        </NavRow>
        {canManage && (
          <NavRow to="/settings/teams" value={list(teams?.length)}>
            <RowIcon>
              <UsersRoundIcon />
            </RowIcon>
            <RowText
              title={<Trans>Teams</Trans>}
              detail={<Trans>Groups of members and the inboxes they cover</Trans>}
            />
          </NavRow>
        )}
        {canManage && (
          <NavRow to="/settings/fields" value={list(fields?.length)}>
            <RowIcon>
              <TextCursorInputIcon />
            </RowIcon>
            <RowText
              title={<Trans>Custom fields</Trans>}
              detail={<Trans>Typed values on conversations and contacts</Trans>}
            />
          </NavRow>
        )}
        {canManage && (
          <NavRow to="/settings/sla" value={list(sla?.length)}>
            <RowIcon>
              <TimerIcon />
            </RowIcon>
            <RowText
              title={<Trans>SLA policies</Trans>}
              detail={<Trans>Reply and resolution targets with due times</Trans>}
            />
          </NavRow>
        )}
        {canManage && (
          <NavRow to="/settings/automation" value={list(rules?.length)}>
            <RowIcon>
              <WorkflowIcon />
            </RowIcon>
            <RowText
              title={<Trans>Automation</Trans>}
              detail={<Trans>Rules that label, assign, reply and call webhooks</Trans>}
            />
          </NavRow>
        )}
        <NavRow to="/settings/labels" value={list(labels?.length)}>
          <RowIcon>
            <TagIcon />
          </RowIcon>
          <RowText title={<Trans>Labels</Trans>} detail={<Trans>Sort conversations your way</Trans>} />
        </NavRow>
      </Group>
      <Group title={<Trans>API & MCP</Trans>}>
        {canManage && (
          <>
            <NavRow to="/settings/api-keys" value={list(apiKeys.data?.length)}>
              <RowIcon>
                <KeyRoundIcon />
              </RowIcon>
              <RowText title={<Trans>API keys</Trans>} detail={<Trans>Connect your servers and bots to Yuva</Trans>} />
            </NavRow>
            <NavRow to="/settings/webhooks" value={hooks.data ? hooks.data.length || t`None` : ""}>
              <RowIcon>
                <ArrowUpRightIcon />
              </RowIcon>
              <RowText title={<Trans>Webhooks</Trans>} detail={<Trans>Send events to your own server</Trans>} />
            </NavRow>
          </>
        )}
        <NavRow to="/settings/connected-apps">
          <RowIcon>
            <PlugIcon />
          </RowIcon>
          <RowText
            title={<Trans>Connected apps</Trans>}
            detail={<Trans>Connect an assistant such as Claude or ChatGPT</Trans>}
          />
        </NavRow>
      </Group>
      {canManage && (
        <Group title={<Trans>Integrations</Trans>}>
          <NavRow to="/settings/issue-trackers" value={trackers.data ? trackers.data.length || t`None` : ""}>
            <RowIcon>
              <CircleDotIcon />
            </RowIcon>
            <RowText
              title={<Trans>Issue trackers</Trans>}
              detail={<Trans>Turn feedback into GitHub or Linear issues</Trans>}
            />
          </NavRow>
          <NavRow to="/settings/slack" value={slack.data?.available ? (slack.data.connected ? t`On` : t`Off`) : ""}>
            <RowIcon>
              <MessageSquareIcon />
            </RowIcon>
            <RowText
              title={<Trans>Slack</Trans>}
              detail={<Trans>Post conversations to a channel and answer from it</Trans>}
            />
          </NavRow>
        </Group>
      )}
      <Group title={<Trans>Workspace</Trans>}>
        <NavRow to="/settings/workspace">
          <RowIcon>
            <BuildingIcon />
          </RowIcon>
          <RowText title={<Trans>General</Trans>} detail={membership.workspace.name} />
        </NavRow>
        {canManage && (
          <NavRow to="/settings/migrations" testId="settings-migrations">
            <RowIcon>
              <ArrowRightLeftIcon />
            </RowIcon>
            <RowText
              title={<Trans>Move to Yuva</Trans>}
              detail={<Trans>Bring a mailbox's history from Gmail, Yandex or any IMAP server</Trans>}
            />
          </NavRow>
        )}
        {isOwner && (
          <NavRow to="/settings/sso" testId="settings-sso">
            <RowIcon>
              <ShieldCheckIcon />
            </RowIcon>
            <RowText
              title={<Trans>Single sign-on</Trans>}
              detail={<Trans>Let your team sign in through your identity provider</Trans>}
            />
          </NavRow>
        )}
        {isOwner && (
          <NavRow to="/settings/audit-log" testId="settings-audit-log">
            <RowIcon>
              <ScrollTextIcon />
            </RowIcon>
            <RowText title={<Trans>Audit log</Trans>} detail={<Trans>Who changed access, keys and data</Trans>} />
          </NavRow>
        )}
      </Group>
    </Column>
  )
}
