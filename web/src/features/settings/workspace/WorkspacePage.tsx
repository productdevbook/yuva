import { Trans, useLingui } from "@lingui/react/macro"
import { BuildingIcon, CompassIcon, MailIcon, TriangleAlertIcon, ChevronRightIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { formatDate, useEnumText } from "@/components/common/text"
import { Skeleton } from "@/components/ui/skeleton"
import { PageTop, look } from "@/features/inbox/PageFrame"
import { useGeneralTabItems } from "@/features/settings/GeneralTabs"
import { BroadcastsSettings } from "@/features/settings/workspace/BroadcastsSettings"
import { DeleteWorkspace } from "@/features/settings/workspace/DeleteWorkspace"
import { FocusForm } from "@/features/settings/workspace/FocusForm"
import { RetentionForm } from "@/features/settings/workspace/RetentionForm"
import { FrameTabs } from "@/features/status/PageFrame"
import { initials } from "@/components/common/text"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useAllInboxes, useMembers } from "@/lib/workspace"

type Fact = { key: string; label: React.ReactNode; value: React.ReactNode }

function useFacts(): Fact[] {
  const { i18n } = useLingui()
  const { membership } = useSession()
  const members = useMembers().data
  const inboxes = useAllInboxes().data
  const num = new Intl.NumberFormat(i18n.locale)
  const owner = members?.find((m) => m.role === "owner")
  const names = inboxes?.map((x) => x.name) ?? []
  const facts: Fact[] = [
    { key: "created", label: <Trans>Created</Trans>, value: formatDate(membership.workspace.created_at, i18n.locale) },
  ]
  if (owner) facts.push({ key: "owner", label: <Trans>Owner</Trans>, value: owner.name || owner.email })
  if (members) facts.push({ key: "members", label: <Trans>Members</Trans>, value: num.format(members.length) })
  if (inboxes)
    facts.push({
      key: "products",
      label: <Trans>Products</Trans>,
      value: names.length > 3 ? `${names.slice(0, 3).join(", ")} +${num.format(names.length - 3)}` : names.join(", "),
    })
  return facts
}

function Aside() {
  const { membership, canManage } = useSession()
  const text = useEnumText()
  const members = useMembers()
  const inboxes = useAllInboxes()
  const facts = useFacts()
  const { i18n } = useLingui()
  const num = new Intl.NumberFormat(i18n.locale)
  const links = [
    { to: "/team", label: <Trans>Members</Trans>, note: members.data && num.format(members.data.length) },
    { to: "/settings", label: <Trans>Products</Trans>, note: inboxes.data && num.format(inboxes.data.length) },
    canManage
      ? { to: "/settings/api-keys", label: <Trans>API & MCP</Trans> }
      : { to: "/settings/connected-apps", label: <Trans>Connected apps</Trans> },
    { to: "/settings?view=channels", label: <Trans>Channels</Trans> },
  ]
  return (
    <aside
      className="sticky top-6 flex flex-col gap-4 self-start @max-[1239px]:hidden"
      aria-label={membership.workspace.name}
      data-testid="workspace-aside"
    >
      <div className="flex items-center gap-3 px-2.5 py-1">
        <span
          aria-hidden
          className="grid size-12 shrink-0 place-items-center rounded-[13px] bg-brand-solid text-[16px] font-bold text-white"
        >
          {initials(membership.workspace.name)}
        </span>
        <span className="flex min-w-0 flex-col">
          <b className="text-[15px] font-semibold [overflow-wrap:anywhere]">{membership.workspace.name}</b>
          <span className="text-small text-faint">{text.role[membership.role]}</span>
        </span>
      </div>
      <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 px-2.5 text-[12.5px]">
        {members.isPending || inboxes.isPending ? (
          <Skeleton className="col-span-full h-24" aria-busy="true" />
        ) : (
          facts.map((f) => (
            <div key={f.key} className="contents">
              <dt className="text-faint">{f.label}</dt>
              <dd className="m-0 text-end text-muted-foreground [overflow-wrap:anywhere]">{f.value}</dd>
            </div>
          ))
        )}
      </dl>
      <div className="flex flex-col gap-1">
        <h3 className={cn(look.over, "px-2.5 pb-1")}>
          <Trans>Elsewhere in settings</Trans>
        </h3>
        {links.map((l) => (
          <Link
            key={l.to}
            to={l.to}
            className="flex h-[34px] items-center gap-2.5 rounded-lg px-2.5 text-body text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span className="min-w-0 flex-1 truncate">{l.label}</span>
            {"note" in l && l.note && <span className="text-small text-faint tabular-nums">{l.note}</span>}
            <ChevronRightIcon className="size-3.5 shrink-0 text-faint rtl:rotate-180" aria-hidden />
          </Link>
        ))}
      </div>
    </aside>
  )
}

