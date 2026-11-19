import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { FormCard } from "@/components/common/FormCard"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useComponentMutations, useGroupMutations } from "@/features/status/queries"
import { StateSelect } from "@/features/status/StateSelect"
import { Field, ToggleRow } from "@/features/settings/ui"
import type { StatusComponent, StatusComponentGroup, StatusComponentStateValue } from "@/lib/api"

const NONE = "none"

export function GroupForm({
  pageId,
  edit,
  count,
  onClose,
  className,
}: {
  className?: string
  pageId: string
  edit?: StatusComponentGroup
  count: number
  onClose: () => void
}) {
  const m = useGroupMutations(pageId)
  const [name, setName] = useState(edit?.name ?? "")
  const pending = m.create.isPending || m.update.isPending
  const submit = () => {
    if (!name.trim()) return
    if (edit) m.update.mutate({ id: edit.id, name: name.trim() }, { onSuccess: onClose })
    else m.create.mutate({ name: name.trim(), position: count }, { onSuccess: onClose })
  }
  return (
    <FormCard
      title={edit ? <Trans>Rename group</Trans> : <Trans>Add group</Trans>}
      submit={edit ? <Trans>Save</Trans> : <Trans>Add group</Trans>}
      ready={!!name.trim()}
      busy={pending}
      error={m.create.error ?? m.update.error}
      onSubmit={submit}
      onCancel={onClose}
      className={className}
      testId="status-group-dialog"
    >
      <Field label={<Trans>Name</Trans>} htmlFor="status-group-name">
        <Input
          id="status-group-name"
          value={name}
          maxLength={100}
          autoFocus
          required
          onChange={(e) => setName(e.target.value)}
          data-testid="status-group-name"
        />
      </Field>
    </FormCard>
  )
}

export function ComponentForm({
  pageId,
  groups,
  edit,
  group,
  onClose,
  className,
}: {
  className?: string
  pageId: string
  groups: StatusComponentGroup[]
  edit?: StatusComponent
  group?: string
  onClose: () => void
}) {
  const { t } = useLingui()
  const m = useComponentMutations(pageId)
  const [name, setName] = useState(edit?.name ?? "")
  const [description, setDescription] = useState(edit?.description ?? "")
  const [groupId, setGroupId] = useState(edit?.group_id ?? group ?? NONE)
  const [hidden, setHidden] = useState(edit?.hidden ?? false)
  const [state, setState] = useState<StatusComponentStateValue>("operational")
  const pending = m.create.isPending || m.update.isPending
  const items = { [NONE]: t`No group`, ...Object.fromEntries(groups.map((g) => [g.id, g.name])) }
  const submit = () => {
    if (!name.trim()) return
    const gid = groupId === NONE ? null : groupId
    if (edit) {
      m.update.mutate(
        { id: edit.id, name: name.trim(), description: description.trim(), group_id: gid, hidden },
        { onSuccess: onClose }
      )
    } else {
      m.create.mutate(
        {
          name: name.trim(),
          description: description.trim() || undefined,
          group_id: gid ?? undefined,
          hidden,
          state,
        },
        { onSuccess: onClose }
      )
    }
  }
  return (
    <FormCard
      title={edit ? <Trans>Edit component</Trans> : <Trans>Add component</Trans>}
      submit={edit ? <Trans>Save</Trans> : <Trans>Add component</Trans>}
      ready={!!name.trim()}
      busy={pending}
      error={m.create.error ?? m.update.error}
      onSubmit={submit}
      onCancel={onClose}
      className={className}
      testId="status-component-dialog"
    >
      <Field label={<Trans>Name</Trans>} htmlFor="status-component-name">
        <Input
          id="status-component-name"
          value={name}
          maxLength={100}
          autoFocus
          required
          placeholder={t`API`}
          onChange={(e) => setName(e.target.value)}
          data-testid="status-component-name"
        />
      </Field>
      <Field
        label={<Trans>Description</Trans>}
        htmlFor="status-component-description"
        hint={<Trans>Optional. Shown on the public page.</Trans>}
      >
        <Input
          id="status-component-description"
          value={description}
          maxLength={500}
          onChange={(e) => setDescription(e.target.value)}
        />
      </Field>
      {groups.length > 0 && (
        <Field label={<Trans>Group</Trans>}>
          <Select value={groupId} items={items} onValueChange={(v) => v && setGroupId(String(v))}>
            <SelectTrigger className="w-full min-w-0" aria-label={t`Group`} data-testid="status-component-group">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE}>{t`No group`}</SelectItem>
              {groups.map((g) => (
                <SelectItem key={g.id} value={g.id}>
                  {g.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}
      {!edit && (
        <Field label={<Trans>State</Trans>}>
          <StateSelect value={state} onChange={setState} label={t`State`} className="w-full" />
        </Field>
      )}
      <ToggleRow title={<Trans>Hidden</Trans>} hint={<Trans>Left off the public page and its overall state.</Trans>}>
        <Switch checked={hidden} onCheckedChange={setHidden} />
      </ToggleRow>
    </FormCard>
  )
}
