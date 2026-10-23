import { plural } from "@lingui/core/macro"
import { useLingui } from "@lingui/react/macro"

import { useEnumText } from "@/components/common/text"
import type { AuditAction, AuditEntry } from "@/lib/api"
import { useInboxes, useMemberMap } from "@/lib/workspace"

export type AuditGroup = "people" | "access" | "webhooks" | "mail" | "data" | "workspace"

export const AUDIT_GROUPS: Record<AuditGroup, AuditAction[]> = {
  people: [
    "member.joined",
    "member.role_changed",
    "member.removed",
    "member.inbox_access_granted",
    "member.inbox_access_revoked",
    "invite.created",
    "invite.deleted",
  ],
  access: ["api_key.created", "api_key.revoked", "oauth_grant.created", "oauth_grant.revoked"],
  webhooks: ["webhook.created", "webhook.updated", "webhook.deleted", "webhook.secret_rotated"],
  mail: [
    "channel.credentials_changed",
    "channel.key_rotated",
    "channel.forwarding_address_rotated",
    "channel.dkim_key_created",
    "channel.dkim_key_deleted",
    "inbox.identity_secret_rotated",
    "inbox.deleted",
  ],
  data: [
    "conversation.trashed",
    "conversation.restored",
    "conversation.deleted",
    "contact.deleted",
    "contact.consent_exported",
    "list.consent_exported",
    "analytics_export.created",
  ],
  workspace: [
    "workspace.retention_changed",
    "workspace.broadcast_approval_requested",
    "workspace.broadcasts_approved",
    "workspace.broadcasts_denied",
    "workspace.broadcasts_suspended",
    "workspace.broadcast_limit_changed",
  ],
}

function str(v: unknown) {
  return typeof v === "string" && v !== "" ? v : undefined
}

function days(v: unknown, forever: string) {
  if (typeof v !== "number") return forever
  const d = v
  return plural(d, { one: "# day", other: "# days" })
}

function recipients(v: unknown, none: string) {
  if (typeof v !== "number") return none
  const n = v
  return plural(n, { one: "# recipient a day", other: "# recipients a day" })
}

export function useAuditSentence() {
  const { t } = useLingui()
  const text = useEnumText()
  const members = useMemberMap()
  const inboxes = useInboxes().data
  const dash = "–"
  return (e: AuditEntry): string => {
    const d = e.details as Record<string, unknown>
    const member = members.get(e.target.id)
    const person = member ? member.name || member.email : t`a former member`
    const roleOf = (v: unknown) => text.role[v as keyof typeof text.role] ?? str(v) ?? dash
    const name = str(d.name) ?? dash
    const host = str(d.host) ?? dash
    const inbox = str(d.inbox_name) ?? inboxes?.find((i) => i.id === d.inbox_id)?.name ?? dash
    switch (e.action) {
      case "member.joined": {
        const role = roleOf(d.role)
        return t`Joined the workspace as ${role}`
      }
      case "member.role_changed": {
        const from = roleOf(d.from)
        const to = roleOf(d.to)
        return t`Changed the role of ${person} from ${from} to ${to}`
      }
      case "member.removed":
        return d.reason === "account_deleted"
          ? t`${person} left the workspace by deleting their account`
          : t`Removed ${person} from the workspace`
      case "member.inbox_access_granted":
        return t`Gave ${person} access to ${inbox}`
      case "member.inbox_access_revoked":
        return t`Took away the access of ${person} to ${inbox}`
      case "invite.created": {
        const email = str(d.email) ?? dash
        const role = roleOf(d.role)
        return t`Invited ${email} as ${role}`
      }
      case "invite.deleted":
        return t`Cancelled an invitation`
      case "api_key.created":
        return t`Created the API key ${name}`
      case "api_key.revoked":
        return t`Revoked the API key ${name}`
      case "oauth_grant.created": {
        const client = str(d.client) ?? dash
        return t`Connected ${client}`
      }
      case "oauth_grant.revoked": {
        const client = str(d.client) ?? dash
        return t`Disconnected ${client}`
      }
      case "webhook.created":
        return t`Added a webhook to ${host}`
      case "webhook.updated":
        return t`Changed the webhook to ${host}`
      case "webhook.deleted":
        return t`Deleted the webhook to ${host}`
      case "webhook.secret_rotated":
        return t`Rotated the signing secret of a webhook`
      case "channel.credentials_changed":
        return t`Changed the mail account or password of a channel`
      case "channel.key_rotated":
        return t`Rotated the key of a channel`
      case "channel.forwarding_address_rotated":
        return t`Rotated the forwarding address of a channel`
      case "channel.dkim_key_created": {
        const selector = str(d.selector) ?? dash
        return t`Created the DKIM key ${selector}`
      }
      case "channel.dkim_key_deleted":
        return t`Deleted a DKIM key`
      case "inbox.identity_secret_rotated":
        return t`Rotated the identity secret of an inbox`
      case "inbox.deleted":
        return t`Deleted the inbox ${name}`
      case "conversation.trashed":
        return t`Moved a conversation to the Trash`
      case "conversation.restored":
        return t`Restored a conversation from the Trash`
      case "conversation.deleted":
        return t`Deleted a conversation for good`
      case "contact.deleted":
        return t`Deleted a contact`
      case "contact.consent_exported":
        return t`Exported the consent records of contacts`
      case "list.consent_exported":
        return t`Exported the consent records of the list ${name}`
      case "analytics_export.created":
        return t`Created an analytics export`
      case "workspace.retention_changed": {
        const from = days(d.from, t`forever`)
        const to = days(d.to, t`forever`)
        return t`Changed how long conversations are kept from ${from} to ${to}`
      }
      case "workspace.broadcast_approval_requested":
        return t`Asked to be allowed to send broadcasts`
      case "workspace.broadcasts_approved":
        return t`Allowed the workspace to send broadcasts`
      case "workspace.broadcasts_denied":
        return t`Declined the request to send broadcasts`
      case "workspace.broadcasts_suspended":
        return t`Suspended broadcasts`
      case "workspace.broadcast_limit_changed": {
        const from = recipients(d.from, t`no limit`)
        const to = recipients(d.to, t`no limit`)
        return t`Changed the daily broadcast limit from ${from} to ${to}`
      }
      default:
        return e.action
    }
  }
}
