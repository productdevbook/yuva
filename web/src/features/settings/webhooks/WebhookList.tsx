import { Trans } from "@lingui/react/macro"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Card, EmptyRow, LinkRow, Rows, Section } from "@/features/settings/ui"
import { EndpointStatus } from "@/features/settings/webhooks/parts"
import { useWebhooks } from "@/features/settings/webhooks/queries"
import { SignatureHint } from "@/features/settings/webhooks/SignatureHint"
import { WebhookDialog } from "@/features/settings/webhooks/WebhookDialog"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export function WebhookList({ inboxId }: { inboxId?: string }) {
  const { canManage } = useSession()
  const inboxes = useInboxes().data ?? []
  const list = useWebhooks(inboxId)
  const [creating, setCreating] = useState(false)
  const navigate = useNavigate()
  if (!canManage) return null
  return (
    <Section
      title={inboxId ? <Trans>Webhooks</Trans> : <Trans>Endpoints</Trans>}
      description={
        inboxId ? (
          <Trans>Signed HTTP calls to your backend for this inbox's conversations, messages and feedback.</Trans>
        ) : (
          <Trans>
            Signed HTTP calls to your backend when conversations, messages, feedback or contacts change. Endpoints of the
            workspace receive every inbox; inbox endpoints only their inbox.
          </Trans>
        )
      }
      action={
        <Button variant="outline" size="sm" onClick={() => setCreating(true)} data-testid="webhook-add">
          <Trans>Add endpoint</Trans>
        </Button>
      }
    >
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
      {creating && (
        <WebhookDialog
          endpoint={null}
          inboxId={inboxId}
          onClose={() => setCreating(false)}
          onCreated={(e, secret) => {
            setCreating(false)
            navigate(`/settings/webhooks/${e.id}`, { state: { secret } })
          }}
        />
      )}
    </Section>
  )
}
