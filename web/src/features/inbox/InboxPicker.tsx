import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronDownIcon, LockIcon } from "lucide-react"

import { Dot } from "@/components/common"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { look } from "@/features/inbox/PageFrame"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

const ALL = "all"

export function InboxPicker({
  className,
  value: controlled,
  onChange,
}: {
  className?: string
  value?: string
  onChange?: (id: string) => void
}) {
  const { t } = useLingui()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const [stored, setStored] = useInboxFilter()
  const value = controlled ?? stored
  const set = onChange ?? setStored
  if (inboxes.length < 2) return null
  const items: Record<string, string> = {
    [ALL]: t`All products`,
    ...Object.fromEntries(inboxes.map((i) => [i.id, i.name])),
  }
  return (
    <Select value={value || ALL} items={items} onValueChange={(v) => set(!v || v === ALL ? "" : String(v))}>
      <SelectTrigger
        aria-label={t`Product`}
        size="sm"
        className={cn(
          "max-w-48 border-transparent bg-transparent ps-2 pe-2 text-muted-foreground shadow-none hover:border-transparent hover:bg-muted hover:text-foreground",
          className
        )}
        data-testid="inbox-picker"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent className="min-w-52">
        <SelectItem value={ALL}>{t`All products`}</SelectItem>
        {inboxes.map((i) => (
          <SelectItem key={i.id} value={i.id}>
            <Dot color={i.branding.color} className="mt-1.5 size-2 rounded-[3px]" />
            {i.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function InboxChip({
  value,
  locked,
  onChange,
}: {
  value: string
  locked: boolean
  onChange: (id: string) => void
}) {
  const { t } = useLingui()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  if (inboxes.length < 2) return null
  const current = inboxes.find((i) => i.id === value)
  const label = (
    <>
      {current && <Dot color={current.branding.color} className="size-2 rounded-[3px]" />}
      <span className="max-w-32 truncate">{current ? current.name : <Trans>All products</Trans>}</span>
    </>
  )
  if (locked) {
    return (
      <button
        type="button"
        disabled
        className={cn(look.chip, "cursor-default opacity-80 hover:border-input")}
        title={t`Set by the product in the sidebar`}
        aria-label={t`Product`}
        data-testid="inbox-chip"
      >
        {label}
        <LockIcon className="size-3 text-faint" />
      </button>
    )
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={look.chip} />}
        aria-label={t`Product`}
        data-testid="inbox-chip"
      >
        {label}
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
        <DropdownMenuRadioGroup value={value} onValueChange={(v) => onChange(String(v))}>
          <DropdownMenuRadioItem value="">
            <Trans>All products</Trans>
          </DropdownMenuRadioItem>
          {inboxes.map((i) => (
            <DropdownMenuRadioItem key={i.id} value={i.id}>
              <Dot color={i.branding.color} className="size-2 rounded-[3px]" />
              <span className="truncate">{i.name}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
