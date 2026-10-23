import {
  DndContext,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core"
import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ChevronDownIcon, ChevronUpIcon, GripVerticalIcon, PencilIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { Card, headButton, Pill, Row, Rows } from "@/features/settings/ui"
import { api, unwrap, type SlaPolicy } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useSlaPolicies } from "@/lib/workspace"
import { PolicyDialog } from "@/features/settings/sla/PolicyDialog"

const LIMIT = 100

function useMinutesText() {
  const { i18n } = useLingui()
  return (minutes: number) => {
    const unit = minutes >= 60 && minutes % 60 === 0 ? "hour" : "minute"
    return new Intl.NumberFormat(i18n.locale, { style: "unit", unit, unitDisplay: "short" }).format(
      unit === "hour" ? minutes / 60 : minutes
    )
  }
}

function Summary({ p }: { p: SlaPolicy }) {
  const minutes = useMinutesText()
  const first = p.first_reply_minutes !== undefined ? minutes(p.first_reply_minutes) : undefined
  const next = p.next_reply_minutes !== undefined ? minutes(p.next_reply_minutes) : undefined
  const resolution = p.resolution_minutes !== undefined ? minutes(p.resolution_minutes) : undefined
  return (
    <span className="flex flex-wrap gap-x-3 gap-y-0.5">
      {first && <Trans>First reply {first}</Trans>}
      {next && <Trans>Next reply {next}</Trans>}
      {resolution && <Trans>Resolution {resolution}</Trans>}
    </span>
  )
}

function Scope({ p }: { p: SlaPolicy }) {
  const count = p.inbox_ids.length + p.channel_kinds.length + p.label_ids.length + p.priorities.length
  if (count === 0) return <Trans>Every conversation</Trans>
  return <Plural value={count} one="# condition" other="# conditions" />
}

function PolicyRow({
  p,
  index,
  total,
  busy,
  onEdit,
  onMove,
  onDelete,
}: {
  p: SlaPolicy
  index: number
  total: number
  busy: boolean
  onEdit: () => void
  onMove: (dir: -1 | 1) => void
  onDelete: () => void
}) {
  const { t } = useLingui()
  const { setNodeRef: dragRef, listeners, attributes, isDragging } = useDraggable({ id: p.id })
  const { setNodeRef: dropRef, isOver } = useDroppable({ id: p.id })
  const name = p.name
  return (
    <Row
      ref={dropRef}
      className={cn("items-start gap-2", isOver && "bg-brand-wash", isDragging && "opacity-50")}
      data-testid="sla-row"
      data-id={p.id}
      onKeyDown={(e) => {
        if (!e.altKey || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return
        e.preventDefault()
        onMove(e.key === "ArrowUp" ? -1 : 1)
      }}
    >
      <button
        ref={dragRef}
        type="button"
        className="grid size-8 shrink-0 cursor-grab touch-none place-items-center rounded-md text-faint outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing"
        aria-label={t`Move ${name}. Drag, or press Alt with the arrow keys.`}
        {...attributes}
        {...listeners}
      >
        <GripVerticalIcon className="size-4" />
      </button>
      <div className="min-w-0 flex-1">
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-body font-medium">
          <span className="truncate">{p.name}</span>
          {!p.enabled && (
            <Pill tone="muted">
              <Trans>Off</Trans>
            </Pill>
          )}
        </p>
        <p className="text-caption text-muted-foreground">
          <Summary p={p} />
        </p>
        <p className="text-caption text-faint">
          <Scope p={p} />
        </p>
      </div>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t`Move up`}
        disabled={index === 0 || busy}
        onClick={() => onMove(-1)}
      >
        <ChevronUpIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t`Move down`}
        disabled={index === total - 1 || busy}
        onClick={() => onMove(1)}
      >
        <ChevronDownIcon />
      </Button>
      <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={onEdit}>
        <PencilIcon />
      </Button>
      <Button variant="ghost" size="icon-sm" aria-label={t`Delete`} data-testid="sla-delete" onClick={onDelete}>
        <Trash2Icon />
      </Button>
    </Row>
  )
}

export function SlaPage() {
  const query = useSlaPolicies()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const [editing, setEditing] = useState<SlaPolicy | "new" | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const policies = [...(query.data ?? [])].sort((a, b) => a.position - b.position)
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: keys.slaPolicies(ws) })
    void qc.invalidateQueries({ queryKey: keys.lists(ws) })
  }
  const remove = useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/sla-policies/{slaPolicyId}", { params: { path: { slaPolicyId: id } } })),
    onSuccess: refresh,
  })
  const reorder = useMutation({
    mutationFn: (ids: string[]) => unwrap(api.PUT("/v1/sla-policies/order", { body: { ids } })),
    onMutate: (ids) => {
      qc.setQueryData<SlaPolicy[]>(keys.slaPolicies(ws), (old) =>
        old ? ids.map((id, position) => ({ ...old.find((p) => p.id === id)!, position })) : old
      )
    },
    onSettled: refresh,
  })
  const full = policies.length >= LIMIT

  const place = (from: number, to: number) => {
    if (to < 0 || to >= policies.length || from === to) return
    const ids = policies.map((p) => p.id)
    ids.splice(to, 0, ids.splice(from, 1)[0])
    reorder.mutate(ids)
  }
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    place(
      policies.findIndex((p) => p.id === active.id),
      policies.findIndex((p) => p.id === over.id)
    )
  }

  return (
    <>
      <GeneralHeader current="sla" />
      <Intro
        action={
          <span className="flex items-center gap-3">
            <span className="text-small text-faint tabular-nums" data-testid="sla-count">
              {policies.length}/{LIMIT}
            </span>
            <Button
              className={headButton.secondary}
              disabled={full}
              onClick={() => setEditing("new")}
              data-testid="sla-new"
            >
              <Trans>New policy</Trans>
            </Button>
          </span>
        }
      >
        <Trans>
          Reply and resolution targets for the conversations a policy matches. The first enabled policy that matches a
          conversation applies, so keep the most specific ones on top. Targets count in the business hours of each
          inbox.
        </Trans>
      </Intro>
      <ErrorLine error={query.error} />
      <Card flush>
        {policies.length === 0 ? (
          <p className="px-4 py-4 text-faint" data-testid="sla-empty">
            <Trans>No policies yet.</Trans>
          </p>
        ) : (
          <DndContext sensors={sensors} onDragEnd={onDragEnd}>
            <Rows>
              {policies.map((p, i) => (
                <PolicyRow
                  key={p.id}
                  p={p}
                  index={i}
                  total={policies.length}
                  busy={reorder.isPending}
                  onEdit={() => setEditing(p)}
                  onMove={(dir) => place(i, i + dir)}
                  onDelete={() => {
                    const name = p.name
                    confirm({
                      title: <Trans>Delete the policy {name}?</Trans>,
                      description: (
                        <Trans>
                          Conversations that follow it lose their SLA unless another policy matches. This cannot be
                          undone.
                        </Trans>
                      ),
                      confirm: <Trans>Delete</Trans>,
                      run: () => remove.mutate(p.id),
                    })
                  }}
                />
              ))}
            </Rows>
          </DndContext>
        )}
        <ErrorLine error={remove.error ?? reorder.error} className="px-5 pb-4" />
      </Card>
      {confirmDialog}
      {editing && (
        <PolicyDialog
          key={editing === "new" ? "new" : editing.id}
          policy={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  )
}
