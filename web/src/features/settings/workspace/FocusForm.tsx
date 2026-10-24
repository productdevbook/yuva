import { Trans } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { FocusChoice } from "@/components/common/FocusChoice"
import { Button } from "@/components/ui/button"
import { Block, headButton } from "@/features/settings/ui"
import { useSaveFocus } from "@/features/settings/workspace/focus"
import type { WorkspaceFocus } from "@/lib/api"
import { useSession } from "@/lib/session"

export function FocusForm() {
  const { membership, canManage } = useSession()
  const current = membership.workspace.focus
  const [picked, setPicked] = useState<WorkspaceFocus | undefined>(current)
  const save = useSaveFocus()
  const dirty = picked !== current
  return (
    <Block
      title={<Trans>What Yuva is for</Trans>}
      description={<Trans>Sets what you see first: the Setup cards, the sidebar and the page Yuva opens on.</Trans>}
      onSubmit={() => picked && save.mutate(picked)}
      testId="workspace-focus"
      footer={
        canManage ? (
          <>
            <ErrorLine error={save.error} className="min-w-0 flex-1" />
            {save.isSuccess && !dirty && !save.error && (
              <span className="flex-1 text-muted-foreground" role="status">
                <Trans>Saved</Trans>
              </span>
            )}
            <Button type="submit" className={headButton.primary} disabled={save.isPending || !dirty || !picked}>
              <Trans>Save</Trans>
            </Button>
          </>
        ) : (
          <p className="me-auto text-muted-foreground">
            <Trans>Only owners and admins can change this.</Trans>
          </p>
        )
      }
    >
      <div className="px-[18px] py-4">
        <FocusChoice value={picked} onChange={setPicked} disabled={!canManage} />
      </div>
    </Block>
  )
}
