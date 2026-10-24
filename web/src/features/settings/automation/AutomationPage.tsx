import { Trans } from "@lingui/react/macro"
import { Navigate } from "react-router"

import { RuleList } from "@/features/settings/automation/RuleList"
import { GeneralHeader } from "@/features/settings/GeneralTabs"
import { useSession } from "@/lib/session"

export function AutomationPage() {
  const { canManage } = useSession()
  if (!canManage) return <Navigate to="/settings/profile" replace />
  return (
    <>
      <GeneralHeader current="automation" />
      <RuleList
        intro={
          <Trans>
            Rules react to a conversation changing: they label, assign, reply, snooze or call a webhook. They run in the
            order of this list, each at most once per trigger, and what a rule changes never starts another rule.
          </Trans>
        }
      />
    </>
  )
}
