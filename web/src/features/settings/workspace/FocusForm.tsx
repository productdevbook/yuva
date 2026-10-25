import { Trans } from "@lingui/react/macro"
import { useState } from "react"

import { FocusChoice } from "@/components/common/FocusChoice"
import { Pill } from "@/features/settings/ui"
import { Sec, SecBody, SecFoot, secCard } from "@/features/settings/workspace/parts"
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
    <Sec
      id="w-focus"
      title={<Trans>What Yuva is for</Trans>}
      description={<Trans>Sets what you see first: the Setup cards, the sidebar and the page Yuva opens on.</Trans>}
      pill={
        current && (
          <Pill tone="muted">
            {current === "support" ? (
              <Trans>Support</Trans>
            ) : current === "mail" ? (
              <Trans>Team mail</Trans>
            ) : (
              <Trans>Both</Trans>
            )}
          </Pill>
        )
      }
    >
      <form
        className={secCard}
        data-testid="workspace-focus"
        onSubmit={(e) => {
          e.preventDefault()
          if (picked) save.mutate(picked)
        }}
      >
        <SecBody>
          <FocusChoice value={picked} onChange={setPicked} disabled={!canManage} cards />
        </SecBody>
        <SecFoot
          dirty={dirty}
          pending={save.isPending}
          saved={save.isSuccess}
          error={save.error}
          invalid={!picked}
          readOnly={canManage ? undefined : <Trans>Only owners and admins can change this.</Trans>}
          onDiscard={() => {
            setPicked(current)
            save.reset()
          }}
        />
      </form>
    </Sec>
  )
}
