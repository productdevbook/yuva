import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { useGo, useSection } from "@/app/Rail"
import { useNavGroups, WorkspaceMark, WorkspaceMenu } from "@/app/Sidebar"
import { UserMenu } from "@/app/UserMenu"
import {
  InfoColumn,
  NavGroup,
  NavList,
  PageLayout,
  PageSearch,
  SideColumn,
  SideTitle,
  type NavItem,
} from "@/components/common/PageLayout"
import {
  DropdownMenu,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useCounts } from "@/features/inbox/queries"
import { useVersion } from "@/lib/api"
import { useSession } from "@/lib/session"
import { isSupport, useInboxes } from "@/lib/workspace"

function ProductsMenuGroup() {
  const { t } = useLingui()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const [value, set] = useInboxFilter()
  const counts = useCounts().data
  const [query, setQuery] = useState("")
  if (inboxes.length < 2) return null
  const openOf = new Map((counts?.inboxes ?? []).map((c) => [c.id, c.count]))
  const sorted = [...inboxes].sort(
    (a, b) => (openOf.get(b.id) ?? 0) - (openOf.get(a.id) ?? 0) || a.name.localeCompare(b.name)
  )
  const total = inboxes.reduce((n, i) => n + (openOf.get(i.id) ?? 0), 0)
  const q = query.trim().toLocaleLowerCase()
  const found = q ? sorted.filter((i) => i.name.toLocaleLowerCase().includes(q)) : sorted
  const count = (n: number | undefined) => <span className="font-mono text-[11px] text-muted-foreground">{n ?? 0}</span>
  return (
    <DropdownMenuGroup>
      <DropdownMenuLabel>
        <Trans>Products</Trans>
      </DropdownMenuLabel>
      {sorted.length > 8 && (
        <Input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.stopPropagation()}
          placeholder={t`Find a product`}
          aria-label={t`Find a product`}
          className="mb-1 rounded-lg bg-subtle"
          data-testid="page-products-find"
        />
      )}
      <DropdownMenuRadioGroup value={value || "all"} onValueChange={(v) => set(v === "all" ? "" : v)}>
        {!q && (
          <DropdownMenuRadioItem value="all" data-testid="page-product-all">
            <span className="min-w-0 flex-1 truncate">
              <Trans>All products</Trans>
            </span>
            {count(total)}
          </DropdownMenuRadioItem>
        )}
        {found.map((i) => (
          <DropdownMenuRadioItem key={i.id} value={i.id} data-testid={`page-product-${i.id}`}>
            <span
              aria-hidden
              className="size-2 shrink-0 rounded-full bg-muted-foreground"
              style={i.branding.color ? { backgroundColor: i.branding.color } : undefined}
            />
            <span className="min-w-0 flex-1 truncate">{i.name}</span>
            {count(openOf.get(i.id))}
          </DropdownMenuRadioItem>
        ))}
        {found.length === 0 && (
          <p className="m-0 px-2.5 py-1.5 text-small text-muted-foreground">
            <Trans>No product by that name.</Trans>
          </p>
        )}
      </DropdownMenuRadioGroup>
    </DropdownMenuGroup>
  )
}

function WorkspaceHeader() {
  const { t } = useLingui()
  const { membership } = useSession()
  const version = useVersion().data?.version
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const [value] = useInboxFilter()
  const name = membership.workspace.name
  const picked = inboxes.length > 1 ? inboxes.find((i) => i.id === value) : undefined
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="flex w-full min-w-0 items-center gap-2.5 rounded-md px-2.5 py-1 text-start text-base font-semibold tracking-[-0.01em] text-foreground outline-none transition-colors hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring data-popup-open:bg-raise"
        aria-label={t`Workspace: ${name}`}
        data-testid="page-workspace"
      >
        <WorkspaceMark name={name} className="size-[22px] rounded-md text-[11px]" />
        <span className="flex min-w-0 flex-1 flex-col leading-tight">
          <span className="truncate">{name}</span>
          {inboxes.length > 1 && (
            <span
              className="flex items-center gap-1.5 truncate text-small font-normal tracking-normal text-muted-foreground"
              data-testid="page-product-current"
            >
              {picked && (
                <span
                  aria-hidden
                  className="size-2 shrink-0 rounded-full bg-muted-foreground"
                  style={picked.branding.color ? { backgroundColor: picked.branding.color } : undefined}
                />
              )}
              <span className="truncate">{picked ? picked.name : <Trans>All products</Trans>}</span>
            </span>
          )}
        </span>
        {version && (
          <span className="shrink-0 rounded border border-input px-1.5 font-mono text-[11px] font-normal text-muted-foreground">
            {version}
          </span>
        )}
      </DropdownMenuTrigger>
      <WorkspaceMenu>
        <ProductsMenuGroup />
      </WorkspaceMenu>
    </DropdownMenu>
  )
}

export function SectionNav() {
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
      <PageSearch label={searchLabel} />
      <SideTitle title={title} back={back} />
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
          {!canManage && <SectionNav />}
        </PageSidebar>
      }
      aside={<InfoColumn label={t`Details`}>{aside}</InfoColumn>}
    >
      {children}
    </PageLayout>
  )
}
