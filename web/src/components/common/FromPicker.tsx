import { useLingui } from "@lingui/react/macro"
import { useId, useMemo } from "react"

import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { useMyAddresses } from "@/features/settings/addresses/queries"
import type { Channel, MemberAddress } from "@/lib/api"

export function sendingAddress(channel?: Channel) {
  return channel?.email?.from_address ?? channel?.email?.address
}

export function catchAllDomain(channel?: Channel) {
  const address = channel?.email?.address
  return address?.startsWith("*@") ? address.slice(2).toLowerCase() : undefined
}

const ADDRESS = /^[^\s@,;<>"()]+@[^\s@,;<>"()]+\.[^\s@,;<>"()]+$/

export type FromProblem = "invalid" | "domain"

export function fromProblem(value: string, domain: string, own: string[]): FromProblem | undefined {
  const address = value.trim().toLowerCase()
  if (!address) return undefined
  if (!ADDRESS.test(address)) return "invalid"
  return address.endsWith(`@${domain}`) || own.includes(address) ? undefined : "domain"
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

export function FromField({
  id: inputId,
  domain,
  value,
  onChange,
  placeholder,
  suggestions,
  problem,
  missing,
  error,
  testId,
  className,
  ref,
}: {
  ref?: React.Ref<HTMLInputElement>
  id?: string
  domain: string
  value: string
  onChange: (address: string) => void
  placeholder?: string
  suggestions: string[]
  problem?: FromProblem
  missing?: boolean
  error?: string
  testId?: string
  className?: string
}) {
  const { t } = useLingui()
  const id = useId()
  const shown =
    problem === "invalid" ? t`Enter a valid e-mail address.` : problem || missing ? t`Use an address on ${domain}.` : ""
  const message = error ?? shown
  const bad = !!error || !!problem
  return (
    <div className={className}>
      <Input
        ref={ref}
        id={inputId}
        type="email"
        inputMode="email"
        autoComplete="off"
        spellCheck={false}
        className="h-8 rounded-md text-small"
        value={value}
        placeholder={placeholder ?? t`name@${domain}`}
        list={suggestions.length > 0 ? `${id}-list` : undefined}
        aria-label={t`Send from`}
        aria-invalid={bad || undefined}
        aria-describedby={message ? `${id}-note` : undefined}
        onChange={(e) => onChange(e.target.value)}
        data-testid={testId}
      />
      {suggestions.length > 0 && (
        <datalist id={`${id}-list`}>
          {suggestions.map((a) => (
            <option key={a} value={a} />
          ))}
        </datalist>
      )}
      {message && (
        <p
          id={`${id}-note`}
          role={bad ? "alert" : undefined}
          className={bad ? "pt-1 text-caption text-destructive" : "pt-1 text-caption text-faint"}
          data-testid={testId ? `${testId}-note` : undefined}
        >
          {message}
        </p>
      )}
    </div>
  )
}
