import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { MessageSquareTextIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { EmptyState, ErrorLine, useConfirm } from "@/components/common"
import { slugify } from "@/components/settings/InboxesSettings"
import { Field, PageTitle, Section } from "@/components/settings/SettingsLayout"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { api, unwrap, type CannedReply } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useCannedReplies } from "@/lib/queries"
import { useSession } from "@/lib/session"

export function CannedRepliesSettings() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const replies = useCannedReplies()
  const [editing, setEditing] = useState<CannedReply | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/canned-replies/{cannedReplyId}", { params: { path: { cannedReplyId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.cannedReplies(ws) }),
  })
  return (
    <>
      <div className="flex items-center justify-between gap-3">
        <PageTitle>
          <Trans>Canned replies</Trans>
        </PageTitle>
        {canManage && (
          <Button onClick={() => setEditing("new")}>
            <PlusIcon />
            <Trans>New canned reply</Trans>
          </Button>
        )}
      </div>
      <Section
        title={<Trans>Saved answers</Trans>}
        description={<Trans>Type / and the shortcut in the composer to insert one.</Trans>}
      >
        {replies.data && replies.data.length === 0 ? (
          <EmptyState icon={MessageSquareTextIcon} title={<Trans>No canned replies yet</Trans>} className="py-8" />
        ) : (
          <ul className="flex flex-col divide-y rounded-lg border">
            {(replies.data ?? []).map((r) => (
              <li key={r.id} className="flex items-start gap-3 px-3 py-2.5" data-testid="canned-row">
                <code className="mt-0.5 shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs">/{r.shortcut}</code>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{r.title}</p>
                  <p className="line-clamp-2 text-xs text-muted-foreground">{r.body}</p>
                </div>
                {canManage && (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={() => setEditing(r)}>
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t`Delete`}
                      onClick={() =>
                        confirm({
                          title: <Trans>Delete this canned reply?</Trans>,
                          confirm: <Trans>Delete</Trans>,
                          run: () => remove.mutate(r.id),
                        })
                      }
                    >
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </li>
            ))}
          </ul>
        )}
        <ErrorLine error={replies.error ?? remove.error} />
      </Section>
      {editing && <ReplyDialog reply={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {confirmDialog}
    </>
  )
}

function ReplyDialog({ reply, onClose }: { reply: CannedReply | null; onClose: () => void }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [title, setTitle] = useState(reply?.title ?? "")
  const [shortcut, setShortcut] = useState(reply?.shortcut ?? "")
  const [touched, setTouched] = useState(!!reply)
  const [body, setBody] = useState(reply?.body ?? "")
  const save = useMutation({
    mutationFn: () =>
      reply
        ? unwrap(
            api.PATCH("/v1/canned-replies/{cannedReplyId}", {
              params: { path: { cannedReplyId: reply.id } },
              body: { title, shortcut, body },
            }),
          )
        : unwrap(api.POST("/v1/canned-replies", { body: { title, shortcut, body } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.cannedReplies(ws) })
      onClose()
    },
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>{reply ? <Trans>Edit canned reply</Trans> : <Trans>New canned reply</Trans>}</DialogTitle>
          </DialogHeader>
          <Field label={<Trans>Title</Trans>} htmlFor="canned-title">
            <Input
              id="canned-title"
              required
              maxLength={200}
              value={title}
              placeholder={t`Thanks for writing`}
              onChange={(e) => {
                setTitle(e.target.value)
                if (!touched) setShortcut(slugify(e.target.value))
              }}
            />
          </Field>
          <Field
            label={<Trans>Shortcut</Trans>}
            htmlFor="canned-shortcut"
            hint={<Trans>Lowercase letters, digits and dashes.</Trans>}
          >
            <Input
              id="canned-shortcut"
              required
              pattern="[a-z0-9]+(-[a-z0-9]+)*"
              maxLength={64}
              value={shortcut}
              onChange={(e) => {
                setTouched(true)
                setShortcut(e.target.value)
              }}
            />
          </Field>
          <Field label={<Trans>Text</Trans>} htmlFor="canned-body">
            <Textarea
              id="canned-body"
              required
              rows={6}
              value={body}
              onChange={(e) => setBody(e.target.value)}
            />
          </Field>
          <ErrorLine error={save.error} />
          <DialogFooter>
            <Button type="submit" disabled={save.isPending}>
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
