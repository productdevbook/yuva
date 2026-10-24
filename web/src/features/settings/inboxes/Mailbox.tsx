import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { LockIcon, TrashIcon, UserRoundIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, toast, TypeToConfirmDialog, useConfirm } from "@/components/common"
import { MemberSelect } from "@/components/common/MemberSelect"
import { formatBytes, formatRelative } from "@/components/common/text"
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
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { Block, Field, StatusTag } from "@/features/settings/ui"
import { api, unwrap, type Inbox, type MailboxDelegateAccess } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useMemberMap, useMembers } from "@/lib/workspace"

export function useMailboxOwner(inbox: Inbox) {
  const members = useMemberMap()
  const owner = inbox.owner_member_id ? members.get(inbox.owner_member_id) : undefined
  return owner ? owner.name || owner.email : undefined
}

function useRefreshInboxes() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
}

export function HandOverDialog({ inbox, onClose }: { inbox: Inbox; onClose: () => void }) {
  const { t } = useLingui()
  const refresh = useRefreshInboxes()
  const [to, setTo] = useState("")
  const hand = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/v1/inboxes/{inboxId}/hand-over", {
          params: { path: { inboxId: inbox.id } },
          body: { to_member_id: to },
        })
      ),
    onSuccess: () => {
      void refresh()
      toast(t`Handed over`)
      onClose()
    },
  })
  const name = inbox.name
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="hand-over-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (to) hand.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Hand over {name}</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>The member you pick becomes its owner and reads it from then on. You still do not read it.</Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>New owner</Trans>} htmlFor="hand-over-to">
            <MemberSelect
              value={to}
              onChange={setTo}
              label={t`New owner`}
              placeholder={t`Pick a member`}
              testId="hand-over-to"
            />
          </Field>
          <ErrorLine error={hand.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!to || hand.isPending} data-testid="hand-over-confirm">
              <Trans>Hand over</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function RenameDialog({ inbox, onClose }: { inbox: Inbox; onClose: () => void }) {
  const { t } = useLingui()
  const refresh = useRefreshInboxes()
  const [name, setName] = useState(inbox.name)
  const save = useMutation({
    mutationFn: () =>
      unwrap(
        api.PATCH("/v1/inboxes/{inboxId}", { params: { path: { inboxId: inbox.id } }, body: { name: name.trim() } })
      ),
    onSuccess: () => {
      void refresh()
      toast(t`Saved`)
      onClose()
    },
  })
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="rename-mailbox-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (name.trim()) save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Rename the mailbox</Trans>
            </DialogTitle>
          </DialogHeader>
          <Field label={<Trans>Name</Trans>} htmlFor="rename-mailbox">
            <Input
              id="rename-mailbox"
              autoFocus
              maxLength={200}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <ErrorLine error={save.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!name.trim() || save.isPending} data-testid="rename-mailbox-save">
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function DeleteMailboxDialog({
  inbox,
  open,
  onOpenChange,
  onDeleted,
}: {
  inbox: Inbox
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeleted?: () => void
}) {
  const refresh = useRefreshInboxes()
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/inboxes/{inboxId}", { params: { path: { inboxId: inbox.id } } })),
    onSuccess: () => {
      void refresh()
      onOpenChange(false)
      onDeleted?.()
    },
  })
  const name = inbox.name
  return (
    <TypeToConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={<Trans>Delete {name}?</Trans>}
      description={
        <Trans>All of its conversations, messages and attachments are deleted with it. This cannot be undone.</Trans>
      }
      label={<Trans>Type {name} to confirm</Trans>}
      expected={name}
      confirm={<Trans>Delete this mailbox</Trans>}
      pending={remove.isPending}
      error={<ErrorLine error={remove.error} />}
      onConfirm={() => remove.mutate()}
    />
  )
}

