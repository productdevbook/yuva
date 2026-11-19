import { useLingui } from "@lingui/react/macro"
import { useLocation, useMatch, useSearchParams } from "react-router"

import { usePage, type PageKey } from "@/app/nav"
import { inboxRoot, isChats } from "@/app/shell"
import { ViewsGroup } from "@/app/ViewsGroup"
import { SectionSidebar, type Place, type PlaceGroup } from "@/components/common/SectionSidebar"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useViewSearch } from "@/features/analytics/state"
import { useBroadcast } from "@/features/broadcasts/broadcastQueries"
import {
  rememberedList,
  useBroadcastCounts,
  useViewNames,
  viewLink,
  viewOf,
  type BroadcastsPlace,
} from "@/features/broadcasts/BroadcastsFrame"
import {
  useDuplicatePairs,
  usePeopleViewNames,
  viewOf as peopleViewOf,
  type PeopleView,
} from "@/features/contact/ContactsPage"
import { centerLink } from "@/features/help/queries"
import {
  rememberChip,
  rememberInbox,
  useChipLabels,
  useChips,
  useRememberedChip,
  useRememberedInbox,
  type Chip,
} from "@/features/inbox/ChatList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useCounts, useDrafts, useMentions } from "@/features/inbox/queries"
import { setRailView } from "@/features/inbox/railView"
import { resetSourceFilter, useOpenOnly } from "@/features/inbox/SourceFilter"
import {
  mailPath,
  useFilterAccess,
  useFolderLabels,
  useFolders,
  useMailInboxes,
  useMailRoute,
} from "@/features/mail/folders"
import { useFolderCounts } from "@/features/mail/MailSide"
import { useMailCounts } from "@/features/mail/queries"
import {
  placeOf,
  SETTINGS_MANAGERS,
  SETTINGS_OWNERS,
  SETTINGS_TEST_IDS,
  SETTINGS_TO,
  useSettingsCounts,
  useSettingsNames,
  type SettingsPlace,
} from "@/features/settings/SettingsFrame"
import { useStatusCounts } from "@/features/status/StatusFrame"
import { useInvites } from "@/features/team/TeamPage"
import { useSession } from "@/lib/session"
import { isSupport, useInboxes, useMembers, useTeams } from "@/lib/workspace"

export type SectionPlaces = { label: string; groups: PlaceGroup[] }

const CHIP_ORDER: Chip[] = ["mine", "unassigned", "waiting", "teams", "snoozed", "done", "all", "spam", "trash"]

function useInboxPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const { apart } = useSession()
  const offered = useChips()
  const labels = useChipLabels()
  const chip = useRememberedChip()
  const listInbox = useRememberedInbox()
  const [scope] = useInboxFilter()
  const openOnly = useOpenOnly()
  const counts = useCounts().data
  const unseen = Number(useMentions().data?.pages[0]?.unseen ?? 0)
  const botDrafts = Number(useDrafts({ author: "bot" }).data?.pages[0]?.total ?? 0)
  const assistantDrafts = Number(useDrafts({ author: "assistant" }).data?.pages[0]?.total ?? 0)
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const chats = isChats(pathname, apart)
  const viewId = chats ? params.get("view") : null
  const outbound = chats && !viewId && params.get("origin") === "outbound"
  const list = chats && !viewId && !outbound && !openOnly
  const byInbox = list && !scope && !!listInbox
  const root = inboxRoot(apart)
  const outboundCount = scope
    ? (counts?.outbound_inboxes.find((c) => c.id === scope)?.count ?? 0)
    : (counts?.outbound ?? 0)
  const chipCount: Partial<Record<Chip, number>> = scope
    ? {}
    : { mine: counts?.mine, unassigned: counts?.unassigned, teams: counts?.mine_teams }
  const views: Place[] = [
    ...CHIP_ORDER.filter((k) => offered.includes(k)).map((k) => ({
      key: k,
      label: labels[k],
      to: root,
      onSelect: () => {
        setRailView("conversations")
        rememberInbox("")
        rememberChip(k)
      },
      count: chipCount[k],
      hot: k === "unassigned",
      current: list && !byInbox && chip === k,
      testId: `page-view-${k}`,
    })),
    ...(outboundCount || outbound
      ? [
          {
            key: "outbound",
            label: t`Replies to mail we sent`,
            to: `${root}?origin=outbound&status=open`,
            onSelect: () => {
              setRailView("conversations")
              resetSourceFilter()
            },
            count: outboundCount,
            current: outbound,
            testId: "page-view-outbound",
          },
        ]
      : []),
    {
      key: "mentions",
      label: t`Mentions`,
      to: "/mentions",
      onSelect: () => setRailView("mentions"),
      count: unseen,
      hot: true,
      ariaLabel: unseen ? t`Mentions, ${unseen} unseen` : undefined,
      current: pathname === "/mentions",
      testId: "page-view-mentions",
    },
    {
      key: "assistants",
      label: t`Assistants`,
      to: "/assistants",
      onSelect: () => setRailView("assistants"),
      count: botDrafts + assistantDrafts,
      current: pathname === "/assistants",
      testId: "page-view-assistants",
    },
  ]
  const open = new Map((counts?.inboxes ?? []).map((c) => [c.id, c.count]))
  const inboxItems: Place[] =
    !scope && inboxes.length > 1
      ? inboxes.map((i) => ({
          key: i.id,
          label: i.name,
          color: i.branding.color ?? "",
          to: root,
          onSelect: () => {
            setRailView("conversations")
            rememberChip("waiting")
            rememberInbox(i.id)
          },
          count: open.get(i.id),
          current: byInbox && listInbox === i.id,
          testId: "page-inbox",
        }))
      : []
  return {
    label: t`Inbox`,
    groups: [
      { key: "views", title: t`Views`, items: views },
      { key: "saved", node: <ViewsGroup /> },
      ...(inboxItems.length > 0 ? [{ key: "inboxes", title: t`Inboxes`, items: inboxItems }] : []),
    ],
  }
}

function useMailPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const { box, folder } = useMailRoute()
  const labels = useFolderLabels()
  const folders = useFolders()
  const counts = useFolderCounts(box)
  const all = useMailCounts()
  const inboxes = useMailInboxes()
  const filters = useFilterAccess(box)
  const onFilters = /^\/mail\/[^/]+\/filters$/.test(pathname)
  const folderItems: Place[] = [
    ...folders.map((f) => ({
      key: f,
      label: labels[f],
      to: mailPath(box, f),
      count: counts[f],
      hot: f === "inbox",
      current: !onFilters && folder === f,
      testId: `mail-folder-${f}`,
    })),
    ...(filters
      ? [
          {
            key: "filters",
            label: t`Filters`,
            to: `/mail/${box}/filters`,
            current: onFilters,
            testId: "mail-filters-link",
          },
        ]
      : []),
  ]
  const boxItems: Place[] = [
    {
      key: "all",
      label: t`All mail`,
      color: "",
      count: all?.all,
      to: mailPath("all", folder),
      testId: "mail-box-all",
    },
    ...inboxes.map((i) => ({
      key: i.id,
      label: i.name,
      color: i.branding.color ?? "",
      count: all?.inboxes.find((c) => c.id === i.id)?.count,
      to: mailPath(i.id, folder),
      title: i.mailbox?.addresses[0],
      testId: "mail-box-option",
    })),
    { key: "new", label: t`New mailbox`, to: "/setup/mail", state: { back: "/mail" }, testId: "mail-box-new" },
  ]
  return {
    label: t`Mail`,
    groups: [
      { key: "folders", title: t`Folders`, items: folderItems },
      { key: "mailboxes", title: t`Mailboxes`, items: boxItems },
    ],
  }
}

function usePeoplePlaces(): SectionPlaces {
  const { t } = useLingui()
  const [params] = useSearchParams()
  const names = usePeopleViewNames()
  const { pairs, complete } = useDuplicatePairs()
  const current = peopleViewOf(params)
  const link = (v: PeopleView) => {
    const next = new URLSearchParams(params)
    if (v === "all") next.delete("tab")
    else next.set("tab", v)
    return { pathname: "/contacts", search: next.toString() ? `?${next.toString()}` : "" }
  }
  const item = (v: PeopleView): Place => ({
    key: v,
    label: names[v],
    to: link(v),
    current: current === v,
    count: v === "duplicates" && complete ? pairs.length : undefined,
    testId: v === "all" ? "people-all" : `people-${v}`,
  })
  return {
    label: t`People`,
    groups: [
      { key: "people", title: t`People`, items: [item("all"), item("customers"), item("visitors")] },
      { key: "tidy", title: t`Views`, items: [item("recent"), item("duplicates")] },
    ],
  }
}

function useTeamPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const { canManage } = useSession()
  const members = useMembers().data
  const teams = useTeams().data
  const pending = useInvites(canManage).data
  const invites = pathname === "/team/invites"
  const teamsPage = pathname === "/settings/teams"
  return {
    label: t`Team`,
    groups: [
      {
        key: "team",
        items: [
          {
            key: "members",
            label: t`Members`,
            to: "/team",
            count: members?.length,
            current: !invites && !teamsPage,
            testId: "team-tab-members",
          },
          ...(canManage
            ? [
                {
                  key: "teams",
                  label: t`Teams`,
                  to: "/settings/teams",
                  count: teams?.length,
                  current: teamsPage,
                  testId: "team-nav-teams",
                },
                {
                  key: "invites",
                  label: t`Invites`,
                  to: "/team/invites",
                  count: pending?.length,
                  current: invites,
                  testId: "team-tab-invites",
                },
              ]
            : []),
        ],
      },
    ],
  }
}

function useReportPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname, search } = useLocation()
  const at = pathname.split("/")[2] || "today"
  const items: Place[] = [
    { key: "today", label: t`Today`, to: { pathname: "/reports/today", search } },
    { key: "trends", label: t`Trends`, to: { pathname: "/reports/trends", search: "" } },
    { key: "ratings", label: t`Ratings`, to: { pathname: "/reports/ratings", search } },
    { key: "members", label: t`Members`, to: { pathname: "/reports/members", search } },
  ].map((p) => ({ ...p, current: at === p.key, testId: `report-tab-${p.key}` }))
  return { label: t`Reports`, groups: [{ key: "reports", items }] }
}

function useBroadcastPlace(): BroadcastsPlace {
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const id = useMatch("/broadcasts/:broadcastId")?.params.broadcastId
  const detail = id && !["lists", "designs", "triggered"].includes(id) ? id : undefined
  const b = useBroadcast(detail).data
  if (pathname.startsWith("/broadcasts/lists")) return "lists"
  if (pathname.startsWith("/broadcasts/designs")) return "designs"
  if (pathname.startsWith("/broadcasts/triggered")) return "triggered"
  if (detail) return b ? viewOf(b) : rememberedList().view
  const tab = params.get("tab")
  return tab === "drafts" || tab === "scheduled" || tab === "sent" ? tab : "all"
}

function useBroadcastPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { canManage } = useSession()
  const names = useViewNames()
  const counts = useBroadcastCounts()
  const place = useBroadcastPlace()
  const item = (key: BroadcastsPlace, label: string, to: string, testId: string): Place => ({
    key,
    label,
    to,
    count: counts[key],
    hot: key === "drafts",
    current: place === key,
    testId,
  })
  return {
    label: t`Broadcasts`,
    groups: [
      {
        key: "broadcasts",
        title: t`Broadcasts`,
        items: [
          item("all", names.all, viewLink("all"), "broadcasts-filter-all"),
          item("drafts", t`Drafts and approvals`, viewLink("drafts"), "broadcasts-filter-drafts"),
          item("scheduled", names.scheduled, viewLink("scheduled"), "broadcasts-filter-scheduled"),
          item("sent", names.sent, viewLink("sent"), "broadcasts-filter-sent"),
          item("triggered", names.triggered, "/broadcasts/triggered", "broadcasts-nav-triggered"),
        ],
      },
      {
        key: "audience",
        title: t`Audience`,
        items: [item("lists", t`Subscribers`, "/broadcasts/lists", "broadcasts-nav-lists")],
      },
      ...(canManage
        ? [
            {
              key: "content",
              title: t`Content`,
              items: [item("designs", names.designs, "/broadcasts/designs", "broadcasts-nav-designs")],
            },
          ]
        : []),
    ],
  }
}

