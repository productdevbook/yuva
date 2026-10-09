import { Trans, useLingui } from "@lingui/react/macro"
import { useQueries, useQuery } from "@tanstack/react-query"
import { ArrowUpRightIcon, BellIcon, BuildingIcon, KeyRoundIcon, PaletteIcon, PlugIcon, PlusIcon, QuoteIcon, TagIcon, UsersIcon } from "lucide-react"
import { Outlet } from "react-router"

import { initials, useEnumText } from "@/components/common/text"
import { Card, LinkRow, PageHeader, RowIcon, Rows, RowText, Section } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useCannedReplies, useInboxes, useLabels, useMembers } from "@/lib/workspace"

export function SettingsLayout() {
  return (
    <main className="mx-auto flex w-full max-w-[640px] flex-col gap-8 px-6 pt-6 pb-24 phone:px-4 phone:pt-4" data-testid="settings">
      <Outlet />
    </main>
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

export function SettingsIndex() {
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
    <>
      <PageHeader title={<Trans>Settings</Trans>} back={false} />
      <Section title={<Trans>You</Trans>}>
        <Card flush>
          <Rows>
            <LinkRow to="profile" testId="settings-profile">
              <RowIcon>{initials(name)}</RowIcon>
              <RowText title={name} detail={me.person.email} />
            </LinkRow>
            <LinkRow to="notifications">
              <RowIcon>
                <BellIcon />
              </RowIcon>
              <RowText title={<Trans>Notifications</Trans>} detail={<Trans>When we let you know</Trans>} />
            </LinkRow>
            <LinkRow to="appearance" testId="settings-appearance">
              <RowIcon>
                <PaletteIcon />
              </RowIcon>
              <RowText title={<Trans>Appearance</Trans>} detail={<Trans>Theme and e-mail view on this device</Trans>} />
            </LinkRow>
          </Rows>
        </Card>
      </Section>
      <Section title={<Trans>Inboxes</Trans>}>
        <Card flush>
          <Rows>
            {inboxes.map(({ inbox, channels }) => (
              <LinkRow key={inbox.id} to={`inboxes/${inbox.id}`} testId="inbox-row">
                <RowIcon>
                  <span style={{ color: inbox.branding.color }}>
                    {inbox.name.charAt(0).toLocaleUpperCase()}
                  </span>
                </RowIcon>
                <RowText
                  title={inbox.name}
                  detail={channels ? [...new Set(channels.map((c) => text.channel[c.kind]))].join(", ") || t`No channels yet` : " "}
                />
              </LinkRow>
            ))}
            {canManage && (
              <LinkRow to="new" testId="add-inbox">
                <RowIcon>
                  <PlusIcon />
                </RowIcon>
                <RowText title={<Trans>Add an inbox</Trans>} detail={<Trans>One per product or brand</Trans>} />
              </LinkRow>
            )}
          </Rows>
        </Card>
      </Section>
      <Section title={<Trans>Team</Trans>}>
        <Card flush>
          <Rows>
            <LinkRow to="members" value={list(members?.length)}>
              <RowIcon>
                <UsersIcon />
              </RowIcon>
              <RowText title={<Trans>Members</Trans>} detail={(members ?? []).map((m) => (m.name || m.email).split(" ")[0]).join(", ")} />
            </LinkRow>
            <LinkRow to="canned-replies" value={list(canned?.length)}>
              <RowIcon>
                <QuoteIcon />
              </RowIcon>
              <RowText title={<Trans>Canned replies</Trans>} detail={<Trans>What you write often</Trans>} />
            </LinkRow>
            <LinkRow to="labels" value={list(labels?.length)}>
              <RowIcon>
                <TagIcon />
              </RowIcon>
              <RowText title={<Trans>Labels</Trans>} detail={<Trans>Sort conversations your way</Trans>} />
            </LinkRow>
            <LinkRow to="connected-apps">
              <RowIcon>
                <PlugIcon />
              </RowIcon>
              <RowText title={<Trans>Connected apps</Trans>} />
            </LinkRow>
          </Rows>
        </Card>
      </Section>
      {canManage && (
        <Section title={<Trans>Developer</Trans>}>
          <Card flush>
            <Rows>
              <LinkRow to="api-keys" value={list(apiKeys.data?.length)}>
                <RowIcon>
                  <KeyRoundIcon />
                </RowIcon>
                <RowText title={<Trans>API keys</Trans>} detail={<Trans>Connect your servers and bots to Yuva</Trans>} />
              </LinkRow>
              <LinkRow to="webhooks" value={hooks.data ? hooks.data.length || t`None` : ""}>
                <RowIcon>
                  <ArrowUpRightIcon />
                </RowIcon>
                <RowText title={<Trans>Webhooks</Trans>} detail={<Trans>Send events to your own server</Trans>} />
              </LinkRow>
            </Rows>
          </Card>
        </Section>
      )}
      <Section title={<Trans>Workspace</Trans>}>
        <Card flush>
          <Rows>
            <LinkRow to="workspace">
              <RowIcon>
                <BuildingIcon />
              </RowIcon>
              <RowText title={membership.workspace.name} detail={<Trans>Data retention and deleting the workspace</Trans>} />
            </LinkRow>
          </Rows>
        </Card>
      </Section>
    </>
  )
}
