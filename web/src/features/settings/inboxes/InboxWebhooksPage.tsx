import { InboxWebhooks, useInboxOutlet } from "@/features/settings/inboxes/InboxPage"
import { WebhookList } from "@/features/settings/webhooks/WebhookList"

function List() {
  const { inbox } = useInboxOutlet()
  return <WebhookList inboxId={inbox.id} />
}

export function InboxWebhooksPage() {
  return (
    <InboxWebhooks>
      <List />
    </InboxWebhooks>
  )
}
