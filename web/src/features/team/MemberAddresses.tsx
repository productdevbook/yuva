import { Trans, useLingui } from "@lingui/react/macro"

import { ErrorLine, toast } from "@/components/common"
import { AddressState } from "@/components/common/AddressState"
import { Button } from "@/components/ui/button"
import { useApproveAddress, useMemberAddresses } from "@/features/settings/addresses/queries"
import type { MemberAddress } from "@/lib/api"
import { cn } from "@/lib/utils"
import { look, Overline } from "@/features/inbox/PageFrame"

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
    <li
      className={cn(
        "flex min-h-10 flex-wrap items-center gap-x-2.5 gap-y-1.5 rounded-[10px] border px-3 py-1.5 text-small",
        !a.verified && a.verification === "approval" && "border-warning/40 bg-warning/5"
      )}
      data-testid="member-address"
    >
      <bdi className="min-w-0 flex-1 basis-48 break-words">{address}</bdi>
      <AddressState address={a} />
      {!a.verified && a.verification === "approval" && (
        <Button
          className={look.btnInkSm}
          disabled={pending}
          aria-label={t`Approve ${address}`}
          onClick={onApprove}
          data-testid="address-approve"
        >
          <Trans>Approve</Trans>
        </Button>
      )}
    </li>
  )
}

export function MemberAddresses({ memberId, name }: { memberId: string; name: string }) {
  const { t } = useLingui()
  const list = useMemberAddresses(memberId, true)
  const approve = useApproveAddress(memberId)
  const items = list.data ?? []
  if (!list.isPending && !list.error && items.length === 0) return null
  return (
    <section className="flex flex-col gap-2.5 border-b px-5 py-4">
      <Overline>
        <Trans>Addresses to send from</Trans>
      </Overline>
      <p className="text-caption text-faint">
        <Trans>Addresses {name} added to send mail from. Those waiting for approval need an owner or admin.</Trans>
      </p>
      <ErrorLine error={list.error ?? approve.error} />
      {items.length > 0 && (
        <ul className="flex flex-col gap-1.5" data-testid="member-addresses">
          {items.map((a) => (
            <MemberAddressRow
              key={a.id}
              address={a}
              pending={approve.isPending}
              onApprove={() => approve.mutate(a.id, { onSuccess: () => toast(t`Address approved`) })}
            />
          ))}
        </ul>
      )}
    </section>
  )
}
