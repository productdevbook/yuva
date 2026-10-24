import { Trans, useLingui } from "@lingui/react/macro"

import { ErrorLine, toast } from "@/components/common"
import { AddressState } from "@/components/common/AddressState"
import { Button } from "@/components/ui/button"
import { useApproveAddress, useMemberAddresses } from "@/features/settings/addresses/queries"
import type { MemberAddress } from "@/lib/api"
import { Card, Row, Rows, Section } from "@/features/settings/ui"

function MemberAddressRow({
  address: a,
  pending,
  onApprove,
}: {
  address: MemberAddress
  pending: boolean
  onApprove: () => void
}) {
  const { t } = useLingui()
  const address = a.address
  return (
    <Row className="flex-wrap" data-testid="member-address">
      <span className="min-w-0 flex-1 basis-48 truncate">{address}</span>
      <AddressState address={a} />
      {!a.verified && a.verification === "approval" && (
        <Button
          size="sm"
          disabled={pending}
          aria-label={t`Approve ${address}`}
          onClick={onApprove}
          data-testid="address-approve"
        >
          <Trans>Approve</Trans>
        </Button>
      )}
    </Row>
  )
}

export function MemberAddresses({ memberId, name }: { memberId: string; name: string }) {
  const { t } = useLingui()
  const list = useMemberAddresses(memberId, true)
  const approve = useApproveAddress(memberId)
  const items = list.data ?? []
  if (!list.isPending && !list.error && items.length === 0) return null
  return (
    <Section
      title={<Trans>Addresses to send from</Trans>}
      description={
        <Trans>Addresses {name} added to send mail from. Those waiting for approval need an owner or admin.</Trans>
      }
    >
      <ErrorLine error={list.error ?? approve.error} />
      {items.length > 0 && (
        <Card flush data-testid="member-addresses">
          <Rows>
            {items.map((a) => (
              <MemberAddressRow
                key={a.id}
                address={a}
                pending={approve.isPending}
                onApprove={() => approve.mutate(a.id, { onSuccess: () => toast(t`Address approved`) })}
              />
            ))}
          </Rows>
        </Card>
      )}
    </Section>
  )
}
