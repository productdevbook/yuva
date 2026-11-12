import { useLingui } from "@lingui/react/macro"

import { InfoSection, KV } from "@/components/common"
import { formatShort } from "@/components/common/text"
import { senderState, useBroadcastsStatus, useNumber, useSenderText } from "@/features/broadcasts/BroadcastParts"
import { listLink, useInboxChannels, useLists } from "@/features/broadcasts/queries"
import { designLink, useDesign } from "@/features/designs/queries"
import { channelLink, useDeliverability } from "@/features/settings/channels/email/deliverability"
import { icons, useLines } from "@/features/settings/channels/email/Deliverability"
import type { Broadcast, DeliverabilityCheck } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useMemberMap } from "@/lib/workspace"

export function atBreak(address: string) {
  const at = address.indexOf("@")
  if (at < 0) return address
  return (
    <>
      {address.slice(0, at)}
      <wbr />
      {address.slice(at)}
    </>
  )
}

function CheckLines({ check }: { check: DeliverabilityCheck }) {
  const lines = useLines(check)
  return lines.map((l) => (
    <KV
      key={l.key}
      label={l.title}
      value={
        <span className="inline-flex justify-end" title={l.text} data-tone={l.tone}>
          {icons[l.tone]}
          <span className="sr-only">{l.text}</span>
        </span>
      }
    />
  ))
}

export function SenderSection({ inboxId, channelId }: { inboxId: string; channelId?: string }) {
  const { t } = useLingui()
  const stateText = useSenderText()
  const channel = useInboxChannels(inboxId).data?.find((c) => c.id === channelId)
  const check = useDeliverability(channel?.email?.smtp ? channelId : undefined).data
  const state = channel ? senderState(channel) : null
  const address = channel?.email?.from_address || channel?.email?.address
  return (
    <InfoSection title={t`Sends from`} testId="broadcast-info-sender">
      {!channel ? (
        <KV label={t`Sender`} value={t`No channel yet`} tone="warn" />
      ) : (
        <>
          {address && (
            <p className="m-0 pb-1 font-mono text-xs text-fg2 [overflow-wrap:anywhere]" dir="ltr">
              {atBreak(address)}
            </p>
          )}
          {state && <KV label={t`Sender`} value={stateText[state]} tone={state === "ready" ? "ok" : "warn"} />}
          {check ? (
            <CheckLines check={check} />
          ) : (
            check === null && (
              <KV
                label={t`Deliverability`}
                value={t`Run a check`}
                tone="warn"
                to={channelLink(inboxId, channel.id, "deliverability")}
              />
            )
          )}
        </>
      )}
    </InfoSection>
  )
}

export function AudienceSection({ b, recipients }: { b: Broadcast; recipients?: number }) {
  const { t } = useLingui()
  const num = useNumber()
  const { canManage } = useSession()
  const open = useLists(b.inbox_id, false)
  const archived = useLists(b.inbox_id, true)
  const lists = new Map(
    [...(open.data?.pages ?? []), ...(archived.data?.pages ?? [])].flatMap((p) => p.items).map((l) => [l.id, l])
  )
  const design = useDesign(b.design_id ?? "").data
  const people = recipients ?? b.counts.recipients
  return (
    <InfoSection title={b.kind === "triggered" ? t`Who gets it` : t`Audience`} testId="broadcast-info-audience">
      {b.audience.list_ids.map((id) => {
        const l = lists.get(id)
        return (
          <KV
            key={id}
            label={l?.name ?? "…"}
            value={l ? num(l.counts.subscribed) : ""}
            to={listLink(id)}
            testId="broadcast-info-list"
          />
        )
      })}
      {people > 0 && <KV label={t`Recipients`} value={num(people)} />}
      {b.audience.conditions.length > 0 && <KV label={t`Conditions`} value={num(b.audience.conditions.length)} />}
      {b.audience.locales.length > 0 && <KV label={t`Languages`} value={b.audience.locales.join(", ")} />}
      <KV
        label={t`Design`}
        value={design?.name ?? (b.design_id ? "…" : "—")}
        to={canManage && b.design_id ? designLink(b.design_id) : undefined}
      />
    </InfoSection>
  )
}

export function ApprovalSection({ b }: { b: Broadcast }) {
  const { t, i18n } = useLingui()
  const status = useBroadcastsStatus()
  const members = useMemberMap()
  const by = b.scheduled_by ? members.get(b.scheduled_by) : undefined
  const author = b.created_by ? members.get(b.created_by) : undefined
  const permission =
    status?.mode === "off"
      ? { value: t`Turned off`, tone: "bad" as const }
      : status?.status === "suspended"
        ? { value: t`Suspended`, tone: "bad" as const }
        : status?.status === "approval_required" || status?.status === "approval_requested"
          ? { value: t`Waiting for approval`, tone: "warn" as const }
          : { value: t`Allowed`, tone: "ok" as const }
  return (
    <InfoSection title={t`Permission to send`} testId="broadcast-info-approval">
      <KV label={t`Workspace`} value={permission.value} tone={permission.tone} />
      {author && <KV label={t`Created`} value={author.name || author.email} />}
      {by ? (
        <KV label={t`Scheduled by`} value={by.name || by.email} />
      ) : (
        b.scheduled_by_api_key && <KV label={t`Scheduled by`} value={t`An API key or assistant`} />
      )}
      <KV label={t`Test mail`} value={b.last_test_at ? formatShort(b.last_test_at, i18n.locale) : t`Never`} />
    </InfoSection>
  )
}
