import { isPersonal } from "@/features/mail/folders"
import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, TriangleAlertIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, Notice } from "@/components/common"
import { ChipGroup } from "@/components/common/ChipGroup"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { ActionRow } from "@/features/settings/automation/ActionRow"
import { ConditionRow } from "@/features/settings/automation/ConditionRow"
import {
  actionTypes,
  blankAction,
  blankCondition,
  blocked,
  conditionFields,
  fromRule,
  idleValid,
  isComplete,
  sanitize,
  toBody,
  type Draft,
} from "@/features/settings/automation/draft"
import { useAuthorityCheck } from "@/features/settings/automation/queries"
import {
  MAX_ACTIONS,
  MAX_CONDITIONS,
  MAX_IDLE,
  MIN_IDLE,
  TRIGGERS,
  useAutomationText,
} from "@/features/settings/automation/text"
import { ChoiceSelect, Field, ToggleRow } from "@/features/settings/ui"
import {
  ApiError,
  api,
  unwrap,
  type AutomationActionType,
  type AutomationRule,
  type AutomationRuleUpdate,
  type MailFilterSuggestion,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useCannedReplies, useInboxes } from "@/lib/workspace"

type Scope = "workspace" | "inbox"

function Problem({ error }: { error: unknown }) {
  if (error instanceof ApiError && error.status === 400 && error.detail)
    return (
      <p role="alert" className="text-body text-destructive" data-testid="automation-problem">
        {error.detail}
      </p>
    )
  return <ErrorLine error={error} />
}

type Source = { rule: AutomationRule | null; prefill?: MailFilterSuggestion; inboxId?: string; filter?: boolean }

