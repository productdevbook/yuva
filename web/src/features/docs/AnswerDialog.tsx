import { Trans, useLingui } from "@lingui/react/macro"
import { GlobeIcon, PencilIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, Notice, toast, useConfirm } from "@/components/common"
import { formatDateTime, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"
import { usePublish, useUnpublish, useUpdateAnswer, type AnswerText } from "@/features/docs/queries"
import { Field } from "@/features/settings/ui"
import type { PageAnswer } from "@/lib/api"

export function pageLabel(url: string, title?: string) {
  if (title) return title
  try {
    const u = new URL(url)
    return u.host + (u.pathname === "/" ? "" : u.pathname)
  } catch {
    return url
  }
}

function AnswerForm({
  title,
  page,
  initial,
  submit,
  pending,
  error,
  submitLabel,
  onClose,
}: {
  title: React.ReactNode
  page: string
  initial: AnswerText
  submit: (v: AnswerText) => void
  pending: boolean
  error: unknown
  submitLabel: React.ReactNode
  onClose: () => void
}) {
  const [question, setQuestion] = useState(initial.question)
  const [answer, setAnswer] = useState(initial.answer)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl" data-testid="answer-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit({ question: question.trim(), answer: answer.trim() })
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription className="break-all">{page}</DialogDescription>
          </DialogHeader>
          <Notice tone="warning" icon={GlobeIcon} data-testid="answer-public-note">
            <p>
              <Trans>Anyone who opens the page can read this question and answer. Remove names, e-mail addresses, order numbers and anything else personal.</Trans>
            </p>
          </Notice>
          <Field label={<Trans>Question</Trans>} htmlFor="answer-question">
            <Textarea id="answer-question" required rows={3} maxLength={2000} value={question} onChange={(e) => setQuestion(e.target.value)} data-testid="answer-question" />
          </Field>
          <Field label={<Trans>Answer</Trans>} htmlFor="answer-answer">
            <Textarea id="answer-answer" required rows={8} maxLength={20000} value={answer} onChange={(e) => setAnswer(e.target.value)} data-testid="answer-answer" />
          </Field>
          <ErrorLine error={error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={pending || !question.trim() || !answer.trim()} data-testid="answer-submit">
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function PublishDialog({ conversationId, page, initial, onClose }: { conversationId: string; page: string; initial: AnswerText; onClose: () => void }) {
  const { t } = useLingui()
  const publish = usePublish(conversationId)
  return (
    <AnswerForm
      title={<Trans>Publish to the page</Trans>}
      page={page}
      initial={initial}
      pending={publish.isPending}
      error={publish.error}
      submitLabel={<Trans>Publish</Trans>}
      onClose={onClose}
      submit={(v) =>
        publish.mutate(v, {
          onSuccess: () => {
            toast(t`Published to the page`)
            onClose()
          },
        })
      }
    />
  )
}

export function EditAnswerDialog({ answer, onClose }: { answer: PageAnswer; onClose: () => void }) {
  const { t } = useLingui()
  const update = useUpdateAnswer(answer.id)
  return (
    <AnswerForm
      title={<Trans>Edit the published answer</Trans>}
      page={answer.page}
      initial={{ question: answer.question, answer: answer.answer }}
      pending={update.isPending}
      error={update.error}
      submitLabel={<Trans>Save</Trans>}
      onClose={onClose}
      submit={(v) =>
        update.mutate(v, {
          onSuccess: () => {
            toast(t`Published answer saved`)
            onClose()
          },
        })
      }
    />
  )
}

export function useAnswerActions() {
  const { t } = useLingui()
  const errorText = useErrorText()
  const unpublish = useUnpublish()
  const [confirm, confirmDialog] = useConfirm()
  const [editing, setEditing] = useState<PageAnswer | null>(null)
  const askUnpublish = (a: PageAnswer) =>
    confirm({
      title: <Trans>Unpublish this answer?</Trans>,
      description: <Trans>It disappears from the page. The conversation stays and can be published again.</Trans>,
      confirm: <Trans>Unpublish</Trans>,
      run: () => unpublish.mutate(a.id, { onSuccess: () => toast(t`Unpublished`), onError: (e) => toast(errorText(e)) }),
    })
  const dialogs = (
    <>
      {confirmDialog}
      {editing && <EditAnswerDialog answer={editing} onClose={() => setEditing(null)} />}
    </>
  )
  return { edit: setEditing, unpublish: askUnpublish, dialogs }
}

export function AnswerCard({ a, onEdit, onUnpublish, footer }: { a: PageAnswer; onEdit: () => void; onUnpublish: () => void; footer?: React.ReactNode }) {
  const { t, i18n } = useLingui()
  const edited = formatDateTime(a.updated_at, i18n.locale)
  const published = formatDateTime(a.published_at, i18n.locale)
  return (
    <article className="flex flex-col gap-2 px-4 py-3.5" data-testid="page-answer">
      <h3 className="text-body font-medium break-words whitespace-pre-wrap">{a.question}</h3>
      <p className="text-body break-words whitespace-pre-wrap text-muted-foreground">{a.answer}</p>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-caption text-faint">
        <time dateTime={a.updated_at} className="me-auto">
          {a.updated_at !== a.published_at ? t`Edited ${edited}` : t`Published ${published}`}
        </time>
        {footer}
        <Button variant="ghost" size="sm" onClick={onEdit} data-testid="answer-edit">
          <PencilIcon />
          <Trans>Edit</Trans>
        </Button>
        <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={onUnpublish} data-testid="answer-unpublish">
          <Trash2Icon />
          <Trans>Unpublish</Trans>
        </Button>
      </div>
    </article>
  )
}
