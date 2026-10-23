import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronRightIcon } from "lucide-react"
import { useState } from "react"

import { useEnumText, WEBHOOK_EVENTS } from "@/components/common/text"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { ToggleRow } from "@/features/settings/ui"
import { ApiError, type WebhookEventType } from "@/lib/api"
import { cn } from "@/lib/utils"

export const DEFAULT_EVENTS: WebhookEventType[] = ["conversation.created", "message.created", "feedback.created"]

type GroupId =
  | "conversations"
  | "messages"
  | "feedback"
  | "contacts"
  | "teams"
  | "drafts"
  | "discussions"
  | "lists"
  | "broadcasts"
  | "analytics"
  | "status"
  | "issues"

const GROUPS: [GroupId, WebhookEventType[]][] = [
  ["conversations", ["conversation.created", "conversation.updated", "conversation.rated"]],
  ["messages", ["message.created"]],
  ["feedback", ["feedback.created"]],
  ["contacts", ["contact.updated", "contact.deleted"]],
  ["teams", ["team.created", "team.updated", "team.deleted"]],
  ["drafts", ["draft.created", "draft.updated", "draft.deleted"]],
  ["discussions", ["discussion.created", "discussion.post_created", "discussion.answered", "discussion.updated"]],
  ["lists", ["subscription.created", "subscription.updated"]],
  ["broadcasts", ["broadcast.updated"]],
  ["analytics", ["analytics.alert"]],
  [
    "status",
    [
      "status.component.updated",
      "status.incident.created",
      "status.incident.updated",
      "status.incident.resolved",
      "status.monitor.down",
      "status.monitor.up",
    ],
  ],
  ["issues", ["issue_link.created", "issue_link.updated", "issue_link.deleted"]],
]

export function sortedEvents(events: Iterable<WebhookEventType>) {
  const set = new Set(events)
  return WEBHOOK_EVENTS.filter((e) => set.has(e))
}

export function urlProblem(err: unknown): "private" | "invalid" | null {
  if (!(err instanceof ApiError) || err.status !== 400 || !err.detail) return null
  if (/private|loopback|link-local/.test(err.detail)) return "private"
  if (/\burl\b/.test(err.detail)) return "invalid"
  return null
}

export function validUrl(raw: string) {
  const value = raw.trim()
  if (!value || value.length > 2000) return false
  try {
    const u = new URL(value)
    return (u.protocol === "http:" || u.protocol === "https:") && !u.username && !u.password && !!u.hostname
  } catch {
    return false
  }
}

export function UrlProblem({ problem }: { problem: "private" | "invalid" }) {
  return problem === "private" ? (
    <Trans>This address is private, loopback or link-local. Webhooks only reach public servers.</Trans>
  ) : (
    <Trans>Enter an http or https URL without a user name or password.</Trans>
  )
}

export function EventPicker({
  events,
  includeNotes,
  onEvents,
  onIncludeNotes,
}: {
  events: WebhookEventType[]
  includeNotes: boolean
  onEvents: (events: WebhookEventType[]) => void
  onIncludeNotes: (on: boolean) => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const chosen = new Set(events)
  const titles: Record<GroupId, string> = {
    conversations: t`Conversations`,
    messages: t`Messages`,
    feedback: t`Feedback`,
    contacts: t`Contacts`,
    teams: t`Teams`,
    drafts: t`Drafts`,
    discussions: t`Discussions`,
    lists: t`Lists`,
    broadcasts: t`Broadcasts`,
    analytics: t`Analytics`,
    status: t`Status`,
    issues: t`Issues`,
  }
  const [open, setOpen] = useState<Set<GroupId>>(
    () => new Set(GROUPS.filter(([, evs]) => evs.some((e) => chosen.has(e))).map(([id]) => id))
  )
  const set = (evs: WebhookEventType[], on: boolean) => {
    const next = new Set(chosen)
    for (const e of evs) {
      if (on) next.add(e)
      else next.delete(e)
    }
    onEvents(sortedEvents(next))
  }
  const toggleOpen = (id: GroupId) =>
    setOpen((s) => {
      const next = new Set(s)
      if (!next.delete(id)) next.add(id)
      return next
    })
  return (
    <ul className="divide-y rounded-xl border" data-testid="webhook-events">
      {GROUPS.map(([id, evs]) => {
        const count = evs.filter((e) => chosen.has(e)).length
        const shown = open.has(id)
        return (
          <li key={id} data-testid={`webhook-group-${id}`}>
            <div className="flex items-center gap-3 px-3.5 py-2.5">
              <Checkbox
                checked={count === evs.length}
                indeterminate={count > 0 && count < evs.length}
                onCheckedChange={(on) => set(evs, on)}
                aria-label={titles[id]}
              />
              <button
                type="button"
                className="flex min-w-0 flex-1 items-center gap-2 rounded text-start outline-none focus-visible:ring-3 focus-visible:ring-ring/40"
                aria-expanded={shown}
                onClick={() => toggleOpen(id)}
              >
                <span className="min-w-0 flex-1 truncate text-body font-medium">{titles[id]}</span>
                <span className="text-caption text-faint">
                  {count}/{evs.length}
                </span>
                <ChevronRightIcon
                  className={cn(
                    "size-4 shrink-0 text-faint transition-transform rtl:rotate-180",
                    shown && "rotate-90 rtl:rotate-90"
                  )}
                />
              </button>
            </div>
            {shown && (
              <div className="flex flex-col border-t bg-surface/50">
                <ul className="divide-y">
                  {evs.map((ev) => (
                    <li key={ev}>
                      <Label className="gap-3 py-2.5 ps-10 pe-3.5 font-normal">
                        <Checkbox
                          checked={chosen.has(ev)}
                          onCheckedChange={(on) => set([ev], on)}
                          aria-label={ev}
                          data-testid={`webhook-event-${ev}`}
                        />
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="text-body">{text.event[ev]}</span>
                          <code className="font-mono text-caption break-all text-faint">{ev}</code>
                        </span>
                      </Label>
                    </li>
                  ))}
                </ul>
                {id === "messages" && (
                  <div className="border-t py-2.5 ps-10 pe-3.5">
                    <ToggleRow
                      title={<Trans>Include notes</Trans>}
                      hint={
                        <Trans>
                          Also send message.created for members' internal notes. Internal events are never sent.
                        </Trans>
                      }
                    >
                      <Switch
                        checked={includeNotes && chosen.has("message.created")}
                        disabled={!chosen.has("message.created")}
                        onCheckedChange={onIncludeNotes}
                        aria-label={t`Include notes`}
                        data-testid="webhook-include-notes"
                      />
                    </ToggleRow>
                  </div>
                )}
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}
