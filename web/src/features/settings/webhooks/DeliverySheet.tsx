import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery } from "@tanstack/react-query"

import { CopyButton, ErrorLine } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { AttemptResult, Attempts, DeliveryStateTag, ManualTag } from "@/features/settings/webhooks/parts"
import { useRefreshLog } from "@/features/settings/webhooks/queries"
import { api, unwrap, type WebhookEndpoint } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function DeliverySheet({ endpoint, deliveryId, onClose }: { endpoint: WebhookEndpoint; deliveryId: string | null; onClose: () => void }) {
  const { i18n } = useLingui()
  const { workspaceId: ws } = useSession()
  const refresh = useRefreshLog(endpoint.id)
  const path = { params: { path: { webhookId: endpoint.id, deliveryId: deliveryId ?? "" } } }
  const delivery = useQuery({
    queryKey: keys.webhookDelivery(ws, endpoint.id, deliveryId ?? ""),
    queryFn: () => unwrap(api.GET("/v1/webhooks/{webhookId}/deliveries/{deliveryId}", path)),
    enabled: !!deliveryId,
  })
  const redeliver = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver", path)),
    onSuccess: () => {
      refresh()
      setTimeout(refresh, 2000)
    },
  })
  const d = delivery.data
  const payload = d ? JSON.stringify(d.payload, null, 2) : ""
  return (
    <Sheet
      open={!!deliveryId}
      onOpenChange={(o) => {
        if (!o) {
          redeliver.reset()
          onClose()
        }
      }}
    >
      <SheetContent side="right" className="w-full sm:w-[36rem]" data-testid="delivery-sheet">
        <SheetHeader>
          <SheetTitle className="font-mono text-body">{d?.event_type ?? "…"}</SheetTitle>
          <SheetDescription>
            <Trans>Delivery details</Trans>
          </SheetDescription>
        </SheetHeader>
        {delivery.isPending ? (
          <div className="flex flex-col gap-3 p-5">
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-40 w-full" />
          </div>
        ) : !d ? (
          <ErrorLine error={delivery.error} className="p-5" />
        ) : (
          <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-5">
            <div className="flex flex-wrap items-center gap-3">
              <DeliveryStateTag state={d.state} />
              <span className="text-caption text-muted-foreground">
                <Attempts n={d.attempts} />
              </span>
              <Button size="sm" className="ms-auto" onClick={() => redeliver.mutate()} disabled={redeliver.isPending} data-testid="redeliver">
                <Trans>Redeliver</Trans>
              </Button>
            </div>
            {redeliver.isSuccess && (
              <p role="status" className="text-caption text-success" data-testid="redeliver-queued">
                <Trans>Queued. The new attempt shows up below in a moment.</Trans>
              </p>
            )}
            <ErrorLine error={redeliver.error} className="text-caption" />
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-2 text-body">
              <dt className="text-muted-foreground">webhook-id</dt>
              <dd className="flex min-w-0 items-center gap-2">
                <code className="min-w-0 truncate font-mono text-caption">{d.message_id}</code>
                <CopyButton value={d.message_id} />
              </dd>
              <dt className="text-muted-foreground">
                <Trans>Created</Trans>
              </dt>
              <dd>{formatDateTime(d.created_at, i18n.locale)}</dd>
              {d.state === "pending" && d.next_attempt_at && (
                <>
                  <dt className="text-muted-foreground">
                    <Trans>Next attempt</Trans>
                  </dt>
                  <dd>{formatDateTime(d.next_attempt_at, i18n.locale)}</dd>
                </>
              )}
            </dl>
            <div className="flex flex-col gap-2">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-body font-medium">
                  <Trans>Payload</Trans>
                </h3>
                <CopyButton value={payload} />
              </div>
              <pre className="max-h-80 overflow-auto rounded-xl bg-surface px-3.5 py-3 font-mono text-caption" data-testid="delivery-payload">
                {payload}
              </pre>
            </div>
            <div className="flex flex-col gap-2">
              <h3 className="text-body font-medium">
                <Trans>Attempts</Trans>
              </h3>
              {d.attempt_log.length === 0 ? (
                <p className="text-body text-muted-foreground">
                  <Trans>No attempt yet.</Trans>
                </p>
              ) : (
                <ol className="divide-y rounded-xl border" data-testid="attempt-log">
                  {d.attempt_log.map((a) => (
                    <li key={a.id} className="flex flex-col gap-1 px-3.5 py-2.5">
                      <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-caption">
                        <AttemptResult a={a} />
                        <span className="text-muted-foreground tabular-nums">{a.latency_ms} ms</span>
                        <time className="text-muted-foreground" dateTime={a.attempted_at}>
                          {formatDateTime(a.attempted_at, i18n.locale)}
                        </time>
                        {a.manual && <ManualTag />}
                      </span>
                      {a.error && <span className="text-caption break-words text-destructive">{a.error}</span>}
                      {a.response_body && (
                        <pre className="max-h-24 overflow-auto rounded-lg bg-surface px-2 py-1 font-mono text-caption break-all whitespace-pre-wrap text-muted-foreground">
                          {a.response_body}
                        </pre>
                      )}
                    </li>
                  ))}
                </ol>
              )}
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
