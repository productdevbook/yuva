import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  EyeIcon,
  EyeOffIcon,
  FolderPlusIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
} from "lucide-react"
import { useState } from "react"

import { ErrorLine, toast, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { STATUS_KEYS } from "@/components/common/ShortcutSheet"
import { Skeleton } from "@/components/ui/skeleton"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { ComponentForm, GroupForm } from "@/features/status/ComponentForms"
import {
  useComponentMutations,
  useGroupMutations,
  usePublicStatus,
  useStatusComponents,
  useStatusGroups,
} from "@/features/status/queries"
import { StatePill } from "@/features/status/StateSelect"
import { UptimeBars, usePercent } from "@/features/status/UptimeBars"
import { Card, EmptyRow, Rows } from "@/features/settings/ui"
import { useSession } from "@/lib/session"
import type { StatusComponent, StatusComponentGroup, StatusPage } from "@/lib/api"
import type { components } from "@/lib/schema.gen"

type PublicComponent = components["schemas"]["PublicStatusComponent"]

type Section = { group: StatusComponentGroup | null; items: StatusComponent[] }

function Row({
  page,
  c,
  index,
  count,
  onEdit,
  onMove,
  uptime,
}: {
  page: StatusPage
  c: StatusComponent
  uptime?: PublicComponent
  index: number
  count: number
  onEdit: () => void
  onMove: (dir: -1 | 1) => void
}) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const m = useComponentMutations(page.id)
  const [confirm, dialog] = useConfirm()
  const since = formatDateTime(c.state_since, i18n.locale)
  const percent = usePercent()
  const share = uptime?.uptime === undefined ? "" : percent(uptime.uptime)
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3" data-testid="status-component">
      <div className="min-w-0 flex-1 basis-48">
        <p className="flex items-center gap-2 text-body font-medium">
          <span className="min-w-0 truncate">{c.name}</span>
          {c.hidden && (
            <span className="inline-flex shrink-0 items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 text-caption font-normal text-muted-foreground">
              <EyeOffIcon className="size-3" />
              <Trans>Hidden</Trans>
            </span>
          )}
        </p>
        <p className="mt-px truncate text-small text-faint">
          {c.state_source === "incident" && c.state !== "operational" ? (
            <Trans>Held by an incident, {since}</Trans>
          ) : c.state_source === "monitor" && c.state !== "operational" ? (
            <Trans>Set by a monitor, {since}</Trans>
          ) : (
            <Trans>Since {since}</Trans>
          )}
          {c.description ? ` · ${c.description}` : ""}
        </p>
      </div>
      <div className="flex items-center gap-1 phone:w-full">
        {share && (
          <span className="me-2 shrink-0 text-small text-faint tabular-nums" data-testid="status-component-uptime">
            <Trans>{share} uptime</Trans>
          </span>
        )}
        <StatePill
          value={c.state}
          label={t`State of ${c.name}`}
          disabled={m.setState.isPending}
          onChange={(state) => m.setState.mutate({ id: c.id, state })}
          testId="status-component-state"
        />
        {canManage && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" size="icon-sm" aria-label={t`More for ${c.name}`} />}
              data-testid="status-component-menu"
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={onEdit}>
                <PencilIcon />
                <Trans>Edit</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem disabled={index === 0} onClick={() => onMove(-1)}>
                <ArrowUpIcon />
                <Trans>Move up</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem disabled={index === count - 1} onClick={() => onMove(1)}>
                <ArrowDownIcon />
                <Trans>Move down</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => m.update.mutate({ id: c.id, hidden: !c.hidden })}>
                {c.hidden ? <EyeIcon /> : <EyeOffIcon />}
                {c.hidden ? <Trans>Show on the page</Trans> : <Trans>Hide from the page</Trans>}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onClick={() =>
                  confirm({
                    title: <Trans>Delete {c.name}?</Trans>,
                    description: <Trans>Its history is deleted. Incidents that named it no longer do.</Trans>,
                    confirm: <Trans>Delete component</Trans>,
                    run: () => m.remove.mutate(c.id, { onSuccess: () => toast(t`Component deleted`) }),
                  })
                }
              >
                <Trash2Icon />
                <Trans>Delete</Trans>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      {uptime?.days && uptime.days.length > 0 && (
        <UptimeBars days={uptime.days} count={page.history_days} className="basis-full" />
      )}
      <ErrorLine error={m.setState.error ?? m.update.error ?? m.remove.error} className="basis-full text-caption" />
      {dialog}
    </li>
  )
}

