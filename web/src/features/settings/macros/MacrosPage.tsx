import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, Trash2Icon, ZapIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Button } from "@/components/ui/button"
import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { MacroDialog } from "@/features/settings/macros/MacroDialog"
import { Card, Chip, headButton, Row, Rows } from "@/features/settings/ui"
import { api, unwrap, type Macro } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useMacros } from "@/lib/workspace"

export function MacrosPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const macros = useMacros()
  const [editing, setEditing] = useState<Macro | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/macros/{macroId}", { params: { path: { macroId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.macros(ws) }),
  })
  const add = (
    <Button className={headButton.primary} onClick={() => setEditing("new")} data-testid="macro-new">
      <Trans>New macro</Trans>
    </Button>
  )
  return (
    <>
      <GeneralHeader current="macros" />
      <Intro action={macros.data && macros.data.length > 0 ? add : undefined}>
        <Trans>
          A macro is a reply plus actions: labels, assignee, status, priority or snooze. Type / and its shortcut in the
          composer; the actions apply when the reply is sent.
        </Trans>
      </Intro>
      {macros.data && macros.data.length === 0 ? (
        <EmptyBox icon={ZapIcon} title={<Trans>No macros yet</Trans>} actions={add} testId="macros-empty">
          <Trans>Save a reply together with what should happen to the conversation when you send it.</Trans>
        </EmptyBox>
      ) : (
        <Card flush>
          <Rows>
            {(macros.data ?? []).map((m) => {
              const editable = m.visibility === "personal" || canManage
              return (
                <Row key={m.id} className="items-start" data-testid="macro-row">
                  <code className="mt-0.5 shrink-0 rounded-md bg-surface px-1.5 py-px font-mono text-caption text-muted-foreground">
                    /{m.shortcut}
                  </code>
                  <div className="min-w-0 flex-1">
                    <p className="flex items-center gap-2 text-body font-medium">
                      <span className="truncate">{m.name}</span>
                      <Chip>{m.visibility === "shared" ? <Trans>Shared</Trans> : <Trans>Personal</Trans>}</Chip>
                    </p>
                    <p className="truncate text-caption text-muted-foreground">
                      {m.body ?? <Trans>Actions only</Trans>}
                    </p>
                  </div>
                  {editable && (
                    <>
                      <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={() => setEditing(m)}>
                        <PencilIcon />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t`Delete`}
                        onClick={() =>
                          confirm({
                            title: <Trans>Delete this macro?</Trans>,
                            confirm: <Trans>Delete</Trans>,
                            run: () => remove.mutate(m.id),
                          })
                        }
                      >
                        <Trash2Icon />
                      </Button>
                    </>
                  )}
                </Row>
              )
            })}
          </Rows>
          <ErrorLine error={macros.error ?? remove.error} className="px-5 pb-4" />
        </Card>
      )}
      {editing && <MacroDialog macro={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {confirmDialog}
    </>
  )
}
