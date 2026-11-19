import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { CheckItem, CheckList, Dot } from "@/components/common"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useCreateBroadcast } from "@/features/broadcasts/broadcastQueries"
import { senderState } from "@/features/broadcasts/BroadcastParts"
import { useInboxChannels, useLists } from "@/features/broadcasts/queries"
import { BasisChoice } from "@/features/broadcasts/Trigger"
import type { Broadcast, BroadcastBasis } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

const DAY = 1440

export function NewBroadcastForm({
  inboxId,
  triggered = false,
  onClose,
  onCreated,
}: {
  inboxId: string
  triggered?: boolean
  onClose: () => void
  onCreated: (b: Broadcast) => void
}) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const [inbox, setInbox] = useState(inboxId || inboxes[0]?.id || "")
  const [name, setName] = useState("")
  const channels = useInboxChannels(inbox).data
  const sender = channels?.find((c) => c.kind === "email" && senderState(c) === "ready")
  const create = useCreateBroadcast()
  const [basis, setBasis] = useState<BroadcastBasis>("service")
  const [picked, setPicked] = useState<string[]>([])
  const lists = useLists(inbox, false)
  const inboxName = inboxes.find((i) => i.id === inbox)?.name ?? ""
  const items = Object.fromEntries(inboxes.map((i) => [i.id, i.name]))
  const chosen = basis === "marketing" ? picked : []
  const incomplete = triggered && basis === "marketing" && chosen.length === 0
  return (
    <FormCard
      title={triggered ? <Trans>New triggered e-mail</Trans> : <Trans>New broadcast</Trans>}
      description={
        triggered ? (
          <Trans>It stays a draft until you start it. Replies come to the inbox you pick.</Trans>
        ) : (
          <Trans>It stays a draft until you send or schedule it. Replies come to the inbox you pick.</Trans>
        )
      }
      submit={<Trans>Create draft</Trans>}
      ready={!!name.trim() && !!inbox && !incomplete}
      busy={create.isPending}
      error={create.error}
      onCancel={onClose}
      onSubmit={() =>
        create.mutate(
          {
            inbox_id: inbox,
            name: name.trim(),
            channel_id: sender?.id,
            ...(triggered
              ? {
                  kind: "triggered" as const,
                  trigger: { event: "contact_identified" as const, delay_minutes: DAY, basis },
                  audience: { list_ids: chosen, conditions: [], locales: [] },
                }
              : {}),
          },
          { onSuccess: onCreated }
        )
      }
      testId="new-broadcast"
    >
      <FormGrid>
        <Field>
          <FieldLabel htmlFor="broadcast-name-new">
            <Trans>Name</Trans>
          </FieldLabel>
          <Input
            id="broadcast-name-new"
            value={name}
            maxLength={200}
            autoFocus
            required
            placeholder={triggered ? t`Welcome mail` : t`October newsletter`}
            onChange={(e) => setName(e.target.value)}
            className="h-9"
            data-testid="new-broadcast-name"
          />
          <FieldDescription>
            <Trans>Only your team sees it.</Trans>
          </FieldDescription>
        </Field>
        {inboxes.length > 1 && (
          <Field>
            <FieldLabel>
              <Trans>Inbox</Trans>
            </FieldLabel>
            <Select value={inbox} items={items} onValueChange={(v) => v && setInbox(String(v))}>
              <SelectTrigger className="h-9 w-full min-w-0" aria-label={t`Inbox`} data-testid="new-broadcast-inbox">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {inboxes.map((i) => (
                  <SelectItem key={i.id} value={i.id}>
                    <Dot color={i.branding.color} className="mt-1.5 size-2 rounded-[3px]" />
                    {i.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        )}
      </FormGrid>
      {triggered && (
        <>
          <Field>
            <FieldLabel>
              <Trans>Why they get this mail</Trans>
            </FieldLabel>
            <BasisChoice value={basis} onChange={setBasis} disabled={false} inboxName={inboxName} />
          </Field>
          {basis === "marketing" && (
            <CheckList
              labelId="new-triggered-lists"
              label={<Trans>Lists</Trans>}
              hint={
                <p className="text-small text-faint">
                  <Trans>Only people subscribed to one of these lists get the mail. Fixed after the draft.</Trans>
                </p>
              }
            >
              {(lists.data?.pages.flatMap((p) => p.items) ?? []).map((l) => (
                <CheckItem
                  key={l.id}
                  checked={picked.includes(l.id)}
                  disabled={!picked.includes(l.id) && picked.length >= 10}
                  onChange={(on) => setPicked(on ? [...picked, l.id] : picked.filter((id) => id !== l.id))}
                  testId="new-triggered-list"
                >
                  <span className="truncate">{l.name}</span>
                </CheckItem>
              ))}
            </CheckList>
          )}
        </>
      )}
    </FormCard>
  )
}
