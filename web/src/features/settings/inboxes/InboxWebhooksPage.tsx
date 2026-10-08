import { useInboxOutlet } from "@/features/settings/inboxes/InboxLayout"
import { WebhookList } from "@/features/settings/webhooks/WebhookList"

export function InboxWebhooksPage() {
  const { inbox } = useInboxOutlet()
  return <WebhookList inboxId={inbox.id} />
}
