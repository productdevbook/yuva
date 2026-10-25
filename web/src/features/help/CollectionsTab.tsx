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
import {
  ArrowDownIcon,
  ArrowUpIcon,
  FolderPlusIcon,
  FoldersIcon,
  GripVerticalIcon,
  LoaderIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
} from "lucide-react"
import { useState } from "react"
import { useSearchParams } from "react-router"

import { useConfirm } from "@/components/common"
import { HELP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { Segmented } from "@/components/common"
import { ICON_COMPONENTS } from "@/features/help/icons"
import { useCollectionMutations, useHelpCollections } from "@/features/help/queries"
import { collectionName, ErrorLine, HELP_ICONS, useIconText, useLocaleName } from "@/features/help/text"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { HelpCenter, HelpCollection, HelpCollectionTranslationInput, HelpIcon } from "@/lib/api"
import { cn } from "@/lib/utils"

type Moves = { id: string; position: number; parent_id?: string | null }[]

function siblings(all: HelpCollection[], parent: string | undefined) {
  return all.filter((c) => c.parent_id === parent).sort((a, b) => a.position - b.position || a.number - b.number)
}

function renumber(list: HelpCollection[], parent?: string | null): Moves {
  return list.flatMap((c, i) =>
    c.position !== i || (parent !== undefined && (c.parent_id ?? null) !== parent)
      ? [{ id: c.id, position: i, ...(parent !== undefined ? { parent_id: parent } : {}) }]
      : []
  )
}

function CollectionDialog({
  center,
  all,
  edit,
  parentId,
  onClose,
}: {
  center: HelpCenter
  all: HelpCollection[]
  edit?: HelpCollection
  parentId?: string
  onClose: () => void
}) {
  const { t } = useLingui()
  const localeName = useLocaleName()
  const iconText = useIconText()
  const m = useCollectionMutations(center.id)
  const [locale, setLocale] = useState(center.default_locale)
  const [text, setText] = useState<Record<string, { name: string; description: string }>>(() =>
    Object.fromEntries(
      center.locales.map((l) => [
        l,
        { name: edit?.translations[l]?.name ?? "", description: edit?.translations[l]?.description ?? "" },
      ])
    )
  )
  const [icon, setIcon] = useState<HelpIcon | "">(edit?.icon ?? "")
  const [parent, setParent] = useState(edit ? (edit.parent_id ?? "") : (parentId ?? ""))
  const hasChildren = !!edit && all.some((c) => c.parent_id === edit.id)
  const parents = all.filter((c) => !c.parent_id && c.id !== edit?.id)
  const filled = center.locales.filter((l) => text[l].name.trim())
  const ok = filled.length > 0
  const pending = m.create.isPending || m.update.isPending
  const cur = text[locale]
  const patchText = (v: Partial<{ name: string; description: string }>) =>
    setText((all) => ({ ...all, [locale]: { ...all[locale], ...v } }))

  const submit = () => {
    if (!ok) return
    const translations: Record<string, HelpCollectionTranslationInput> = {}
    for (const l of filled) translations[l] = { name: text[l].name.trim(), description: text[l].description.trim() }
    if (edit) {
      const remove_locales = Object.keys(edit.translations).filter((l) => !translations[l])
      m.update.mutate(
        {
          id: edit.id,
          body: {
            translations,
            remove_locales: remove_locales.length ? remove_locales : undefined,
            icon: icon || null,
            ...(parent !== (edit.parent_id ?? "") ? { parent_id: parent || null } : {}),
          },
        },
        { onSuccess: onClose }
      )
    } else {
      m.create.mutate(
        {
          translations,
          icon: icon || undefined,
          parent_id: parent || undefined,
          position: siblings(all, parent || undefined).length,
        },
        { onSuccess: onClose }
      )
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="collection-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{edit ? <Trans>Edit collection</Trans> : <Trans>New collection</Trans>}</DialogTitle>
            <DialogDescription>
              <Trans>A group of articles. Name it in each language you publish; one name is enough to start.</Trans>
            </DialogDescription>
          </DialogHeader>
          {center.locales.length > 1 && (
            <div className="-my-1 overflow-x-auto py-1">
              <Segmented
                value={locale}
                onChange={setLocale}
                label={t`Language`}
                items={center.locales.map((l) => ({
                  value: l,
                  label: `${localeName(l)}${text[l].name.trim() ? "" : " ·"}`,
                  testId: `collection-locale-${l}`,
                }))}
              />
            </div>
          )}
          <Field label={<Trans>Name</Trans>} htmlFor="collection-name">
            <Input
              id="collection-name"
              value={cur.name}
              maxLength={100}
              autoFocus
              placeholder={t`Getting started`}
              onChange={(e) => patchText({ name: e.target.value })}
              data-testid="collection-name"
            />
          </Field>
          <Field label={<Trans>Description</Trans>} htmlFor="collection-description">
            <Textarea
              id="collection-description"
              value={cur.description}
              maxLength={500}
              rows={2}
              onChange={(e) => patchText({ description: e.target.value })}
              data-testid="collection-description"
            />
          </Field>
          <Field label={<Trans>Icon</Trans>}>
            <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t`Icon`}>
              {HELP_ICONS.map((name) => {
                const Icon = ICON_COMPONENTS[name]
                const on = icon === name
                return (
                  <button
                    key={name}
                    type="button"
                    role="radio"
                    aria-checked={on}
                    aria-label={iconText[name]}
                    title={iconText[name]}
                    onClick={() => setIcon(on ? "" : name)}
                    className={cn(
                      "grid size-9 place-items-center rounded-lg border text-muted-foreground outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
                      on && "border-brand bg-brand-wash text-brand"
                    )}
                  >
                    <Icon className="size-4" />
                  </button>
                )
              })}
            </div>
          </Field>
          {!hasChildren && parents.length > 0 && (
            <Field label={<Trans>Inside</Trans>} htmlFor="collection-parent">
              <ChoiceSelect
                id="collection-parent"
                value={parent}
                onChange={setParent}
                options={[
                  ["", t`Top level`] as const,
                  ...parents.map((c) => [c.id, collectionName(c, center.default_locale)] as const),
                ]}
                label={t`Inside`}
              />
            </Field>
          )}
          <ErrorLine error={m.create.error ?? m.update.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ok || pending} data-testid="save-collection">
              {pending && <LoaderIcon className="animate-spin" />}
              {edit ? <Trans>Save</Trans> : <Trans>Create collection</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Row({
  center,
  c,
  depth,
  index,
  count,
  onEdit,
  onAdd,
  onMove,
  onDelete,
}: {
  center: HelpCenter
  c: HelpCollection
  depth: number
  index: number
  count: number
  onEdit: () => void
  onAdd: () => void
  onMove: (dir: -1 | 1) => void
  onDelete: () => void
}) {
  const { t, i18n } = useLingui()
  const { setNodeRef: setDragRef, listeners, attributes, isDragging } = useDraggable({ id: c.id })
  const { setNodeRef: setDropRef, isOver } = useDroppable({ id: c.id })
  const Icon = c.icon ? ICON_COMPONENTS[c.icon] : FoldersIcon
  const name = collectionName(c, i18n.locale, center.default_locale)
  const missing = center.locales.filter((l) => !c.translations[l])
  return (
    <li
      ref={setDropRef}
      className={cn(
        "flex min-h-14 items-center gap-2 py-2 pe-2 transition-colors hover:bg-surface/60",
        isOver && "bg-brand-wash",
        isDragging && "opacity-50"
      )}
      style={{ paddingInlineStart: `${0.5 + depth * 1.75}rem` }}
      data-testid="help-collection-row"
      onKeyDown={(e) => {
        if (!e.altKey || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return
        e.preventDefault()
        onMove(e.key === "ArrowUp" ? -1 : 1)
      }}
    >
      <button
        ref={setDragRef}
        type="button"
        className="grid size-8 shrink-0 cursor-grab touch-none place-items-center rounded-md text-faint outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing"
        aria-label={t`Move ${name}. Drag, or press Alt with the arrow keys.`}
        {...attributes}
        {...listeners}
      >
        <GripVerticalIcon className="size-4" />
      </button>
      <Icon className="size-4 shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-body font-semibold">{name}</span>
        {missing.length > 0 && center.locales.length > 1 && (
          <span className="block truncate text-caption text-faint">
            {missing.map((l) => l.toUpperCase()).join(" ")} <Trans>not named</Trans>
          </span>
        )}
      </span>
      <span className="shrink-0 text-small text-faint tabular-nums" title={t`Articles in this collection`}>
        {c.article_count}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="ghost" size="icon-sm" aria-label={t`More for ${name}`} />}
          data-testid="help-collection-menu"
        >
          <MoreHorizontalIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={onEdit}>
            <PencilIcon />
            <Trans>Edit</Trans>
          </DropdownMenuItem>
          {depth === 0 && (
            <DropdownMenuItem onClick={onAdd}>
              <FolderPlusIcon />
              <Trans>Add a sub-collection</Trans>
            </DropdownMenuItem>
          )}
          <DropdownMenuItem disabled={index === 0} onClick={() => onMove(-1)}>
            <ArrowUpIcon />
            <Trans>Move up</Trans>
          </DropdownMenuItem>
          <DropdownMenuItem disabled={index === count - 1} onClick={() => onMove(1)}>
            <ArrowDownIcon />
            <Trans>Move down</Trans>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <Trash2Icon />
            <Trans>Delete</Trans>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  )
}

export function CollectionsTab({ center }: { center: HelpCenter }) {
  const { t, i18n } = useLingui()
  const query = useHelpCollections(center.id)
  const m = useCollectionMutations(center.id)
  const [confirm, dialog] = useConfirm()
  const [params] = useSearchParams()
  const [dialogState, setDialog] = useState<{ edit?: HelpCollection; parentId?: string } | null>(
    params.get("new") ? {} : null
  )
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const all = query.data ?? []
  const tops = siblings(all, undefined)
  const newKey = keyLabel(HELP_KEYS.newCollection).join("")

  useHotkeys({ [HELP_KEYS.newCollection]: () => setDialog({}) })

  const reorder = (moves: Moves) => moves.length > 0 && m.reorder.mutate(moves)

  const move = (c: HelpCollection, dir: -1 | 1) => {
    const group = siblings(all, c.parent_id)
    const from = group.findIndex((x) => x.id === c.id)
    const to = from + dir
    if (to < 0 || to >= group.length) return
    const next = [...group]
    next.splice(to, 0, next.splice(from, 1)[0])
    reorder(renumber(next))
  }

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const a = all.find((c) => c.id === active.id)
    const o = all.find((c) => c.id === over.id)
    if (!a || !o) return
    if ((a.parent_id ?? null) === (o.parent_id ?? null)) {
      const group = siblings(all, a.parent_id)
      const next = [...group]
      next.splice(
        group.findIndex((x) => x.id === o.id),
        0,
        next.splice(
          group.findIndex((x) => x.id === a.id),
          1
        )[0]
      )
      reorder(renumber(next))
      return
    }
    if (siblings(all, a.id).length > 0) return
    const parent = o.parent_id ?? o.id
    const group = siblings(all, parent).filter((x) => x.id !== a.id)
    const next = [...group]
    next.splice(o.parent_id ? group.findIndex((x) => x.id === o.id) : group.length, 0, a)
    const old = siblings(all, a.parent_id).filter((x) => x.id !== a.id)
    reorder([...renumber(next, parent), ...renumber(old)])
  }

  const remove = (c: HelpCollection) => {
    const name = collectionName(c, i18n.locale, center.default_locale)
    confirm({
      title: <Trans>Delete {name}?</Trans>,
      description: (
        <Trans>Its articles stay, with no collection. Its sub-collections become top-level collections.</Trans>
      ),
      confirm: <Trans>Delete collection</Trans>,
      run: () => m.remove.mutate(c.id),
    })
  }

  if (query.isPending) return <Skeleton className="h-40 w-full rounded-[14px]" />
  return (
    <div className="flex flex-col gap-4" data-testid="help-collections">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="min-w-60 flex-1 text-body text-muted-foreground">
          <Trans>
            Group articles so visitors can browse. A collection can hold one level of sub-collections. Drag to change
            the order.
          </Trans>
        </p>
        <Button
          size="sm"
          onClick={() => setDialog({})}
          title={t`New collection (${newKey})`}
          data-testid="new-collection"
        >
          <PlusIcon />
          <Trans>New collection</Trans>
        </Button>
      </div>
      <ErrorLine error={query.error ?? m.reorder.error ?? m.remove.error} />
      {all.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-[14px] border bg-card px-6 py-12 text-center">
          <span className="mb-1 grid size-12 place-items-center rounded-2xl bg-muted text-faint">
            <FoldersIcon className="size-5" />
          </span>
          <p className="text-body text-muted-foreground" data-testid="help-collections-empty">
            <Trans>No collections yet. Articles can stay without one.</Trans>
          </p>
        </div>
      ) : (
        <DndContext sensors={sensors} onDragEnd={onDragEnd}>
          <ul className="flex flex-col divide-y overflow-hidden rounded-[14px] border bg-card">
            {tops.flatMap((top, i) => [
              <Row
                key={top.id}
                center={center}
                c={top}
                depth={0}
                index={i}
                count={tops.length}
                onEdit={() => setDialog({ edit: top })}
                onAdd={() => setDialog({ parentId: top.id })}
                onMove={(d) => move(top, d)}
                onDelete={() => remove(top)}
              />,
              ...siblings(all, top.id).map((sub, j, subs) => (
                <Row
                  key={sub.id}
                  center={center}
                  c={sub}
                  depth={1}
                  index={j}
                  count={subs.length}
                  onEdit={() => setDialog({ edit: sub })}
                  onAdd={() => undefined}
                  onMove={(d) => move(sub, d)}
                  onDelete={() => remove(sub)}
                />
              )),
            ])}
          </ul>
        </DndContext>
      )}
      {dialogState && (
        <CollectionDialog
          center={center}
          all={all}
          edit={dialogState.edit}
          parentId={dialogState.parentId}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog}
    </div>
  )
}
