import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronsUpDownIcon, PlusIcon, SearchIcon, SettingsIcon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import { Drawer, DrawerContent, DrawerTitle } from "@/components/ui/drawer"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useIsPhone } from "@/hooks/use-media-query"
import { useCounts } from "@/features/inbox/queries"
import { useVersion } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

const MANY = 8
const BUSIEST = 5

function useSelector() {
  const { me, membership, switchWorkspace } = useSession()
  const signup = useVersion().data?.signup
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const [value, set] = useInboxFilter()
  const counts = useCounts().data
  const [query, setQuery] = useState("")
  const [expanded, setExpanded] = useState(false)
  const openOf = new Map((counts?.inboxes ?? []).map((c) => [c.id, c.count]))
  const sorted = [...inboxes].sort(
    (a, b) => (openOf.get(b.id) ?? 0) - (openOf.get(a.id) ?? 0) || a.name.localeCompare(b.name)
  )
  const picked = inboxes.length > 1 ? inboxes.find((i) => i.id === value) : undefined
  const q = query.trim().toLocaleLowerCase()
  const many = sorted.length > MANY
  const collapsed = many && !q && !expanded
  const top = collapsed
    ? sorted.slice(0, BUSIEST)
    : q
      ? sorted.filter((i) => i.name.toLocaleLowerCase().includes(q))
      : sorted
  const visible = collapsed && picked && !top.includes(picked) ? [...top, picked] : top
  return {
    workspaces: me.memberships,
    currentId: membership.workspace.id,
    name: membership.workspace.name,
    switchWorkspace,
    creatable: signup === "open",
    products: sorted.length > 1,
    value,
    set,
    picked,
    visible,
    hidden: collapsed ? sorted.length - visible.length : 0,
    total: inboxes.reduce((n, i) => n + (openOf.get(i.id) ?? 0), 0),
    openOf,
    query,
    setQuery,
    showAll: () => setExpanded(true),
    many,
    searching: !!q,
  }
}

type Selector = ReturnType<typeof useSelector>

function ProductDot({ color, className }: { color?: string; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn("size-2 shrink-0 rounded-full bg-muted-foreground", className)}
      style={color ? { backgroundColor: color } : undefined}
    />
  )
}

function Count({ n }: { n: number | undefined }) {
  return <span className="font-mono text-[11px] text-muted-foreground">{n ?? 0}</span>
}

