import { isPersonal } from "@/features/mail/folders"
import {
  DndContext,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core"
import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import {
  ChevronDownIcon,
  ChevronUpIcon,
  GripVerticalIcon,
  PencilIcon,
  ScrollTextIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from "lucide-react"
import { useState } from "react"

import { ErrorLine, Notice, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { RuleDialog } from "@/features/settings/automation/RuleDialog"
import { RunLog } from "@/features/settings/automation/RunLog"
import { useAuthorityCheck, useAutomationRules } from "@/features/settings/automation/queries"
import { LIMIT, useAutomationText } from "@/features/settings/automation/text"
import { Intro } from "@/features/settings/GeneralTabs"
import { Card, headButton, Pill, Row, Rows } from "@/features/settings/ui"
import { api, unwrap, type AutomationRule } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useCannedReplies, useInboxes } from "@/lib/workspace"

function Summary({ rule }: { rule: AutomationRule }) {
  const text = useAutomationText()
  const { t } = useLingui()
  const idle = rule.idle_minutes ?? 0
  const when = rule.trigger === "idle" ? t`Idle for ${idle} min` : text.trigger[rule.trigger]
  const then = rule.actions.map((a) => text.action[a.type]).join(", ")
  const n = rule.conditions.length
  return (
    <>
      <p className="text-caption text-muted-foreground">
        {when}
        <span aria-hidden> · </span>
        {n === 0 ? (
          rule.trigger === "mail_received" ? (
            <Trans>every mail</Trans>
          ) : (
            <Trans>every conversation</Trans>
          )
        ) : (
          <Trans>Conditions: {n}</Trans>
        )}
      </p>
      <p className="text-caption text-faint">{then}</p>
    </>
  )
}

function Warning({ rule }: { rule: AutomationRule }) {
  const replies = useCannedReplies().data ?? []
  const authority = useAuthorityCheck()
  const problem = authority.problem(rule, replies)
  if (!problem) return null
  const title = problem.reply?.title ?? ""
  return (
    <Notice tone="warning" icon={TriangleAlertIcon} className="mt-1.5" data-testid="rule-reply-warning">
      <p className="text-small">
        {problem.rule && problem.reply ? (
          <Trans>
            This reply will not be sent until an owner or admin saves both this rule and the canned reply {title}, or
            bots may send is turned on in the API key settings.
          </Trans>
        ) : problem.rule ? (
          <Trans>
            This reply will not be sent until an owner or admin saves this rule, or bots may send is turned on in the
            API key settings.
          </Trans>
        ) : (
          <Trans>
            This reply will not be sent until an owner or admin saves the canned reply {title}, or bots may send is
            turned on in the API key settings.
          </Trans>
        )}
      </p>
    </Notice>
  )
}

function RuleRow({
  rule,
  index,
  total,
  busy,
  onEdit,
  onLog,
  onMove,
  onDelete,
  onToggle,
}: {
  rule: AutomationRule
  index: number
  total: number
  busy: boolean
  onEdit: () => void
  onLog: () => void
  onMove: (dir: -1 | 1) => void
  onDelete: () => void
  onToggle: (on: boolean) => void
}) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const { setNodeRef: dragRef, listeners, attributes, isDragging } = useDraggable({ id: rule.id })
  const { setNodeRef: dropRef, isOver } = useDroppable({ id: rule.id })
  const name = rule.name
  const inbox = rule.inbox_id ? (inboxes.find((i) => i.id === rule.inbox_id)?.name ?? "?") : undefined
  return (
    <Row
      ref={dropRef}
      className={cn("flex-wrap items-start gap-x-2 gap-y-1", isOver && "bg-brand-wash", isDragging && "opacity-50")}
      data-testid="automation-row"
      data-id={rule.id}
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
      <div className="min-w-0 flex-1 basis-48">
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-body font-medium">
          <span className="truncate">{rule.name}</span>
          {inbox && <Pill tone="muted">{inbox}</Pill>}
        </p>
        <Summary rule={rule} />
      </div>
      <Switch
        checked={rule.enabled}
        disabled={busy}
        onCheckedChange={onToggle}
        aria-label={t`Enabled: ${name}`}
        className="mt-1.5"
        data-testid="automation-enabled"
      />
      <Button variant="ghost" size="icon-sm" aria-label={t`Log`} onClick={onLog} data-testid="automation-log">
        <ScrollTextIcon />
      </Button>
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
      <Button variant="ghost" size="icon-sm" aria-label={t`Delete`} data-testid="automation-delete" onClick={onDelete}>
        <Trash2Icon />
      </Button>
      <div className="basis-full ps-10">
        <Warning rule={rule} />
      </div>
    </Row>
  )
}

export function RuleList({
  inboxId,
  filter = false,
  intro,
}: {
  inboxId?: string
  filter?: boolean
  intro: React.ReactNode
}) {
  const inboxes = useInboxes().data ?? []
  const personal = isPersonal(inboxes.find((i) => i.id === inboxId))
  const query = useAutomationRules(inboxId)
  const workspace = useAutomationRules(undefined, !!inboxId && !personal)
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const [editing, setEditing] = useState<AutomationRule | "new" | null>(null)
  const [logging, setLogging] = useState<AutomationRule | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const rules = [...(query.data ?? [])]
    .filter((r) => !filter || r.trigger === "mail_received")
    .sort((a, b) => a.position - b.position)
  const refresh = () => void qc.invalidateQueries({ queryKey: keys.automationRules(ws) })
  const remove = useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/automation-rules/{automationRuleId}", { params: { path: { automationRuleId: id } } })),
    onSuccess: refresh,
  })
  const toggle = useMutation({
    mutationFn: ({ id, on }: { id: string; on: boolean }) =>
      unwrap(
        api.PATCH("/v1/automation-rules/{automationRuleId}", {
          params: { path: { automationRuleId: id } },
          body: { enabled: on },
        })
      ),
    onSettled: refresh,
  })
  const reorder = useMutation({
    mutationFn: (ids: string[]) => {
      if (personal || !inboxId) return unwrap(api.PUT("/v1/automation-rules/order", { body: { ids } }))
      const mine = new Set(ids)
      const queue = [...ids]
      const everyone = [...(workspace.data ?? [])].sort((a, b) => a.position - b.position).map((r) => r.id)
      const full = everyone.map((id) => (mine.has(id) ? queue.shift()! : id))
      return unwrap(api.PUT("/v1/automation-rules/order", { body: { ids: full } }))
    },
    onMutate: (ids) => {
      qc.setQueryData<AutomationRule[]>(keys.automationRules(ws, inboxId), (old) =>
        old ? ids.map((id, position) => ({ ...old.find((r) => r.id === id)!, position })) : old
      )
    },
    onSettled: refresh,
  })
  const full = rules.length >= LIMIT

  const place = (from: number, to: number) => {
    if (to < 0 || to >= rules.length || from === to) return
    const ids = rules.map((r) => r.id)
    ids.splice(to, 0, ids.splice(from, 1)[0]!)
    reorder.mutate(ids)
  }
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    place(
      rules.findIndex((r) => r.id === active.id),
      rules.findIndex((r) => r.id === over.id)
    )
  }

  return (
    <>
      <Intro
        action={
          <span className="flex items-center gap-3">
            <span className="text-small text-faint tabular-nums" data-testid="automation-count">
              {rules.length}/{LIMIT}
            </span>
            <Button
              variant="outline"
              className={headButton.secondary}
              disabled={full}
              onClick={() => setEditing("new")}
              data-testid="automation-new"
            >
              {filter ? <Trans>New filter</Trans> : <Trans>New rule</Trans>}
            </Button>
          </span>
        }
      >
        {intro}
      </Intro>
      <ErrorLine error={query.error} />
      <Card flush>
        {rules.length === 0 ? (
          <p className="px-4 py-4 text-faint" data-testid="automation-empty">
            {filter ? <Trans>No filters yet.</Trans> : <Trans>No rules yet.</Trans>}
          </p>
        ) : (
          <DndContext sensors={sensors} onDragEnd={onDragEnd}>
            <Rows>
              {rules.map((r, i) => (
                <RuleRow
                  key={r.id}
                  rule={r}
                  index={i}
                  total={rules.length}
                  busy={reorder.isPending}
                  onEdit={() => setEditing(r)}
                  onLog={() => setLogging(r)}
                  onMove={(dir) => place(i, i + dir)}
                  onToggle={(on) => toggle.mutate({ id: r.id, on })}
                  onDelete={() => {
                    const name = r.name
                    confirm({
                      title: filter ? <Trans>Delete the filter {name}?</Trans> : <Trans>Delete the rule {name}?</Trans>,
                      description: (
                        <Trans>Its log goes with it. What it already changed stays. This cannot be undone.</Trans>
                      ),
                      confirm: <Trans>Delete</Trans>,
                      run: () => remove.mutate(r.id),
                    })
                  }}
                />
              ))}
            </Rows>
          </DndContext>
        )}
        <ErrorLine error={remove.error ?? reorder.error ?? toggle.error} className="px-5 pb-4" />
      </Card>
      {confirmDialog}
      {editing && (
        <RuleDialog
          key={editing === "new" ? "new" : editing.id}
          rule={editing === "new" ? null : editing}
          inboxId={inboxId}
          filter={filter}
          onClose={() => setEditing(null)}
        />
      )}
      {logging && <RunLog rule={logging} onClose={() => setLogging(null)} />}
    </>
  )
}