function FactsStrip() {
  const facts = useFacts()
  return (
    <dl className="m-0 hidden flex-wrap gap-1.5 @max-[1239px]:flex" data-testid="workspace-facts">
      {facts.map((f) => (
        <div key={f.key} className={cn(look.listChip, "h-auto min-h-7 max-w-full gap-1.5 py-0.5")}>
          <dt className="text-faint">{f.label}</dt>
          <dd className="m-0 min-w-0 truncate text-foreground">{f.value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function WorkspacePage() {
  const { t } = useLingui()
  const { membership } = useSession()
  const tabs = useGeneralTabItems()
  const [current, setCurrent] = useState("w-ws")
  const nav = [
    { id: "w-ws", icon: BuildingIcon, label: <Trans>Workspace</Trans> },
    { id: "w-focus", icon: CompassIcon, label: <Trans>What Yuva is for</Trans> },
    { id: "w-bc", icon: MailIcon, label: <Trans>Broadcast sending</Trans> },
    { id: "w-danger", icon: TriangleAlertIcon, label: <Trans>Danger zone</Trans> },
  ]
  return (
    <main
      className="@container flex h-svh min-h-0 min-w-0 flex-1 flex-col bg-surface max-md:h-[calc(100svh-49px)] phone:h-auto"
      data-testid="settings"
    >
      <PageTop
        title={<Trans>General</Trans>}
        sub={<Trans>Your workspace name, how long closed conversations are kept, and who may send broadcasts.</Trans>}
        tabs={<FrameTabs label={t`General`} current="workspace" items={tabs} />}
      />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="grid max-w-[1360px] items-start gap-8 px-6 pt-6 pb-24 phone:px-4 phone:pt-4 @min-[760px]:grid-cols-[180px_minmax(0,1fr)] @min-[1240px]:grid-cols-[196px_minmax(0,1fr)_296px]">
          <nav
            className="sticky top-6 flex flex-col gap-px self-start @max-[759px]:hidden"
            aria-label={t`On this page`}
          >
            <h3 className={cn(look.over, "px-2.5 pb-1.5")}>
              <Trans>On this page</Trans>
            </h3>
            {nav.map((n) => (
              <a
                key={n.id}
                href={`#${n.id}`}
                aria-current={current === n.id ? "true" : undefined}
                onClick={(e) => {
                  e.preventDefault()
                  setCurrent(n.id)
                  document.getElementById(n.id)?.scrollIntoView({ block: "start" })
                }}
                className="flex h-[34px] items-center gap-2.5 rounded-lg px-2.5 text-body text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=true]:bg-card aria-[current=true]:font-semibold aria-[current=true]:text-foreground aria-[current=true]:shadow-[0_0_0_1px_var(--border)]"
              >
                <n.icon className="size-4 shrink-0" aria-hidden />
                <span className="min-w-0 flex-1 truncate">{n.label}</span>
              </a>
            ))}
          </nav>
          <div className="flex min-w-0 max-w-[760px] flex-col gap-7">
            <FactsStrip />
            <RetentionForm key={`r${membership.workspace.retention_days ?? 0}`} />
            <FocusForm key={`f${membership.workspace.focus ?? ""}`} />
            <BroadcastsSettings />
            <DeleteWorkspace />
          </div>
          <Aside />
        </div>
      </div>
    </main>
  )
}
