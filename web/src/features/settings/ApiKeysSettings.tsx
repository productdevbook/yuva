import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { KeyIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { Navigate } from "react-router"

import { ErrorLine, SecretDialog, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { PageTitle, Section } from "@/features/settings/SettingsLayout"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useMemberMap } from "@/lib/workspace"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function ApiKeysSettings() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const members = useMemberMap()
  const [name, setName] = useState("")
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({
    queryKey: keys.apiKeys(ws),
    queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items),
    enabled: canManage,
  })
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
      <PageTitle>
        <Trans>API keys</Trans>
      </PageTitle>
      <Section
        title={<Trans>Workspace API keys</Trans>}
        description={
          <Trans>
            For your backends and scripts. A key acts on this workspace only and cannot manage members or keys.
          </Trans>
        }
      >
        <form
          className="flex flex-col gap-2 sm:flex-row"
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
          <Button type="submit" disabled={create.isPending}>
            <PlusIcon />
            <Trans>Create key</Trans>
          </Button>
        </form>
        <ErrorLine error={list.error ?? create.error ?? revoke.error} />
        {list.data && list.data.length > 0 && (
          <ul className="flex flex-col divide-y rounded-lg border">
            {list.data.map((k) => {
              const created = formatDateTime(k.created_at, i18n.locale)
              const used = k.last_used_at ? formatDateTime(k.last_used_at, i18n.locale) : null
              const creator = k.created_by ? members.get(k.created_by) : undefined
              const by = creator ? creator.name || creator.email : null
              return (
                <li
                  key={k.id}
                  className={cn("flex flex-wrap items-center gap-3 px-3 py-2.5", k.revoked_at && "opacity-60")}
                  data-testid="api-key-row"
                >
                  <KeyIcon className="size-4 shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <p className="flex items-center gap-2 text-sm font-medium">
                      <span className="truncate">{k.name}</span>
                      <code className="rounded bg-muted px-1 text-xs font-normal">{k.prefix}…</code>
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {by ? <Trans>Created {created} by {by}</Trans> : <Trans>Created {created}</Trans>}
                      {" · "}
                      {used ? <Trans>last used {used}</Trans> : <Trans>never used</Trans>}
                    </p>
                  </div>
                  {k.revoked_at ? (
                    <Badge variant="outline">
                      <Trans>Revoked</Trans>
                    </Badge>
                  ) : (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        const keyName = k.name
                        confirm({
                          title: <Trans>Revoke {keyName}?</Trans>,
                          description: <Trans>Anything using this key stops working at once.</Trans>,
                          confirm: <Trans>Revoke</Trans>,
                          run: () => revoke.mutate(k.id),
                        })
                      }}
                    >
                      <Trans>Revoke</Trans>
                    </Button>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </Section>
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
