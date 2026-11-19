import { Trans } from "@lingui/react/macro"
import { Link, useLocation } from "react-router"

import { Button } from "@/components/ui/button"
import { HOME_BACK, type SetupState } from "@/features/setup/setup"

export function useFromHome() {
  return ((useLocation().state ?? {}) as SetupState).back === HOME_BACK
}

export function BackHome({ variant }: { variant?: "default" | "outline" }) {
  const fromHome = useFromHome()
  if (!fromHome) return null
  return (
    <Button size="lg" variant={variant} render={<Link to={HOME_BACK} replace />} data-testid="setup-back-home">
      <Trans>Back to Home</Trans>
    </Button>
  )
}
