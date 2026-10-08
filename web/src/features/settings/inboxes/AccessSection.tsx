import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { Checkbox } from "@/components/ui/checkbox"
import { useInboxOutlet } from "@/features/settings/inboxes/InboxLayout"
import { Card, EmptyRow, Rows, RowText, Section } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxMembers, useMembers } from "@/lib/workspace"

export function AccessSection() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { inbox } = useInboxOutlet()
  const { workspaceId: ws, canManage } = useSession()
  const members = useMembers().data ?? []
  const granted = new Set((useInboxMembers(inbox.id).data ?? []).map((m) => m.id))
  const toggle = useMutation({
    mutationFn: ({ memberId, on }: { memberId: string; on: boolean }) => {
      const params = { params: { path: { inboxId: inbox.id, memberId } } }
      return on
        ? unwrap(api.PUT("/v1/inboxes/{inboxId}/members/{memberId}", params))
        : unwrap(api.DELETE("/v1/inboxes/{inboxId}/members/{memberId}", params))
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.inboxMembers(ws, inbox.id) }),
  })
  const agents = members.filter((m) => m.role === "agent")
  return (
    <Section title={<Trans>Access</Trans>} description={<Trans>Agents see this inbox only when they are given access. Owners and admins always see it.</Trans>}>
      <Card flush>
        {agents.length === 0 ? (
          <EmptyRow>
            <Trans>There are no agents in this workspace.</Trans>
          </EmptyRow>
        ) : (
          <Rows>
            {agents.map((m) => {
              const name = m.name || m.email
              return (
                <li key={m.id}>
                  <label className="flex items-center gap-3 px-5 py-3.5">
                    <Checkbox
                      checked={granted.has(m.id)}
                      disabled={!canManage || toggle.isPending}
                      onCheckedChange={(on) => toggle.mutate({ memberId: m.id, on })}
                      aria-label={t`Access for ${name}`}
                    />
                    <PersonAvatar name={name} className="size-8" />
                    <RowText title={name} detail={m.email} />
                  </label>
                </li>
              )
            })}
          </Rows>
        )}
        <ErrorLine error={toggle.error} className="px-5 pb-4" />
      </Card>
    </Section>
  )
}
