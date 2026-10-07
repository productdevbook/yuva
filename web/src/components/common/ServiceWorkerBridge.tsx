import { Trans } from "@lingui/react/macro"
import { SparklesIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { useNavigate } from "react-router"

import { Button } from "@/components/ui/button"
import { onServiceWorkerNavigate, useWaitingUpdate } from "@/lib/pwa"

export function ServiceWorkerBridge() {
  const navigate = useNavigate()
  const update = useWaitingUpdate()
  const [dismissed, setDismissed] = useState(false)
  useEffect(() => {
    onServiceWorkerNavigate((path) => navigate(path))
    return () => onServiceWorkerNavigate(null)
  }, [navigate])
  if (!update || dismissed) return null
  return (
    <div
      role="status"
      data-testid="update-toast"
      className="pointer-events-none fixed inset-x-0 bottom-4 z-50 flex justify-center px-4"
    >
      <div className="pointer-events-auto flex max-w-full items-center gap-2 rounded-full border bg-background py-1 pr-1 pl-3 text-sm shadow-md">
        <SparklesIcon className="size-4 shrink-0 text-primary" />
        <span className="min-w-0">
          <Trans>A new version of Yuva is ready.</Trans>
        </span>
        <Button size="sm" className="rounded-full" onClick={update} data-testid="update-reload">
          <Trans>Reload</Trans>
        </Button>
        <Button variant="ghost" size="sm" className="rounded-full" onClick={() => setDismissed(true)}>
          <Trans>Later</Trans>
        </Button>
      </div>
    </div>
  )
}
