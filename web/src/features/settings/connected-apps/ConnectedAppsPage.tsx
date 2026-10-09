import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlugIcon } from "lucide-react"

import { ErrorLine, useConfirm } from "@/components/common"
import { API_KEY_SCOPES, formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Card, EmptyRow, PageHeader, Row, Rows, Section } from "@/features/settings/ui"
import { api, unwrap, type OAuthGrant } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useMemberMap } from "@/lib/workspace"

function GrantRow({ g, showMember, onRevoke }: { g: OAuthGrant; showMember: boolean; onRevoke: () => void }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const members = useMemberMap()
  const { membership } = useSession()
  const created = formatDateTime(g.created_at, i18n.locale)
  const used = g.last_used_at ? formatDateTime(g.last_used_at, i18n.locale) : null
  const member = members.get(g.member_id)
  const memberName = g.member_id === membership.member_id ? t`You` : member ? member.name || member.email : t`Deleted member`
  const allScopes = API_KEY_SCOPES.every((s) => g.scopes.includes(s))
  return (
    <Row className="items-start" data-testid="grant-row">
      <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-full border text-faint">
        <PlugIcon className="size-4" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex min-w-0 items-start justify-between gap-2">
          <p className="min-w-0 text-body font-medium break-words" data-testid="grant-client">
            {g.client.name}
          </p>
          <Button variant="ghost" size="sm" className="-my-1" onClick={onRevoke} data-testid="grant-revoke">
            <Trans>Revoke</Trans>
          </Button>
        </div>
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-0.5 text-caption text-muted-foreground">
          {showMember && (
            <>
              <dt className="text-faint">
                <Trans>Member</Trans>
              </dt>
              <dd className="truncate" data-testid="grant-member">
                {memberName}
              </dd>
            </>
          )}
          <dt className="text-faint">
            <Trans>Access</Trans>
          </dt>
          <dd className="truncate" data-testid="grant-resource">
            {g.resource === "mcp" ? <Trans>MCP only</Trans> : <Trans>Full API</Trans>}
          </dd>
          <dt className="text-faint">
            <Trans>Scopes</Trans>
          </dt>
          <dd title={g.scopes.map((s) => text.scope[s]).join("\n")} data-testid="grant-scopes">
            {allScopes ? (
              <Trans>All scopes</Trans>
            ) : (
              <span className="flex flex-wrap gap-x-2 font-mono">
                {g.scopes.map((s) => (
                  <span key={s} className="whitespace-nowrap">
                    {s}
                  </span>
                ))}
              </span>
            )}
          </dd>
          <dt className="text-faint">
            <Trans>This month</Trans>
          </dt>
          <dd className="truncate" data-testid="grant-requests">
            <Plural value={g.requests_this_month} one="# request" other="# requests" />
          </dd>
        </dl>
        <p className="text-caption text-faint">
          <Trans>Connected {created}</Trans>
          {" · "}
          {used ? <Trans>last used {used}</Trans> : <Trans>never used</Trans>}
        </p>
      </div>
    </Row>
  )
}

export function ConnectedAppsPage() {
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({ queryKey: keys.oauthGrants(ws), queryFn: () => unwrap(api.GET("/v1/oauth/grants")).then((r) => r.items) })
  const revoke = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/oauth/grants/{oauthGrantId}", { params: { path: { oauthGrantId: id } } })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.oauthGrants(ws) }),
  })
  return (
    <>
      <PageHeader
        title={<Trans>Connected apps</Trans>}
        description={
          canManage ? (
            <Trans>
              Assistants and apps that members connected with their own sign-in. Each acts as that member, only within the
              access they allowed. As an owner or admin you see and can revoke every member's connections.
            </Trans>
          ) : (
            <Trans>
              Assistants and apps you connected with your sign-in. Each acts as you, only within the access you allowed.
            </Trans>
          )
        }
      />
      <Section title={<Trans>Connections</Trans>}>
        <Card flush>
          {list.data && list.data.length === 0 ? (
            <EmptyRow>
              <Trans>No connected apps.</Trans>
            </EmptyRow>
          ) : (
            <Rows>
              {(list.data ?? []).map((g) => (
                <GrantRow
                  key={g.id}
                  g={g}
                  showMember={canManage}
                  onRevoke={() => {
                    const clientName = g.client.name
                    confirm({
                      title: <Trans>Revoke {clientName}?</Trans>,
                      description: <Trans>Its access ends at once. To use it again, connect it again.</Trans>,
                      confirm: <Trans>Revoke</Trans>,
                      run: () => revoke.mutate(g.id),
                    })
                  }}
                />
              ))}
            </Rows>
          )}
          <ErrorLine error={list.error ?? revoke.error} className="px-5 pb-4" />
        </Card>
      </Section>
      {confirmDialog}
    </>
  )
}
