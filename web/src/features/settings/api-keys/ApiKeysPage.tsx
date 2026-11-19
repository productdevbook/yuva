import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { Navigate } from "react-router"

import { ErrorLine, useConfirm } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import {
  API_KEY_PRESET_SCOPES,
  formatDate,
  formatRelative,
  formatShort,
  WORKSPACE_WIDE_SCOPES,
  type ApiKeyPreset,
} from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { ApiKeyDialog } from "@/features/settings/api-keys/ApiKeyDialog"
import { ApiKeyForm, ApiKeySecret } from "@/features/settings/api-keys/ApiKeyForm"
import { useWorkspaceSettings } from "@/features/settings/connected-apps/queries"
import { DeveloperHeader } from "@/features/settings/DeveloperTabs"
import { Card, headButton, ProductTag, tableHeadClass, ToggleRow } from "@/features/settings/ui"
import { api, unwrap, type ApiKey } from "@/lib/api"
import { keys } from "@/lib/keys"
import { meKey, useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMemberMap } from "@/lib/workspace"

function BotSending() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const workspace = useWorkspaceSettings()
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
        hint={
          on ? (
            <Trans>API keys and assistants deliver replies at once, without a member reviewing them first.</Trans>
          ) : (
            <Trans>When off, API keys can only write drafts, and a member reviews and sends them.</Trans>
          )
        }
      >
        <Switch
          checked={on}
          disabled={save.isPending || !workspace.data}
          onCheckedChange={(v) =>
            v
              ? confirm({
                  title: <Trans>Let bots send replies?</Trans>,
                  description: (
                    <Trans>
                      API keys and assistants will deliver replies at once. Customers receive what they write, with
                      nobody reviewing it first. You can turn this off again at any time.
                    </Trans>
                  ),
                  confirm: <Trans>Turn on</Trans>,
                  run: () => save.mutate(true),
                })
              : save.mutate(false)
          }
          aria-label={t`Bots may send`}
          data-testid="bots-may-send"
        />
      </ToggleRow>
      <ErrorLine error={workspace.error ?? save.error} />
      {confirmDialog}
    </Card>
  )
}

const KEY_GRID =
  "grid grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_minmax(0,1.3fr)_140px_90px_120px] items-center gap-4 px-4 phone:grid-cols-[minmax(0,1fr)_auto] phone:gap-x-3 phone:gap-y-1"

function usePreset() {
  const { t } = useLingui()
  return (k: ApiKey): { label: string; preset: ApiKeyPreset } => {
    const limited = (k.inbox_ids?.length ?? 0) > 0
    const mine = new Set(k.scopes)
    const presets: [Exclude<ApiKeyPreset, "custom">, string][] = [
      ["read", t`Read only`],
      ["support", t`Support bot`],
      ["everything", t`Everything`],
    ]
    for (const [id, label] of presets) {
      const want = API_KEY_PRESET_SCOPES[id].filter((s) => !limited || !WORKSPACE_WIDE_SCOPES.includes(s))
      if (want.length === mine.size && want.every((s) => mine.has(s))) return { label, preset: id }
    }
    return { label: t`Custom`, preset: "custom" }
  }
}

function KeyRow({ k, onEdit, onRevoke }: { k: ApiKey; onEdit: () => void; onRevoke: () => void }) {
  const [now] = useState(Date.now)
  const { t, i18n } = useLingui()
  const members = useMemberMap()
  const inboxes = useInboxes().data ?? []
  const preset = usePreset()(k)
  const created = formatShort(k.created_at, i18n.locale)
  const creator = k.created_by ? members.get(k.created_by) : undefined
  const by = creator ? creator.name || creator.email : null
  const bot = k.bot_name || k.name
  const expires = k.expires_at ? formatDate(k.expires_at, i18n.locale) : null
  const expired = !!k.expires_at && new Date(k.expires_at).getTime() <= now
  const picked = (k.inbox_ids ?? []).map((id) => inboxes.find((i) => i.id === id))
  const inactive = !!k.revoked_at || expired
  const keyName = k.name
  const scopeList = k.scopes.join("\n")
  const count = k.scopes.length
  return (
    <li className={cn(KEY_GRID, "border-t py-3")} data-testid="api-key-row">
      <span className={cn("flex min-w-0 flex-col leading-[1.3]", inactive && "opacity-60")} title={bot}>
        <span className="truncate font-medium">{k.name}</span>
        <code className="truncate font-mono text-caption text-faint">{k.prefix}…</code>
        <span className="sr-only" data-testid="api-key-bot">
          {bot}
        </span>
      </span>
      <span className="contents phone:col-span-2 phone:row-start-2 phone:flex phone:flex-wrap phone:items-center phone:gap-x-3 phone:gap-y-1 phone:text-caption">
        <span
          className={cn("flex min-w-0 flex-col leading-[1.3] phone:flex-row phone:gap-1.5", inactive && "opacity-60")}
          title={scopeList}
          data-testid="api-key-scopes"
        >
          <span className="truncate">{preset.label}</span>
          <span className="truncate text-caption text-faint">
            <Plural value={count} one="# scope" other="# scopes" />
          </span>
        </span>
        <span className={cn("flex min-w-0 flex-wrap gap-1", inactive && "opacity-60")} data-testid="api-key-inboxes">
          {picked.length === 0 ? (
            <span className="truncate text-muted-foreground">
              <Trans>All products</Trans>
            </span>
          ) : (
            picked.map((i, n) => (
              <ProductTag key={i?.id ?? n} name={i?.name ?? t`Deleted inbox`} color={i?.branding.color} />
            ))
          )}
        </span>
        <span className="truncate phone:text-caption phone:text-muted-foreground">
          {k.revoked_at ? (
            <span className="text-faint">
              <Trans>Revoked</Trans>
            </span>
          ) : k.last_used_at ? (
            formatRelative(k.last_used_at, i18n.locale, now)
          ) : (
            <span className="text-faint">
              <Trans>never</Trans>
            </span>
          )}
        </span>
        <span className="flex min-w-0 flex-col text-caption leading-[1.3] text-faint" title={by ?? undefined}>
          <span className="truncate">{created}</span>
          {expires && (
            <span className={cn("truncate", expired && "text-destructive")} data-testid="api-key-expiry">
              {expired ? <Trans>expired {expires}</Trans> : <Trans>until {expires}</Trans>}
            </span>
          )}
        </span>
      </span>
      <span className="flex justify-end gap-1 phone:col-start-2 phone:row-start-1">
        {!k.revoked_at && (
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={onEdit}
              aria-label={t`Edit ${keyName}`}
              data-testid="api-key-edit"
            >
              <Trans>Edit</Trans>
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={onRevoke}
              aria-label={t`Revoke ${keyName}`}
              data-testid="api-key-revoke"
            >
              <Trans>Revoke</Trans>
            </Button>
          </>
        )}
      </span>
    </li>
  )
}

