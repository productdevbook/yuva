import { Trans, useLingui } from "@lingui/react/macro"
import { Outlet, useOutletContext, useParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { Skeleton } from "@/components/ui/skeleton"
import { useInbox } from "@/features/settings/inboxes/queries"
import { PageHeader, TabNav } from "@/features/settings/ui"
import type { Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"

export function useInboxOutlet() {
  return useOutletContext<{ inbox: Inbox }>()
}

export function InboxLayout() {
  const { t } = useLingui()
  const { inboxId = "" } = useParams()
  const { canManage } = useSession()
  const inbox = useInbox(inboxId)
  const tabs = [
    { to: "", label: <Trans>General</Trans>, end: true },
    { to: "hours", label: <Trans>Business hours</Trans> },
    { to: "channels", label: <Trans>Channels</Trans> },
    { to: "access", label: <Trans>Access</Trans> },
    ...(canManage
      ? [
          { to: "webhooks", label: <Trans>Webhooks</Trans> },
          { to: "advanced", label: <Trans>Advanced</Trans> },
        ]
      : []),
  ].map((tab) => ({ ...tab, to: `/settings/inboxes/${inboxId}${tab.to ? `/${tab.to}` : ""}` }))
  return (
    <>
      <PageHeader title={inbox.data?.name ?? <Trans>Inbox</Trans>} back={{ to: "/settings/inboxes", label: t`Inboxes` }}>
        <TabNav items={tabs} label={t`Inbox settings`} />
      </PageHeader>
      {inbox.isPending ? (
        <Skeleton className="h-64 w-full rounded-2xl" />
      ) : inbox.data ? (
        <Outlet context={{ inbox: inbox.data }} />
      ) : (
        <ErrorLine error={inbox.error} />
      )}
    </>
  )
}
