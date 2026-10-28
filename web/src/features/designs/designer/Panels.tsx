import { useDraggable, useDroppable } from "@dnd-kit/core"
import { Trans, useLingui } from "@lingui/react/macro"
import { CircleAlertIcon, EyeOffIcon, GripVerticalIcon, TriangleAlertIcon } from "lucide-react"

import { BLOCK_TYPES, containerOf, hasMessageBlock, ROOT, type Block, type BlockType } from "@/features/designs/doc"
import { useDesigner } from "@/features/designs/designer/context"
import { BlockIcon } from "@/features/designs/designer/icons"
import { LayoutHint } from "@/features/designs/designer/Languages"
import { plainText } from "@/features/designs/rich"
import { isWarning, useBlockHints, useBlockNames } from "@/features/designs/text"
import { useIsPhone } from "@/hooks/use-media-query"
import { cn } from "@/lib/utils"

function PaletteItem({ type, onAdd }: { type: BlockType; onAdd: (type: BlockType) => void }) {
  const names = useBlockNames()
  const hints = useBlockHints()
  const { t } = useLingui()
  const { locked, doc } = useDesigner()
  const phone = useIsPhone()
  const used = type === "message" && hasMessageBlock(doc)
  const { setNodeRef, listeners, attributes, isDragging } = useDraggable({
    id: `new:${type}`,
    data: { kind: "new", type },
    disabled: locked || phone || used,
  })
  return (
    <button
      type="button"
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      role="button"
      disabled={locked || used}
      onClick={() => onAdd(type)}
      title={used ? t`A design can hold one message block.` : hints[type]}
      className={cn(
        "flex cursor-grab flex-col items-center gap-1.5 rounded-xl border bg-card px-2 py-3 text-small text-muted-foreground transition-colors outline-none hover:border-faint hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing disabled:cursor-default disabled:opacity-50",
        isDragging && "opacity-50"
      )}
      data-testid={`palette-${type}`}
    >
      <BlockIcon type={type} className="size-5" />
      <span className="max-w-full text-center leading-tight text-balance break-words">{names[type]}</span>
    </button>
  )
}

export function BlockPalette({ onAdd }: { onAdd: (type: BlockType) => void }) {
  const { t } = useLingui()
  return (
    <div className="flex flex-col gap-3">
      <LayoutHint />
      <div className="grid grid-cols-2 gap-2" role="group" aria-label={t`Blocks`} data-testid="block-palette">
        {BLOCK_TYPES.map((type) => (
          <PaletteItem key={type} type={type} onAdd={onAdd} />
        ))}
      </div>
      <p className="px-1 text-caption text-faint">
        <Trans>Drag a block onto the mail, or press Enter to add it after the selected block.</Trans>
      </p>
    </div>
  )
}

function snippet(b: Block, names: Record<BlockType, string>) {
  switch (b.type) {
    case "heading":
      return plainText(b.text) || names.heading
    case "text":
    case "footer":
      return plainText(b.html) || names[b.type]
    case "button":
      return plainText(b.label) || names.button
    case "image":
      return b.alt || names.image
    default:
      return names[b.type]
  }
}

function LayerIndicator({ show, depth }: { show: boolean; depth: number }) {
  if (!show) return null
  return (
    <div className="relative h-0" aria-hidden>
      <div
        className="absolute -top-px h-0.5 rounded-full bg-brand"
        style={{ insetInline: `${depth * 14 + 8}px 8px` }}
      />
    </div>
  )
}

function LayerEmpty({ container, depth }: { container: string; depth: number }) {
  const { drop } = useDesigner()
  const { setNodeRef, isOver } = useDroppable({
    id: `layer-drop:${container}`,
    data: { kind: "container", container },
  })
  return (
    <div
      ref={setNodeRef}
      className={cn(
        "my-0.5 rounded-md border border-dashed py-1 text-caption text-faint",
        (isOver || drop?.container === container) && "border-brand text-brand"
      )}
      style={{ marginInlineStart: depth * 14 + 8, marginInlineEnd: 8, paddingInlineStart: 8 }}
    >
      <Trans>Empty</Trans>
    </div>
  )
}