export function ApiKeysPage() {
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const [editing, setEditing] = useState<ApiKey | null>(null)
  const [adding, setAdding] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)
  const [showRevoked, setShowRevoked] = useState(false)
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({
    queryKey: keys.apiKeys(ws),
    queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items),
    enabled: canManage,
  })
  const revoke = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/api-keys/{apiKeyId}", { params: { path: { apiKeyId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.apiKeys(ws) }),
  })
  if (!canManage) return <Navigate to="/settings/profile" replace />
  const all = list.data ?? []
  const revoked = all.filter((k) => k.revoked_at)
  const shown = showRevoked ? all : all.filter((k) => !k.revoked_at)
  const nRevoked = revoked.length
  return (
    <>
      <DeveloperHeader
        current="api-keys"
        onCreate={() => {
          setSecret(null)
          setAdding(true)
        }}
        creating={adding}
      />
      {adding && (
        <ApiKeyForm
          onClose={() => setAdding(false)}
          onCreated={(s) => {
            setAdding(false)
            setSecret(s)
          }}
        />
      )}
      {secret && <ApiKeySecret secret={secret} onDone={() => setSecret(null)} />}
      <BotSending />
      {list.data && all.length === 0 ? (
        <EmptyBox
          title={<Trans>No API keys yet</Trans>}
          testId="api-keys-empty"
          actions={
            !adding && (
              <Button
                className={headButton.primary}
                onClick={() => {
                  setSecret(null)
                  setAdding(true)
                }}
              >
                <Trans>New API key</Trans>
              </Button>
            )
          }
        >
          <Trans>
            A key lets your backend, a script or a bot read and write conversations. Limit it to the products and scopes
            it needs.
          </Trans>
        </EmptyBox>
      ) : (
        <section className="overflow-x-auto rounded-xl border bg-card">
          <div className="min-w-[860px] phone:min-w-0">
            <div className={cn(KEY_GRID, tableHeadClass, "py-2.5 phone:hidden")}>
              <span>
                <Trans>Key</Trans>
              </span>
              <span>
                <Trans>Access</Trans>
              </span>
              <span>
                <Trans>Products</Trans>
              </span>
              <span>
                <Trans>Last used</Trans>
              </span>
              <span>
                <Trans>Created</Trans>
              </span>
              <span />
            </div>
            <ul>
              {shown.map((k) => (
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
            </ul>
            <div className="flex items-center gap-3 border-t px-4 py-2.5 text-caption text-faint">
              {nRevoked > 0 && (
                <button
                  type="button"
                  className="whitespace-nowrap text-muted-foreground outline-none hover:text-foreground focus-visible:underline"
                  onClick={() => setShowRevoked(!showRevoked)}
                  data-testid="api-keys-revoked"
                >
                  {showRevoked ? <Trans>Hide revoked</Trans> : <Trans>Show {nRevoked} revoked</Trans>}
                </button>
              )}
              <span className="ms-auto">
                <Trans>The secret is shown once, when a key is made.</Trans>
              </span>
            </div>
            <ErrorLine error={list.error ?? revoke.error} className="px-4 pb-4" />
          </div>
        </section>
      )}
      {editing && <ApiKeyDialog apiKey={editing} onClose={() => setEditing(null)} />}
      {confirmDialog}
    </>
  )
}
