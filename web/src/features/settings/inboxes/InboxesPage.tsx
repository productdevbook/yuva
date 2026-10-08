import { Trans } from "@lingui/react/macro"
import { useState } from "react"

import { Dot, ErrorLine } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { CreateInboxDialog } from "@/features/settings/inboxes/CreateInboxDialog"
import { Card, EmptyRow, LinkRow, PageHeader, Rows, RowText } from "@/features/settings/ui"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export function InboxesPage() {
  const { canManage } = useSession()
  const inboxes = useInboxes()
  const text = useEnumText()
  const [creating, setCreating] = useState(false)
  return (
    <>
      <PageHeader
        title={<Trans>Inboxes</Trans>}
        description={<Trans>One inbox per product or brand. Each has its own channels, hours and team.</Trans>}
        action={
          canManage && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Trans>New inbox</Trans>
            </Button>
          )
        }
      />
      <Card flush>
        {inboxes.data && inboxes.data.length === 0 ? (
          <EmptyRow>
            <Trans>No inboxes yet</Trans>
            {canManage && (
              <>
                {" "}
                <Trans>Create one to start receiving conversations.</Trans>
              </>
            )}
          </EmptyRow>
        ) : (
          <Rows>
            {(inboxes.data ?? []).map((inbox) => (
              <LinkRow key={inbox.id} to={inbox.id} testId="inbox-row">
                <Dot color={inbox.branding.color} className="size-2.5" />
                <RowText title={inbox.name} detail={`${inbox.slug} · ${text.mode[inbox.mode]} · ${inbox.timezone}`} />
              </LinkRow>
            ))}
          </Rows>
        )}
        <ErrorLine error={inboxes.error} className="px-5 pb-4" />
      </Card>
      <CreateInboxDialog open={creating} onOpenChange={setCreating} />
    </>
  )
}
