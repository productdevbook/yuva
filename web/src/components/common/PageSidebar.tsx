import { useLingui } from "@lingui/react/macro"

import { useGo, useSection } from "@/app/Rail"
import { useNavGroups } from "@/app/Sidebar"
import { UserMenu } from "@/app/UserMenu"
import {
  BackButton,
  InfoColumn,
  NavGroup,
  NavList,
  PageLayout,
  PageSearch,
  SideColumn,
  SideTitle,
  type NavItem,
} from "@/components/common/PageLayout"
import { WorkspaceSelector } from "@/components/common/WorkspaceSelector"
import { useSession } from "@/lib/session"

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
  const back = canManage && section !== "home" ? { to: homePath, label: t`Back to Home` } : undefined
  return (
    <SideColumn label={t`Navigation`}>
      <WorkspaceSelector />
      <div className="flex gap-1.5">
        {back && <BackButton to={back.to} label={back.label} />}
        <PageSearch label={searchLabel} />
      </div>
      <SideTitle title={title} />
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
