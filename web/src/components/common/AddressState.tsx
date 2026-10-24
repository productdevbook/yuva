import { Trans } from "@lingui/react/macro"

import { StatusTag } from "@/features/settings/ui"
import type { MemberAddress } from "@/lib/api"

export function AddressState({ address: a }: { address: MemberAddress }) {
  if (a.verified)
    return (
      <StatusTag tone="success" data-testid="address-state">
        <Trans>Verified</Trans>
      </StatusTag>
    )
  return (
    <StatusTag tone="warning" data-testid="address-state">
      {a.verification === "approval" ? <Trans>Waiting for approval</Trans> : <Trans>Waiting for the code</Trans>}
    </StatusTag>
  )
}
