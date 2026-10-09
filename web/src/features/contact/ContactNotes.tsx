import { Trans, useLingui } from "@lingui/react/macro"
import { Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { mod } from "@/components/common/ShortcutSheet"
import { formatDateTime, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { useAddContactNote, useContactNotes, useDeleteContactNote } from "@/features/contact/queries"
import type { ContactNote } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useMemberMap } from "@/lib/workspace"

export function useNoteCount(contactId: string) {
  const notes = useContactNotes(contactId)
  const n = notes.data?.pages.reduce((sum, p) => sum + p.items.length, 0)
  return n === undefined ? undefined : `${n}${notes.hasNextPage ? "+" : ""}`
}

function Note({ note, onDelete }: { note: ContactNote; onDelete?: () => void }) {
  const { t, i18n } = useLingui()
  const members = useMemberMap()
  const a = note.author
  const m = a.member_id ? members.get(a.member_id) : undefined
  const who = a.type === "bot" ? a.name || t`Bot` : m ? m.name || m.email : t`A former teammate`
  return (
    <li className="group/note flex flex-col gap-1 rounded-xl border border-note-border bg-note px-3 py-2.5" data-testid="contact-note">
      <span className="flex items-center gap-2 text-caption font-medium text-note-ink">
        <span className="min-w-0 flex-1 truncate">
          {who} · <time dateTime={note.created_at} title={formatDateTime(note.created_at, i18n.locale)}>{formatShort(note.created_at, i18n.locale)}</time>
        </span>
        {onDelete && (
          <Button variant="ghost" size="icon-xs" className="-my-1 text-note-ink hover:bg-note-border/60" onClick={onDelete} aria-label={t`Delete this note`} data-testid="delete-contact-note">
            <Trash2Icon />
          </Button>
        )}
      </span>
      <p className="break-words whitespace-pre-wrap">{note.body}</p>
    </li>
  )
}

export function ContactNotes({ contactId, form = "bottom" }: { contactId: string; form?: "top" | "bottom" }) {
  const { t } = useLingui()
  const { membership, canManage } = useSession()
  const notes = useContactNotes(contactId)
  const add = useAddContactNote(contactId)
  const del = useDeleteContactNote(contactId)
  const [confirm, confirmDialog] = useConfirm()
  const [body, setBody] = useState("")
  const items = notes.data?.pages.flatMap((p) => p.items) ?? []
  const text = body.trim()
  const submit = () => text && !add.isPending && add.mutate(text, { onSuccess: () => setBody("") })
  const id = `contact-note-${contactId}`

  const editor = (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Label htmlFor={id} className="text-small font-normal text-faint">
        <Trans>Note about this contact</Trans>
      </Label>
      <Textarea
        id={id}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault()
            submit()
          }
        }}
        maxLength={10000}
        rows={2}
        placeholder={t`Something the team should know…`}
        data-testid="contact-note-input"
      />
      <ErrorLine error={add.error} />
      <span className="flex items-center gap-2">
        <Button type="submit" variant="outline" size="sm" disabled={!text || add.isPending} data-testid="contact-note-add">
          <Trans>Add note</Trans>
        </Button>
        <span className="text-caption text-faint">{mod} ↵</span>
      </span>
    </form>
  )

  return (
    <div className="flex flex-col gap-3" data-testid="contact-notes">
      {form === "top" && editor}
      {notes.isPending ? (
        <Skeleton className="h-14 w-full rounded-xl" />
      ) : notes.error ? (
        <ErrorLine error={notes.error} />
      ) : (
        items.length > 0 && (
          <ul className="flex flex-col gap-2">
            {items.map((n) => {
              const mine = !!n.author.member_id && n.author.member_id === membership.member_id
              return (
                <Note
                  key={n.id}
                  note={n}
                  onDelete={
                    mine || canManage
                      ? () => confirm({ title: <Trans>Delete this note?</Trans>, confirm: <Trans>Delete</Trans>, run: () => del.mutate(n.id) })
                      : undefined
                  }
                />
              )
            })}
          </ul>
        )
      )}
      {notes.hasNextPage && (
        <Button variant="ghost" size="sm" className="self-center" onClick={() => void notes.fetchNextPage()} disabled={notes.isFetchingNextPage}>
          <Trans>Load more</Trans>
        </Button>
      )}
      <ErrorLine error={del.error} />
      {form === "bottom" && editor}
      {confirmDialog}
    </div>
  )
}
