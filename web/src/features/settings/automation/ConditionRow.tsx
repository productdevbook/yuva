import { useLingui } from "@lingui/react/macro"
import { XIcon } from "lucide-react"

import { ChipGroup } from "@/components/common/ChipGroup"
import { ChipMulti } from "@/components/common/ChipMulti"
import { useFeedbackCategories } from "@/components/common/feedbackCategories"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { blankCondition, type CondDraft } from "@/features/settings/automation/draft"
import { MAIL_FIELDS, OPERATORS, operatorShape, useAutomationText } from "@/features/settings/automation/text"
import { ChoiceSelect } from "@/features/settings/ui"
import type { AutomationConditionField, ChannelKind, ConversationStatus, Priority } from "@/lib/api"
import { useCustomFields, useLabels, useTeams } from "@/lib/workspace"

const CHANNELS: ChannelKind[] = ["chat", "email", "app", "api"]
const PRIORITIES: Priority[] = ["urgent", "high", "normal", "low"]
const STATUSES: ConversationStatus[] = ["open", "pending", "snoozed", "closed"]

function useChoices(field: AutomationConditionField): (readonly [string, string])[] {
  const enums = useEnumText()
  const text = useAutomationText()
  const labels = useLabels().data ?? []
  const teams = useTeams().data ?? []
  const categories = useFeedbackCategories().list
  switch (field) {
    case "channel_kind":
      return CHANNELS.map((c) => [c, enums.channel[c]] as const)
    case "kind":
      return [
        ["conversation", text.kind.conversation],
        ["feedback", text.kind.feedback],
      ]
    case "feedback_category":
      return categories.map((c) => [c.key, c.name] as const)
    case "priority":
      return PRIORITIES.map((p) => [p, enums.priority[p]] as const)
    case "status":
      return STATUSES.map((s) => [s, enums.status[s]] as const)
    case "label":
      return labels.map((l) => [l.id, l.name] as const)
    case "team":
      return teams.map((x) => [x.id, x.name] as const)
    default:
      return []
  }
}

export function ConditionRow({
  c,
  fields: allowed,
  onChange,
  onRemove,
}: {
  c: CondDraft
  fields: AutomationConditionField[]
  onChange: (next: CondDraft) => void
  onRemove: () => void
}) {
  const { t } = useLingui()
  const text = useAutomationText()
  const choices = useChoices(c.field)
  const fields = useCustomFields("conversation").data ?? []
  const shape = operatorShape(c.operator)
  const keyed = c.field === "custom_field" || c.field === "contact_attribute"
  const keyOptions = fields.map((f) => [f.key, f.name] as const)
  if (c.field === "custom_field" && c.key && !fields.some((f) => f.key === c.key)) keyOptions.push([c.key, c.key])
  return (
    <li className="flex flex-col gap-2.5 rounded-xl border p-3" data-testid="automation-condition">
      <div className="flex flex-wrap items-center gap-2">
        <ChoiceSelect<AutomationConditionField>
          value={c.field}
          onChange={(field) => onChange({ ...blankCondition(field), id: c.id })}
          options={allowed.map((f) => [f, text.field[f]] as const)}
          label={t`Condition`}
          className="w-44"
        />
        {keyed &&
          (c.field === "custom_field" ? (
            <ChoiceSelect
              value={c.key}
              onChange={(key) => onChange({ ...c, key })}
              options={keyOptions}
              placeholder={t`Pick a field`}
              label={t`Custom field`}
              className="w-44"
            />
          ) : (
            <Input
              value={c.key}
              maxLength={100}
              placeholder={t`Attribute name`}
              aria-label={t`Attribute name`}
              className="w-44"
              onChange={(e) => onChange({ ...c, key: e.target.value })}
            />
          ))}
        {OPERATORS[c.field].length > 1 && (
          <ChoiceSelect
            value={c.operator}
            onChange={(operator) => onChange({ ...c, operator })}
            options={OPERATORS[c.field].map((o) => [o, text.operator[o]] as const)}
            label={t`Operator`}
            className="w-40"
          />
        )}
        <Button variant="ghost" size="icon-sm" className="ms-auto" aria-label={t`Remove condition`} onClick={onRemove}>
          <XIcon />
        </Button>
      </div>
      {c.field === "has_attachment" ? (
        <ChipGroup
          value={c.value}
          onChange={(value) => onChange({ ...c, value })}
          options={[
            ["true", text.attachment.true],
            ["false", text.attachment.false],
          ]}
          label={text.field.has_attachment}
        />
      ) : c.field === "business_hours" ? (
        <ChipGroup
          value={c.value}
          onChange={(value) => onChange({ ...c, value })}
          options={[
            ["inside", text.inside.inside],
            ["outside", text.inside.outside],
          ]}
          label={t`Business hours`}
        />
      ) : shape === "values" ? (
        choices.length > 0 ? (
          <ChipMulti
            value={c.values}
            onChange={(values) => onChange({ ...c, values })}
            options={choices}
            label={text.field[c.field]}
          />
        ) : (
          <p className="text-small text-faint">{t`Nothing to pick from yet.`}</p>
        )
      ) : shape === "value" ? (
        <Input
          value={c.value}
          maxLength={500}
          placeholder={MAIL_FIELDS.includes(c.field) ? t`At least 3 characters` : t`Text`}
          aria-label={t`Text`}
          onChange={(e) => onChange({ ...c, value: e.target.value })}
        />
      ) : null}
    </li>
  )
}
