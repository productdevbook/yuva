import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ChevronDownIcon, ChevronUpIcon, PencilIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { byPosition, useFieldTypeText } from "@/features/fields/value"
import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { FieldDialog } from "@/features/settings/fields/FieldDialog"
import { Card, headButton, Pill, Row, Rows, Section } from "@/features/settings/ui"
import { api, unwrap, type CustomField, type CustomFieldObject } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useCustomFields } from "@/lib/workspace"

const LIMIT = 50
const STEP = 10

type Editing = { field: CustomField | null; object: CustomFieldObject }

function ObjectSection({
  object,
  title,
  fields,
  onEdit,
}: {
  object: CustomFieldObject
  title: React.ReactNode
  fields: CustomField[]
  onEdit: (e: Editing) => void
}) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const typeText = useFieldTypeText()
  const [confirm, confirmDialog] = useConfirm()
  const refresh = () => qc.invalidateQueries({ queryKey: keys.customFields(ws) })
  const remove = useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/custom-fields/{customFieldId}", { params: { path: { customFieldId: id } } })),
    onSuccess: refresh,
  })
  const move = useMutation({
    mutationFn: async ({ from, to }: { from: number; to: number }) => {
      const order = [...fields]
      const [item] = order.splice(from, 1)
      order.splice(to, 0, item)
      await Promise.all(
        order.flatMap((f, i) =>
          f.position === i * STEP
            ? []
            : [
                unwrap(
                  api.PATCH("/v1/custom-fields/{customFieldId}", {
                    params: { path: { customFieldId: f.id } },
                    body: { position: i * STEP },
                  })
                ),
              ]
        )
      )
    },
    onSettled: refresh,
  })
  const full = fields.length >= LIMIT
  const add = (
    <span className="flex items-center gap-3">
      <span className="text-small text-faint tabular-nums" data-testid={`fields-count-${object}`}>
        {fields.length}/{LIMIT}
      </span>
      <Button
        variant="outline"
        className={headButton.secondary}
        disabled={full}
        onClick={() => onEdit({ field: null, object })}
        data-testid={`field-new-${object}`}
      >
        <Trans>New field</Trans>
      </Button>
    </span>
  )
  return (
    <Section title={title} action={add}>
      <Card flush>
        {fields.length === 0 ? (
          <p className="px-4 py-4 text-faint" data-testid={`fields-empty-${object}`}>
            <Trans>No fields yet.</Trans>
          </p>
        ) : (
          <Rows>
            {fields.map((f, i) => (
              <Row key={f.id} className="items-start" data-testid="field-row" data-key={f.key}>
                <div className="min-w-0 flex-1">
                  <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-body font-medium">
                    <span className="truncate">{f.name}</span>
                    {f.required_on_close && (
                      <Pill tone="warning">
                        <Trans>Required to close</Trans>
                      </Pill>
                    )}
                  </p>
                  <p className="truncate text-caption text-muted-foreground">
                    {typeText[f.type]}
                    {" · "}
                    <span className="font-mono">{f.key}</span>
                    {f.type === "select" && ` · ${f.options.length}`}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Move up`}
                  disabled={i === 0 || move.isPending}
                  onClick={() => move.mutate({ from: i, to: i - 1 })}
                >
                  <ChevronUpIcon />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Move down`}
                  disabled={i === fields.length - 1 || move.isPending}
                  onClick={() => move.mutate({ from: i, to: i + 1 })}
                >
                  <ChevronDownIcon />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Edit`}
                  onClick={() => onEdit({ field: f, object })}
                >
                  <PencilIcon />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Delete`}
                  data-testid="field-delete"
                  onClick={() => {
                    const name = f.name
                    confirm({
                      title: <Trans>Delete the field {name}?</Trans>,
                      description:
                        object === "conversation" ? (
                          <Trans>
                            Its values are cleared from every conversation, in the background. This cannot be undone.
                          </Trans>
                        ) : (
                          <Trans>
                            Contacts keep the values they hold; they show as plain attributes instead of a typed field.
                          </Trans>
                        ),
                      confirm: <Trans>Delete</Trans>,
                      run: () => remove.mutate(f.id),
                    })
                  }}
                >
                  <Trash2Icon />
                </Button>
              </Row>
            ))}
          </Rows>
        )}
        <ErrorLine error={remove.error ?? move.error} className="px-5 pb-4" />
      </Card>
      {confirmDialog}
    </Section>
  )
}

export function FieldsPage() {
  const fields = useCustomFields()
  const [editing, setEditing] = useState<Editing | null>(null)
  const sorted = [...(fields.data ?? [])].sort(byPosition)
  const of = (o: CustomFieldObject) => sorted.filter((f) => f.object === o)
  const nextPosition = (o: CustomFieldObject) =>
    Math.min(10000, of(o).reduce((m, f) => Math.max(m, f.position), -STEP) + STEP)
  return (
    <>
      <GeneralHeader current="fields" />
      <Intro>
        <Trans>
          Typed values your team keeps on conversations and contacts, such as an order number or a plan. Agents edit
          them beside the thread.
        </Trans>
      </Intro>
      <ErrorLine error={fields.error} />
      <ObjectSection
        object="conversation"
        title={<Trans>Conversation fields</Trans>}
        fields={of("conversation")}
        onEdit={setEditing}
      />
      <ObjectSection
        object="contact"
        title={<Trans>Contact fields</Trans>}
        fields={of("contact")}
        onEdit={setEditing}
      />
      {editing && (
        <FieldDialog
          key={editing.field?.id ?? editing.object}
          field={editing.field}
          object={editing.object}
          nextPosition={nextPosition(editing.object)}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  )
}
