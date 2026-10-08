import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Skeleton } from "@/components/ui/skeleton"
import { Card, EmptyRow, Section } from "@/features/settings/ui"
import { DeliverySheet } from "@/features/settings/webhooks/DeliverySheet"
import { AttemptResult, ManualTag } from "@/features/settings/webhooks/parts"
import { api, unwrap, type WebhookEndpoint } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function AttemptLog({ endpoint }: { endpoint: WebhookEndpoint }) {
  const { i18n } = useLingui()
  const { workspaceId: ws } = useSession()
  const [open, setOpen] = useState<string | null>(null)
  const list = useQuery({
    queryKey: keys.webhookAttempts(ws, endpoint.id),
    queryFn: () =>
      unwrap(api.GET("/v1/webhooks/{webhookId}/attempts", { params: { path: { webhookId: endpoint.id }, query: { limit: 100 } } })).then((r) => r.items),
  })
  const th = "px-4 py-2.5 text-start font-medium whitespace-nowrap"
  return (
    <Section title={<Trans>Attempt log</Trans>} description={<Trans>Every request to this endpoint, newest first. The newest 100 are kept.</Trans>}>
      <Card flush>
        {list.isPending ? (
          <Skeleton className="m-5 h-16" />
        ) : !list.data || list.data.length === 0 ? (
          <EmptyRow>
            <Trans>No attempts yet.</Trans>
          </EmptyRow>
        ) : (
          <div className="max-h-96 overflow-auto">
            <table className="w-full text-xs" data-testid="attempts-table">
              <thead className="sticky top-0 bg-surface text-muted-foreground">
                <tr>
                  <th className={th}>
                    <Trans>Time</Trans>
                  </th>
                  <th className={th}>
                    <Trans>Status</Trans>
                  </th>
                  <th className={`${th} text-end`}>
                    <Trans>Latency</Trans>
                  </th>
                  <th className={th}>
                    <Trans>Error or answer</Trans>
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {list.data.map((a) => (
                  <tr key={a.id} className="cursor-pointer align-top transition-colors hover:bg-surface" onClick={() => setOpen(a.delivery_id)} data-testid="attempt-row">
                    <td className="px-4 py-2 whitespace-nowrap">
                      {formatDateTime(a.attempted_at, i18n.locale)}
                      {a.manual && <ManualTag />}
                    </td>
                    <td className="px-4 py-2">
                      <AttemptResult a={a} />
                    </td>
                    <td className="px-4 py-2 text-end whitespace-nowrap tabular-nums">{a.latency_ms} ms</td>
                    <td className="max-w-64 min-w-40 truncate px-4 py-2 text-muted-foreground" title={a.error ?? a.response_body}>
                      {a.error || a.response_body || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <ErrorLine error={list.error} className="px-5 pb-4" />
      </Card>
      <DeliverySheet endpoint={endpoint} deliveryId={open} onClose={() => setOpen(null)} />
    </Section>
  )
}
