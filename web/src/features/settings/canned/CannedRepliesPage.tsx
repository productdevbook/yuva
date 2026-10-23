import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, QuoteIcon, Trash2Icon, TriangleAlertIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { useAuthorText } from "@/components/common/authorText"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Button } from "@/components/ui/button"
import { CannedReplyDialog } from "@/features/settings/canned/CannedReplyDialog"
import { useAuthorityCheck, useAutomationRules } from "@/features/settings/automation/queries"
import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { Card, headButton, Row, Rows } from "@/features/settings/ui"
import { api, unwrap, type CannedReply } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useCannedReplies } from "@/lib/workspace"

function SavedBy({ r }: { r: CannedReply }) {
  const authorText = useAuthorText()
  const authority = useAuthorityCheck()
  const rules = useAutomationRules().data
  const who = authorText(r.saved_by)
  const blocked =
    !authority.botsMaySend &&
    !authority.isManager(r.saved_by) &&
    (rules ?? []).some((x) => x.actions.some((a) => a.type === "send_reply" && a.canned_reply_id === r.id))
  return (
    <>
      <p className="truncate text-caption text-faint" data-testid="canned-saved-by">
        <Trans>Text last saved by {who}</Trans>
      </p>
      {blocked && (
        <p className="flex items-start gap-1.5 text-caption text-warning" data-testid="canned-rule-warning">
          <TriangleAlertIcon className="mt-px size-3.5 shrink-0" />
          <span>
            <Trans>
              An automation rule sends this reply, but it will not go out until an owner or admin saves the text, or
              bots may send is turned on.
            </Trans>
          </span>
        </p>
      )}
    </>
  )
}

export function CannedRepliesPage() {
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
  const add = canManage && (
    <Button className={headButton.primary} onClick={() => setEditing("new")} data-testid="canned-new">
      <Trans>New canned reply</Trans>
    </Button>
  )
  return (
    <>
      <GeneralHeader current="canned-replies" />
      <Intro action={replies.data && replies.data.length > 0 ? add : undefined}>
        <Trans>Type / and the shortcut in the composer to insert one.</Trans>
      </Intro>
      {replies.data && replies.data.length === 0 ? (
        <EmptyBox icon={QuoteIcon} title={<Trans>No canned replies yet</Trans>} actions={add} testId="canned-empty">
          <Trans>Save the answers you write often, then insert them in the composer with a shortcut.</Trans>
        </EmptyBox>
      ) : (
        <Card flush>
          <Rows>
            {(replies.data ?? []).map((r) => (
              <Row key={r.id} className="items-start" data-testid="canned-row">
                <code className="mt-0.5 shrink-0 rounded-md bg-surface px-1.5 py-px font-mono text-caption text-muted-foreground">
                  /{r.shortcut}
                </code>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-body font-medium">{r.title}</p>
                  <p className="truncate text-caption text-muted-foreground">{r.body}</p>
                  <SavedBy r={r} />
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
              </Row>
            ))}
          </Rows>
          <ErrorLine error={replies.error ?? remove.error} className="px-5 pb-4" />
        </Card>
      )}
      {editing && <CannedReplyDialog reply={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {confirmDialog}
    </>
  )
}
