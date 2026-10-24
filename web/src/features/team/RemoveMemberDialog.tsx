import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { LockIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { MemberSelect } from "@/components/common/MemberSelect"
import { formatBytes } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { api, unwrap, type Inbox, type MailboxOnLeave, type Member } from "@/lib/api"

type Choice = { action: "delete" | "hand_over"; to: string }

export function RemoveMemberDialog({
  member,
  mailboxes,
  onClose,
  onRemoved,
}: {
  member: Member
  mailboxes: Inbox[]
  onClose: () => void
  onRemoved: () => void
}) {
  const { t, i18n } = useLingui()
  const [choices, setChoices] = useState<Record<string, Choice>>(() =>
    Object.fromEntries(mailboxes.map((i) => [i.id, { action: "delete", to: "" } as Choice]))
  )
  const name = member.name || member.email
  const set = (id: string, patch: Partial<Choice>) => setChoices((c) => ({ ...c, [id]: { ...c[id], ...patch } }))
  const complete = mailboxes.every((i) => choices[i.id].action === "delete" || choices[i.id].to !== "")
  const remove = useMutation({
    mutationFn: () => {
      const body: MailboxOnLeave[] = mailboxes.map((i) => {
        const c = choices[i.id]
        return c.action === "delete"
          ? { inbox_id: i.id, action: "delete" }
          : { inbox_id: i.id, action: "hand_over", to_member_id: c.to }
      })
      return unwrap(
        api.DELETE("/v1/members/{memberId}", { params: { path: { memberId: member.id } }, body: { mailboxes: body } })
      )
    },
    onSuccess: onRemoved,
  })
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-xl" data-testid="remove-member-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (complete) remove.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Remove {name}?</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                {name} owns personal mailboxes. Decide what happens to each: delete it with everything in it, or hand it
                to another member. You do not read them either way.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <ul className="flex flex-col gap-3" data-testid="remove-member-mailboxes">
            {mailboxes.map((i) => {
              const c = choices[i.id]
              const boxName = i.name
              const label = t`What happens to ${boxName}`
              return (
                <li
                  key={i.id}
                  className="flex flex-col gap-2.5 rounded-xl border px-3.5 py-3"
                  data-testid="remove-mailbox"
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <LockIcon className="size-4 shrink-0 text-faint" />
                    <div className="flex min-w-0 flex-1 flex-col leading-[1.3]">
                      <span className="truncate font-medium">{i.name}</span>
                      <span className="truncate text-caption text-faint">
                        {(i.mailbox?.addresses ?? []).join(", ") || "–"} ·{" "}
                        {formatBytes(i.mailbox?.storage_bytes ?? 0, i18n.locale)}
                      </span>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <NativeSelect
                      size="sm"
                      value={c.action}
                      aria-label={label}
                      onChange={(e) => set(i.id, { action: e.target.value as Choice["action"] })}
                      data-testid="remove-mailbox-action"
                    >
                      <NativeSelectOption value="delete">{t`Delete`}</NativeSelectOption>
                      <NativeSelectOption value="hand_over">{t`Hand over to`}</NativeSelectOption>
                    </NativeSelect>
                    {c.action === "hand_over" && (
                      <MemberSelect
                        value={c.to}
                        onChange={(to) => set(i.id, { to })}
                        exclude={[member.id]}
                        label={t`New owner of ${boxName}`}
                        placeholder={t`Pick a member`}
                        className="min-w-40 flex-1"
                        testId="remove-mailbox-to"
                      />
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
          <ErrorLine error={remove.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={!complete || remove.isPending}
              data-testid="remove-member-confirm"
            >
              <Trans>Remove {name}</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
