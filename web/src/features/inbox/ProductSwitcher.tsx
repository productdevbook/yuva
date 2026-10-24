import { useLingui } from "@lingui/react/macro"
import { ChevronsUpDownIcon, LayersIcon } from "lucide-react"

import { InboxMark } from "@/components/common"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

const ALL = "all"

export function ProductSwitcher({ className }: { className?: string }) {
  const { t } = useLingui()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const [value, set] = useInboxFilter()
  if (inboxes.length < 2) return null
  const current = inboxes.find((i) => i.id === value)
  const color = current?.branding.color
  const label = current?.name ?? t`All products`
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={cn(
          "flex h-10 w-full items-center gap-2.5 rounded-md border border-sidebar-border bg-sidebar ps-2.5 pe-2 text-start text-body text-sidebar-foreground outline-none transition-colors hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-sidebar-ring data-popup-open:bg-sidebar-accent",
          className
        )}
        style={
          color
            ? {
                borderColor: `color-mix(in srgb, ${color} 40%, transparent)`,
                backgroundColor: `color-mix(in srgb, ${color} 6%, transparent)`,
              }
            : undefined
        }
        aria-label={t`Product: ${label}`}
        data-testid="product-switcher"
      >
        {current ? (
          <InboxMark name={current.name} color={color} />
        ) : (
          <span className="grid size-5 shrink-0 place-items-center rounded-md bg-sidebar-accent text-sidebar-muted">
            <LayersIcon className="size-3.5" />
          </span>
        )}
        <span className="min-w-0 flex-1 truncate font-medium">{label}</span>
        <ChevronsUpDownIcon className="size-4 shrink-0 text-sidebar-muted" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-(--anchor-width) min-w-56">
        <DropdownMenuRadioGroup value={value || ALL} onValueChange={(v) => set(!v || v === ALL ? "" : String(v))}>
          <DropdownMenuRadioItem value={ALL} data-testid="product-all">
            <span className="grid size-5 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground">
              <LayersIcon className="size-3.5" />
            </span>
            {t`All products`}
          </DropdownMenuRadioItem>
          <DropdownMenuSeparator />
          {inboxes.map((i) => (
            <DropdownMenuRadioItem key={i.id} value={i.id} data-testid={`product-${i.id}`}>
              <InboxMark name={i.name} color={i.branding.color} />
              <span className="min-w-0 flex-1 truncate">{i.name}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
