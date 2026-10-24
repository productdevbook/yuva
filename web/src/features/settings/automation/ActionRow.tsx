import { useLingui } from "@lingui/react/macro"
import { XIcon } from "lucide-react"

import { ChipGroup } from "@/components/common/ChipGroup"
import { ChipMulti } from "@/components/common/ChipMulti"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import type { ActionDraft } from "@/features/settings/automation/draft"
import { useAutomationText } from "@/features/settings/automation/text"
import { useWebhooks } from "@/features/settings/webhooks/queries"
import { ChoiceSelect } from "@/features/settings/ui"
import type { ConversationStatus, Priority } from "@/lib/api"
import { useCannedReplies, useInboxMembers, useLabels, useMembers, useTeams } from "@/lib/workspace"

const PRIORITIES: Priority[] = ["urgent", "high", "normal", "low"]
const STATUSES: ConversationStatus[] = ["open", "pending", "closed"]

export function ActionRow({
  a,
  inboxId,
  personal,
  onChange,
  onRemove,
}: {
  a: ActionDraft
  inboxId: string
  personal: boolean
  onChange: (next: ActionDraft) => void
  onRemove: () => void
}) {
  const { t } = useLingui()
  const text = useAutomationText()
  const enums = useEnumText()
  const labels = useLabels().data ?? []
  const all = useMembers().data ?? []
  const readers = useInboxMembers(inboxId || undefined).data
  const members = inboxId && readers ? readers : all
  const teams = useTeams().data ?? []
  const replies = useCannedReplies().data ?? []
  const hooks = useWebhooks().data ?? []
  let body: React.ReactNode = null
  switch (a.type) {
    case "add_labels":
    case "remove_labels":
      body =
        labels.length === 0 ? (
          <p className="text-small text-faint">{t`Nothing to pick from yet.`}</p>
        ) : (
          <ChipMulti
            value={a.label_ids ?? []}
            onChange={(label_ids) => onChange({ ...a, label_ids })}
            options={labels.map((l) => [l.id, l.name] as const)}
            label={text.action[a.type]}
          />
        )
      break
    case "assign_member":
      body = (
        <ChoiceSelect
          value={a.member_id ?? ""}
          onChange={(member_id) => onChange({ ...a, member_id })}
          options={members.map((m) => [m.id, m.name || m.email] as const)}
          placeholder={t`Pick a member`}
          label={t`Member`}
        />
      )
      break
    case "assign_team":
      body = (
        <ChoiceSelect
          value={a.team_id ?? ""}
          onChange={(team_id) => onChange({ ...a, team_id })}
          options={teams.map((x) => [x.id, x.name] as const)}
          placeholder={t`Pick a team`}
          label={t`Team`}
        />
      )
      break
    case "set_priority":
      body = (
        <ChipGroup
          value={a.priority ?? "normal"}
          onChange={(priority) => onChange({ ...a, priority })}
          options={PRIORITIES.map((p) => [p, enums.priority[p]] as const)}
          label={t`Priority`}
        />
      )
      break
    case "set_status":
      body = (
        <ChipGroup
          value={a.status ?? "closed"}
          onChange={(status) => onChange({ ...a, status })}
          options={STATUSES.map((s) => [s, enums.status[s]] as const)}
          label={t`Status`}
        />
      )
      break
    case "snooze":
      body = (
        <div className="flex items-center gap-2">
          <Input
            type="number"
            inputMode="numeric"
            min={1}
            max={525600}
            value={a.snooze_minutes ?? ""}
            aria-label={t`Minutes`}
            className="w-28"
            onChange={(e) =>
              onChange({ ...a, snooze_minutes: e.target.value === "" ? undefined : Number(e.target.value) })
            }
          />
          <span className="text-body text-muted-foreground">{t`minutes`}</span>
        </div>
      )
      break
    case "add_note":
      body = (
        <Textarea
          rows={3}
          maxLength={10000}
          value={a.body ?? ""}
          aria-label={t`Note`}
          placeholder={t`Only the team sees this`}
          onChange={(e) => onChange({ ...a, body: e.target.value })}
        />
      )
      break
    case "star":
      body = personal ? (
        <p className="text-small text-faint">{t`Stars the mail for you.`}</p>
      ) : members.length === 0 ? (
        <p className="text-small text-faint">{t`Nothing to pick from yet.`}</p>
      ) : (
        <ChipMulti
          value={a.member_ids ?? []}
          onChange={(member_ids) => onChange({ ...a, member_ids })}
          options={members.map((m) => [m.id, m.name || m.email] as const)}
          label={t`Star for`}
        />
      )
      break
    case "archive":
      body = <p className="text-small text-faint">{t`Moves the mail to the Archive.`}</p>
      break
    case "trash":
      body = <p className="text-small text-faint">{t`Moves the mail to the Trash. Runs after the other actions.`}</p>
      break
    case "forward_to_member":
      body = (
        <ChoiceSelect
          value={a.member_id ?? ""}
          onChange={(member_id) => onChange({ ...a, member_id })}
          options={members.map((m) => [m.id, m.name || m.email] as const)}
          placeholder={t`Pick a member`}
          label={t`Member`}
        />
      )
      break
    case "assign_pool":
      body = (
        <p className="text-small text-faint">
          {t`After the other rules have run, the conversation goes to the team's pool, or the product's when it has no team.`}
        </p>
      )
      break
    case "send_reply":
      body = (
        <ChoiceSelect
          value={a.canned_reply_id ?? ""}
          onChange={(canned_reply_id) => onChange({ ...a, canned_reply_id })}
          options={replies.map((r) => [r.id, r.title] as const)}
          placeholder={t`Pick a canned reply`}
          label={t`Canned reply`}
        />
      )
      break
    case "send_webhook":
      body = (
        <ChoiceSelect
          value={a.webhook_id ?? ""}
          onChange={(webhook_id) => onChange({ ...a, webhook_id })}
          options={hooks.map((h) => [h.id, h.description || new URL(h.url).host] as const)}
          placeholder={t`Pick an endpoint`}
          label={t`Webhook endpoint`}
        />
      )
      break
  }
  return (
    <li className="flex flex-col gap-2.5 rounded-xl border p-3" data-testid="automation-action">
      <div className="flex items-center gap-2">
        <span className="min-w-0 flex-1 truncate text-body font-medium">{text.action[a.type]}</span>
        <Button variant="ghost" size="icon-sm" aria-label={t`Remove action`} onClick={onRemove}>
          <XIcon />
        </Button>
      </div>
      {body}
    </li>
  )
}
