import type { Conversation, Discussion } from "@/lib/api"
import type { IssueDraft } from "@/features/issues/IssueDialog"

function firstLine(text: string) {
  const line = (text.split("\n").find((l) => l.trim()) ?? "").trim()
  const chars = [...line]
  return chars.length > 120 ? chars.slice(0, 120).join("").trim() + "…" : line
}

export function feedbackDraft(c: Conversation): IssueDraft {
  const text = c.feedback?.text?.trim() ?? ""
  return { title: firstLine(text) || c.subject.slice(0, 200), body: text }
}

export function ideaDraft(d: Discussion): IssueDraft {
  return { title: d.title, body: d.body.trim() }
}
