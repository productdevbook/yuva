import { Trans } from "@lingui/react/macro"

import { useSettingsAbout } from "@/features/settings/SettingsFrame"
import { Page, PageHeader } from "@/features/settings/ui"
import { BroadcastsSettings } from "@/features/settings/workspace/BroadcastsSettings"
import { DeleteWorkspace } from "@/features/settings/workspace/DeleteWorkspace"
import { FocusForm } from "@/features/settings/workspace/FocusForm"
import { RetentionForm } from "@/features/settings/workspace/RetentionForm"
import { useSession } from "@/lib/session"

export function WorkspacePage() {
  const { membership } = useSession()
  const about = useSettingsAbout()
  return (
    <Page testId="settings" width="form" className="@container gap-7">
      <PageHeader back={false} title={<Trans>General</Trans>} description={about.workspace} />
      <RetentionForm key={`r${membership.workspace.retention_days ?? 0}`} />
      <FocusForm key={`f${membership.workspace.focus ?? ""}`} />
      <BroadcastsSettings />
      <DeleteWorkspace />
    </Page>
  )
}