function Form({ rule, prefill, inboxId: fixed, filter, onClose }: Source & { onClose: () => void }) {
  const { t } = useLingui()
  const text = useAutomationText()
  const qc = useQueryClient()
  const { workspaceId: ws, membership } = useSession()
  const inboxes = useInboxes().data ?? []
  const replies = useCannedReplies().data ?? []
  const authority = useAuthorityCheck()
  const [raw, setRaw] = useState<Draft>(() => {
    const d = fromRule(rule ?? prefill ?? null)
    if (!rule && fixed) d.inboxId = fixed
    if (!rule && fixed && !prefill) d.trigger = "mail_received"
    return d
  })
  const [scope, setScope] = useState<Scope>(rule?.inbox_id || prefill || fixed ? "inbox" : "workspace")
  const inbox = inboxes.find((i) => i.id === (scope === "inbox" ? raw.inboxId : ""))
  const personal = isPersonal(inbox)
  const draft = sanitize({ ...raw, inboxId: scope === "inbox" ? raw.inboxId : "" }, inbox)
  const set = (patch: Partial<Draft>) => setRaw(sanitize({ ...draft, ...patch }, inbox))
  const complete = isComplete(draft, personal) && (scope === "workspace" || draft.inboxId !== "")
  const body = toBody(draft)
  const changed = !rule || JSON.stringify(body) !== JSON.stringify(toBody(fromRule(rule)))
  const taken = draft.actions.map((a) => a.type)
  const addable = actionTypes(draft.trigger, personal).filter((type) => !blocked(type, taken))
  const triggers = personal
    ? (["mail_received"] as const)
    : TRIGGERS.filter((x) => x !== "mail_received" || inbox?.purpose === "mail")
  const fields = conditionFields(draft.trigger)
  const warning = authority.problem(
    { actions: body.actions, saved_by: { type: "member", member_id: membership.member_id } },
    replies
  )
  const title = warning?.reply?.title ?? ""

  const save = useMutation({
    mutationFn: () => {
      if (!rule) return unwrap(api.POST("/v1/automation-rules", { body }))
      const patch: AutomationRuleUpdate = {
        name: body.name,
        enabled: body.enabled,
        inbox_id: body.inbox_id ?? null,
        trigger: body.trigger,
        idle_minutes: body.idle_minutes ?? null,
        conditions: body.conditions,
        actions: body.actions,
      }
      return unwrap(
        api.PATCH("/v1/automation-rules/{automationRuleId}", {
          params: { path: { automationRuleId: rule.id } },
          body: patch,
        })
      )
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.automationRules(ws) })
      onClose()
    },
  })

  return (
    <form
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <Field label={<Trans>Name</Trans>} htmlFor="rule-name">
        <Input
          id="rule-name"
          required
          maxLength={100}
          value={draft.name}
          placeholder={t`Close idle conversations`}
          onChange={(e) => set({ name: e.target.value })}
          data-testid="rule-name"
        />
      </Field>
      <ToggleRow
        title={<Trans>Enabled</Trans>}
        hint={<Trans>A rule that is off never runs. Rules run in the order of the list.</Trans>}
      >
        <Switch
          checked={draft.enabled}
          onCheckedChange={(enabled) => set({ enabled })}
          aria-label={t`Enabled`}
          data-testid="rule-enabled"
        />
      </ToggleRow>
      {!fixed && (
        <Field label={<Trans>Applies to</Trans>}>
          <ChipGroup<Scope>
            value={scope}
            onChange={(next) => {
              setScope(next)
              setRaw(draft)
            }}
            options={[
              ["workspace", t`Every inbox`],
              ["inbox", t`One inbox`],
            ]}
            label={t`Applies to`}
          />
          {scope === "inbox" && (
            <ChoiceSelect
              value={draft.inboxId}
              onChange={(inboxId) =>
                setRaw(
                  sanitize(
                    { ...draft, inboxId },
                    inboxes.find((i) => i.id === inboxId)
                  )
                )
              }
              options={inboxes.map((i) => [i.id, i.name] as const)}
              placeholder={t`Pick an inbox`}
              label={t`Inbox`}
            />
          )}
        </Field>
      )}
      <fieldset className="flex flex-col gap-3 rounded-xl border p-4">
        <legend className="px-1 text-small font-medium text-muted-foreground">
          <Trans>When</Trans>
        </legend>
        <ChoiceSelect
          value={draft.trigger}
          onChange={(trigger) => set({ trigger })}
          options={triggers.map((x) => [x, text.trigger[x]] as const)}
          label={t`Trigger`}
        />
        {draft.trigger === "idle" && (
          <Field
            label={<Trans>Quiet for (minutes)</Trans>}
            htmlFor="rule-idle"
            hint={<Trans>Checked every 5 minutes, once per quiet period. Only open and pending conversations.</Trans>}
          >
            <Input
              id="rule-idle"
              type="number"
              inputMode="numeric"
              min={MIN_IDLE}
              max={MAX_IDLE}
              value={draft.idle}
              aria-invalid={!idleValid(draft) || undefined}
              className="w-32"
              onChange={(e) => set({ idle: e.target.value })}
              data-testid="rule-idle"
            />
          </Field>
        )}
      </fieldset>
      <fieldset className="flex flex-col gap-3 rounded-xl border p-4">
        <legend className="px-1 text-small font-medium text-muted-foreground">
          <Trans>If all of these match</Trans>
        </legend>
        {draft.conditions.length === 0 ? (
          <p className="text-small text-faint">
            <Trans>No conditions: the rule runs for every conversation the trigger fits.</Trans>
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {draft.conditions.map((c) => (
              <ConditionRow
                key={c.id}
                c={c}
                fields={fields}
                onChange={(next) => set({ conditions: draft.conditions.map((x) => (x.id === c.id ? next : x)) })}
                onRemove={() => set({ conditions: draft.conditions.filter((x) => x.id !== c.id) })}
              />
            ))}
          </ul>
        )}
        <Button
          type="button"
          variant="outline"
          className="self-start"
          disabled={draft.conditions.length >= MAX_CONDITIONS}
          onClick={() => set({ conditions: [...draft.conditions, blankCondition("channel_kind")] })}
          data-testid="rule-add-condition"
        >
          <PlusIcon />
          <Trans>Add condition</Trans>
        </Button>
      </fieldset>
      <fieldset className="flex flex-col gap-3 rounded-xl border p-4">
        <legend className="px-1 text-small font-medium text-muted-foreground">
          <Trans>Then</Trans>
        </legend>
        {draft.actions.length > 0 && (
          <ul className="flex flex-col gap-2">
            {draft.actions.map((a) => (
              <ActionRow
                key={a.id}
                a={a}
                inboxId={draft.inboxId}
                personal={personal}
                onChange={(next) => set({ actions: draft.actions.map((x) => (x.id === a.id ? next : x)) })}
                onRemove={() => set({ actions: draft.actions.filter((x) => x.id !== a.id) })}
              />
            ))}
          </ul>
        )}
        {draft.actions.length < MAX_ACTIONS && (
          <ChoiceSelect<AutomationActionType | "">
            value=""
            onChange={(type) => type && set({ actions: [...draft.actions, blankAction(type)] })}
            options={addable.map((x) => [x, text.action[x]] as const)}
            placeholder={t`Add an action`}
            label={t`Add an action`}
            className="w-64"
          />
        )}
        <p className="text-small text-faint">
          <Trans>Each action once, at most 10. Actions run as one change; if one fails, none of them applies.</Trans>
        </p>
      </fieldset>
      {warning?.reply && (
        <Notice tone="warning" icon={TriangleAlertIcon} data-testid="rule-reply-warning">
          <p>
            <Trans>
              This reply will not be sent until an owner or admin saves the canned reply {title}, or bots may send is
              turned on in the API key settings.
            </Trans>
          </p>
        </Notice>
      )}
      <Problem error={save.error} />
      <DialogFooter>
        <Button type="submit" disabled={save.isPending || !complete || !changed} data-testid="rule-save">
          {rule ? <Trans>Save</Trans> : filter ? <Trans>Create filter</Trans> : <Trans>Create rule</Trans>}
        </Button>
      </DialogFooter>
    </form>
  )
}

export function RuleDialog({ onClose, ...source }: Source & { onClose: () => void }) {
  const { rule, filter } = source
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {filter ? (
              rule ? (
                <Trans>Edit filter</Trans>
              ) : (
                <Trans>New filter</Trans>
              )
            ) : rule ? (
              <Trans>Edit rule</Trans>
            ) : (
              <Trans>New rule</Trans>
            )}
          </DialogTitle>
        </DialogHeader>
        <Form {...source} onClose={onClose} />
      </DialogContent>
    </Dialog>
  )
}
