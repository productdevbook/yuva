import { Trans } from "@lingui/react/macro"

import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { BroadcastsSettings } from "@/features/settings/workspace/BroadcastsSettings"
import { DeleteWorkspace } from "@/features/settings/workspace/DeleteWorkspace"
import { FocusForm } from "@/features/settings/workspace/FocusForm"
import { RetentionForm } from "@/features/settings/workspace/RetentionForm"
import { useSession } from "@/lib/session"

export function WorkspacePage() {
  const { membership } = useSession()
  return (
    <>
      <GeneralHeader current="workspace" />
      <Intro>
        <Trans>Your workspace name, how long closed conversations are kept, and who may send broadcasts.</Trans>
      </Intro>
      <RetentionForm key={membership.workspace.retention_days ?? 0} />
      <FocusForm key={membership.workspace.focus ?? ""} />
      <BroadcastsSettings />
      <DeleteWorkspace />
    </>
  )
}
