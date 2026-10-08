import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PencilIcon } from "lucide-react"
import { useState } from "react"
import { Navigate } from "react-router"

import { BotAvatar, ErrorLine, SecretDialog, useConfirm } from "@/components/common"
import { API_KEY_SCOPES, formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { ApiKeyDialog } from "@/features/settings/api-keys/ApiKeyDialog"
import { Card, EmptyRow, PageHeader, Row, Rows, Section, StatusTag, ToggleRow } from "@/features/settings/ui"
import { api, unwrap, type ApiKey } from "@/lib/api"
import { keys } from "@/lib/keys"
import { meKey, useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMemberMap } from "@/lib/workspace"

function BotSending() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const workspace = useQuery({ queryKey: keys.workspace(ws), queryFn: () => unwrap(api.GET("/v1/workspace")) })
  const save = useMutation({
    mutationFn: (on: boolean) => unwrap(api.PATCH("/v1/workspace", { body: { bots_may_send: on } })),
    onSuccess: (w) => {
      qc.setQueryData(keys.workspace(ws), w)
      void qc.invalidateQueries({ queryKey: meKey })
    },
  })
  const on = save.isPending ? save.variables : (workspace.data?.bots_may_send ?? false)
  return (
    <Card>
      <ToggleRow
        title={<Trans>Bots may send</Trans>}
        hint={<Trans>When off, API keys can only write drafts, and a member reviews and sends them.</Trans>}
      >
        <Switch checked={on} disabled={save.isPending || !workspace.data} onCheckedChange={(v) => save.mutate(v)} aria-label={t`Bots may send`} data-testid="bots-may-send" />
      </ToggleRow>
      <ErrorLine error={workspace.error ?? save.error} />
    </Card>
  )
}

function KeyRow({ k, onEdit, onRevoke }: { k: ApiKey; onEdit: () => void; onRevoke: () => void }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const members = useMemberMap()
  const inboxes = useInboxes().data ?? []
  const created = formatDateTime(k.created_at, i18n.locale)
  const used = k.last_used_at ? formatDateTime(k.last_used_at, i18n.locale) : null
  const creator = k.created_by ? members.get(k.created_by) : undefined
  const by = creator ? creator.name || creator.email : null
  const bot = k.bot_name || k.name
  const expires = k.expires_at ? formatDateTime(k.expires_at, i18n.locale) : null
  const expired = !!k.expires_at && new Date(k.expires_at).getTime() <= Date.now()
  const allScopes = API_KEY_SCOPES.every((s) => k.scopes.includes(s))
  const inboxNames = k.inbox_ids?.map((id) => inboxes.find((i) => i.id === id)?.name ?? t`Deleted inbox`).join(", ")
  const inactive = !!k.revoked_at || expired
  const actions = k.revoked_at ? (
    <StatusTag className="h-7">
      <Trans>Revoked</Trans>
    </StatusTag>
  ) : (
    <div className="-my-1 flex shrink-0 items-center gap-1">
      <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={onEdit} data-testid="api-key-edit">
        <PencilIcon />
      </Button>
      <Button variant="ghost" size="sm" onClick={onRevoke}>
        <Trans>Revoke</Trans>
      </Button>
    </div>
  )
  return (
    <Row className={cn("items-start", inactive && "opacity-60")} data-testid="api-key-row">
      <BotAvatar name={bot} url={k.bot_avatar_url} className="mt-0.5 size-8" />
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex min-w-0 items-start justify-between gap-2">
          <p className="min-w-0 text-sm font-medium break-words">
            {k.name} <code className="font-mono text-xs font-normal text-faint">{k.prefix}…</code>
          </p>
          {actions}
        </div>
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
          <dt className="text-faint">
            <Trans>Bot</Trans>
          </dt>
          <dd className="truncate" data-testid="api-key-bot">
            {bot}
          </dd>
          <dt className="text-faint">
            <Trans>Scopes</Trans>
          </dt>
          <dd title={k.scopes.map((s) => text.scope[s]).join("\n")} data-testid="api-key-scopes">
            {allScopes ? (
              <Trans>All scopes</Trans>
            ) : (
              <span className="flex flex-wrap gap-x-2 font-mono">
                {k.scopes.map((s) => (
                  <span key={s} className="whitespace-nowrap">
                    {s}
                  </span>
                ))}
              </span>
            )}
          </dd>
          <dt className="text-faint">
            <Trans>Inboxes</Trans>
          </dt>
          <dd className="truncate" title={inboxNames} data-testid="api-key-inboxes">
            {inboxNames ? inboxNames : <Trans>Every inbox</Trans>}
          </dd>
          <dt className="text-faint">
            <Trans>Expires</Trans>
          </dt>
          <dd className={cn("truncate", expired && "text-destructive")} data-testid="api-key-expiry">
            {!expires ? <Trans>Never</Trans> : expired ? <Trans>Expired {expires}</Trans> : expires}
          </dd>
        </dl>
        <p className="text-xs text-faint">
          {by ? <Trans>Created {created} by {by}</Trans> : <Trans>Created {created}</Trans>}
          {" · "}
          {used ? <Trans>last used {used}</Trans> : <Trans>never used</Trans>}
        </p>
      </div>
    </Row>
  )
}

export function ApiKeysPage() {
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const [editing, setEditing] = useState<ApiKey | "new" | null>(null)
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({ queryKey: keys.apiKeys(ws), queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items), enabled: canManage })
  const revoke = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/api-keys/{apiKeyId}", { params: { path: { apiKeyId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.apiKeys(ws) }),
  })
  if (!canManage) return <Navigate to="/settings/profile" replace />
  return (
    <>
      <PageHeader
        title={<Trans>API keys</Trans>}
        description={
          <Trans>
            For your backends, scripts and bots. A key acts on this workspace only, within its scopes, and cannot manage
            members or keys. Its messages show its bot name.
          </Trans>
        }
        action={
          <Button size="sm" onClick={() => setEditing("new")} data-testid="api-key-new">
            <Trans>New key</Trans>
          </Button>
        }
      />
      <BotSending />
      <Section title={<Trans>Keys</Trans>}>
        <Card flush>
          {list.data && list.data.length === 0 ? (
            <EmptyRow>
              <Trans>No API keys yet.</Trans>
            </EmptyRow>
          ) : (
            <Rows>
              {(list.data ?? []).map((k) => (
                <KeyRow
                  key={k.id}
                  k={k}
                  onEdit={() => setEditing(k)}
                  onRevoke={() => {
                    const keyName = k.name
                    confirm({
                      title: <Trans>Revoke {keyName}?</Trans>,
                      description: <Trans>Anything using this key stops working at once.</Trans>,
                      confirm: <Trans>Revoke</Trans>,
                      run: () => revoke.mutate(k.id),
                    })
                  }}
                />
              ))}
            </Rows>
          )}
          <ErrorLine error={list.error ?? revoke.error} className="px-5 pb-4" />
        </Card>
      </Section>
      {editing && (
        <ApiKeyDialog
          apiKey={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onCreated={(s) => {
            setEditing(null)
            setSecret(s)
          }}
        />
      )}
      <SecretDialog
        secret={secret}
        title={<Trans>Your new API key</Trans>}
        description={<Trans>Copy it now and store it safely; it is not shown again.</Trans>}
        onClose={() => setSecret(null)}
      />
      {confirmDialog}
    </>
  )
}
