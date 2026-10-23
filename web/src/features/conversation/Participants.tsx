import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, XIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { parseAddresses } from "@/components/common/AddressChips"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { CardSection } from "@/features/contact/ContactCard"
import { useConversation, useParticipantActions, useParticipants } from "@/features/conversation/queries"
import type { ConversationParticipant } from "@/lib/api"
import { useChannelMap } from "@/lib/workspace"

function Row({
  p,
  busy,
  onAdd,
  onRemove,
}: {
  p: ConversationParticipant
  busy: boolean
  onAdd: () => void
  onRemove: () => void
}) {
  const { t } = useLingui()
  const suggested = p.status === "suggested"
  return (
    <li className="flex min-w-0 items-start gap-2" data-testid="participant" data-status={p.status}>
      <span className="flex min-w-0 flex-1 flex-col leading-[1.35]">
        {p.name && <span className="truncate font-medium">{p.name}</span>}
        <span className={p.name ? "truncate text-caption text-muted-foreground" : "truncate"} title={p.email}>
          {p.email}
        </span>
        {suggested && (
          <span className="text-caption text-warning" data-testid="participant-unverified">
            <Trans>Not verified: the mail did not pass the sender check, so this address may be forged.</Trans>
          </span>
        )}
      </span>
      {suggested ? (
        <Button
          variant="outline"
          size="sm"
          onClick={onAdd}
          disabled={busy}
          aria-label={t`Add ${p.email}`}
          data-testid="participant-add"
        >
          <Trans>Add</Trans>
        </Button>
      ) : (
        <Button
          variant="ghost"
          size="icon-xs"
          className="text-faint"
          onClick={onRemove}
          disabled={busy}
          aria-label={t`Remove ${p.email}`}
          title={t`Remove ${p.email}`}
          data-testid="participant-remove"
        >
          <XIcon />
        </Button>
      )}
    </li>
  )
}

export function Participants({ conversationId }: { conversationId: string }) {
  const { t } = useLingui()
  const conversation = useConversation(conversationId).data
  const channels = useChannelMap()
  const email = conversation?.channel_id ? channels.get(conversation.channel_id)?.kind === "email" : false
  const list = useParticipants(conversationId, email)
  const { add, remove } = useParticipantActions(conversationId)
  const [text, setText] = useState("")
  const [invalid, setInvalid] = useState(false)
  const items = list.data?.items ?? []
  if (!email && items.length === 0) return null

  const active = items.filter((p) => p.status === "active")
  const suggested = items.filter((p) => p.status === "suggested")
  const busy = add.isPending || remove.isPending
  const submit = () => {
    const { valid, invalid: bad } = parseAddresses(text)
    if (valid.length !== 1 || bad.length > 0) {
      setInvalid(true)
      return
    }
    setInvalid(false)
    add.mutate(valid[0], { onSuccess: () => setText("") })
  }

  return (
    <CardSection title={<Trans>Participants</Trans>} testId="participants-section">
      <div className="flex flex-col gap-3">
        {list.isPending ? (
          <Skeleton className="h-10 w-full" />
        ) : (
          <>
            {active.length === 0 && suggested.length === 0 && (
              <p className="text-faint">
                <Trans>Nobody else is copied. Replies go to the contact only.</Trans>
              </p>
            )}
            {active.length > 0 && (
              <ul className="flex flex-col gap-2.5" data-testid="participants-active">
                {active.map((p) => (
                  <Row
                    key={p.email}
                    p={p}
                    busy={busy}
                    onAdd={() => add.mutate(p.email)}
                    onRemove={() => remove.mutate(p.email)}
                  />
                ))}
              </ul>
            )}
            {suggested.length > 0 && (
              <div className="flex flex-col gap-2">
                <p className="text-caption text-faint">
                  <Trans>Suggested, never copied unless you add them</Trans>
                </p>
                <ul className="flex flex-col gap-2.5" data-testid="participants-suggested">
                  {suggested.map((p) => (
                    <Row
                      key={p.email}
                      p={p}
                      busy={busy}
                      onAdd={() => add.mutate(p.email)}
                      onRemove={() => remove.mutate(p.email)}
                    />
                  ))}
                </ul>
              </div>
            )}
          </>
        )}
        <form
          className="flex items-center gap-1.5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <Input
            type="email"
            name="participant"
            value={text}
            onChange={(e) => {
              setText(e.target.value)
              setInvalid(false)
            }}
            placeholder={t`Add an address`}
            aria-label={t`Add a participant by e-mail address`}
            aria-invalid={invalid}
            autoComplete="off"
            data-testid="participant-input"
          />
          <Button
            type="submit"
            variant="outline"
            size="icon-sm"
            disabled={text.trim() === "" || busy}
            aria-label={t`Add participant`}
            title={t`Add participant`}
            data-testid="participant-submit"
          >
            <PlusIcon />
          </Button>
        </form>
        {invalid && (
          <p role="alert" className="text-caption text-destructive">
            <Trans>Enter one valid e-mail address.</Trans>
          </p>
        )}
        <ErrorLine error={add.error ?? remove.error} className="text-caption" />
      </div>
    </CardSection>
  )
}
