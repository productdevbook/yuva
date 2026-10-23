import { Trans } from "@lingui/react/macro"

import { Pill } from "@/features/settings/ui"
import type { Channel } from "@/lib/api"

type SesState = NonNullable<NonNullable<Channel["email"]>["ses"]>["state"]

export type ChannelStateName = "ok" | "receive" | "unproven" | "taken" | "paused" | "identity_missing" | "error"

export function channelState(ch: Channel): ChannelStateName {
  if (ch.kind !== "email") return "ok"
  const proof = ch.email?.verification
  if (proof && proof.state !== "verified") return proof.problem === "taken" ? "taken" : "unproven"
  if (!ch.email?.smtp) return "receive"
  const ses: SesState | undefined = ch.email.ses?.state
  return ses === "paused" || ses === "identity_missing" || ses === "error" ? ses : "ok"
}

export type ChannelTab = "settings" | "deliverability" | "install" | "keys"

export function channelTabs(ch: Channel): ChannelTab[] {
  return ch.kind === "email"
    ? ["settings", "deliverability"]
    : ch.kind === "api"
      ? ["settings", "keys"]
      : ["settings", "install"]
}

export function channelTabLink(ch: Channel, tab: ChannelTab) {
  const base = `/settings/inboxes/${ch.inbox_id}/channels/${ch.id}`
  return tab === "settings" ? base : `${base}?tab=${tab}`
}

export function channelAction(ch: Channel): { tab: ChannelTab; label: React.ReactNode } {
  const state = channelState(ch)
  if (state === "identity_missing" || state === "paused" || state === "error")
    return { tab: "deliverability", label: <Trans>Fix</Trans> }
  if (state === "receive" || state === "unproven" || state === "taken")
    return { tab: "settings", label: <Trans>Fix</Trans> }
  if (ch.kind === "email") return { tab: "settings", label: <Trans>Settings</Trans> }
  if (ch.kind === "api") return { tab: "keys", label: <Trans>Keys</Trans> }
  return { tab: "install", label: <Trans>Install</Trans> }
}

export function ChannelPill({ ch }: { ch: Channel }) {
  const state = channelState(ch)
  return (
    <span data-testid="channel-status" data-state={state}>
      {state === "ok" ? (
        <Pill tone="success">
          <Trans>Live</Trans>
        </Pill>
      ) : state === "receive" ? (
        <Pill tone="warning">
          <Trans>Receives only</Trans>
        </Pill>
      ) : state === "unproven" ? (
        <Pill tone="warning">
          <Trans>Unproven address</Trans>
        </Pill>
      ) : state === "taken" ? (
        <Pill tone="danger">
          <Trans>Address lost</Trans>
        </Pill>
      ) : state === "paused" ? (
        <Pill tone="danger">
          <Trans>Paused</Trans>
        </Pill>
      ) : state === "identity_missing" ? (
        <Pill tone="warning">
          <Trans>Check DNS</Trans>
        </Pill>
      ) : (
        <Pill tone="danger">
          <Trans>Sending error</Trans>
        </Pill>
      )}
    </span>
  )
}

export function needsAttention(ch: Channel) {
  return channelState(ch) !== "ok"
}

export function channelDomain(channels: Channel[] | undefined) {
  const chat = channels?.find((c) => c.chat)?.chat?.allowed_origins[0]
  if (chat) {
    try {
      return new URL(chat).hostname.replace(/^www\./, "")
    } catch {
      return chat
    }
  }
  const mail = channels?.find((c) => c.email)?.email?.address
  return mail ? mail.slice(mail.lastIndexOf("@") + 1) : ""
}
