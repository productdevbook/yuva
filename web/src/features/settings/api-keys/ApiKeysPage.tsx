import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { Navigate } from "react-router"

import { ErrorLine, SecretDialog, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, EmptyRow, PageHeader, Row, Rows, RowText, StatusTag } from "@/features/settings/ui"
import { api, unwrap, type ApiKey } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useMemberMap } from "@/lib/workspace"

function KeyRow({ k, onRevoke }: { k: ApiKey; onRevoke: () => void }) {
  const { i18n } = useLingui()
  const members = useMemberMap()
  const created = formatDateTime(k.created_at, i18n.locale)
  const used = k.last_used_at ? formatDateTime(k.last_used_at, i18n.locale) : null
  const creator = k.created_by ? members.get(k.created_by) : undefined
  const by = creator ? creator.name || creator.email : null
  return (
    <Row className={cn("flex-wrap", k.revoked_at && "opacity-60")} data-testid="api-key-row">
      <RowText
        title={
          <>
            {k.name} <code className="ms-1 font-mono text-xs font-normal text-faint">{k.prefix}…</code>
          </>
        }
        detail={
          <>
            {by ? <Trans>Created {created} by {by}</Trans> : <Trans>Created {created}</Trans>}
            {" · "}
            {used ? <Trans>last used {used}</Trans> : <Trans>never used</Trans>}
          </>
        }
      />
      {k.revoked_at ? (
        <StatusTag>
          <Trans>Revoked</Trans>
        </StatusTag>
      ) : (
        <Button variant="ghost" size="sm" onClick={onRevoke}>
          <Trans>Revoke</Trans>
        </Button>
      )}
    </Row>
  )
}

export function ApiKeysPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const [name, setName] = useState("")
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({ queryKey: keys.apiKeys(ws), queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items), enabled: canManage })
  const refresh = () => qc.invalidateQueries({ queryKey: keys.apiKeys(ws) })
  const create = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/api-keys", { body: { name } })),
    onSuccess: (r) => {
      setName("")
      setSecret(r.secret)
      void refresh()
    },
  })
  const revoke = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/api-keys/{apiKeyId}", { params: { path: { apiKeyId: id } } })),
    onSuccess: refresh,
  })
  if (!canManage) return <Navigate to="/settings/profile" replace />
  return (
    <>
      <PageHeader
        title={<Trans>API keys</Trans>}
        description={<Trans>For your backends and scripts. A key acts on this workspace only and cannot manage members or keys.</Trans>}
      />
      <Card
        flush
        footer={
          <form
            className="flex w-full flex-col gap-2 sm:flex-row"
            onSubmit={(e) => {
              e.preventDefault()
              create.mutate()
            }}
          >
            <Input
              required
              maxLength={200}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t`Name, e.g. Production backend`}
              aria-label={t`Key name`}
              className="sm:max-w-72"
            />
            <Button type="submit" variant="outline" disabled={create.isPending}>
              <Trans>Create key</Trans>
            </Button>
          </form>
        }
      >
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
        <ErrorLine error={list.error ?? create.error ?? revoke.error} className="px-5 pb-4" />
      </Card>
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
