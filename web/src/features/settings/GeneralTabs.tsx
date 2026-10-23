import { Trans, useLingui } from "@lingui/react/macro"

import { SettingsStrip } from "@/components/common/SettingsStrip"
import { useMigrations } from "@/features/migrations/queries"
import { PageHeader, PageTabs } from "@/features/settings/ui"
import { useSession } from "@/lib/session"
import { useCannedReplies, useLabels, useMacros, useTeams } from "@/lib/workspace"

type Tab = "workspace" | "labels" | "canned-replies" | "macros" | "teams" | "migrations" | "audit-log"

export function GeneralTabs({ current }: { current: Tab }) {
  const { t } = useLingui()
  const { canManage, isOwner } = useSession()
  const labels = useLabels().data
  const canned = useCannedReplies().data
  const macros = useMacros().data
  const teams = useTeams().data
  const moves = useMigrations().data?.pages[0]?.items
  return (
    <PageTabs
      label={t`General`}
      current={current}
      items={[
        { key: "workspace", to: "/settings/workspace", label: t`Workspace`, testId: "general-tab-workspace" },
        {
          key: "labels",
          to: "/settings/labels",
          label: t`Labels`,
          count: labels?.length,
          testId: "general-tab-labels",
        },
        {
          key: "canned-replies",
          to: "/settings/canned-replies",
          label: t`Canned replies`,
          count: canned?.length,
          testId: "general-tab-canned-replies",
        },
        {
          key: "macros",
          to: "/settings/macros",
          label: t`Macros`,
          count: macros?.length,
          testId: "general-tab-macros",
        },
        ...(canManage
          ? [
              {
                key: "teams",
                to: "/settings/teams",
                label: t`Teams`,
                count: teams?.length,
                testId: "general-tab-teams",
              },
              {
                key: "migrations",
                to: "/settings/migrations",
                label: t`Move to Yuva`,
                count: moves?.length,
                testId: "general-tab-migrations",
              },
            ]
          : []),
        ...(isOwner
          ? [{ key: "audit-log", to: "/settings/audit-log", label: t`Audit log`, testId: "general-tab-audit-log" }]
          : []),
      ]}
    />
  )
}

export function GeneralHeader({ current }: { current: Tab }) {
  return (
    <>
      <PageHeader
        title={<Trans>General</Trans>}
        description={
          <Trans>The workspace itself: name, retention, labels, canned replies, moving in and the audit log</Trans>
        }
        tabs={<GeneralTabs current={current} />}
      />
      <SettingsStrip current="general" />
    </>
  )
}

export function Intro({ children, action }: { children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
      <p className="min-w-0 flex-1 basis-64 text-muted-foreground">{children}</p>
      {action}
    </div>
  )
}
