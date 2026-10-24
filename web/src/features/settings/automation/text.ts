import { useLingui } from "@lingui/react/macro"

import type {
  AutomationActionType,
  AutomationConditionField,
  AutomationConditionOperator,
  AutomationRunAction,
  AutomationRunOutcome,
  AutomationTrigger,
} from "@/lib/api"

export const TRIGGERS: AutomationTrigger[] = [
  "conversation_created",
  "message_received",
  "status_changed",
  "idle",
  "mail_received",
]
export const OUTCOMES: AutomationRunOutcome[] = ["applied", "failed", "not_matched"]

export const CONDITION_FIELDS: AutomationConditionField[] = [
  "channel_kind",
  "kind",
  "feedback_category",
  "subject",
  "body",
  "contact_attribute",
  "label",
  "priority",
  "status",
  "team",
  "assignee",
  "business_hours",
  "custom_field",
  "from",
  "to",
  "cc",
  "list_id",
  "has_attachment",
]

export const MAIL_FIELDS: AutomationConditionField[] = ["from", "to", "cc", "list_id", "has_attachment"]
export const MAIL_ACTIONS: AutomationActionType[] = ["star", "archive", "trash", "forward_to_member"]
export const PERSONAL_ACTIONS: AutomationActionType[] = [
  "add_labels",
  "remove_labels",
  "assign_member",
  "unassign",
  "set_priority",
  "set_status",
  "snooze",
  ...MAIL_ACTIONS,
]
export const MIN_MAIL_TEXT = 3
export const MAX_MAIL_TEXT = 200

export const ACTION_TYPES: AutomationActionType[] = [
  "add_labels",
  "remove_labels",
  "assign_member",
  "assign_team",
  "assign_pool",
  "unassign",
  "set_priority",
  "set_status",
  "snooze",
  "add_note",
  "send_reply",
  "send_webhook",
  "star",
  "archive",
  "trash",
  "forward_to_member",
]

export const MAX_ACTIONS = 10
export const MAX_CONDITIONS = 20
export const MIN_IDLE = 5
export const MAX_IDLE = 43200
export const LIMIT = 100

const SET: AutomationConditionOperator[] = ["is_set", "is_not_set"]
const TEXT: AutomationConditionOperator[] = ["contains", "not_contains"]
const MEMBER: AutomationConditionOperator[] = ["in", "not_in"]
const FIELD: AutomationConditionOperator[] = [
  "equals",
  "not_equals",
  "contains",
  "not_contains",
  "is_set",
  "is_not_set",
]

export const OPERATORS: Record<AutomationConditionField, AutomationConditionOperator[]> = {
  channel_kind: MEMBER,
  kind: MEMBER,
  feedback_category: MEMBER,
  priority: MEMBER,
  status: MEMBER,
  label: MEMBER,
  team: [...MEMBER, ...SET],
  subject: TEXT,
  body: TEXT,
  contact_attribute: FIELD,
  custom_field: FIELD,
  assignee: SET,
  business_hours: ["equals"],
  from: TEXT,
  to: TEXT,
  cc: TEXT,
  list_id: TEXT,
  has_attachment: ["equals"],
}

export const EXCLUSIVE: AutomationActionType[][] = [
  ["set_status", "snooze"],
  ["assign_member", "assign_pool", "unassign"],
  ["archive", "set_status"],
  ["archive", "snooze"],
  ["trash", "forward_to_member"],
]

export function operatorShape(op: AutomationConditionOperator): "values" | "value" | "none" {
  if (op === "in" || op === "not_in") return "values"
  if (op === "is_set" || op === "is_not_set") return "none"
  return "value"
}

export function useAutomationText() {
  const { t } = useLingui()
  const trigger: Record<AutomationTrigger, string> = {
    conversation_created: t`A conversation starts`,
    message_received: t`The contact writes a message`,
    status_changed: t`The status changes`,
    idle: t`A conversation is idle`,
    mail_received: t`A mail arrives`,
  }
  const field: Record<AutomationConditionField, string> = {
    channel_kind: t`Channel`,
    kind: t`Kind`,
    feedback_category: t`Feedback category`,
    priority: t`Priority`,
    status: t`Status`,
    subject: t`Subject`,
    body: t`Message text`,
    contact_attribute: t`Contact attribute`,
    custom_field: t`Custom field`,
    label: t`Labels`,
    team: t`Team`,
    assignee: t`Assignee`,
    business_hours: t`Business hours`,
    from: t`Sender`,
    to: t`Recipient (To)`,
    cc: t`Recipient (Cc)`,
    list_id: t`Mailing list (List-Id)`,
    has_attachment: t`Has an attachment`,
  }
  const operator: Record<AutomationConditionOperator, string> = {
    in: t`is any of`,
    not_in: t`is none of`,
    equals: t`is`,
    not_equals: t`is not`,
    contains: t`contains`,
    not_contains: t`does not contain`,
    is_set: t`is set`,
    is_not_set: t`is not set`,
  }
  const action: Record<AutomationActionType, string> = {
    add_labels: t`Add labels`,
    remove_labels: t`Remove labels`,
    assign_member: t`Assign to a member`,
    assign_team: t`Assign to a team`,
    assign_pool: t`Hand to automatic assignment`,
    unassign: t`Remove the assignee`,
    set_priority: t`Set the priority`,
    set_status: t`Set the status`,
    snooze: t`Snooze`,
    add_note: t`Add an internal note`,
    send_reply: t`Reply with a canned reply`,
    send_webhook: t`Send to a webhook endpoint`,
    star: t`Star`,
    archive: t`Archive`,
    trash: t`Move to the Trash`,
    forward_to_member: t`Notify a member`,
  }
  const outcome: Record<AutomationRunOutcome, string> = {
    applied: t`Applied`,
    failed: t`Failed`,
    not_matched: t`Not matched`,
  }
  const result: Record<AutomationRunAction["result"], string> = {
    applied: t`done`,
    unchanged: t`already so`,
    skipped: t`skipped`,
    not_run: t`not run`,
  }
  const kind = { conversation: t`Conversation`, feedback: t`Feedback` }
  const attachment = { true: t`Yes`, false: t`No` }
  const inside = { inside: t`Inside business hours`, outside: t`Outside business hours` }
  const reason: Record<string, string> = {
    auto_reply: t`Reply held back to protect the customer`,
    bot_sending_disabled: t`Bots may not send and the author is not an owner or admin`,
    field_required: t`A required field is empty`,
    validation_failed: t`The change was refused`,
    team_access: t`The team does not cover this inbox`,
    email_undeliverable: t`The address bounced`,
    email_not_configured: t`The channel has no SMTP account`,
    label_gone: t`A label was deleted`,
    canned_reply_gone: t`The canned reply was deleted`,
    webhook_unavailable: t`The webhook endpoint is off or not available here`,
  }
  return { trigger, field, operator, action, outcome, result, kind, inside, attachment, reason }
}
