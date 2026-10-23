import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine, Segmented } from "@/components/common"
import { STATUSES, PRIORITIES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { slugify } from "@/features/settings/inboxes/slugify"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import {
  api,
  unwrap,
  type ConversationStatus,
  type Macro,
  type MacroActions,
  type MacroVisibility,
  type Priority,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useLabels, useMembers } from "@/lib/workspace"

type LabelMode = "keep" | "add" | "remove"
type StatusChoice = "keep" | ConversationStatus
type AssigneeChoice = "keep" | "none" | string
type PriorityChoice = "keep" | Priority

function labelModes(a: MacroActions) {
  const modes = new Map<string, LabelMode>()
  for (const id of a.add_labels ?? []) modes.set(id, "add")
  for (const id of a.remove_labels ?? []) modes.set(id, "remove")
  return modes
}

function assigneeOf(a: MacroActions): AssigneeChoice {
  return a.assignee_id === undefined ? "keep" : a.assignee_id === null ? "none" : a.assignee_id
}

export function MacroDialog({ macro, onClose }: { macro: Macro | null; onClose: () => void }) {
  const { t } = useLingui()
  const text = useEnumText()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const labels = useLabels().data ?? []
  const members = useMembers().data ?? []
  const [visibility, setVisibility] = useState<MacroVisibility>(macro?.visibility ?? "personal")
  const [name, setName] = useState(macro?.name ?? "")
  const [shortcut, setShortcut] = useState(macro?.shortcut ?? "")
  const [touched, setTouched] = useState(!!macro)
  const [body, setBody] = useState(macro?.body ?? "")
  const [modes, setModes] = useState(() => labelModes(macro?.actions ?? {}))
  const [assignee, setAssignee] = useState<AssigneeChoice>(assigneeOf(macro?.actions ?? {}))
  const [status, setStatus] = useState<StatusChoice>(
    macro?.actions.snooze_minutes !== undefined ? "snoozed" : (macro?.actions.status ?? "keep")
  )
  const [minutes, setMinutes] = useState(String(macro?.actions.snooze_minutes ?? 60))
  const [priority, setPriority] = useState<PriorityChoice>(macro?.actions.priority ?? "keep")
  const valid = /^[a-z0-9]+(-[a-z0-9]+)*$/.test(shortcut)
  const snooze = Number(minutes)
  const snoozeOk = status !== "snoozed" || (Number.isInteger(snooze) && snooze >= 1 && snooze <= 525600)

  const actions: MacroActions = {}
  const add = labels.filter((l) => modes.get(l.id) === "add").map((l) => l.id)
  const drop = labels.filter((l) => modes.get(l.id) === "remove").map((l) => l.id)
  if (add.length) actions.add_labels = add
  if (drop.length) actions.remove_labels = drop
  if (assignee === "none") actions.assignee_id = null
  else if (assignee !== "keep") actions.assignee_id = assignee
  if (status === "snoozed") {
    actions.status = "snoozed"
    actions.snooze_minutes = snooze
  } else if (status !== "keep") actions.status = status
  if (priority !== "keep") actions.priority = priority
  const empty = body.trim() === "" && Object.keys(actions).length === 0

  const save = useMutation({
    mutationFn: () =>
      macro
        ? unwrap(
            api.PATCH("/v1/macros/{macroId}", {
              params: { path: { macroId: macro.id } },
              body: { name, shortcut, body: body.trim() ? body : null, actions },
            })
          )
        : unwrap(
            api.POST("/v1/macros", {
              body: { visibility, name, shortcut, body: body.trim() ? body : undefined, actions },
            })
          ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.macros(ws) })
      onClose()
    },
  })

  const keep = t`Leave as is`
  const labelOptions = [
    ["keep", keep],
    ["add", t`Add`],
    ["remove", t`Remove`],
  ] as const
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>{macro ? <Trans>Edit macro</Trans> : <Trans>New macro</Trans>}</DialogTitle>
          </DialogHeader>
          {!macro && canManage && (
            <Field label={<Trans>Who can use it</Trans>}>
              <Segmented
                value={visibility}
                onChange={setVisibility}
                label={t`Who can use it`}
                items={[
                  { value: "personal", label: <Trans>Only me</Trans> },
                  { value: "shared", label: <Trans>Everyone</Trans> },
                ]}
              />
            </Field>
          )}
          <Field label={<Trans>Name</Trans>} htmlFor="macro-name">
            <Input
              id="macro-name"
              required
              maxLength={200}
              value={name}
              placeholder={t`Refund approved`}
              onChange={(e) => {
                setName(e.target.value)
                if (!touched) setShortcut(slugify(e.target.value))
              }}
            />
          </Field>
          <Field
            label={<Trans>Shortcut</Trans>}
            htmlFor="macro-shortcut"
            hint={<Trans>Lowercase letters, digits and dashes.</Trans>}
            error={
              shortcut !== "" && !valid ? <Trans>Use lowercase letters, digits and single dashes.</Trans> : undefined
            }
          >
            <div className="relative">
              <span aria-hidden className="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-faint">
                /
              </span>
              <Input
                id="macro-shortcut"
                required
                maxLength={64}
                value={shortcut}
                aria-invalid={(shortcut !== "" && !valid) || undefined}
                className="ps-6 font-mono"
                onChange={(e) => {
                  setTouched(true)
                  setShortcut(e.target.value)
                }}
              />
            </div>
          </Field>
          <Field label={<Trans>Text</Trans>} htmlFor="macro-body">
            <Textarea id="macro-body" rows={5} value={body} onChange={(e) => setBody(e.target.value)} />
          </Field>
          <fieldset className="flex flex-col gap-4 rounded-xl border p-4">
            <legend className="px-1.5 text-body font-medium">
              <Trans>Actions</Trans>
            </legend>
            {labels.length > 0 && (
              <div className="flex flex-col gap-2">
                <Label>
                  <Trans>Labels</Trans>
                </Label>
                <ul className="max-h-40 divide-y overflow-y-auto rounded-lg border">
                  {labels.map((l) => (
                    <li key={l.id} className="flex items-center gap-3 px-3 py-1.5">
                      <span className="min-w-0 flex-1 truncate text-body">{l.name}</span>
                      <ChoiceSelect<LabelMode>
                        value={modes.get(l.id) ?? "keep"}
                        label={l.name}
                        options={labelOptions}
                        className="w-36"
                        onChange={(mode) => setModes((all) => new Map(all).set(l.id, mode))}
                      />
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <Field label={<Trans>Assignee</Trans>}>
              <ChoiceSelect<AssigneeChoice>
                value={assignee}
                label={t`Assignee`}
                options={[
                  ["keep", keep],
                  ["none", t`Unassigned`],
                  ...members.map((m) => [m.id, m.name || m.email] as const),
                ]}
                onChange={setAssignee}
              />
            </Field>
            <Field label={<Trans>Status</Trans>}>
              <ChoiceSelect<StatusChoice>
                value={status}
                label={t`Status`}
                options={[["keep", keep], ...STATUSES.map((s) => [s, text.status[s]] as const)]}
                onChange={setStatus}
              />
            </Field>
            {status === "snoozed" && (
              <Field label={<Trans>Snooze for (minutes)</Trans>} htmlFor="macro-minutes">
                <Input
                  id="macro-minutes"
                  type="number"
                  min={1}
                  max={525600}
                  value={minutes}
                  aria-invalid={!snoozeOk || undefined}
                  onChange={(e) => setMinutes(e.target.value)}
                  className="max-w-40"
                />
              </Field>
            )}
            <Field label={<Trans>Priority</Trans>}>
              <ChoiceSelect<PriorityChoice>
                value={priority}
                label={t`Priority`}
                options={[["keep", keep], ...PRIORITIES.map((p) => [p, text.priority[p]] as const)]}
                onChange={setPriority}
              />
            </Field>
          </fieldset>
          <ErrorLine error={save.error} />
          <DialogFooter>
            <Button type="submit" disabled={save.isPending || !valid || !snoozeOk || empty} data-testid="macro-save">
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
