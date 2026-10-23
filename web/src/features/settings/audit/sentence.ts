import { plural } from "@lingui/core/macro"
import { useLingui } from "@lingui/react/macro"

import { useEnumText } from "@/components/common/text"
import type { AuditAction, AuditEntry } from "@/lib/api"
import { useInboxes, useMemberMap, useTeams } from "@/lib/workspace"

export type AuditGroup = "people" | "access" | "signin" | "webhooks" | "mail" | "data" | "workspace"

export const AUDIT_GROUPS: Record<AuditGroup, AuditAction[]> = {
  people: [
    "member.joined",
    "member.role_changed",
    "member.removed",
    "member.inbox_access_granted",
    "member.inbox_access_revoked",
    "invite.created",
    "invite.deleted",
    "team.created",
    "team.deleted",
    "team.member_added",
    "team.member_removed",
    "team.inbox_access_granted",
    "team.inbox_access_revoked",
    "member.slack_linked",
    "member.slack_unlinked",
  ],
  access: ["api_key.created", "api_key.revoked", "oauth_grant.created", "oauth_grant.revoked"],
  signin: [
    "sso.provider_created",
    "sso.provider_updated",
    "sso.provider_deleted",
    "sso.settings_changed",
    "sso.signed_in",
    "sso.sign_in_refused",
    "sso_domain.added",
    "sso_domain.verified",
    "sso_domain.deleted",
    "sso_domain.lapsed",
    "workspace.slack_connected",
    "workspace.slack_disconnected",
    "issue_integration.connected",
    "issue_integration.updated",
    "issue_integration.disconnected",
    "issue_integration.secret_rotated",
  ],
  webhooks: ["webhook.created", "webhook.updated", "webhook.deleted", "webhook.secret_rotated"],
  mail: [
    "channel.credentials_changed",
    "channel.key_rotated",
    "channel.forwarding_address_rotated",
    "channel.address_verified",
    "channel.address_lost",
    "channel.address_proven",
    "channel.dkim_key_created",
    "channel.dkim_key_deleted",
    "inbox.identity_secret_rotated",
    "inbox.deleted",
  ],
  data: [
    "conversation.trashed",
    "conversation.restored",
    "conversation.deleted",
    "message.redacted",
    "contact.deleted",
    "contact.consent_exported",
    "list.consent_exported",
    "analytics_export.created",
  ],
  workspace: [
    "automation_rule.created",
    "automation_rule.updated",
    "automation_rule.deleted",
    "workspace.retention_changed",
    "workspace.bots_may_send_changed",
    "workspace.broadcasts_by_keys_changed",
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

const SSO_PROTOCOLS: Record<string, string> = { oidc: "OpenID Connect", saml: "SAML" }
const ISSUE_PROVIDERS: Record<string, string> = { github: "GitHub", linear: "Linear" }

export function useAuditSentence() {
  const { t } = useLingui()
  const text = useEnumText()
  const members = useMemberMap()
  const inboxes = useInboxes().data
  const teams = useTeams().data
  const dash = "–"
  return (e: AuditEntry): string => {
    const d = e.details as Record<string, unknown>
    const member = members.get(e.target.id)
    const person = member ? member.name || member.email : t`a former member`
    const roleOf = (v: unknown) => text.role[v as keyof typeof text.role] ?? str(v) ?? dash
    const name = str(d.name) ?? dash
    const host = str(d.host) ?? dash
    const inbox = str(d.inbox_name) ?? inboxes?.find((i) => i.id === d.inbox_id)?.name ?? dash
    const team = str(d.name) ?? teams?.find((x) => x.id === e.target.id)?.name ?? dash
    const teamMember = members.get(str(d.member_id) ?? "")
    const who = teamMember ? teamMember.name || teamMember.email : t`a former member`
    const protocol = SSO_PROTOCOLS[str(d.protocol) ?? ""] ?? str(d.protocol) ?? dash
    const domain = str(d.domain) ?? dash
    const provider = ISSUE_PROVIDERS[str(d.provider) ?? ""] ?? str(d.provider) ?? dash
    const refusal = (reason: unknown) => {
      switch (reason) {
        case "failed":
          return t`the provider did not complete the sign-in`
        case "invalid_token":
          return t`the provider's response was not accepted`
        case "email_unverified":
          return t`the provider has not verified the e-mail address`
        case "domain_not_claimed":
          return t`the e-mail domain is not verified for this workspace`
        case "group_not_allowed":
          return t`the person is in none of the allowed groups`
        case "not_member":
          return t`the person is not a member of this workspace`
        default:
          return t`unknown reason`
      }
    }
    switch (e.action) {
      case "team.created":
        return t`Created the team ${team}`
      case "team.deleted":
        return t`Deleted the team ${team}`
      case "team.member_added":
        return t`Added ${who} to the team ${team}`
      case "team.member_removed":
        return t`Removed ${who} from the team ${team}`
      case "team.inbox_access_granted":
        return t`Gave the team ${team} access to ${inbox}`
      case "team.inbox_access_revoked":
        return t`Took away the access of the team ${team} to ${inbox}`
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
      case "channel.address_verified": {
        const address = str(d.address) ?? dash
        return d.method === "dns"
          ? t`Proved the address ${address} with the DNS record`
          : t`Proved the address ${address} with a code`
      }
      case "channel.address_lost": {
        const address = str(d.address) ?? dash
        const via = str(d.via)
        return via
          ? t`Mail to ${address} no longer reaches ${via}: another workspace proved the address`
          : t`Lost the address ${address}: another workspace proved it later`
      }
      case "channel.address_proven": {
        const address = str(d.address) ?? dash
        return t`The server's operator vouched for the address ${address}`
      }
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
      case "message.redacted":
        return d.kind === "note" ? t`Removed a note from a conversation` : t`Removed a message from a conversation`
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
      case "workspace.bots_may_send_changed":
        return d.to === true
          ? t`Let assistants and keys send messages to customers`
          : t`Stopped assistants and keys from sending messages to customers; they write drafts only`
      case "workspace.broadcasts_by_keys_changed":
        return d.to === true
          ? t`Let keys and assistants schedule broadcasts`
          : t`Stopped keys and assistants from scheduling broadcasts`
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
      case "automation_rule.created":
        return t`Created the automation rule ${name}`
      case "automation_rule.updated":
        return t`Changed the automation rule ${name}`
      case "automation_rule.deleted":
        return t`Deleted the automation rule ${name}`
      case "sso.provider_created":
        return t`Set up single sign-on with ${protocol}`
      case "sso.provider_updated":
        return t`Changed the ${protocol} single sign-on provider`
      case "sso.provider_deleted":
        return t`Removed the ${protocol} single sign-on provider`
      case "sso.settings_changed": {
        const to = (d.to ?? {}) as Record<string, unknown>
        if (to.sso_required === true) return t`Made single sign-on required`
        if (to.sso_required === false) return t`Made single sign-on optional`
        return t`Changed the single sign-on settings`
      }
      case "sso.signed_in": {
        const who = str(d.email) ?? t`an unknown person`
        return t`Sign-in with single sign-on (${protocol}) by ${who}`
      }
      case "sso.sign_in_refused": {
        const who = str(d.email) ?? t`an unknown person`
        const reason = refusal(d.reason)
        return t`Refused a sign-in with single sign-on (${protocol}) by ${who}: ${reason}`
      }
      case "sso_domain.added":
        return t`Added the domain ${domain} for single sign-on`
      case "sso_domain.verified":
        return t`Verified the domain ${domain} for single sign-on`
      case "sso_domain.deleted":
        return t`Removed the domain ${domain} from single sign-on`
      case "sso_domain.lapsed":
        return d.sso_required_off === true
          ? t`The domain ${domain} lost its verification: its DNS record is missing, so single sign-on is no longer required`
          : t`The domain ${domain} lost its verification: its DNS record is missing`
      case "workspace.slack_connected": {
        const slackTeam = str(d.team_name) ?? dash
        return t`Connected the Slack workspace ${slackTeam}`
      }
      case "workspace.slack_disconnected":
        return d.reason === "uninstalled"
          ? t`Disconnected Slack because the app was removed in Slack`
          : t`Disconnected Slack`
      case "member.slack_linked":
        return t`Linked ${person} to Slack`
      case "member.slack_unlinked":
        return t`Unlinked ${person} from Slack`
      case "issue_integration.connected":
        return t`Connected the issue tracker ${name} (${provider})`
      case "issue_integration.updated":
        return t`Changed the settings of an issue tracker connection`
      case "issue_integration.disconnected":
        return t`Disconnected the issue tracker ${name} (${provider})`
      case "issue_integration.secret_rotated":
        return t`Rotated the webhook secret of an issue tracker connection`
      default:
        return e.action
    }
  }
}
