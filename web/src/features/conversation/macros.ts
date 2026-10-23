import type { Conversation, ConversationUpdate, Macro } from "@/lib/api"

export type MacroPick = { macro: Macro; dropped: string[] }

export type MacroStep = { key: string; kind: "add" | "remove" | "assignee" | "status" | "priority"; id?: string }

export function macroSteps(m: Macro): MacroStep[] {
  const a = m.actions
  const steps: MacroStep[] = []
  for (const id of a.add_labels ?? []) steps.push({ key: `add:${id}`, kind: "add", id })
  for (const id of a.remove_labels ?? []) steps.push({ key: `remove:${id}`, kind: "remove", id })
  if (a.assignee_id !== undefined) steps.push({ key: "assignee", kind: "assignee", id: a.assignee_id ?? undefined })
  if (a.status !== undefined || a.snooze_minutes !== undefined) steps.push({ key: "status", kind: "status" })
  if (a.priority !== undefined) steps.push({ key: "priority", kind: "priority" })
  return steps
}

export function macroSetsStatus(pick: MacroPick) {
  const a = pick.macro.actions
  return (a.status !== undefined || a.snooze_minutes !== undefined) && !pick.dropped.includes("status")
}

function current(c: Conversation): ConversationUpdate {
  if (c.status === "snoozed" && c.snooze_until && new Date(c.snooze_until) > new Date()) {
    return { status: "snoozed", snooze_until: c.snooze_until }
  }
  return { status: c.status === "snoozed" ? "open" : c.status }
}

export function macroThen(c: Conversation, pick: MacroPick | undefined, close: boolean) {
  const then: ConversationUpdate = {}
  if (pick) {
    const a = pick.macro.actions
    const dropped = new Set(pick.dropped)
    const adds = a.add_labels ?? []
    const removes = a.remove_labels ?? []
    if (adds.some((id) => dropped.has(`add:${id}`)) || removes.some((id) => dropped.has(`remove:${id}`))) {
      const labels = new Set(c.labels)
      for (const id of adds) if (!dropped.has(`add:${id}`)) labels.add(id)
      for (const id of removes) if (!dropped.has(`remove:${id}`)) labels.delete(id)
      then.labels = [...labels]
    }
    if (a.assignee_id !== undefined && dropped.has("assignee")) then.assignee_id = c.assignee_id ?? null
    if ((a.status !== undefined || a.snooze_minutes !== undefined) && dropped.has("status"))
      Object.assign(then, current(c))
    if (a.priority !== undefined && dropped.has("priority")) then.priority = c.priority
  }
  if (close) {
    delete then.snooze_until
    then.status = "closed"
  }
  return Object.keys(then).length > 0 ? then : undefined
}
