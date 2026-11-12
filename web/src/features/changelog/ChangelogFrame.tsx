import { useLingui } from "@lingui/react/macro"

import { NavGroup, ProductsGroup, SectionPage, TopSep, type NavItem } from "@/components/common"
import { changelogLink, useChangelogs } from "@/features/changelog/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import type { Changelog } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export const VIEWS = ["all", "draft", "scheduled", "published"] as const
export type View = (typeof VIEWS)[number]
export type ChangelogPlace = View | "changelogs" | "entries" | "settings"

export function viewTo(v: View) {
  return v === "all" ? "/updates" : `/updates?state=${v}`
}

export function useChangelogNames(): Record<ChangelogPlace, string> {
  const { t } = useLingui()
  return {
    all: t`All`,
    draft: t`Drafts`,
    scheduled: t`Scheduled`,
    published: t`Published`,
    changelogs: t`Changelogs`,
    entries: t`Entries`,
    settings: t`Settings`,
  }
}

export function useViewCounts(): Record<View, number> | undefined {
  const [inboxId] = useInboxFilter()
  const changelogs = useChangelogs().data
  if (!changelogs) return undefined
  const items = changelogs.filter((c) => !inboxId || c.inbox_id === inboxId)
  const n = items.reduce(
    (s, c) => ({
      all: s.all + c.counts.entries,
      published: s.published + c.counts.published,
      scheduled: s.scheduled + c.counts.scheduled,
    }),
    { all: 0, published: 0, scheduled: 0 }
  )
  return { ...n, draft: n.all - n.published - n.scheduled }
}

function ChangelogNav({ place, changelog }: { place: ChangelogPlace; changelog?: Changelog }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const [inboxId] = useInboxFilter()
  const names = useChangelogNames()
  const counts = useViewCounts()
  const inboxes = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const changelogs = (useChangelogs().data ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
  const views: NavItem[] = VIEWS.map((v) => ({
    key: v,
    label: names[v],
    to: viewTo(v),
    count: counts?.[v],
    hot: v === "draft",
    current: place === v,
    testId: `changelog-view-${v}`,
  }))
  const logs: NavItem[] = changelogs.flatMap((c) => {
    const row: NavItem = {
      key: c.id,
      label: c.name,
      color: inboxes.get(c.inbox_id)?.branding.color ?? "",
      to: changelogLink(c.id),
      count: c.counts.entries,
      current: changelog?.id === c.id && place === "entries",
      testId: "changelog-nav",
    }
    if (changelog?.id !== c.id) return [row]
    return [
      row,
      ...(canManage
        ? [
            {
              key: `${c.id}-settings`,
              label: names.settings,
              to: changelogLink(c.id, "settings"),
              current: place === "settings",
              sub: true,
              testId: "changelog-tab-settings",
            },
          ]
        : []),
    ]
  })
  return (
    <>
      <NavGroup items={views} />
      <NavGroup
        title={names.changelogs}
        items={[
          ...logs,
          {
            key: "manage",
            label: t`All changelogs`,
            to: "/updates/changelogs",
            current: place === "changelogs",
            testId: "changelog-home-menu",
          },
        ]}
      />
      <ProductsGroup />
    </>
  )
}

export function ChangelogLayout({
  place,
  changelog,
  aside,
  testId,
  children,
}: {
  place: ChangelogPlace
  changelog?: Changelog
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <SectionPage
      title={t`What’s new`}
      nav={<ChangelogNav place={place} changelog={changelog} />}
      aside={aside}
      testId={testId}
    >
      {children}
    </SectionPage>
  )
}

export function ChangelogTrail({ place, name }: { place: string; name?: string }) {
  const { t } = useLingui()
  const [inboxId] = useInboxFilter()
  const product = useInboxes().data?.find((i) => i.id === inboxId)?.name
  return (
    <>
      <span className="font-medium text-foreground">{t`What’s new`}</span>
      <TopSep />
      <span className="truncate">{place}</span>
      <TopSep />
      <span className="truncate normal-case">{name ?? product ?? t`All products`}</span>
    </>
  )
}
