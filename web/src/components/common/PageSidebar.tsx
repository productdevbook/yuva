import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { EllipsisIcon, SearchIcon } from "lucide-react"
import { useState } from "react"

import { useGo, useSection } from "@/app/Rail"
import { useNavGroups, WorkspaceMark, WorkspaceMenu } from "@/app/Sidebar"
import { UserMenu } from "@/app/UserMenu"
import {
  InfoColumn,
  NavGroup,
  NavList,
  NavRow,
  PageLayout,
  PageSearch,
  SideColumn,
  SideTitle,
  type NavItem,
} from "@/components/common/PageLayout"
import { DropdownMenu, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useCounts } from "@/features/inbox/queries"
import { useVersion } from "@/lib/api"
import { useSession } from "@/lib/session"
import { isSupport, useInboxes } from "@/lib/workspace"

function WorkspaceHeader() {
  const { t } = useLingui()
  const { membership } = useSession()
  const version = useVersion().data?.version
  const name = membership.workspace.name
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="flex w-full min-w-0 items-center gap-2.5 rounded-md px-2.5 py-1 text-start text-base font-semibold tracking-[-0.01em] text-foreground outline-none transition-colors hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring data-popup-open:bg-raise"
        aria-label={t`Workspace: ${name}`}
        data-testid="page-workspace"
      >
        <WorkspaceMark name={name} className="size-[22px] rounded-md text-[11px]" />
        <span className="min-w-0 flex-1 truncate">{name}</span>
        {version && (
          <span className="shrink-0 rounded border border-input px-1.5 font-mono text-[11px] font-normal text-muted-foreground">
            {version}
          </span>
        )}
      </DropdownMenuTrigger>
      <WorkspaceMenu />
    </DropdownMenu>
  )
}

export function ProductsGroup({ limit = 4 }: { limit?: number }) {
  const { t } = useLingui()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const [value, set] = useInboxFilter()
  const counts = useCounts().data
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  if (inboxes.length < 2) return null
  const openOf = new Map((counts?.inboxes ?? []).map((c) => [c.id, c.count]))
  const sorted = [...inboxes].sort(
    (a, b) => (openOf.get(b.id) ?? 0) - (openOf.get(a.id) ?? 0) || a.name.localeCompare(b.name)
  )
  const shown = sorted.slice(0, limit)
  const picked = sorted.find((i) => i.id === value)
  if (picked && !shown.includes(picked)) shown.splice(limit - 1, 1, picked)
  const total = inboxes.reduce((n, i) => n + (openOf.get(i.id) ?? 0), 0)
  const row = (i: (typeof inboxes)[number], close?: boolean): NavItem => ({
    key: i.id,
    label: i.name,
    color: i.branding.color ?? "",
    count: openOf.get(i.id),
    current: value === i.id,
    onSelect: () => {
      set(i.id)
      if (close) setOpen(false)
    },
    testId: `page-product-${i.id}`,
  })
  const rest = sorted.length - shown.length
  const q = query.trim().toLocaleLowerCase()
  const found = q ? sorted.filter((i) => i.name.toLocaleLowerCase().includes(q)) : sorted
  return (
    <NavGroup
      title={t`Products`}
      items={[
        {
          key: "all",
          label: t`All products`,
          color: "",
          count: total,
          current: !value,
          onSelect: () => set(""),
          testId: "page-product-all",
        },
        ...shown.map((i) => row(i)),
      ]}
    >
      {rest > 0 && (
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next)
            if (!next) setQuery("")
          }}
        >
          <PopoverTrigger
            className="flex w-full items-center gap-2 rounded-sm px-2.5 py-1 text-start text-body leading-5 text-muted-foreground outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
            data-rail-item
            data-testid="page-products-more"
          >
            <EllipsisIcon aria-hidden className="size-3.5 shrink-0" />
            <Plural value={rest} one="# more" other="# more" />
          </PopoverTrigger>
          <PopoverContent side="right" align="start" className="w-[300px] gap-2 rounded-xl p-2.5">
            <div className="flex items-center gap-2 px-0.5">
              <span className="flex-1 text-small font-semibold">
                <Trans>Products</Trans>
              </span>
              <span className="font-mono text-[11px] text-muted2">{sorted.length}</span>
            </div>
            <label className="relative block">
              <span className="sr-only">
                <Trans>Find a product</Trans>
              </span>
              <SearchIcon
                aria-hidden
                className="pointer-events-none absolute start-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t`Find a product`}
                className="rounded-lg bg-subtle ps-8"
                data-testid="page-products-find"
              />
            </label>
            <div className="-mx-1 flex max-h-[300px] flex-col overflow-y-auto px-1">
              {found.map((i) => (
                <NavRow key={i.id} item={row(i, true)} />
              ))}
              {found.length === 0 && (
                <p className="m-0 px-2.5 py-1.5 text-small text-muted-foreground">
                  <Trans>No product by that name.</Trans>
                </p>
              )}
            </div>
          </PopoverContent>
        </Popover>
      )}
    </NavGroup>
  )
}

export function SectionNav({ products = true }: { products?: boolean }) {
  const { t } = useLingui()
  const section = useSection()
  const go = useGo()
  const groups = useNavGroups()
  const items = (g: (typeof groups)[number]): NavItem[] =>
    g.rows.map((r) => ({
      key: r.key,
      label: r.label,
      count: r.badge,
      hot: r.hot,
      current: section === r.key,
      onSelect: () => go(r.key),
      ariaLabel: r.badge ? r.badgeLabel : undefined,
      title: `${r.label} · ${r.keys.join(" ")}`,
      testId: `page-nav-${r.key}`,
    }))
  const [first, ...rest] = groups
  return (
    <NavList label={t`Sections`}>
      {first && <NavGroup title={first.title} items={items(first)} />}
      {products && <ProductsGroup />}
      {rest.map((g) => (
        <NavGroup key={g.title ?? "main"} title={g.title} items={items(g)} />
      ))}
    </NavList>
  )
}

export function PageSidebar({
  title,
  searchLabel,
  children,
}: {
  title: React.ReactNode
  searchLabel?: string
  children?: React.ReactNode
}) {
  const { t } = useLingui()
  const { homePath, canManage } = useSession()
  const section = useSection()
  const back = canManage && section !== "home" ? { to: homePath, label: t`Home` } : undefined
  return (
    <SideColumn label={t`Navigation`}>
      <WorkspaceHeader />
      <SideTitle title={title} back={back} />
      <PageSearch label={searchLabel} />
      {children ?? <SectionNav />}
      <div className="border-t border-line-soft pt-3">
        <UserMenu row align="start" side="top" />
      </div>
    </SideColumn>
  )
}

export function SectionPage({
  title,
  searchLabel,
  nav,
  aside,
  testId,
  children,
}: {
  title: string
  searchLabel?: string
  nav: React.ReactNode
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  const { canManage } = useSession()
  return (
    <PageLayout
      testId={testId}
      label={title}
      sidebar={
        <PageSidebar title={title} searchLabel={searchLabel}>
          <NavList label={title}>{nav}</NavList>
          {!canManage && <SectionNav products={false} />}
        </PageSidebar>
      }
      aside={<InfoColumn label={t`Details`}>{aside}</InfoColumn>}
    >
      {children}
    </PageLayout>
  )
}
