import { Trans, useLingui } from "@lingui/react/macro"
import { LockIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, Notice, toast } from "@/components/common"
import { AddressChips } from "@/components/common/AddressChips"
import { formatBytes } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { useSuggestions } from "@/features/conversation/composer/Recipients"
import { useForward } from "@/features/conversation/queries"
import { Field } from "@/features/settings/ui"
import { ApiError, type Conversation, type Message } from "@/lib/api"

export const MAX_FORWARD_TO = 10

const forwardable = (m: Message) => m.kind === "message" && !m.draft && !m.redacted_at

function failedAddress(error: unknown) {
  if (!(error instanceof ApiError) || (error.code !== "validation_failed" && error.code !== "email_undeliverable"))
    return undefined
  return /^to: (\S+)/.exec(error.detail ?? "")?.[1]?.toLowerCase()
}

function ForwardError({ error }: { error: unknown }) {
  const address = failedAddress(error)
  if (address && error instanceof ApiError && error.code === "email_undeliverable")
    return (
      <p role="alert" className="text-small text-destructive" data-testid="forward-error">
        <Trans>
          {address} bounced or complained before. Remove it here, or clear it on the contact to forward there.
        </Trans>
      </p>
    )
  if (address)
    return (
      <p role="alert" className="text-small text-destructive" data-testid="forward-error">
        <Trans>
          {address} cannot be a recipient. It may be the conversation's contact, an address already copied on it, an
          address of this server, or not valid.
        </Trans>
      </p>
    )
  return <ErrorLine error={error} className="text-small" />
}

export function ForwardDialog({
  c,
  target,
  messages,
  onClose,
}: {
  c: Conversation
  target?: Message
  messages: Message[]
  onClose: () => void
}) {
  const { t, i18n } = useLingui()
  const forward = useForward(c.id)
  const hints = useSuggestions()
  const [to, setTo] = useState<string[]>([])
  const [note, setNote] = useState("")
  const [attachments, setAttachments] = useState(true)
  const sources = target ? [target] : messages.filter(forwardable).slice(-100)
  const size = sources.reduce((sum, m) => sum + m.attachments.reduce((s, a) => s + a.size, 0), 0)
  const files = sources.reduce((sum, m) => sum + m.attachments.length, 0)
  const bytes = formatBytes(size, i18n.locale)
  const ready = to.length > 0 && !forward.isPending

  const submit = (draft: boolean) => {
    if (!ready) return
    forward.mutate(
      { to, message_id: target?.id, note: note.trim() || undefined, attachments, draft: draft || undefined },
      {
        onSuccess: () => {
          toast(draft ? t`Forward saved as a draft` : t`Forwarded by e-mail`)
          onClose()
        },
      }
    )
  }

  return (
    <Dialog open onOpenChange={(o) => !o && !forward.isPending && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl" data-testid="forward-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit(false)
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              submit(false)
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {target ? <Trans>Forward this message</Trans> : <Trans>Forward the conversation</Trans>}
            </DialogTitle>
            <DialogDescription>
              {target ? (
                <Trans>Sends this message by e-mail to people outside Yuva.</Trans>
              ) : (
                <Trans>Sends the latest messages of this conversation by e-mail to people outside Yuva.</Trans>
              )}
            </DialogDescription>
          </DialogHeader>
          <Notice tone="info" icon={LockIcon} data-testid="forward-private-note">
            <p>
              <Trans>
                Notes are never forwarded. The contact does not see the forward or the answers; only your team does.
              </Trans>
            </p>
          </Notice>
          <Field label={<Trans>To</Trans>} hint={<Trans>Up to 10 addresses. Press Enter after each one.</Trans>}>
            <AddressChips
              label={t`To`}
              value={to}
              onChange={setTo}
              options={hints.options}
              onType={hints.onType}
              max={MAX_FORWARD_TO}
              below
              flagged={failedAddress(forward.error) ? [failedAddress(forward.error)!] : []}
              testId="forward-to"
            />
          </Field>
          <Field
            label={<Trans>Note (optional)</Trans>}
            htmlFor="forward-note"
            hint={<Trans>Plain text, shown above the forwarded mail.</Trans>}
          >
            <Textarea
              id="forward-note"
              rows={4}
              maxLength={20000}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              className="max-h-56"
              data-testid="forward-note"
            />
          </Field>
          <Label className="font-normal">
            <Checkbox
              checked={attachments}
              onCheckedChange={(on) => setAttachments(on === true)}
              data-testid="forward-attachments"
            />
            <span>
              <Trans>Include attachments</Trans>
              {files > 0 && (
                <span className="ms-1.5 text-faint" data-testid="forward-size">
                  {bytes}
                </span>
              )}
            </span>
          </Label>
          <ForwardError error={forward.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose} disabled={forward.isPending}>
              <Trans>Cancel</Trans>
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={!ready}
              onClick={() => submit(true)}
              data-testid="forward-draft"
            >
              <Trans>Save draft</Trans>
            </Button>
            <Button type="submit" disabled={!ready} data-testid="forward-send">
              <Trans>Send</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
