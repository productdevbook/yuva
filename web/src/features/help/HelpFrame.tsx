import { useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useNavigate, useParams, useSearchParams } from "react-router"

import { NavGroup, SectionPage, TopSep, toast, type NavItem } from "@/components/common"
import { CreateCenterDialog } from "@/features/help/CreateCenterDialog"
import { centerLink, useHelpCenter, useHelpCenters, useHelpCollections } from "@/features/help/queries"
import { collectionName } from "@/features/help/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import type { HelpCenter, Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"
import { isSupport, useInboxes } from "@/lib/workspace"

export type HelpPlace = "articles" | "drafts" | "collections" | "report" | "settings"

export function useHelpNames(): Record<HelpPlace, string> {
  const { t } = useLingui()
  return {
    articles: t`Articles`,
    drafts: t`Drafts`,
    collections: t`Collections`,
    report: t`Report`,
    settings: t`Settings`,
  }
}

export function collectionTo(centerId: string, collectionId: string) {
  return `/knowledge/${centerId}?collection=${collectionId}`
}

function HelpNav({ place, center }: { place?: HelpPlace; center?: HelpCenter }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const [filter, setFilter] = useInboxFilter()
  const names = useHelpNames()
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const centers = useHelpCenters().data ?? []
  const cols = useHelpCollections(center?.id ?? "").data ?? []
  const [creating, setCreating] = useState<Inbox | null>(null)
  const picked = params.get("collection") ?? ""
  const prefer = [i18n.locale, center?.default_locale ?? "en"]
  const views: NavItem[] = center
    ? (["articles", "drafts", "collections", "report", ...(canManage ? (["settings"] as const) : [])] as const).map(
        (v) => ({
          key: v,
          label: names[v],
          to: centerLink(center.id, v === "articles" ? undefined : v),
          count:
            v === "articles"
              ? center.counts.articles
              : v === "drafts"
                ? center.counts.articles - center.counts.published_articles
                : v === "collections"
                  ? center.counts.collections
                  : undefined,
          hot: v === "drafts",
          current: place === v && !(v === "articles" && picked),
          testId: `help-tab-${v}`,
        })
      )
    : []
  const ordered = cols
    .filter((c) => !c.parent_id)
    .sort((a, b) => a.position - b.position || a.number - b.number)
    .flatMap((top) => [
      top,
      ...cols.filter((c) => c.parent_id === top.id).sort((a, b) => a.position - b.position || a.number - b.number),
    ])
  const collections: NavItem[] = center
    ? ordered.map((c) => ({
        key: c.id,
        label: collectionName(c, ...prefer),
        to: collectionTo(center.id, c.id),
        count: c.article_count,
        sub: !!c.parent_id,
        current: place === "articles" && picked === c.id,
        testId: "help-collection",
      }))
    : []
  const sites: NavItem[] = inboxes.flatMap((inbox) => {
    const c = centers.find((x) => x.inbox_id === inbox.id)
    if (c)
      return [
        {
          key: inbox.id,
          label: inbox.name,
          color: inbox.branding.color ?? "",
          count: c.counts.articles,
          to: centerLink(c.id),
          onSelect: () => filter && setFilter(inbox.id),
          current: c.id === center?.id,
          testId: "help-row",
        },
      ]
    if (!canManage) return []
    const product = inbox.name
    return [
      {
        key: inbox.id,
        label: t`New help center for ${product}`,
        color: "",
        onSelect: () => setCreating(inbox),
        testId: "help-nav-create",
      },
    ]
  })
  return (
    <>
      {views.length > 0 && <NavGroup items={views} />}
      {collections.length > 0 && <NavGroup title={names.collections} items={collections} />}
      {sites.length > 0 && <NavGroup title={t`Help centers`} items={sites} />}
      {creating && (
        <CreateCenterDialog
          inboxId={creating.id}
          inboxName={creating.name}
          onClose={() => setCreating(null)}
          onCreated={(c) => {
            setCreating(null)
            toast(t`Help center created. Write the first article, then publish it.`)
            navigate(centerLink(c.id))
          }}
        />
      )}
    </>
  )
}

export function HelpLayout({
  place,
  center,
  aside,
  testId,
  children,
}: {
  place?: HelpPlace
  center?: HelpCenter
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  const { centerId = "" } = useParams()
  const fallback = useHelpCenter(center ? "" : centerId).data
  return (
    <SectionPage
      title={t`Help center`}
      nav={<HelpNav place={place} center={center ?? fallback} />}
      aside={aside}
      testId={testId}
    >
      {children}
    </SectionPage>
  )
}

export function HelpTrail({ place, center }: { place: string; center?: HelpCenter }) {
  const { t } = useLingui()
  return (
    <>
      <span className="font-medium text-foreground">{t`Help center`}</span>
      <TopSep />
      <span className="truncate">{place}</span>
      {center && (
        <>
          <TopSep />
          <span className="truncate normal-case">{center.name}</span>
        </>
      )}
    </>
  )
}
