import { useState } from "react"
import { Navigate } from "react-router"

import { DeveloperHeader } from "@/features/settings/DeveloperTabs"
import { WebhookForm, WebhookSecret, type CreatedEndpoint } from "@/features/settings/webhooks/WebhookForm"
import { WebhookTable } from "@/features/settings/webhooks/WebhookList"
import { useSession } from "@/lib/session"

export function WebhooksPage() {
  const { canManage } = useSession()
  const [adding, setAdding] = useState(false)
  const [created, setCreated] = useState<CreatedEndpoint | null>(null)
  if (!canManage) return <Navigate to="/settings/profile" replace />
  const add = () => {
    setCreated(null)
    setAdding(true)
  }
  return (
    <>
      <DeveloperHeader current="webhooks" onCreate={add} creating={adding} />
      {adding && (
        <WebhookForm
          onClose={() => setAdding(false)}
          onCreated={(e) => {
            setAdding(false)
            setCreated(e)
          }}
        />
      )}
      {created && <WebhookSecret created={created} onDone={() => setCreated(null)} />}
      <WebhookTable onAdd={adding ? undefined : add} />
    </>
  )
}
