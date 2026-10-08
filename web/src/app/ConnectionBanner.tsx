import { Trans } from "@lingui/react/macro"
import { useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { reconnectNow, useRealtimeStatus } from "@/lib/realtime"

export function ConnectionBanner() {
  const status = useRealtimeStatus()
  const [shown, setShown] = useState(false)
  useEffect(() => {
    if (status !== "offline") {
      setShown(false)
      return
    }
    const id = setTimeout(() => setShown(true), 1500)
    return () => clearTimeout(id)
  }, [status])
  if (!shown) return null
  return (
    <div role="status" data-testid="offline-banner" className="pointer-events-none fixed inset-x-0 top-3 z-50 flex justify-center px-4">
      <div className="pointer-events-auto flex items-center gap-3 rounded-full border bg-card py-1 ps-4 pe-1 text-sm shadow-[0_12px_32px_-16px_rgb(15_23_42/0.3)]">
        <span className="size-1.5 shrink-0 rounded-full bg-destructive" />
        <span>
          <Trans>Connection lost. Reconnecting…</Trans>
        </span>
        <Button variant="ghost" size="sm" onClick={reconnectNow}>
          <Trans>Retry now</Trans>
        </Button>
      </div>
    </div>
  )
}
