import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { Link } from "react-router"

import { ErrorLine } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Card, EmptyRow, headButton, LinkRow, ProductTag, Rows, Section } from "@/features/settings/ui"
import { EndpointStatus } from "@/features/settings/webhooks/parts"
import { useWebhooks } from "@/features/settings/webhooks/queries"
import { SignatureHint } from "@/features/settings/webhooks/SignatureHint"
import { WebhookForm, WebhookSecret, type CreatedEndpoint } from "@/features/settings/webhooks/WebhookForm"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { api, unwrap, type WebhookEndpoint } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

function Dot({ e }: { e: WebhookEndpoint }) {
  return (
    <span
      aria-hidden
      className={cn(
        "size-2.5 shrink-0 rounded-full",
        !e.enabled ? (e.disabled_reason ? "bg-destructive" : "bg-input") : e.failing_since ? "bg-warning" : "bg-success"
      )}
    />
  )
}

function useLastDelivery(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.webhookDeliveries(ws, id, "all"), "last"],
    queryFn: () =>
      unwrap(
        api.GET("/v1/webhooks/{webhookId}/deliveries", { params: { path: { webhookId: id }, query: { limit: 1 } } })
      ).then((r) => r.items[0]),
  })
}

function LastDelivery({ e }: { e: WebhookEndpoint }) {
  const { i18n } = useLingui()
  const last = useLastDelivery(e.id).data
  return (
    <span
      className="w-52 shrink-0 truncate text-caption text-muted-foreground max-lg:hidden"
      data-testid="webhook-last"
    >
      {last ? (
        <>
          <span className={last.state === "failed" ? "text-destructive" : undefined}>{last.event_type}</span>
          {" · "}
          {formatRelative(last.created_at, i18n.locale)}
        </>
      ) : (
        <span className="text-faint">
          <Trans>No deliveries yet</Trans>
        </span>
      )}
    </span>
  )
}

export function WebhookTable({ onAdd }: { onAdd?: () => void }) {
  const { canManage } = useSession()
  const inboxes = useInboxes().data ?? []
  const list = useWebhooks(undefined)
  if (!canManage) return null
  return (
    <>
      <p className="text-muted-foreground">
        <Trans>
          Signed HTTP calls to your backend for conversations, messages and feedback. Each endpoint can listen to one
          product or to all.
        </Trans>
      </p>
      {list.data && list.data.length === 0 ? (
        <EmptyBox
          title={<Trans>No endpoints yet</Trans>}
          testId="webhooks-empty"
          actions={
            onAdd && (
              <Button className={headButton.primary} onClick={onAdd}>
                <Trans>Add endpoint</Trans>
              </Button>
            )
          }
        >
          <Trans>Add an address on your server and pick the events Yuva should tell it about.</Trans>
        </EmptyBox>
      ) : (
        <section className="overflow-hidden rounded-xl border bg-card">
          {list.isPending ? (
            <Skeleton className="m-4 h-10" />
          ) : (
            <ul>
              {(list.data ?? []).map((e) => {
                const inbox = e.inbox_id ? inboxes.find((i) => i.id === e.inbox_id) : undefined
                const count = e.events.length
                const status = !e.enabled
                  ? e.disabled_reason
                    ? "disabled"
                    : "off"
                  : e.failing_since
                    ? "failing"
                    : "on"
                return (
                  <li key={e.id} className="flex items-center gap-3 border-b px-4 py-[11px] last:border-b-0">
                    <Dot e={e} />
                    <Link
                      to={`/settings/webhooks/${e.id}`}
                      className="flex min-w-0 flex-1 flex-col leading-[1.3] outline-none focus-visible:underline"
                      data-testid="webhook-row"
                      data-status={status}
                    >
                      <span className="truncate font-mono text-[13px]" dir="ltr">
                        {e.url}
                      </span>
                      <span className="truncate text-caption text-faint">
                        {e.description && <>{e.description} · </>}
                        {count === 1 ? <Trans>1 event</Trans> : <Trans>{count} events</Trans>}
                        {status === "disabled" && (
                          <span className="text-destructive">
                            {" · "}
                            <Trans>Turned off by Yuva</Trans>
                          </span>
                        )}
                        {status === "failing" && (
                          <span className="text-warning">
                            {" · "}
                            <Trans>Failing</Trans>
                          </span>
                        )}
                      </span>
                    </Link>
                    {inbox ? (
                      <ProductTag name={inbox.name} color={inbox.branding.color} />
                    ) : (
                      <span className="shrink-0 text-caption text-muted-foreground phone:hidden">
                        <Trans>All products</Trans>
                      </span>
                    )}
                    <LastDelivery e={e} />
                    <Button
                      variant="outline"
                      className="h-[30px] rounded-lg border-input px-2.5 font-normal"
                      render={<Link to={`/settings/webhooks/${e.id}?tab=deliveries`} />}
                    >
                      <Trans>Deliveries</Trans>
                    </Button>
                  </li>
                )
              })}
            </ul>
          )}
          <ErrorLine error={list.error} className="px-4 pb-4" />
        </section>
      )}
      <SignatureHint />
    </>
  )
}

