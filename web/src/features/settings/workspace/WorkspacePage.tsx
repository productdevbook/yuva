import { Trans } from "@lingui/react/macro"

import { DeleteWorkspace } from "@/features/settings/workspace/DeleteWorkspace"
import { RetentionForm } from "@/features/settings/workspace/RetentionForm"
import { PageHeader } from "@/features/settings/ui"
import { useSession } from "@/lib/session"

export function WorkspacePage() {
  const { membership } = useSession()
  return (
    <>
      <PageHeader title={<Trans>Workspace</Trans>} description={membership.workspace.name} />
      <RetentionForm key={membership.workspace.retention_days ?? 0} />
      <DeleteWorkspace />
    </>
  )
}