export function LockedMailbox({ inbox }: { inbox: Inbox }) {
  const { t, i18n } = useLingui()
  const owner = useMailboxOwner(inbox) ?? t`a member`
  const [dialog, setDialog] = useState<"rename" | "delete" | "hand-over" | null>(null)
  const box = inbox.mailbox
  if (!box) return null
  const conversations = box.conversations
  return (
    <li className="flex flex-col overflow-hidden rounded-xl border bg-card" data-testid="locked-mailbox">
      <div className="flex items-center gap-3 border-b px-[18px] py-4">
        <span className="grid size-10 shrink-0 place-items-center rounded-[10px] bg-muted text-muted-foreground">
          <LockIcon className="size-[18px]" />
        </span>
        <div className="flex min-w-0 flex-1 flex-col leading-[1.3]">
          <span className="truncate text-[16px] font-semibold" data-testid="locked-mailbox-name">
            {inbox.name}
          </span>
          <span className="truncate text-caption text-faint" data-testid="locked-mailbox-owner">
            {box.orphaned ? (
              <Trans>No owner</Trans>
            ) : (
              <>
                <UserRoundIcon className="me-1 inline size-3" />
                <Trans>Personal mailbox of {owner}</Trans>
              </>
            )}
          </span>
        </div>
        {box.orphaned && (
          <StatusTag tone="warning">
            <Trans>Needs an owner</Trans>
          </StatusTag>
        )}
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2.5 px-[18px] pt-3.5 pb-3">
        <div className="col-span-2 flex min-w-0 flex-col">
          <dt className="text-caption text-faint">
            <Trans>Addresses</Trans>
          </dt>
          <dd className="truncate" data-testid="locked-mailbox-addresses">
            {box.addresses.length > 0 ? box.addresses.join(", ") : "–"}
          </dd>
        </div>
        <div className="flex flex-col">
          <dt className="text-caption text-faint">
            <Trans>Conversations</Trans>
          </dt>
          <dd data-testid="locked-mailbox-conversations">{new Intl.NumberFormat(i18n.locale).format(conversations)}</dd>
        </div>
        <div className="flex flex-col">
          <dt className="text-caption text-faint">
            <Trans>Size</Trans>
          </dt>
          <dd data-testid="locked-mailbox-size">{formatBytes(box.storage_bytes, i18n.locale)}</dd>
        </div>
      </dl>
      <div className="mt-auto flex flex-wrap items-center gap-2 border-t bg-surface/50 px-[18px] py-3">
        <span className="min-w-40 flex-1 text-caption text-faint">
          <Trans>Only its owner reads it. You can rename it or delete it.</Trans>
        </span>
        {box.orphaned && (
          <Button variant="outline" size="sm" onClick={() => setDialog("hand-over")} data-testid="locked-hand-over">
            <Trans>Hand over</Trans>
          </Button>
        )}
        <Button variant="outline" size="sm" onClick={() => setDialog("rename")} data-testid="locked-rename">
          <Trans>Rename</Trans>
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="text-destructive"
          onClick={() => setDialog("delete")}
          data-testid="locked-delete"
        >
          <TrashIcon />
          <Trans>Delete</Trans>
        </Button>
      </div>
      {dialog === "rename" && <RenameDialog inbox={inbox} onClose={() => setDialog(null)} />}
      {dialog === "hand-over" && <HandOverDialog inbox={inbox} onClose={() => setDialog(null)} />}
      <DeleteMailboxDialog inbox={inbox} open={dialog === "delete"} onOpenChange={(o) => !o && setDialog(null)} />
    </li>
  )
}

export function PersonalMailboxes({ items }: { items: Inbox[] }) {
  if (items.length === 0) return null
  return (
    <section className="flex flex-col gap-3" data-testid="locked-mailboxes">
      <div className="flex flex-col leading-[1.3]">
        <h2 className="font-semibold">
          <Trans>Personal mailboxes</Trans>
        </h2>
        <p className="text-caption text-faint">
          <Trans>Only their owners read them. You see what they hold, never what is in them.</Trans>
        </p>
      </div>
      <ul className="grid gap-4 lg:grid-cols-2">
        {items.map((i) => (
          <LockedMailbox key={i.id} inbox={i} />
        ))}
      </ul>
    </section>
  )
}