export function WebhookList({ inboxId }: { inboxId?: string }) {
  const { canManage } = useSession()
  const inboxes = useInboxes().data ?? []
  const list = useWebhooks(inboxId)
  const [adding, setAdding] = useState(false)
  const [created, setCreated] = useState<CreatedEndpoint | null>(null)
  if (!canManage) return null
  return (
    <Section
      title={inboxId ? <Trans>Webhooks</Trans> : <Trans>Endpoints</Trans>}
      description={
        inboxId ? (
          <Trans>Signed HTTP calls to your backend for this inbox's conversations, messages and feedback.</Trans>
        ) : (
          <Trans>
            Signed HTTP calls to your backend when conversations, messages, feedback or contacts change. Endpoints of
            the workspace receive every inbox; inbox endpoints only their inbox.
          </Trans>
        )
      }
      action={
        <Button
          variant="outline"
          size="sm"
          disabled={adding}
          onClick={() => {
            setCreated(null)
            setAdding(true)
          }}
          data-testid="webhook-add"
        >
          <Trans>Add endpoint</Trans>
        </Button>
      }
    >
      {adding && (
        <WebhookForm
          lockedInbox={inboxId}
          onClose={() => setAdding(false)}
          onCreated={(e) => {
            setAdding(false)
            setCreated(e)
          }}
        />
      )}
      {created && <WebhookSecret created={created} onDone={() => setCreated(null)} />}
      <Card flush>
        {list.isPending ? (
          <Skeleton className="m-5 h-10" />
        ) : list.data && list.data.length === 0 ? (
          <EmptyRow>
            <Trans>No endpoints yet.</Trans>
          </EmptyRow>
        ) : (
          <Rows>
            {(list.data ?? []).map((e) => {
              const inbox = e.inbox_id ? inboxes.find((i) => i.id === e.inbox_id) : undefined
              const count = e.events.length
              return (
                <LinkRow key={e.id} to={`/settings/webhooks/${e.id}`} testId="webhook-row">
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-caption font-medium">{e.url}</p>
                    <p className="truncate text-caption text-muted-foreground">
                      {e.description && <>{e.description} · </>}
                      {!inboxId && (
                        <>
                          {inbox ? inbox.name : <Trans>All inboxes</Trans>}
                          {" · "}
                        </>
                      )}
                      {count === 1 ? <Trans>1 event</Trans> : <Trans>{count} events</Trans>}
                    </p>
                  </div>
                  <EndpointStatus e={e} />
                </LinkRow>
              )
            })}
          </Rows>
        )}
        <ErrorLine error={list.error} className="px-5 pb-4" />
      </Card>
      <SignatureHint />
    </Section>
  )
}
