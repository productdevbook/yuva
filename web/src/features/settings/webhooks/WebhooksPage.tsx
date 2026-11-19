import { Navigate } from "react-router"

import { DeveloperHeader } from "@/features/settings/DeveloperTabs"
import { WebhookTable } from "@/features/settings/webhooks/WebhookList"
import { useSession } from "@/lib/session"

export function WebhooksPage() {
  const { canManage } = useSession()
  if (!canManage) return <Navigate to="/settings/profile" replace />
  return (
    <>
      <DeveloperHeader current="webhooks" />
      <WebhookTable />
    </>
  )
}
