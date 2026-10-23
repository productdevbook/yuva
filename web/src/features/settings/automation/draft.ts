import { EXCLUSIVE, OPERATORS, operatorShape, MAX_IDLE, MIN_IDLE } from "@/features/settings/automation/text"
import type {
  AutomationAction,
  AutomationActionType,
  AutomationCondition,
  AutomationConditionField,
  AutomationRule,
  AutomationRuleCreate,
  AutomationTrigger,
} from "@/lib/api"

export type CondDraft = {
  id: number
  field: AutomationConditionField
  operator: AutomationCondition["operator"]
  key: string
  value: string
  values: string[]
}

export type ActionDraft = AutomationAction & { id: number }

export type Draft = {
  name: string
  enabled: boolean
  inboxId: string
  trigger: AutomationTrigger
  idle: string
  conditions: CondDraft[]
  actions: ActionDraft[]
}

let seq = 0
export const nextId = () => ++seq

export function blankCondition(field: AutomationConditionField): CondDraft {
  return { id: nextId(), field, operator: OPERATORS[field][0]!, key: "", value: "", values: [] }
}

export function blankAction(type: AutomationActionType): ActionDraft {
  switch (type) {
    case "set_priority":
      return { id: nextId(), type, priority: "high" }
    case "set_status":
      return { id: nextId(), type, status: "closed" }
    case "snooze":
      return { id: nextId(), type, snooze_minutes: 60 }
    case "add_labels":
    case "remove_labels":
      return { id: nextId(), type, label_ids: [] }
    case "add_note":
      return { id: nextId(), type, body: "" }
    default:
      return { id: nextId(), type }
  }
}

export function fromRule(rule: AutomationRule | null): Draft {
  return {
    name: rule?.name ?? "",
    enabled: rule?.enabled ?? true,
    inboxId: rule?.inbox_id ?? "",
    trigger: rule?.trigger ?? "conversation_created",
    idle: String(rule?.idle_minutes ?? 60),
    conditions: (rule?.conditions ?? []).map((c) => ({
      id: nextId(),
      field: c.field,
      operator: c.operator,
      key: c.key ?? "",
      value: c.value ?? "",
      values: c.values ?? [],
    })),
    actions: (rule?.actions ?? []).map((a) => ({ id: nextId(), ...a })),
  }
}

export function conditionComplete(c: CondDraft) {
  if ((c.field === "contact_attribute" || c.field === "custom_field") && c.key.trim() === "") return false
  const shape = operatorShape(c.operator)
  if (shape === "values") return c.values.length > 0
  if (shape === "value") return c.value.trim() !== ""
  return true
}

export function actionComplete(a: ActionDraft) {
  switch (a.type) {
    case "add_labels":
    case "remove_labels":
      return (a.label_ids ?? []).length > 0
    case "assign_member":
      return !!a.member_id
    case "assign_team":
      return !!a.team_id
    case "set_priority":
      return !!a.priority
    case "set_status":
      return !!a.status
    case "snooze":
      return Number.isInteger(a.snooze_minutes) && a.snooze_minutes! >= 1 && a.snooze_minutes! <= 525600
    case "add_note":
      return (a.body ?? "").trim() !== ""
    case "send_reply":
      return !!a.canned_reply_id
    case "send_webhook":
      return !!a.webhook_id
    default:
      return true
  }
}

export function idleValid(d: Draft) {
  if (d.trigger !== "idle") return true
  const n = Number(d.idle)
  return Number.isInteger(n) && n >= MIN_IDLE && n <= MAX_IDLE
}

export function isComplete(d: Draft) {
  return (
    d.name.trim() !== "" &&
    idleValid(d) &&
    d.actions.length > 0 &&
    d.conditions.every(conditionComplete) &&
    d.actions.every(actionComplete)
  )
}

export function blocked(type: AutomationActionType, taken: AutomationActionType[]) {
  if (taken.includes(type)) return true
  return EXCLUSIVE.some((g) => g.includes(type) && g.some((o) => o !== type && taken.includes(o)))
}

function condition(c: CondDraft): AutomationCondition {
  const shape = operatorShape(c.operator)
  return {
    field: c.field,
    operator: c.operator,
    ...(c.field === "contact_attribute" || c.field === "custom_field" ? { key: c.key.trim() } : {}),
    ...(shape === "values" ? { values: c.values } : {}),
    ...(shape === "value" ? { value: c.value } : {}),
  }
}

function action({ id: _id, ...a }: ActionDraft): AutomationAction {
  void _id
  const out: AutomationAction = { type: a.type }
  if (a.type === "add_labels" || a.type === "remove_labels") out.label_ids = a.label_ids
  if (a.type === "assign_member") out.member_id = a.member_id
  if (a.type === "assign_team") out.team_id = a.team_id
  if (a.type === "set_priority") out.priority = a.priority
  if (a.type === "set_status") out.status = a.status
  if (a.type === "snooze") out.snooze_minutes = a.snooze_minutes
  if (a.type === "add_note") out.body = a.body?.trim()
  if (a.type === "send_reply") out.canned_reply_id = a.canned_reply_id
  if (a.type === "send_webhook") out.webhook_id = a.webhook_id
  return out
}

export function toBody(d: Draft): AutomationRuleCreate {
  return {
    name: d.name.trim(),
    enabled: d.enabled,
    ...(d.inboxId ? { inbox_id: d.inboxId } : {}),
    trigger: d.trigger,
    ...(d.trigger === "idle" ? { idle_minutes: Number(d.idle) } : {}),
    conditions: d.conditions.map(condition),
    actions: d.actions.map(action),
  }
}
