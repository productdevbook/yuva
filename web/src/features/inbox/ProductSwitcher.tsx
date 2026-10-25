import { useLingui } from "@lingui/react/macro"
import { ChevronDownIcon } from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { SidebarMenuButton, useSidebar } from "@/components/ui/sidebar"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

const ALL = "all"

function ProductDot({ color }: { color?: string }) {
  return (
    <span
      aria-hidden
      className="mx-[3px] size-[9px] shrink-0 rounded-[3px] bg-sidebar-key"
      style={color ? { backgroundColor: color } : undefined}
    />
  )
}

function useProduct() {
  const { t } = useLingui()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const [value, set] = useInboxFilter()
  const current = inboxes.find((i) => i.id === value)
  return { inboxes, value, set, current, label: current?.name ?? t`All products` }
}

export function ProductSwitcher({ className }: { className?: string }) {
  const { t } = useLingui()
  const { inboxes, value, set, current, label } = useProduct()
  if (inboxes.length < 2) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={cn(
          "flex h-[34px] w-full items-center gap-2.5 rounded-[10px] border border-sidebar-border bg-card ps-2.5 pe-2 text-start text-small text-sidebar-accent-foreground outline-none transition-colors hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-sidebar-ring data-popup-open:bg-sidebar-accent in-data-[mobile=true]:h-11 in-data-[mobile=true]:text-base",
          className
        )}
        aria-label={t`Product: ${label}`}
        data-testid="product-switcher"
      >
        <ProductDot color={current?.branding.color} />
        <span className="min-w-0 flex-1 truncate font-medium">{label}</span>
        <ChevronDownIcon className="size-3.5 shrink-0 text-sidebar-muted" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-(--anchor-width) min-w-56">
        <DropdownMenuRadioGroup value={value || ALL} onValueChange={(v) => set(!v || v === ALL ? "" : String(v))}>
          <DropdownMenuRadioItem value={ALL} data-testid="product-all">
            <ProductDot />
            {t`All products`}
          </DropdownMenuRadioItem>
          <DropdownMenuSeparator />
          {inboxes.map((i) => (
            <DropdownMenuRadioItem key={i.id} value={i.id} data-testid={`product-${i.id}`}>
              <ProductDot color={i.branding.color} />
              <span className="min-w-0 flex-1 truncate">{i.name}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function ProductScope() {
  const { t } = useLingui()
  const { inboxes, current, label } = useProduct()
  const { setOpen } = useSidebar()
  if (inboxes.length < 2 || !current) return null
  return (
    <div className="mx-auto hidden h-7 w-9 group-data-[collapsible=icon]:block">
      <SidebarMenuButton
        variant="outline"
        tooltip={t`Product: ${label}`}
        aria-label={t`Product: ${label}`}
        onClick={() => setOpen(true)}
        className="justify-center rounded-lg group-data-[collapsible=icon]:size-full! group-data-[collapsible=icon]:p-0!"
        data-testid="product-scope"
      >
        <ProductDot color={current.branding.color} />
      </SidebarMenuButton>
    </div>
  )
}
