import { useLingui } from "@lingui/react/macro"
import { useMemo } from "react"

import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { useMyAddresses } from "@/features/settings/addresses/queries"
import type { Channel, MemberAddress } from "@/lib/api"

export function sendingAddress(channel?: Channel) {
  return channel?.email?.from_address ?? channel?.email?.address
}

export function useOwnSenders(channelIds: string[]): MemberAddress[] {
  const all = useMyAddresses().data
  const key = channelIds.join(",")
  return useMemo(() => {
    const ids = new Set(key ? key.split(",") : [])
    return (all ?? []).filter((a) => a.verified && a.usable_channel_ids.some((id) => ids.has(id)))
  }, [all, key])
}

export function FromPicker({
  channelAddress,
  senders,
  value,
  onChange,
  testId,
  className,
  ref,
}: {
  ref?: React.Ref<HTMLSelectElement>
  channelAddress?: string
  senders: MemberAddress[]
  value: string
  onChange: (address: string) => void
  testId?: string
  className?: string
}) {
  const { t } = useLingui()
  if (senders.length === 0) return null
  return (
    <NativeSelect
      className={className}
      size="sm"
      value={senders.some((a) => a.address === value) ? value : ""}
      ref={ref}
      aria-label={t`Send from`}
      onChange={(e) => onChange(e.target.value)}
      data-testid={testId}
    >
      <NativeSelectOption value="">{channelAddress ?? t`The inbox address`}</NativeSelectOption>
      {senders.map((a) => (
        <NativeSelectOption key={a.id} value={a.address}>
          {a.address}
        </NativeSelectOption>
      ))}
    </NativeSelect>
  )
}
