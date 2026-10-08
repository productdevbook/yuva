import { useLingui } from "@lingui/react/macro"

import type { Inbox, NotificationChannels, NotificationEvents } from "@/lib/api"

export type EventName = keyof NotificationEvents
export type Choice = "inherit" | "both" | "push" | "email" | "none"

export const EVENTS: EventName[] = [
  "new_live_conversation",
  "new_async_conversation",
  "message_in_my_conversation",
  "message_in_unassigned_conversation",
  "assigned_to_me",
]

export const DELAYS = [5, 15, 30, 60, 120, 240, 1440]

export function useEventText(): Record<EventName, { label: string; hint: string }> {
  const { t } = useLingui()
  return {
    new_live_conversation: {
      label: t`New conversation in a live inbox`,
      hint: t`The first message of a conversation in an inbox that shows who is available.`,
    },
    new_async_conversation: {
      label: t`New conversation in an async inbox`,
      hint: t`The first message of a conversation in an inbox that shows a reply time.`,
    },
    message_in_my_conversation: { label: t`New message in a conversation assigned to me`, hint: t`A later message from the contact.` },
    message_in_unassigned_conversation: {
      label: t`New message in an unassigned conversation`,
      hint: t`A later message from the contact while nobody is assigned.`,
    },
    assigned_to_me: { label: t`A conversation is assigned to me`, hint: t`When someone else assigns it to you.` },
  }
}

export function usePlainChoiceText(): Record<Exclude<Choice, "inherit">, string> {
  const { t } = useLingui()
  return { both: t`Push and e-mail`, push: t`Push only`, email: t`E-mail only`, none: t`Nothing` }
}

export function toChoice(c: NotificationChannels | undefined): Choice {
  if (!c) return "inherit"
  if (c.push && c.email) return "both"
  if (c.push) return "push"
  if (c.email) return "email"
  return "none"
}

export function fromChoice(c: Choice): NotificationChannels | undefined {
  if (c === "inherit") return undefined
  return { push: c === "both" || c === "push", email: c === "both" || c === "email" }
}

export function inboxEvents(inbox: Inbox): EventName[] {
  return EVENTS.filter((e) => e !== (inbox.mode === "live" ? "new_async_conversation" : "new_live_conversation"))
}

export function sameEvents(a: NotificationEvents, b: NotificationEvents) {
  return EVENTS.every((e) => a[e].push === b[e].push && a[e].email === b[e].email)
}