export function ComponentsTab({ page }: { page: StatusPage }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const groups = useStatusGroups(page.id)
  const components = useStatusComponents(page.id)
  const gm = useGroupMutations(page.id)
  const cm = useComponentMutations(page.id)
  const [confirm, dialog] = useConfirm()
  const pub = usePublicStatus(page)
  const uptime = new Map((pub.data?.components ?? []).map((c) => [c.id, c]))
  const days = page.history_days
  const [dialogState, setDialog] = useState<
    | { kind: "component"; edit?: StatusComponent; group?: string }
    | { kind: "group"; edit?: StatusComponentGroup }
    | null
  >(null)
  const gs = groups.data ?? []
  const cs = components.data ?? []
  const sections: Section[] = [
    ...gs.map((g) => ({ group: g, items: cs.filter((c) => c.group_id === g.id) })),
    ...(cs.some((c) => !c.group_id) ? [{ group: null, items: cs.filter((c) => !c.group_id) }] : []),
  ]

  const moveComponent = (items: StatusComponent[], from: number, dir: -1 | 1) => {
    const next = [...items]
    const [moved] = next.splice(from, 1)
    next.splice(from + dir, 0, moved)
    next.forEach((c, i) => c.position !== i && cm.update.mutate({ id: c.id, position: i }))
  }
  const moveGroup = (from: number, dir: -1 | 1) => {
    const next = [...gs]
    const [moved] = next.splice(from, 1)
    next.splice(from + dir, 0, moved)
    next.forEach((g, i) => g.position !== i && gm.update.mutate({ id: g.id, position: i }))
  }

  useHotkeys(canManage ? { [STATUS_KEYS.addComponent]: () => setDialog({ kind: "component" }) } : {}, !dialogState)

  if (components.isPending || groups.isPending) return <Skeleton className="h-40 w-full rounded-2xl" />
  return (
    <div className="flex flex-col gap-4" data-testid="status-components">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-60 flex-1">
          <p className="text-body text-muted-foreground">
            <Trans>The parts of your product. Set a state by hand, or let an incident or a monitor do it.</Trans>
          </p>
          <p className="mt-0.5 text-small text-faint" data-testid="status-uptime-note">
            {!page.show_uptime ? (
              <Trans>Uptime is hidden on this page. Turn it on in the settings to see each day here.</Trans>
            ) : !page.published ? (
              <Trans>Each day’s uptime shows here once the page is published.</Trans>
            ) : (
              <Trans>The bars are the last {days} days, as visitors see them.</Trans>
            )}
          </p>
        </div>
        {canManage && (
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDialog({ kind: "group" })}
              data-testid="status-add-group"
            >
              <FolderPlusIcon />
              <Trans>Add group</Trans>
            </Button>
            <Button size="sm" onClick={() => setDialog({ kind: "component" })} data-testid="status-add-component">
              <PlusIcon />
              <Trans>Add component</Trans>
            </Button>
          </div>
        )}
      </div>
      {dialogState?.kind === "component" && (
        <ComponentForm
          key={dialogState.edit?.id ?? dialogState.group ?? "new"}
          pageId={page.id}
          groups={gs}
          edit={dialogState.edit}
          group={dialogState.group}
          className="mt-0"
          onClose={() => setDialog(null)}
        />
      )}
      {dialogState?.kind === "group" && (
        <GroupForm
          key={dialogState.edit?.id ?? "new"}
          pageId={page.id}
          edit={dialogState.edit}
          count={gs.length}
          className="mt-0"
          onClose={() => setDialog(null)}
        />
      )}
      <ErrorLine error={components.error ?? groups.error ?? gm.update.error ?? cm.update.error} />
      {sections.length === 0 && (
        <Card flush>
          <EmptyRow>
            <Trans>No components yet. Add the parts of your product that customers depend on.</Trans>
          </EmptyRow>
        </Card>
      )}
      {sections.map((s) => {
        const gi = s.group ? gs.findIndex((g) => g.id === s.group!.id) : -1
        return (
          <section key={s.group?.id ?? "none"} className="flex flex-col gap-2" data-testid="status-group">
            <Card flush>
              {(s.group || gs.length > 0) && (
                <div className="flex items-center gap-2 border-b bg-surface px-4 py-2">
                  <h2 className="min-w-0 flex-1 truncate text-small font-medium text-muted-foreground">
                    {s.group ? s.group.name : <Trans>No group</Trans>}
                  </h2>
                  {s.group && canManage && (
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={<Button variant="ghost" size="icon-xs" aria-label={t`More for ${s.group.name}`} />}
                      >
                        <MoreHorizontalIcon />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => setDialog({ kind: "component", group: s.group!.id })}>
                          <PlusIcon />
                          <Trans>Add component</Trans>
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => setDialog({ kind: "group", edit: s.group! })}>
                          <PencilIcon />
                          <Trans>Rename</Trans>
                        </DropdownMenuItem>
                        <DropdownMenuItem disabled={gi <= 0} onClick={() => moveGroup(gi, -1)}>
                          <ArrowUpIcon />
                          <Trans>Move up</Trans>
                        </DropdownMenuItem>
                        <DropdownMenuItem disabled={gi >= gs.length - 1} onClick={() => moveGroup(gi, 1)}>
                          <ArrowDownIcon />
                          <Trans>Move down</Trans>
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() =>
                            confirm({
                              title: <Trans>Delete {s.group!.name}?</Trans>,
                              description: <Trans>Its components stay, without a group.</Trans>,
                              confirm: <Trans>Delete group</Trans>,
                              run: () => gm.remove.mutate(s.group!.id),
                            })
                          }
                        >
                          <Trash2Icon />
                          <Trans>Delete group</Trans>
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </div>
              )}
              {s.items.length === 0 ? (
                <EmptyRow>
                  <Trans>No components in this group.</Trans>
                </EmptyRow>
              ) : (
                <Rows>
                  {s.items.map((c, i) => (
                    <Row
                      key={c.id}
                      page={page}
                      c={c}
                      index={i}
                      count={s.items.length}
                      onEdit={() => setDialog({ kind: "component", edit: c })}
                      onMove={(dir) => moveComponent(s.items, i, dir)}
                      uptime={uptime.get(c.id)}
                    />
                  ))}
                </Rows>
              )}
            </Card>
          </section>
        )
      })}
      <ErrorLine error={gm.remove.error} />
      {dialog}
    </div>
  )
}
