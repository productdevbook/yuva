import { Trans, useLingui } from "@lingui/react/macro"
import { useInfiniteQuery } from "@tanstack/react-query"
import { ChevronRightIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, Segmented } from "@/components/common"
import { DELIVERY_STATES, formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Card, EmptyRow, Rows, Section } from "@/features/settings/ui"
import { DeliverySheet } from "@/features/settings/webhooks/DeliverySheet"
import { AttemptResult, Attempts, DeliveryStateTag, NextAttempt } from "@/features/settings/webhooks/parts"
import { useRefreshLog } from "@/features/settings/webhooks/queries"
import { api, unwrap, type WebhookDeliveryState, type WebhookEndpoint } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { Item } from "@/components/ui/item"

export function Deliveries({ endpoint }: { endpoint: WebhookEndpoint }) {
  const { t, i18n } = useLingui()
  const { workspaceId: ws } = useSession()
  const text = useEnumText()
  const [state, setState] = useState<WebhookDeliveryState | "all">("all")
  const [open, setOpen] = useState<string | null>(null)
  const refresh = useRefreshLog(endpoint.id)
  const list = useInfiniteQuery({
    queryKey: keys.webhookDeliveries(ws, endpoint.id, state),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/webhooks/{webhookId}/deliveries", {
          params: { path: { webhookId: endpoint.id }, query: { cursor: pageParam, limit: 25, ...(state === "all" ? {} : { state }) } },
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  const states = [{ value: "all" as const, label: t`All` }, ...DELIVERY_STATES.map((s) => ({ value: s, label: text.delivery[s] }))]
  return (
    <Section
      title={<Trans>Deliveries</Trans>}
      description={<Trans>Newest first. Finished deliveries are kept for 7 days.</Trans>}
      action={
        <Button variant="ghost" size="sm" onClick={refresh} disabled={list.isFetching} data-testid="deliveries-refresh">
          <Trans>Refresh</Trans>
        </Button>
      }
    >
      <Segmented label={t`Deliveries`} value={state} onChange={setState} items={states} className="-mx-1" />
      <Card flush>
        {list.isPending ? (
          <Skeleton className="m-5 h-16" />
        ) : items.length === 0 ? (
          <EmptyRow>
            <Trans>Deliveries show up here once a subscribed event happens.</Trans>
          </EmptyRow>
        ) : (
          <Rows>
            {items.map((d) => (
              <li key={d.id}>
                <Item render={<button type="button" />} className="flex-nowrap rounded-none [button]:hover:bg-muted"
                  onClick={() => setOpen(d.id)}
                  data-testid="delivery-row"
                >
                  <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <span className="flex min-w-0 items-center gap-3">
                      <code className="truncate font-mono text-caption font-medium">{d.event_type}</code>
                      <DeliveryStateTag state={d.state} />
                    </span>
                    <span className="flex flex-wrap items-center gap-x-2 text-caption text-muted-foreground">
                      <time dateTime={d.created_at}>{formatDateTime(d.created_at, i18n.locale)}</time>
                      <Attempts n={d.attempts} />
                      {d.state === "pending" && d.next_attempt_at && <NextAttempt at={d.next_attempt_at} />}
                    </span>
                  </div>
                  {d.last_attempt && (
                    <span className="flex shrink-0 flex-col items-end gap-0.5" title={t`Last attempt`}>
                      <AttemptResult a={d.last_attempt} />
                      <span className="text-caption text-faint tabular-nums">{d.last_attempt.latency_ms} ms</span>
                    </span>
                  )}
                  <ChevronRightIcon className="size-4 shrink-0 text-faint rtl:rotate-180" />
                </Item>
              </li>
            ))}
          </Rows>
        )}
        {list.hasNextPage && (
          <div className="border-t px-3 py-2">
            <Button variant="ghost" size="sm" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage}>
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
        <ErrorLine error={list.error} className="px-5 pb-4" />
      </Card>
      <DeliverySheet endpoint={endpoint} deliveryId={open} onClose={() => setOpen(null)} />
    </Section>
  )
}
