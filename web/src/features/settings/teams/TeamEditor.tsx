import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { AssignmentFields, capValid } from "@/components/common/AssignmentFields"
import { ErrorLine } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, unwrap, type AssignmentMode, type Team } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes, useMembers, useTeams } from "@/lib/workspace"

const MAX_MEMBERS = 500
const MAX_INBOXES = 100

function Details({
  team,
  onSaved,
  onClose,
}: {
  team: Team | null
  onSaved: (id: string) => void
  onClose: () => void
}) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [name, setName] = useState(team?.name ?? "")
  const [description, setDescription] = useState(team?.description ?? "")
  const [mode, setMode] = useState<AssignmentMode>(team?.assignment_mode ?? "off")
  const [cap, setCap] = useState(team?.assignment_cap ? String(team.assignment_cap) : "")
  const capNumber = cap === "" ? null : Number(cap)
  const save = useMutation({
    mutationFn: () =>
      team
        ? unwrap(
            api.PATCH("/v1/teams/{teamId}", {
              params: { path: { teamId: team.id } },
              body: { name, description, assignment_mode: mode, assignment_cap: capNumber },
            })
          )
        : unwrap(
            api.POST("/v1/teams", {
              body: {
                name,
                description,
                assignment_mode: mode,
                ...(capNumber === null ? {} : { assignment_cap: capNumber }),
              },
            })
          ),
    onSuccess: (saved) => {
      if (!team) qc.setQueryData<Team[]>(keys.teams(ws), (old) => (old ? [...old, saved] : old))
      void qc.invalidateQueries({ queryKey: keys.teams(ws) })
      if (team) onClose()
      else onSaved(saved.id)
    },
  })
  const changed =
    !team ||
    name !== team.name ||
    description !== team.description ||
    mode !== team.assignment_mode ||
    capNumber !== (team.assignment_cap ?? null)
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="team-name">
            <Trans>Name</Trans>
          </FieldLabel>
          <Input
            id="team-name"
            required
            autoFocus={!team}
            maxLength={64}
            value={name}
            placeholder={t`Billing`}
            onChange={(e) => setName(e.target.value)}
            className="h-9"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="team-description">
            <Trans>Description</Trans>
          </FieldLabel>
          <Input
            id="team-description"
            maxLength={500}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="h-9"
          />
        </Field>
      </div>
      <AssignmentFields scope="team" mode={mode} cap={cap} onMode={setMode} onCap={setCap} />
      <ErrorLine error={save.error} />
      <div className="flex flex-wrap gap-2">
        <Button
          type="submit"
          disabled={save.isPending || name.trim() === "" || !changed || !capValid(cap)}
          data-testid="team-save"
        >
          {team ? <Trans>Save</Trans> : <Trans>Create team</Trans>}
        </Button>
        <Button type="button" variant="outline" onClick={onClose}>
          {team ? <Trans>Close</Trans> : <Trans>Cancel</Trans>}
        </Button>
      </div>
    </form>
  )
}

function Picks({
  title,
  count,
  limit,
  children,
}: {
  title: React.ReactNode
  count: number
  limit: number
  children: React.ReactNode
}) {
  return (
    <fieldset className="flex min-w-0 flex-col gap-2">
      <legend className="mb-1 flex w-full items-center justify-between text-body font-medium">
        {title}
        <span className="text-caption font-normal text-faint tabular-nums">
          {count}/{limit}
        </span>
      </legend>
      <ul className="max-h-48 divide-y overflow-y-auto rounded-xl border">{children}</ul>
    </fieldset>
  )
}

function Pick({
  checked,
  disabled,
  onChange,
  label,
  detail,
  testId,
}: {
  checked: boolean
  disabled: boolean
  onChange: (on: boolean) => void
  label: string
  detail?: string
  testId: string
}) {
  return (
    <li>
      <Label className="gap-3 px-3.5 py-2.5 font-normal">
        <Checkbox checked={checked} disabled={disabled} onCheckedChange={onChange} data-testid={testId} />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-body">{label}</span>
          {detail && <span className="truncate text-caption text-faint">{detail}</span>}
        </span>
      </Label>
    </li>
  )
}

function Access({ team }: { team: Team }) {
  const text = useEnumText()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const members = useMembers().data ?? []
  const inboxes = useInboxes().data ?? []
  const toggle = useMutation({
    mutationFn: ({ kind, id, on }: { kind: "members" | "inboxes"; id: string; on: boolean }) => {
      if (kind === "members") {
        const params = { params: { path: { teamId: team.id, memberId: id } } }
        return on
          ? unwrap(api.PUT("/v1/teams/{teamId}/members/{memberId}", params))
          : unwrap(api.DELETE("/v1/teams/{teamId}/members/{memberId}", params))
      }
      const params = { params: { path: { teamId: team.id, inboxId: id } } }
      return on
        ? unwrap(api.PUT("/v1/teams/{teamId}/inboxes/{inboxId}", params))
        : unwrap(api.DELETE("/v1/teams/{teamId}/inboxes/{inboxId}", params))
    },
    onSettled: () => qc.invalidateQueries({ queryKey: keys.teams(ws) }),
  })
  return (
    <div className="grid gap-5 sm:grid-cols-2">
      <Picks title={<Trans>Members</Trans>} count={team.member_ids.length} limit={MAX_MEMBERS}>
        {members.map((m) => (
          <Pick
            key={m.id}
            checked={team.member_ids.includes(m.id)}
            disabled={toggle.isPending}
            onChange={(on) => toggle.mutate({ kind: "members", id: m.id, on })}
            label={m.name || m.email}
            detail={text.role[m.role]}
            testId="team-member"
          />
        ))}
      </Picks>
      <Picks title={<Trans>Inbox access</Trans>} count={team.inbox_ids.length} limit={MAX_INBOXES}>
        {inboxes.map((i) => (
          <Pick
            key={i.id}
            checked={team.inbox_ids.includes(i.id)}
            disabled={toggle.isPending}
            onChange={(on) => toggle.mutate({ kind: "inboxes", id: i.id, on })}
            label={i.name}
            testId="team-inbox"
          />
        ))}
      </Picks>
      <ErrorLine error={toggle.error} />
    </div>
  )
}

export function TeamEditor({
  teamId,
  onSaved,
  onClose,
}: {
  teamId: string | null
  onSaved: (id: string) => void
  onClose: () => void
}) {
  const team = useTeams().data?.find((x) => x.id === teamId) ?? null
  return (
    <Card className="mt-4 gap-0 py-0 shadow-none" data-testid="team-editor">
      <CardHeader className="border-b px-4 py-3 [.border-b]:pb-3">
        <CardTitle className="text-[15px] font-semibold">
          {teamId ? <Trans>Edit team</Trans> : <Trans>New team</Trans>}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-6 px-4 py-4">
        {(teamId === null || team) && (
          <Details key={team?.id ?? "new"} team={team} onSaved={onSaved} onClose={onClose} />
        )}
        {team && <Access team={team} />}
      </CardContent>
    </Card>
  )
}