const ANALYTICS_VIEWS = [
  "overview",
  "realtime",
  "sources",
  "pages",
  "locations",
  "devices",
  "goals",
  "events",
  "funnels",
  "retention",
  "support",
] as const

function useAnalyticsPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const search = useViewSearch(true)
  const [inboxId] = useInboxFilter()
  const properties = (useAnalyticsProperties().data?.items ?? []).filter((p) => !inboxId || p.inbox_id === inboxId)
  const matched = useMatch("/analytics/:propertyId/*")?.params.propertyId
  const propertyId = matched && matched !== "new" ? matched : undefined
  const target = propertyId ?? properties[0]?.id
  const view = propertyId ? (pathname.split("/")[3] ?? "overview") : pathname === "/analytics" ? "overview" : ""
  const keep = propertyId ? search : ""
  const to = (v: string) => (target ? `/analytics/${target}${v === "overview" ? "" : `/${v}`}${keep}` : "/analytics")
  const names: Record<(typeof ANALYTICS_VIEWS)[number], string> = {
    overview: t`Overview`,
    realtime: t`Realtime`,
    sources: t`Sources`,
    pages: t`Pages`,
    locations: t`Locations`,
    devices: t`Devices`,
    goals: t`Goals`,
    events: t`Events`,
    funnels: t`Funnels`,
    retention: t`Retention`,
    support: t`Support`,
  }
  return {
    label: t`Analytics`,
    groups: [
      {
        key: "reports",
        title: t`Reports`,
        items: ANALYTICS_VIEWS.map((v) => ({
          key: v,
          label: names[v],
          to: to(v),
          current: view === v,
          testId: `analytics-nav-${v}`,
        })),
      },
      {
        key: "site",
        title: t`This site`,
        items: [
          {
            key: "settings",
            label: t`Settings`,
            to: target ? `/analytics/${target}/settings` : "/analytics",
            current: view === "settings",
            testId: "analytics-nav-settings",
          },
        ],
      },
    ],
  }
}

function useFeedbackPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const feedback = Number(useCounts().data?.feedback ?? 0)
  const roadmap = pathname.startsWith("/board")
  return {
    label: t`Feedback and roadmap`,
    groups: [
      {
        key: "feedback",
        items: [
          {
            key: "feedback",
            label: t`Feedback`,
            to: "/feedback",
            count: feedback,
            hot: true,
            current: !roadmap,
            testId: "feedback-nav",
          },
          { key: "roadmap", label: t`Roadmap`, to: "/board", current: roadmap, testId: "roadmap-nav" },
        ],
      },
    ],
  }
}

function useHelpPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const centerId = useMatch("/knowledge/:centerId/*")?.params.centerId
  const knowledge = pathname.startsWith("/knowledge")
  const tab = knowledge ? (params.get("tab") ?? "articles") : ""
  const link = (v?: string) => (centerId ? centerLink(centerId, v) : `/knowledge${v ? `?tab=${v}` : ""}`)
  const items: Place[] = [
    {
      key: "articles",
      label: t`Articles`,
      to: link(),
      current: knowledge && (tab === "articles" || tab === "drafts"),
      testId: "help-nav-articles",
    },
    {
      key: "collections",
      label: t`Collections`,
      to: link("collections"),
      current: tab === "collections",
      testId: "help-nav-collections",
    },
    {
      key: "discussions",
      label: t`Discussions`,
      to: "/docs/discussions",
      current: pathname.startsWith("/docs/discussions"),
      testId: "help-nav-discussions",
    },
    { key: "report", label: t`Search gaps`, to: link("report"), current: tab === "report", testId: "help-nav-report" },
    {
      key: "updates",
      label: t`Updates`,
      to: "/updates",
      current: pathname.startsWith("/updates"),
      testId: "help-nav-updates",
    },
    {
      key: "docs",
      label: t`Docs`,
      to: "/docs",
      current: pathname.startsWith("/docs") && !pathname.startsWith("/docs/discussions"),
      testId: "help-nav-docs",
    },
    ...(canManage
      ? [
          {
            key: "settings",
            label: t`Settings`,
            to: link("settings"),
            current: tab === "settings",
            testId: "help-nav-settings",
          },
        ]
      : []),
  ]
  return { label: t`Help center`, groups: [{ key: "help", items }] }
}

function useStatusPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const counts = useStatusCounts()
  const down = counts.down
  const at = (path: string) => pathname === path || pathname.startsWith(`${path}/`)
  return {
    label: t`Status`,
    groups: [
      {
        key: "status",
        items: [
          {
            key: "pages",
            label: t`Pages`,
            to: "/status",
            count: counts.pages,
            current: !at("/incidents") && !at("/monitors"),
            testId: "status-nav-overview",
          },
          {
            key: "incidents",
            label: t`Incidents`,
            to: "/incidents",
            count: counts.active,
            hot: true,
            current: at("/incidents"),
            testId: "status-nav-incidents",
          },
          {
            key: "monitors",
            label: t`Monitors`,
            to: "/monitors",
            count: counts.down || counts.monitors,
            hot: counts.down > 0,
            ariaLabel: down > 0 ? t`Monitors, ${down} down` : undefined,
            current: at("/monitors"),
            testId: "status-nav-monitors",
          },
        ],
      },
    ],
  }
}

const SETTINGS_GROUPS: [string, SettingsPlace[]][] = [
  ["workspace", ["workspace", "products"]],
  ["channels", ["channels", "addresses"]],
  ["conversations", ["labels", "canned-replies", "macros", "automation", "sla", "fields", "signature"]],
  ["developers", ["api-keys", "webhooks", "connected-apps", "issue-trackers", "slack"]],
  ["security", ["sso", "audit-log"]],
  ["you", ["profile", "notifications", "appearance"]],
]

function useSettingsPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname, search } = useLocation()
  const { canManage, isOwner } = useSession()
  const names = useSettingsNames()
  const counts = useSettingsCounts()
  const place = placeOf(pathname, search)
  const attention = counts.attention ?? 0
  const allowed = (p: SettingsPlace) =>
    SETTINGS_OWNERS.includes(p) ? isOwner : SETTINGS_MANAGERS.includes(p) ? canManage : true
  const titles: Record<string, string> = {
    workspace: names.group.workspace,
    channels: t`Channels`,
    conversations: t`Conversations`,
    developers: names.group.developers,
    security: names.group.security,
    you: names.group.you,
  }
  return {
    label: t`Settings`,
    groups: SETTINGS_GROUPS.map(([key, places]) => ({
      key,
      title: titles[key],
      items: places.filter(allowed).map((p) => ({
        key: p,
        label: names.place[p],
        to: SETTINGS_TO[p],
        count: p === "channels" && attention > 0 ? attention : counts[p],
        hot: p === "channels" && attention > 0,
        ariaLabel: p === "channels" && attention > 0 ? t`Channels needing attention: ${attention}` : undefined,
        current: p === place,
        testId: SETTINGS_TEST_IDS[p] ?? `settings-nav-${p}`,
      })),
    })).filter((g) => g.items.length > 0),
  }
}

function useImportPlaces(): SectionPlaces {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const fresh = pathname === "/migrations/new"
  return {
    label: t`Imports`,
    groups: [
      {
        key: "imports",
        items: [
          { key: "imports", label: t`Imports`, to: "/settings/migrations", current: !fresh, testId: "imports-nav" },
          { key: "new", label: t`New import`, to: "/migrations/new", current: fresh, testId: "imports-nav-new" },
        ],
      },
    ],
  }
}

const builders: Record<Exclude<PageKey, "home">, () => SectionPlaces> = {
  inbox: useInboxPlaces,
  mail: useMailPlaces,
  people: usePeoplePlaces,
  team: useTeamPlaces,
  reports: useReportPlaces,
  broadcasts: useBroadcastPlaces,
  analytics: useAnalyticsPlaces,
  feedback: useFeedbackPlaces,
  help: useHelpPlaces,
  status: useStatusPlaces,
  settings: useSettingsPlaces,
  imports: useImportPlaces,
}

function PlacesSidebar({ places }: { places: () => SectionPlaces }) {
  const p = places()
  return <SectionSidebar label={p.label} groups={p.groups} />
}

export function SectionPlacesSidebar() {
  const page = usePage()
  if (page === "home") return null
  return <PlacesSidebar key={page} places={builders[page]} />
}
