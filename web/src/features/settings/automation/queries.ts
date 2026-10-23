import { useInfiniteQuery, useQuery } from "@tanstack/react-query"

import type { AutomationRule, AutomationRuleAuthor, AutomationRunOutcome, CannedReply } from "@/lib/api"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useMemberMap } from "@/lib/workspace"
import { useWorkspaceSettings } from "@/features/settings/connected-apps/queries"

export function useAutomationRules() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.automationRules(ws),
    queryFn: () => unwrap(api.GET("/v1/automation-rules")).then((r) => r.items),
    staleTime: 30_000,
  })
}

export function useRuleRuns(ruleId: string, outcome: AutomationRunOutcome | "all") {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.automationRuns(ws, ruleId, outcome),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/automation-rules/{automationRuleId}/runs", {
          params: {
            path: { automationRuleId: ruleId },
            query: { cursor: pageParam, limit: 25, ...(outcome !== "all" ? { outcome } : {}) },
          },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useAuthorityCheck() {
  const members = useMemberMap()
  const botsMaySend = useWorkspaceSettings().data?.bots_may_send
  const isManager = (a: AutomationRuleAuthor | undefined) => {
    if (a?.type !== "member" || !a.member_id) return false
    const role = members.get(a.member_id)?.role
    return role === "owner" || role === "admin"
  }
  return {
    botsMaySend: botsMaySend ?? true,
    isManager,
    problem: (rule: Pick<AutomationRule, "actions" | "saved_by">, replies: CannedReply[]) => {
      if (botsMaySend !== false) return null
      const send = rule.actions.find((a) => a.type === "send_reply")
      if (!send) return null
      const reply = replies.find((r) => r.id === send.canned_reply_id)
      const ruleBad = !isManager(rule.saved_by)
      const replyBad = !!reply && !isManager(reply.saved_by)
      if (!ruleBad && !replyBad) return null
      return { rule: ruleBad, reply: replyBad ? reply : undefined }
    },
  }
}
