import { useLingui } from "@lingui/react/macro"

import { STATUSES } from "@/components/common/text"
import type { ConversationStatus, FeedbackCategory } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"

export const VIEWS = ["all", "mine", "unassigned", "spam"] as const
export type View = (typeof VIEWS)[number]

export function useViewLabels(): Record<View, string> {
  const { t } = useLingui()
  return { all: t`All`, mine: t`Mine`, unassigned: t`Unassigned`, spam: t`Spam` }
}

export type ListBase = { kind: "view" | "inbox" | "label" | "feedback"; id: string }

export type ListFilters = {
  status: ConversationStatus | "all"
  q: string
  inbox: string
  label: string
  assignee: string
}

export const FILTER_KEYS = ["status", "q", "inbox", "label", "assignee"] as const

export function readFilters(sp: URLSearchParams): ListFilters {
  const status = sp.get("status")
  return {
    status: status === "all" || STATUSES.includes(status as ConversationStatus) ? (status as ListFilters["status"]) : "open",
    q: sp.get("q") ?? "",
    inbox: sp.get("inbox") ?? "",
    label: sp.get("label") ?? "",
    assignee: sp.get("assignee") ?? "",
  }
}

export function toQuery(base: ListBase, f: ListFilters): ConversationFilters {
  const q: ConversationFilters = {}
  if (f.status !== "all") q.status = f.status
  if (f.q) q.q = f.q
  if (base.kind === "inbox") q.inbox_id = base.id
  else if (f.inbox) q.inbox_id = f.inbox
  if (base.kind === "label") q.label_id = base.id
  else if (f.label) q.label_id = f.label
  if (base.kind === "feedback") {
    q.kind = "feedback"
    if (base.id !== "all") q.category = base.id as FeedbackCategory
  }
  if (base.kind === "view") {
    if (base.id === "mine") q.assignee = "me"
    if (base.id === "unassigned") q.assignee = "unassigned"
    if (base.id === "spam") q.spam = true
  } else if (f.assignee) q.assignee = f.assignee
  return q
}

export function basePath(base: ListBase) {
  return base.kind === "view" ? `/${base.id}` : `/${base.kind}/${base.id}`
}