function LayerList({ container, blocks, depth }: { container: string; blocks: Block[]; depth: number }) {
  const { drop } = useDesigner()
  if (blocks.length === 0) return <LayerEmpty container={container} depth={depth} />
  const here = drop?.container === container
  return (
    <>
      {blocks.map((b, i) => (
        <div key={b.id}>
          <LayerIndicator show={here && drop.index === i} depth={depth} />
          <LayerRow block={b} container={container} index={i} depth={depth} />
        </div>
      ))}
      <LayerIndicator show={here && drop.index === blocks.length} depth={depth} />
    </>
  )
}

function LayerRow({
  block,
  container,
  index,
  depth,
}: {
  block: Block
  container: string
  index: number
  depth: number
}) {
  const { t } = useLingui()
  const d = useDesigner()
  const names = useBlockNames()
  const kind = names[block.type]
  const { setNodeRef, setActivatorNodeRef, listeners, attributes, isDragging } = useDraggable({
    id: `layer:${block.id}`,
    data: { kind: "block", type: block.type, id: block.id },
    disabled: d.locked,
  })
  const { setNodeRef: setDropRef } = useDroppable({
    id: `layer-over:${block.id}`,
    data: { kind: "block", container, index, id: block.id },
  })
  const issues = d.issues.get(block.id) ?? []
  const error = issues.some((i) => !isWarning(i))
  const selected = d.selected === block.id
  return (
    <>
      <div
        ref={(el) => {
          setNodeRef(el)
          setDropRef(el)
        }}
        className={cn(
          "group/layer flex h-8 items-center gap-1.5 rounded-lg pe-2 text-body transition-colors",
          selected ? "bg-brand-wash text-brand" : "text-muted-foreground hover:bg-muted hover:text-foreground",
          isDragging && "opacity-40"
        )}
        style={{ paddingInlineStart: depth * 14 + 2 }}
        data-testid="layer-row"
        data-selected={selected || undefined}
      >
        <span
          ref={setActivatorNodeRef}
          {...listeners}
          {...attributes}
          role="button"
          tabIndex={-1}
          aria-label={t`Drag ${kind}`}
          className="grid size-6 shrink-0 cursor-grab place-items-center rounded text-faint opacity-0 group-hover/layer:opacity-100 active:cursor-grabbing"
        >
          <GripVerticalIcon className="size-3.5" />
        </span>
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-2 text-start outline-none focus-visible:underline"
          onClick={() => {
            d.select(block.id)
            document
              .querySelector(`[data-block="${block.id}"]`)
              ?.scrollIntoView({ block: "nearest", behavior: "smooth" })
          }}
          aria-current={selected || undefined}
        >
          <BlockIcon type={block.type} className="size-3.5 shrink-0" />
          <span className="min-w-0 flex-1 truncate">{snippet(block, names)}</span>
        </button>
        {block.hide && block.hide !== "none" && <EyeOffIcon className="size-3.5 shrink-0 text-faint" />}
        {issues.length > 0 &&
          (error ? (
            <CircleAlertIcon className="size-3.5 shrink-0 text-destructive" aria-label={t`Has problems`} />
          ) : (
            <TriangleAlertIcon className="size-3.5 shrink-0 text-warning" aria-label={t`Has warnings`} />
          ))}
      </div>
      {block.type === "section" && (
        <LayerList container={containerOf("section", block.id)} blocks={block.blocks as Block[]} depth={depth + 1} />
      )}
      {block.type === "columns" &&
        block.columns.map((col, i) => {
          const n = i + 1
          const width = col.width
          return (
            <div key={i}>
              <div
                className="flex h-6 items-center text-caption text-faint"
                style={{ paddingInlineStart: (depth + 1) * 14 + 8 }}
              >
                <Trans>
                  Column {n} · {width}%
                </Trans>
              </div>
              <LayerList
                container={containerOf("column", block.id, i)}
                blocks={col.blocks as Block[]}
                depth={depth + 2}
              />
            </div>
          )
        })}
    </>
  )
}

export function Layers() {
  const { t } = useLingui()
  const { doc } = useDesigner()
  if (doc.blocks.length === 0)
    return (
      <p className="px-2 py-6 text-center text-small text-faint">
        <Trans>The mail is empty. Add a block to start.</Trans>
      </p>
    )
  return (
    <nav aria-label={t`Layers`} className="flex flex-col" data-testid="layers">
      <LayerList container={ROOT} blocks={doc.blocks} depth={0} />
    </nav>
  )
}
