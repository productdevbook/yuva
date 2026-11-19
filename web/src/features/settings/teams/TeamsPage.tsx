import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, PageLayout, useConfirm } from "@/components/common"
import { ListBox, ListBoxEmpty, ListBoxSkeleton, PageTitle } from "@/components/common/ListBox"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { TeamEditor } from "@/features/settings/teams/TeamEditor"
import { api, unwrap, type AssignmentMode } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMembers, useTeams } from "@/lib/workspace"

const head = "h-9 px-3 text-start text-caption font-medium text-muted-foreground"

export function TeamsPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const teams = useTeams()
  const members = new Map((useMembers().data ?? []).map((m) => [m.id, m]))
  const inboxes = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const [editing, setEditing] = useState<string | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/teams/{teamId}", { params: { path: { teamId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.teams(ws) }),
  })
  const modes: Record<AssignmentMode, string> = {
    off: t`Off`,
    round_robin: t`Round robin`,
    least_open: t`Fewest open conversations`,
  }
  const open = (id: string) => {
    if (!canManage) return
    setEditing(id)
    window.scrollTo({ top: 0 })
  }
  return (
    <PageLayout testId="teams-page" label={t`Teams`}>
      <section className="@container/list min-w-0" aria-label={t`Teams`}>
        <PageTitle
          title={t`Teams`}
          sub={t`A team gives its members the inboxes it covers`}
          actions={
            canManage && (
              <Button onClick={() => setEditing("new")} disabled={editing === "new"} data-testid="team-new">
                <PlusIcon />
                <Trans>New team</Trans>
              </Button>
            )
          }
          testId="teams-head"
        />
        {editing && (
          <TeamEditor
            key={editing}
            teamId={editing === "new" ? null : editing}
            onSaved={setEditing}
            onClose={() => setEditing(null)}
          />
        )}
        <ListBox testId="teams-list">
          {teams.isPending ? (
            <ListBoxSkeleton rows={2} />
          ) : (teams.data ?? []).length === 0 ? (
            <ListBoxEmpty
              testId="teams-empty"
              action={
                canManage &&
                !editing && (
                  <Button variant="outline" size="sm" onClick={() => setEditing("new")}>
                    <Trans>New team</Trans>
                  </Button>
                )
              }
            >
              <span className="block font-medium text-foreground">
                <Trans>No teams yet</Trans>
              </span>
              <Trans>Group members, such as billing or support, and give the group its inboxes in one place.</Trans>
            </ListBoxEmpty>
          ) : (
            <Table className="table-fixed text-body">
              <TableHeader>
                <TableRow className="border-line-soft hover:bg-transparent">
                  <TableHead className={cn(head, "ps-4")}>
                    <Trans>Team</Trans>
                  </TableHead>
                  <TableHead className={cn(head, "w-[26%] @max-xl/list:hidden")}>
                    <Trans>Members</Trans>
                  </TableHead>
                  <TableHead className={cn(head, "w-[20%] @max-3xl/list:hidden")}>
                    <Trans>Inbox access</Trans>
                  </TableHead>
                  <TableHead className={cn(head, "w-44 @max-4xl/list:hidden")}>
                    <Trans>Automatic assignment</Trans>
                  </TableHead>
                  {canManage && (
                    <TableHead className={cn(head, "w-24 pe-4")}>
                      <span className="sr-only">
                        <Trans>Actions</Trans>
                      </span>
                    </TableHead>
                  )}
                </TableRow>
              </TableHeader>
              <TableBody>
                {(teams.data ?? []).map((team) => {
                  const who = team.member_ids
                    .map((id) => members.get(id))
                    .map((m) => (m ? m.name || m.email : ""))
                    .filter(Boolean)
                  const where = team.inbox_ids.map((id) => inboxes.get(id)?.name).filter(Boolean)
                  const name = team.name
                  return (
                    <TableRow
                      key={team.id}
                      onClick={() => open(team.id)}
                      className={cn(
                        "border-line-soft hover:bg-subtle",
                        canManage && "cursor-pointer",
                        editing === team.id && "bg-subtle"
                      )}
                      data-testid="team-row"
                    >
                      <TableCell className="min-w-0 overflow-hidden px-4 py-2.5">
                        <span className="flex min-w-0 flex-col leading-5">
                          <span className="truncate font-medium text-foreground">{team.name}</span>
                          <span className="truncate text-small text-muted-foreground">
                            {team.description || (
                              <>
                                <Plural value={team.member_ids.length} one="# member" other="# members" />
                                {" · "}
                                <Plural value={team.inbox_ids.length} one="# inbox" other="# inboxes" />
                              </>
                            )}
                          </span>
                        </span>
                      </TableCell>
                      <TableCell className="truncate px-3 py-2.5 text-small text-fg2 @max-xl/list:hidden">
                        {who.length > 0 ? who.join(", ") : "–"}
                      </TableCell>
                      <TableCell className="truncate px-3 py-2.5 text-small text-fg2 @max-3xl/list:hidden">
                        {where.length > 0 ? where.join(", ") : "–"}
                      </TableCell>
                      <TableCell className="truncate px-3 py-2.5 text-small text-muted-foreground @max-4xl/list:hidden">
                        {modes[team.assignment_mode]}
                      </TableCell>
                      {canManage && (
                        <TableCell className="px-3 py-1.5 pe-4 text-end" onClick={(e) => e.stopPropagation()}>
                          <span className="inline-flex gap-0.5">
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={t`Edit ${name}`}
                              title={t`Edit`}
                              onClick={() => open(team.id)}
                              data-testid="team-edit"
                            >
                              <PencilIcon />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              className="text-destructive hover:text-destructive"
                              aria-label={t`Delete ${name}`}
                              title={t`Delete`}
                              onClick={() =>
                                confirm({
                                  title: <Trans>Delete the team {name}?</Trans>,
                                  description: (
                                    <Trans>
                                      Its members lose the inbox access the team gave them, and its conversations lose
                                      the team but keep their assignee.
                                    </Trans>
                                  ),
                                  confirm: <Trans>Delete</Trans>,
                                  run: () => {
                                    if (editing === team.id) setEditing(null)
                                    remove.mutate(team.id)
                                  },
                                })
                              }
                            >
                              <Trash2Icon />
                            </Button>
                          </span>
                        </TableCell>
                      )}
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </ListBox>
        <ErrorLine error={teams.error ?? remove.error} className="py-2" />
      </section>
      {confirmDialog}
    </PageLayout>
  )
}
