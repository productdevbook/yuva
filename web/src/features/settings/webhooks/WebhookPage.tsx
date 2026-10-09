import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { Navigate, useLocation, useNavigate, useParams } from "react-router"

import { ErrorLine, Notice, SecretDialog, useConfirm } from "@/components/common"
import { formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Card, PageHeader, ToggleRow } from "@/features/settings/ui"
import { AttemptLog } from "@/features/settings/webhooks/AttemptLog"
import { Deliveries } from "@/features/settings/webhooks/Deliveries"
import { EndpointStatus } from "@/features/settings/webhooks/parts"
import { useSetEndpoint, useWebhook } from "@/features/settings/webhooks/queries"
import { SignatureHint } from "@/features/settings/webhooks/SignatureHint"
import { WebhookDialog } from "@/features/settings/webhooks/WebhookDialog"
import { api, unwrap, type WebhookEndpoint } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

function EndpointCard({ e }: { e: WebhookEndpoint }) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const text = useEnumText()
  const { workspaceId: ws } = useSession()
  const setEndpoint = useSetEndpoint()
  const [editing, setEditing] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const params = { params: { path: { webhookId: e.id } } }
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => unwrap(api.PATCH("/v1/webhooks/{webhookId}", { ...params, body: { enabled } })),
    onSuccess: setEndpoint,
  })
  const rotate = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/webhooks/{webhookId}/secret", params)),
    onSuccess: (r) => {
      setEndpoint(r.endpoint)
      setSecret(r.secret)
    },
  })
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/webhooks/{webhookId}", params)),
    onSuccess: () => {
      qc.removeQueries({ queryKey: keys.webhook(ws, e.id) })
      void qc.invalidateQueries({ queryKey: keys.webhooks(ws) })
      navigate("/settings/webhooks")
    },
  })
  const disabledAt = e.disabled_at ? formatDateTime(e.disabled_at, i18n.locale) : null
  const failingSince = e.failing_since ? formatDateTime(e.failing_since, i18n.locale) : ""
  const rotatedUntil = e.secret_rotated_at
    ? formatDateTime(new Date(new Date(e.secret_rotated_at).getTime() + 24 * 3600_000).toISOString(), i18n.locale)
    : null
  return (
    <Card
      footer={
        <>
          <ErrorLine error={toggle.error ?? rotate.error ?? remove.error} className="min-w-0 flex-1" />
          <div className="ms-auto flex flex-wrap gap-2">
            <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" disabled={remove.isPending} onClick={() =>
              confirm({
                title: <Trans>Remove this endpoint?</Trans>,
                description: <Trans>Pending deliveries are dropped with it.</Trans>,
                confirm: <Trans>Remove</Trans>,
                run: () => remove.mutate(),
              })
            }>
              <Trans>Remove</Trans>
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={rotate.isPending}
              data-testid="webhook-rotate"
              onClick={() =>
                confirm({
                  title: <Trans>Rotate the signing secret?</Trans>,
                  description: (
                    <Trans>
                      You get a new secret once. For 24 hours every delivery carries two signatures, one with the new and
                      one with the old secret, so you can update your receiver without losing events.
                    </Trans>
                  ),
                  confirm: <Trans>Rotate</Trans>,
                  run: () => rotate.mutate(),
                })
              }
            >
              <Trans>Rotate secret</Trans>
            </Button>
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              <Trans>Edit</Trans>
            </Button>
          </div>
        </>
      }
    >
      {!e.enabled && e.disabled_reason && (
        <Notice tone="danger" data-testid="webhook-disabled">
          <p className="font-medium">
            {disabledAt ? <Trans>Yuva turned this endpoint off on {disabledAt}.</Trans> : <Trans>Yuva turned this endpoint off.</Trans>}
          </p>
          <p className="break-words text-muted-foreground">{e.disabled_reason}</p>
          <p className="text-muted-foreground">
            <Trans>Turn it on again once the receiver works; send failed deliveries again from the log below.</Trans>
          </p>
        </Notice>
      )}
      {e.enabled && e.failing_since && (
        <Notice tone="warning" data-testid="webhook-failing">
          <p>
            <Trans>Every attempt has failed since {failingSince}. After 24 hours of failures Yuva turns the endpoint off.</Trans>
          </p>
        </Notice>
      )}
      <ToggleRow title={<Trans>Send events</Trans>} hint={<Trans>While off, no deliveries are made.</Trans>}>
        <Switch checked={e.enabled} disabled={toggle.isPending} onCheckedChange={(on) => toggle.mutate(on)} aria-label={t`Send events`} data-testid="webhook-enabled" />
      </ToggleRow>
      <div className="flex flex-col gap-2">
        <p className="text-body font-medium">
          <Trans>Events</Trans>
        </p>
        <ul className="flex flex-wrap gap-1.5">
          {e.events.map((ev) => (
            <li key={ev} className="rounded-full bg-muted px-2.5 py-0.5 font-mono text-caption" title={text.event[ev]}>
              {ev}
            </li>
          ))}
          {e.include_notes && (
            <li className="rounded-full border px-2.5 py-0.5 text-caption text-muted-foreground">
              <Trans>with notes</Trans>
            </li>
          )}
        </ul>
      </div>
      {rotatedUntil && (
        <p className="text-caption text-muted-foreground" data-testid="webhook-rotated">
          <Trans>The previous secret also signs until {rotatedUntil}.</Trans>
        </p>
      )}
      {editing && <WebhookDialog endpoint={e} inboxId={e.inbox_id} onClose={() => setEditing(false)} />}
      <SecretDialog
        secret={secret}
        title={<Trans>New signing secret</Trans>}
        description={<Trans>Copy it now and update your receiver; the old secret keeps signing for 24 hours. It is not shown again.</Trans>}
        onClose={() => setSecret(null)}
      />
      {confirmDialog}
    </Card>
  )
}

export function WebhookPage() {
  const { t } = useLingui()
  const { webhookId = "" } = useParams()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const location = useLocation()
  const inboxes = useInboxes().data ?? []
  const endpoint = useWebhook(webhookId)
  const created = (location.state as { secret?: string } | null)?.secret ?? null
  if (!canManage) return <Navigate to="/settings/profile" replace />
  const e = endpoint.data
  const inbox = e?.inbox_id ? inboxes.find((i) => i.id === e.inbox_id) : undefined
  return (
    <>
      <PageHeader
        back={{ to: "/settings/webhooks", label: t`Webhooks` }}
        title={
          e ? (
            <span className="font-mono text-title break-all" data-testid="webhook-url">
              {e.url}
            </span>
          ) : (
            <Trans>Webhook endpoint</Trans>
          )
        }
        description={
          e && (
            <>
              {e.description && <>{e.description} · </>}
              {inbox ? inbox.name : <Trans>All inboxes</Trans>}
            </>
          )
        }
        action={e && <EndpointStatus e={e} />}
      />
      {endpoint.isPending ? (
        <Skeleton className="h-48 w-full rounded-2xl" />
      ) : !e ? (
        <ErrorLine error={endpoint.error} />
      ) : (
        <>
          <EndpointCard e={e} />
          <Deliveries endpoint={e} />
          <AttemptLog endpoint={e} />
          <SignatureHint />
        </>
      )}
      <SecretDialog
        secret={created}
        title={<Trans>Signing secret</Trans>}
        description={<Trans>Copy it now and store it with your receiver; it is not shown again.</Trans>}
        onClose={() => navigate(location.pathname, { replace: true, state: null })}
      />
    </>
  )
}
