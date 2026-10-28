import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"
import { useState } from "react"

import { CheckItem, CheckList, Dot, ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useCreateBroadcast } from "@/features/broadcasts/broadcastQueries"
import { senderState } from "@/features/broadcasts/BroadcastParts"
import { useInboxChannels, useLists } from "@/features/broadcasts/queries"
import { BasisChoice } from "@/features/broadcasts/Trigger"
import { Field } from "@/features/settings/ui"
import type { Broadcast, BroadcastBasis } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

const DAY = 1440

export function NewBroadcastDialog({
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
  const submit = () => {
    if (!name.trim() || !inbox || incomplete) return
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
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="new-broadcast-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{triggered ? <Trans>New triggered e-mail</Trans> : <Trans>New broadcast</Trans>}</DialogTitle>
            <DialogDescription>
              {triggered ? (
                <Trans>It stays a draft until you start it. Replies come to the inbox you pick.</Trans>
              ) : (
                <Trans>It stays a draft until you send or schedule it. Replies come to the inbox you pick.</Trans>
              )}
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Name</Trans>} htmlFor="broadcast-name-new" hint={<Trans>Only your team sees it.</Trans>}>
            <Input
              id="broadcast-name-new"
              value={name}
              maxLength={200}
              autoFocus
              required
              placeholder={triggered ? t`Welcome mail` : t`October newsletter`}
              onChange={(e) => setName(e.target.value)}
              data-testid="new-broadcast-name"
            />
          </Field>
          {inboxes.length > 1 && (
            <Field label={<Trans>Inbox</Trans>}>
              <Select value={inbox} items={items} onValueChange={(v) => v && setInbox(String(v))}>
                <SelectTrigger className="w-full min-w-0" aria-label={t`Inbox`} data-testid="new-broadcast-inbox">
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
          {triggered && (
            <>
              <Field label={<Trans>Why they get this mail</Trans>}>
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
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button
              type="submit"
              disabled={!name.trim() || !inbox || incomplete || create.isPending}
              data-testid="create-broadcast"
            >
              {create.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Create draft</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
