import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon } from "lucide-react"

import { ErrorLine, PersonAvatar, useConfirm } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { RoleSelect } from "@/features/settings/members/RoleSelect"
import { Card, Row, Rows, RowText, Section } from "@/features/settings/ui"
import { api, unwrap, type Member, type Role } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useMembers } from "@/lib/workspace"

export function MemberList() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const text = useEnumText()
  const { workspaceId: ws, membership, canManage } = useSession()
  const members = useMembers()
  const [confirm, confirmDialog] = useConfirm()
  const isOwner = membership.role === "owner"
  const refresh = () => qc.invalidateQueries({ queryKey: keys.members(ws) })
  const setRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) =>
      unwrap(api.PATCH("/v1/members/{memberId}", { params: { path: { memberId: id } }, body: { role } })),
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/members/{memberId}", { params: { path: { memberId: id } } })),
    onSuccess: refresh,
  })
  const editable = (m: Member) => canManage && (isOwner || m.role !== "owner")
  return (
    <Section title={<Trans>Workspace members</Trans>}>
      <Card flush>
        <Rows>
          {(members.data ?? []).map((m) => {
            const name = m.name || m.email
            const you = m.id === membership.member_id
            return (
              <Row key={m.id} className="flex-wrap" data-testid="member-row">
                <PersonAvatar name={name} className="size-8" />
                <RowText
                  title={
                    <>
                      {name}
                      {you && (
                        <span className="font-normal text-faint">
                          {" "}
                          <Trans>(you)</Trans>
                        </span>
                      )}
                    </>
                  }
                  detail={m.email}
                />
                {editable(m) ? (
                  <RoleSelect value={m.role} allowOwner={isOwner} label={t`Role of ${name}`} onChange={(role) => setRole.mutate({ id: m.id, role })} />
                ) : (
                  <span className="w-32 text-sm text-muted-foreground">{text.role[m.role]}</span>
                )}
                {editable(m) && !you ? (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t`Remove ${name}`}
                    onClick={() =>
                      confirm({
                        title: <Trans>Remove {name} from the workspace?</Trans>,
                        description: <Trans>Their conversations stay; they lose access at once.</Trans>,
                        confirm: <Trans>Remove</Trans>,
                        run: () => remove.mutate(m.id),
                      })
                    }
                  >
                    <Trash2Icon />
                  </Button>
                ) : (
                  <span className="size-8" />
                )}
              </Row>
            )
          })}
        </Rows>
        <ErrorLine error={members.error ?? setRole.error ?? remove.error} className="px-5 pb-4" />
      </Card>
      {confirmDialog}
    </Section>
  )
}
