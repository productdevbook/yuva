import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { Segmented } from "@/components/common/Segmented"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useViewMutations } from "@/features/inbox/views/queries"
import { Field, ToggleRow } from "@/features/settings/ui"
import { ApiError, type SavedView, type SavedViewFilters, type SavedViewVisibility } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useMyTeams, useTeams } from "@/lib/workspace"

function useVisibilityNames() {
  const { t } = useLingui()
  return { personal: t`Only me`, workspace: t`Everyone`, team: t`Team` } satisfies Record<SavedViewVisibility, string>
}

export function ViewDialog({
  view,
  filters,
  onClose,
  onDone,
}: {
  view?: SavedView
  filters: SavedViewFilters
  onClose: () => void
  onDone: (view: SavedView) => void
}) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const names = useVisibilityNames()
  const myTeams = useMyTeams()
  const allTeams = useTeams().data ?? []
  const { create, update } = useViewMutations()
  const [name, setName] = useState(view?.name ?? "")
  const [visibility, setVisibility] = useState<SavedViewVisibility>("personal")
  const [teamId, setTeamId] = useState(myTeams[0]?.id ?? "")
  const [replace, setReplace] = useState(false)
  const pending = create.isPending || update.isPending
  const error = create.error ?? update.error
  const refused = error instanceof ApiError && error.field ? error : null
  const nameError = refused?.field === "name" ? refused.detail : undefined
  const field = refused?.field ?? ""
  const detail = refused?.detail ?? ""
  const filterError = refused && field !== "name" ? t`The ${field} filter was refused. ${detail}`.trim() : undefined
  const choices: SavedViewVisibility[] = canManage
    ? ["personal", "workspace", ...(myTeams.length > 0 ? (["team"] as const) : [])]
    : ["personal"]
  const items = Object.fromEntries(myTeams.map((x) => [x.id, x.name]))
  const teamName = allTeams.find((x) => x.id === view?.team_id)?.name
  const submit = () => {
    if (view) {
      update.mutate(
        { id: view.id, body: { name: name.trim(), ...(replace ? { filters } : {}) } },
        { onSuccess: onDone }
      )
      return
    }
    create.mutate(
      { visibility, name: name.trim(), filters, team_id: visibility === "team" ? teamId : undefined },
      { onSuccess: onDone }
    )
  }
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{view ? <Trans>Edit view</Trans> : <Trans>Save as view</Trans>}</DialogTitle>
          </DialogHeader>
          <Field
            label={<Trans>Name</Trans>}
            htmlFor="view-name"
            error={nameError}
            hint={view ? undefined : <Trans>Saves the list as it is now: its tab, filters, search and order.</Trans>}
          >
            <Input
              id="view-name"
              required
              autoFocus
              maxLength={80}
              value={name}
              onChange={(e) => setName(e.target.value)}
              data-testid="view-name"
            />
          </Field>
          {view ? (
            <>
              <Field label={<Trans>Visible to</Trans>}>
                <p className="text-body" data-testid="view-visibility">
                  {view.visibility === "team" && teamName ? `${names.team}: ${teamName}` : names[view.visibility]}
                </p>
              </Field>
              <ToggleRow
                title={<Trans>Use the current list</Trans>}
                hint={<Trans>Replace the view’s filters, search and order with the ones on screen.</Trans>}
              >
                <Switch checked={replace} onCheckedChange={setReplace} data-testid="view-replace" />
              </ToggleRow>
            </>
          ) : (
            choices.length > 1 && (
              <>
                <Field label={<Trans>Visible to</Trans>}>
                  <Segmented
                    value={visibility}
                    onChange={setVisibility}
                    label={t`Visible to`}
                    items={choices.map((c) => ({ value: c, label: names[c], testId: `view-visibility-${c}` }))}
                  />
                </Field>
                {visibility === "team" && (
                  <Field label={<Trans>Team</Trans>}>
                    <Select value={teamId} items={items} onValueChange={(v) => v && setTeamId(String(v))}>
                      <SelectTrigger className="w-full min-w-0" aria-label={t`Team`} data-testid="view-team">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {myTeams.map((x) => (
                          <SelectItem key={x.id} value={x.id}>
                            {x.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </Field>
                )}
              </>
            )
          )}
          {filterError && (
            <p role="alert" className="text-small text-destructive" data-testid="view-filter-error">
              {filterError}
            </p>
          )}
          {!refused && <ErrorLine error={error} />}
          <DialogFooter>
            <Button type="submit" disabled={pending || !name.trim()} data-testid="view-save">
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