export function DelegatesTab({ inbox }: { inbox: Inbox }) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const [member, setMember] = useState("")
  const [access, setAccess] = useState<MailboxDelegateAccess>("read")
  const key = keys.delegates(ws, inbox.id)
  const list = useQuery({
    queryKey: key,
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/delegates", { params: { path: { inboxId: inbox.id } } })).then(
        (r) => r.items
      ),
  })
  const delegates = list.data ?? []
  const set = useMutation({
    mutationFn: (v: { memberId: string; access: MailboxDelegateAccess }) =>
      unwrap(
        api.PUT("/v1/inboxes/{inboxId}/delegates/{memberId}", {
          params: { path: { inboxId: inbox.id, memberId: v.memberId } },
          body: { access: v.access },
        })
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: key })
      setMember("")
    },
  })
  const remove = useMutation({
    mutationFn: (memberId: string) =>
      unwrap(
        api.DELETE("/v1/inboxes/{inboxId}/delegates/{memberId}", {
          params: { path: { inboxId: inbox.id, memberId } },
        })
      ),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
  })
  const members = useMembers().data ?? []
  const taken = [inbox.owner_member_id ?? "", ...delegates.map((d) => d.member_id)]
  const labels: Record<MailboxDelegateAccess, string> = {
    read: t({ message: "Read", context: "mailbox access" }),
    send: t`Read and send`,
  }
  return (
    <div className="flex max-w-[860px] flex-col gap-4" data-testid="delegates-tab">
      <Block
        title={<Trans>Delegates</Trans>}
        description={
          <Trans>
            Members you let into this mailbox. They can read it, or read and send mail on your behalf. They never change
            its settings, filters or migrations.
          </Trans>
        }
      >
        {list.isPending ? null : delegates.length === 0 ? (
          <p className="border-b px-[18px] py-4 text-body text-faint" data-testid="delegates-empty">
            <Trans>Nobody else reads this mailbox.</Trans>
          </p>
        ) : (
          <ul className="divide-y border-b" data-testid="delegate-list">
            {delegates.map((d) => {
              const person = d.name || d.email
              return (
                <li
                  key={d.member_id}
                  className="flex flex-wrap items-center gap-3 px-[18px] py-2.5"
                  data-testid="delegate"
                >
                  <span className="flex min-w-40 flex-1 flex-col leading-[1.3]">
                    <span className="truncate">{d.name || d.email}</span>
                    <span className="truncate text-caption text-faint">
                      {d.email} · {formatRelative(d.created_at, i18n.locale)}
                    </span>
                  </span>
                  <NativeSelect
                    size="sm"
                    value={d.access}
                    aria-label={t`Access of ${person}`}
                    disabled={set.isPending}
                    onChange={(e) =>
                      set.mutate({ memberId: d.member_id, access: e.target.value as MailboxDelegateAccess })
                    }
                    data-testid="delegate-access"
                  >
                    <NativeSelectOption value="read">{labels.read}</NativeSelectOption>
                    <NativeSelectOption value="send">{labels.send}</NativeSelectOption>
                  </NativeSelect>
                  <Button
                    variant="outline"
                    size="sm"
                    className="text-destructive"
                    disabled={remove.isPending}
                    onClick={() => {
                      const who = person
                      confirm({
                        title: <Trans>Remove {who}?</Trans>,
                        description: <Trans>They stop reading this mailbox at once.</Trans>,
                        confirm: <Trans>Remove</Trans>,
                        run: () => remove.mutate(d.member_id),
                      })
                    }}
                    data-testid="delegate-remove"
                  >
                    <Trans>Remove</Trans>
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
        <form
          className="flex flex-wrap items-end gap-3 px-[18px] py-3.5"
          onSubmit={(e) => {
            e.preventDefault()
            if (member) set.mutate({ memberId: member, access })
          }}
        >
          <Field label={<Trans>Add a member</Trans>} className="min-w-48 flex-1">
            <MemberSelect
              value={member}
              onChange={setMember}
              exclude={members.filter((m) => taken.includes(m.id)).map((m) => m.id)}
              label={t`Add a member`}
              placeholder={t`Pick a member`}
              testId="delegate-add-member"
            />
          </Field>
          <Field label={<Trans>Access</Trans>}>
            <NativeSelect
              size="sm"
              value={access}
              aria-label={t`Access`}
              onChange={(e) => setAccess(e.target.value as MailboxDelegateAccess)}
              data-testid="delegate-add-access"
            >
              <NativeSelectOption value="read">{labels.read}</NativeSelectOption>
              <NativeSelectOption value="send">{labels.send}</NativeSelectOption>
            </NativeSelect>
          </Field>
          <Button type="submit" size="sm" disabled={!member || set.isPending} data-testid="delegate-add">
            <Trans>Add</Trans>
          </Button>
        </form>
        <div className="px-[18px] pb-3">
          <ErrorLine error={list.error ?? set.error ?? remove.error} />
        </div>
      </Block>
      {confirmDialog}
    </div>
  )
}
