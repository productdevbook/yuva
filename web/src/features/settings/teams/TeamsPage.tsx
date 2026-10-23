import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, Trash2Icon, UsersRoundIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Button } from "@/components/ui/button"
import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { TeamDialog } from "@/features/settings/teams/TeamDialog"
import { Card, headButton, Row, Rows } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useTeams } from "@/lib/workspace"

export function TeamsPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const teams = useTeams()
  const [editing, setEditing] = useState<string | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/teams/{teamId}", { params: { path: { teamId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.teams(ws) }),
  })
  const add = canManage && (
    <Button className={headButton.primary} onClick={() => setEditing("new")} data-testid="team-new">
      <Trans>New team</Trans>
    </Button>
  )
  return (
    <>
      <GeneralHeader current="teams" />
      <Intro action={teams.data && teams.data.length > 0 ? add : undefined}>
        <Trans>
          A team gives its members the inboxes it covers. A conversation can go to a team as well as to a member.
        </Trans>
      </Intro>
      {teams.data && teams.data.length === 0 ? (
        <EmptyBox icon={UsersRoundIcon} title={<Trans>No teams yet</Trans>} actions={add} testId="teams-empty">
          <Trans>Group members, such as billing or support, and give the group its inboxes in one place.</Trans>
        </EmptyBox>
      ) : (
        <Card flush>
          <Rows>
            {(teams.data ?? []).map((team) => (
              <Row key={team.id} className="items-start" data-testid="team-row">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-body font-medium">{team.name}</p>
                  <p className="truncate text-caption text-muted-foreground">
                    <Trans>Members</Trans> {team.member_ids.length}
                    {" · "}
                    <Trans>Inboxes</Trans> {team.inbox_ids.length}
                    {team.description && ` · ${team.description}`}
                  </p>
                </div>
                {canManage && (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={() => setEditing(team.id)}>
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t`Delete`}
                      onClick={() => {
                        const name = team.name
                        confirm({
                          title: <Trans>Delete the team {name}?</Trans>,
                          description: (
                            <Trans>
                              Its members lose the inbox access the team gave them, and its conversations lose the team
                              but keep their assignee.
                            </Trans>
                          ),
                          confirm: <Trans>Delete</Trans>,
                          run: () => remove.mutate(team.id),
                        })
                      }}
                    >
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </Row>
            ))}
          </Rows>
          <ErrorLine error={teams.error ?? remove.error} className="px-5 pb-4" />
        </Card>
      )}
      {editing && (
        <TeamDialog teamId={editing === "new" ? null : editing} onSaved={setEditing} onClose={() => setEditing(null)} />
      )}
      {confirmDialog}
    </>
  )
}
