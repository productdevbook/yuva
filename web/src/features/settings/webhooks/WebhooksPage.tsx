import { Trans } from "@lingui/react/macro"
import { Navigate } from "react-router"

import { PageHeader } from "@/features/settings/ui"
import { WebhookList } from "@/features/settings/webhooks/WebhookList"
import { useSession } from "@/lib/session"

export function WebhooksPage() {
  const { canManage } = useSession()
  if (!canManage) return <Navigate to="/settings/profile" replace />
  return (
    <>
      <PageHeader title={<Trans>Webhooks</Trans>} />
      <WebhookList />
    </>
  )
}
