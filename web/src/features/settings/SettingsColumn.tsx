import { Trans, useLingui } from "@lingui/react/macro"
import { useQueries, useQuery } from "@tanstack/react-query"
import { ArrowUpRightIcon, BellIcon, BuildingIcon, KeyRoundIcon, PaletteIcon, PlugIcon, PlusIcon, QuoteIcon, SettingsIcon, TagIcon, UsersIcon } from "lucide-react"
import { Outlet } from "react-router"

import { Column, ColumnHeading, ColumnLink, Pane, PaneEmpty } from "@/components/common/Column"
import { initials, useEnumText } from "@/components/common/text"
import { RowIcon, RowText } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useCannedReplies, useInboxes, useLabels, useMembers } from "@/lib/workspace"

export function SettingsPane() {
  return (
    <Pane testId="settings">
      <Outlet />
    </Pane>
  )
}

export function SettingsHome() {
  return (
    <PaneEmpty icon={SettingsIcon} title={<Trans>Settings</Trans>} testId="settings-home">
      <Trans>Pick a setting on the left.</Trans>
    </PaneEmpty>
  )
}

function Group({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <>
      <ColumnHeading>{title}</ColumnHeading>
      <ul className="flex flex-col">{children}</ul>
    </>
  )
}

function NavRow({ to, state, children, value, testId }: { to: string; state?: unknown; children: React.ReactNode; value?: React.ReactNode; testId?: string }) {
  return (
    <li>
      <ColumnLink to={to} state={state} testId={testId}>
        {children}
        {value !== undefined && value !== "" && <span className="shrink-0 text-small whitespace-nowrap text-faint">{value}</span>}
      </ColumnLink>
    </li>
  )
}

function useInboxChannels() {
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data ?? []
  const results = useQueries({
    queries: inboxes.map((i) => ({
      queryKey: keys.channels(ws, i.id),
      queryFn: () => unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: i.id } } })).then((r) => r.items),
      staleTime: 5 * 60_000,
    })),
  })
  return inboxes.map((inbox, i) => ({ inbox, channels: results[i]?.data }))
}

export function SettingsColumn() {
  const { t } = useLingui()
  const text = useEnumText()
  const { me, membership, canManage, workspaceId: ws } = useSession()
  const inboxes = useInboxChannels()
  const members = useMembers().data
  const canned = useCannedReplies().data
  const labels = useLabels().data
  const apiKeys = useQuery({ queryKey: keys.apiKeys(ws), queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items), enabled: canManage })
  const hooks = useQuery({ queryKey: keys.webhooks(ws), queryFn: () => unwrap(api.GET("/v1/webhooks")).then((r) => r.items), enabled: canManage })
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
      <Group title={<Trans>Inboxes</Trans>}>
        {inboxes.map(({ inbox, channels }) => (
          <NavRow key={inbox.id} to={`/settings/inboxes/${inbox.id}`} testId="inbox-row">
            <RowIcon>
              <span style={{ color: inbox.branding.color }}>
                {inbox.name.charAt(0).toLocaleUpperCase()}
              </span>
            </RowIcon>
            <RowText
              title={inbox.name}
              detail={channels ? [...new Set(channels.map((c) => text.channel[c.kind]))].join(", ") || t`No channels yet` : " "}
            />
          </NavRow>
        ))}
        {canManage && (
          <NavRow to="/setup" state={{ back: "/settings" }} testId="add-inbox">
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
          <RowText title={<Trans>Members</Trans>} detail={(members ?? []).map((m) => (m.name || m.email).split(" ")[0]).join(", ")} />
        </NavRow>
        <NavRow to="/settings/canned-replies" value={list(canned?.length)}>
          <RowIcon>
            <QuoteIcon />
          </RowIcon>
          <RowText title={<Trans>Canned replies</Trans>} detail={<Trans>What you write often</Trans>} />
        </NavRow>
        <NavRow to="/settings/labels" value={list(labels?.length)}>
          <RowIcon>
            <TagIcon />
          </RowIcon>
          <RowText title={<Trans>Labels</Trans>} detail={<Trans>Sort conversations your way</Trans>} />
        </NavRow>
        <NavRow to="/settings/connected-apps">
          <RowIcon>
            <PlugIcon />
          </RowIcon>
          <RowText title={<Trans>Connected apps</Trans>} detail={<Trans>Connect an assistant such as Claude or ChatGPT</Trans>} />
        </NavRow>
      </Group>
      {canManage && (
        <Group title={<Trans>Developer</Trans>}>
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
        </Group>
      )}
      <Group title={<Trans>Workspace</Trans>}>
        <NavRow to="/settings/workspace">
          <RowIcon>
            <BuildingIcon />
          </RowIcon>
          <RowText title={membership.workspace.name} detail={<Trans>Data retention and deleting the workspace</Trans>} />
        </NavRow>
      </Group>
    </Column>
  )
}
