import { useLingui } from "@lingui/react/macro"

import type { AutomationRuleAuthor } from "@/lib/api"
import { useMemberMap } from "@/lib/workspace"

export function useAuthorText() {
  const { t } = useLingui()
  const members = useMemberMap()
  return (a: AutomationRuleAuthor | undefined): string => {
    if (!a) return t`Unknown`
    if (a.type === "api_key") return t`An API key`
    const m = a.member_id ? members.get(a.member_id) : undefined
    const name = m ? m.name || m.email : t`a former member`
    return a.type === "oauth_grant" ? t`An assistant connected by ${name}` : name
  }
}