function DesktopMenu({ s }: { s: Selector }) {
  const { t } = useLingui()
  const hidden = s.hidden
  const navigate = useNavigate()
  return (
    <DropdownMenuContent align="start" className="w-72" data-testid="workspace-menu">
      <DropdownMenuGroup>
        <DropdownMenuLabel>
          <Trans>Workspace</Trans>
        </DropdownMenuLabel>
        {s.workspaces.map((m) => {
          const current = m.workspace.id === s.currentId
          return (
            <DropdownMenuItem
              key={m.workspace.id}
              onClick={() => !current && s.switchWorkspace(m.workspace.id)}
              aria-current={current ? "true" : undefined}
              data-testid="workspace-option"
            >
              <WorkspaceMark name={m.workspace.name} className="size-[22px] rounded-md text-[11px]" />
              <span className="min-w-0 flex-1 truncate">{m.workspace.name}</span>
              <CheckIcon className={cn("size-3.5 text-foreground", !current && "invisible")} />
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuGroup>
      {s.products && (
        <>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <Trans>Product</Trans>
            </DropdownMenuLabel>
            {s.many && (
              <Input
                type="search"
                value={s.query}
                onChange={(e) => s.setQuery(e.target.value)}
                onKeyDown={(e) => e.stopPropagation()}
                placeholder={t`Find a product`}
                aria-label={t`Find a product`}
                className="mb-1 rounded-lg bg-subtle"
                data-testid="page-products-find"
              />
            )}
            <DropdownMenuRadioGroup value={s.value || "all"} onValueChange={(v) => s.set(v === "all" ? "" : v)}>
              {!s.searching && (
                <DropdownMenuRadioItem value="all" data-testid="page-product-all">
                  <span className="min-w-0 flex-1 truncate">
                    <Trans>All products</Trans>
                  </span>
                  <Count n={s.total} />
                </DropdownMenuRadioItem>
              )}
              {s.visible.map((i) => (
                <DropdownMenuRadioItem key={i.id} value={i.id} data-testid={`page-product-${i.id}`}>
                  <ProductDot color={i.branding.color} />
                  <span className="min-w-0 flex-1 truncate">{i.name}</span>
                  <Count n={s.openOf.get(i.id)} />
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
            {s.visible.length === 0 && (
              <p className="m-0 px-2.5 py-1.5 text-small text-muted-foreground">
                <Trans>No product by that name.</Trans>
              </p>
            )}
            {s.hidden > 0 && (
              <DropdownMenuItem
                closeOnClick={false}
                onClick={s.showAll}
                className="text-muted-foreground"
                data-testid="page-products-more"
              >
                {t`Show ${hidden} more`}
              </DropdownMenuItem>
            )}
          </DropdownMenuGroup>
        </>
      )}
      <DropdownMenuSeparator />
      {s.creatable && (
        <DropdownMenuItem onClick={() => navigate("/workspaces/new")} data-testid="workspace-create">
          <PlusIcon />
          <Trans>New workspace</Trans>
        </DropdownMenuItem>
      )}
      <DropdownMenuItem onClick={() => navigate("/settings/workspace")} data-testid="workspace-settings">
        <SettingsIcon />
        <Trans>Workspace settings</Trans>
      </DropdownMenuItem>
    </DropdownMenuContent>
  )
}

const sheetRow =
  "flex h-11 w-full items-center gap-3 rounded-lg px-3 text-start text-base outline-none hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring"

function SheetLabel({ children }: { children: React.ReactNode }) {
  return <div className="px-3 pt-2 pb-1 text-caption font-medium text-muted2">{children}</div>
}

function SheetBody({ s, close }: { s: Selector; close: () => void }) {
  const { t } = useLingui()
  const hidden = s.hidden
  const navigate = useNavigate()
  const go = (to: string) => {
    close()
    navigate(to)
  }
  const pick = (id: string) => {
    s.set(id)
    close()
  }
  return (
    <div className="flex min-h-0 flex-col overflow-y-auto px-2 pt-1 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
      <SheetLabel>
        <Trans>Workspace</Trans>
      </SheetLabel>
      {s.workspaces.map((m) => {
        const current = m.workspace.id === s.currentId
        return (
          <button
            key={m.workspace.id}
            type="button"
            className={sheetRow}
            aria-current={current ? "true" : undefined}
            onClick={() => {
              close()
              if (!current) s.switchWorkspace(m.workspace.id)
            }}
            data-testid="workspace-option"
          >
            <WorkspaceMark name={m.workspace.name} className="size-[22px] rounded-md text-[11px]" />
            <span className="min-w-0 flex-1 truncate">{m.workspace.name}</span>
            <CheckIcon className={cn("size-4", !current && "invisible")} />
          </button>
        )
      })}
      {s.products && (
        <>
          <div className="my-1.5 h-px bg-line-soft" />
          <SheetLabel>
            <Trans>Product</Trans>
          </SheetLabel>
          {s.many && (
            <div className="relative mx-1 mb-1">
              <SearchIcon
                aria-hidden
                className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                type="search"
                value={s.query}
                onChange={(e) => s.setQuery(e.target.value)}
                placeholder={t`Find a product`}
                aria-label={t`Find a product`}
                className="h-11 rounded-lg bg-subtle ps-9 text-base"
                data-testid="page-products-find"
              />
            </div>
          )}
          {!s.searching && (
            <button
              type="button"
              className={sheetRow}
              aria-current={!s.value ? "true" : undefined}
              onClick={() => pick("")}
              data-testid="page-product-all"
            >
              <span className="w-2 shrink-0" />
              <span className="min-w-0 flex-1 truncate">
                <Trans>All products</Trans>
              </span>
              <Count n={s.total} />
              <CheckIcon className={cn("size-4", s.value && "invisible")} />
            </button>
          )}
          {s.visible.map((i) => (
            <button
              key={i.id}
              type="button"
              className={sheetRow}
              aria-current={s.value === i.id ? "true" : undefined}
              onClick={() => pick(i.id)}
              data-testid={`page-product-${i.id}`}
            >
              <ProductDot color={i.branding.color} />
              <span className="min-w-0 flex-1 truncate">{i.name}</span>
              <Count n={s.openOf.get(i.id)} />
              <CheckIcon className={cn("size-4", s.value !== i.id && "invisible")} />
            </button>
          ))}
          {s.visible.length === 0 && (
            <p className="m-0 px-3 py-2 text-small text-muted-foreground">
              <Trans>No product by that name.</Trans>
            </p>
          )}
          {s.hidden > 0 && (
            <button
              type="button"
              className={cn(sheetRow, "text-muted-foreground")}
              onClick={s.showAll}
              data-testid="page-products-more"
            >
              <span className="w-2 shrink-0" />
              {t`Show ${hidden} more`}
            </button>
          )}
        </>
      )}
      <div className="my-1.5 h-px bg-line-soft" />
      {s.creatable && (
        <button type="button" className={sheetRow} onClick={() => go("/workspaces/new")} data-testid="workspace-create">
          <PlusIcon className="size-4" />
          <Trans>New workspace</Trans>
        </button>
      )}
      <button
        type="button"
        className={sheetRow}
        onClick={() => go("/settings/workspace")}
        data-testid="workspace-settings"
      >
        <SettingsIcon className="size-4" />
        <Trans>Workspace settings</Trans>
      </button>
    </div>
  )
}

export function WorkspaceMark({ name, className }: { name: string; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "grid size-[18px] shrink-0 place-items-center rounded-[5px] bg-foreground text-[10px] font-bold text-background uppercase",
        className
      )}
    >
      {Array.from(name.trim())[0] ?? ""}
    </span>
  )
}

const triggerClass =
  "flex h-7 max-w-60 min-w-0 shrink items-center gap-1.5 rounded-md px-1.5 text-small font-semibold text-foreground outline-none transition-colors hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring data-popup-open:bg-raise aria-expanded:bg-raise max-sm:max-w-36"

function Trigger({ s }: { s: Selector }) {
  return (
    <>
      <WorkspaceMark name={s.name} />
      <span className="min-w-0 truncate max-sm:sr-only" data-testid="header-workspace">
        {s.name}
      </span>
      {s.products && (
        <span
          className="flex h-[18px] max-w-32 min-w-0 shrink items-center gap-1 rounded-[4px] bg-brand-wash px-1.5 text-[11.5px] font-semibold text-brand-ink max-sm:hidden"
          data-testid="page-product-current"
        >
          {s.picked && <ProductDot color={s.picked.branding.color} className="size-1.5" />}
          <span className="truncate">{s.picked ? s.picked.name : <Trans>All products</Trans>}</span>
        </span>
      )}
      <ChevronsUpDownIcon aria-hidden className="size-3.5 shrink-0 text-muted-foreground" />
    </>
  )
}

export function WorkspaceSelector() {
  const { t } = useLingui()
  const s = useSelector()
  const phone = useIsPhone()
  const [open, setOpen] = useState(false)
  const name = s.name
  if (!phone)
    return (
      <DropdownMenu>
        <DropdownMenuTrigger className={triggerClass} aria-label={t`Workspace: ${name}`} data-testid="page-workspace">
          <Trigger s={s} />
        </DropdownMenuTrigger>
        <DesktopMenu s={s} />
      </DropdownMenu>
    )
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={t`Workspace: ${name}`}
        className={triggerClass}
        data-testid="page-workspace"
      >
        <Trigger s={s} />
      </button>
      <Drawer open={open} onOpenChange={setOpen} showSwipeHandle>
        <DrawerContent data-testid="workspace-menu">
          <DrawerTitle className="sr-only">{t`Workspace: ${name}`}</DrawerTitle>
          <SheetBody s={s} close={() => setOpen(false)} />
        </DrawerContent>
      </Drawer>
    </>
  )
}
