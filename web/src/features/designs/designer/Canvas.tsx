import { useDraggable, useDroppable } from "@dnd-kit/core"
import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  CopyIcon,
  EyeOffIcon,
  GripVerticalIcon,
  ImageIcon,
  LanguagesIcon,
  LockIcon,
  PlusIcon,
  Trash2Icon,
} from "lucide-react"
import { useState } from "react"
import { createPortal } from "react-dom"

import { Button } from "@/components/ui/button"
import { textOn } from "@/features/designs/colors"
import {
  childContainers,
  containerOf,
  footerInEnglish,
  ROOT,
  versions,
  type Block,
  type BlockOf,
} from "@/features/designs/doc"
import { useDesigner } from "@/features/designs/designer/context"
import { RichEditor } from "@/features/designs/designer/editor"
import { BlockIcon } from "@/features/designs/designer/icons"
import { isWarning, useBlockNames, useNetworkNames } from "@/features/designs/text"
import type { EmailFont } from "@/lib/api"
import { cn } from "@/lib/utils"

export const PHONE_WIDTH = 375

export const FONTS: Record<EmailFont, string> = {
  system: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
  humanist: '"Segoe UI", "Helvetica Neue", Arial, sans-serif',
  geometric: "Avenir, Montserrat, Futura, Arial, sans-serif",
  serif: 'Georgia, "Times New Roman", serif',
  mono: 'ui-monospace, "SF Mono", Menlo, Consolas, monospace',
}

const HEADING = {
  1: { fontSize: 30, lineHeight: "38px" },
  2: { fontSize: 22, lineHeight: "30px" },
  3: { fontSize: 18, lineHeight: "26px" },
} as const

const richClass =
  "[&_p]:my-0 [&_p+p]:mt-[0.9em] [&_ul]:my-[0.6em] [&_ul]:list-disc [&_ul]:ps-6 [&_ol]:my-[0.6em] [&_ol]:list-decimal [&_ol]:ps-6 [&_li_p]:my-0 [&_a]:text-(--email-link) [&_a]:underline [&_a]:underline-offset-2 [&_code]:rounded [&_code]:bg-black/6 [&_code]:px-1 [&_code]:font-mono [&_code]:text-[0.9em]"

function defaultPadding(b: Block, nested: boolean): [number, number, number, number] {
  const x = nested ? 0 : 24
  switch (b.type) {
    case "section":
      return [24, 24, 24, 24]
    case "spacer":
      return [0, 0, 0, 0]
    case "heading":
      return [12, x, 4, x]
    case "text":
      return [4, x, 12, x]
    default:
      return [12, x, 12, x]
  }
}

function alignOf(b: Block) {
  const a = b.align ?? (["image", "button", "social", "divider"].includes(b.type) ? "center" : "left")
  return a === "left" ? "start" : a === "right" ? "end" : "center"
}

function margins(align: string) {
  return align === "center"
    ? { marginInline: "auto" }
    : align === "end"
      ? { marginInlineStart: "auto" }
      : { marginInlineEnd: "auto" }
}

function Indicator({ show }: { show: boolean }) {
  if (!show) return null
  return (
    <div className="pointer-events-none relative z-20 h-0" aria-hidden>
      <div className="absolute inset-x-1 -top-[2px] h-[3px] rounded-full bg-brand shadow-[0_0_0_2px_white]" />
    </div>
  )
}

function EmptyDrop({ container, label }: { container: string; label: React.ReactNode }) {
  const { drop, dragging } = useDesigner()
  const { setNodeRef, isOver } = useDroppable({ id: `drop:${container}`, data: { kind: "container", container } })
  const active = drop?.container === container
  return (
    <div
      ref={setNodeRef}
      className={cn(
        "grid min-h-14 place-items-center rounded-md border border-dashed px-3 py-4 text-center font-sans text-caption transition-colors",
        active || isOver
          ? "border-brand bg-brand-wash/70 text-brand"
          : dragging
            ? "border-brand/40 text-faint"
            : "border-black/15 text-black/40"
      )}
      data-testid="empty-drop"
    >
      {label}
    </div>
  )
}

export function BlockList({ container, blocks, nested }: { container: string; blocks: Block[]; nested: boolean }) {
  const { drop } = useDesigner()
  const here = drop?.container === container
  if (blocks.length === 0 && container !== ROOT)
    return <EmptyDrop container={container} label={<Trans>Drop blocks here</Trans>} />
  return (
    <div className="flex flex-col">
      {blocks.map((b, i) => (
        <div key={b.id}>
          <Indicator show={here && drop.index === i} />
          <BlockView block={b} container={container} index={i} count={blocks.length} nested={nested} />
        </div>
      ))}
      <Indicator show={here && drop.index === blocks.length} />
    </div>
  )
}

function Tools({ block, container, index, count }: { block: Block; container: string; index: number; count: number }) {
  const { t } = useLingui()
  const { remove, duplicate, nudge } = useDesigner()
  const stop = (fn: () => void) => (e: React.MouseEvent) => {
    e.stopPropagation()
    fn()
  }
  return (
    <div className="flex items-center gap-px phone:hidden" data-container={container}>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={t`Move up`}
        disabled={index === 0}
        onClick={stop(() => nudge(block.id, -1))}
      >
        <ArrowUpIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={t`Move down`}
        disabled={index === count - 1}
        onClick={stop(() => nudge(block.id, 1))}
      >
        <ArrowDownIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={t`Duplicate`}
        onClick={stop(() => duplicate(block.id))}
        data-testid="block-duplicate"
      >
        <CopyIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={t`Delete`}
        className="hover:text-destructive"
        onClick={stop(() => remove(block.id))}
        data-testid="block-delete"
      >
        <Trash2Icon />
      </Button>
    </div>
  )
}

function BlockView({
  block,
  container,
  index,
  count,
  nested,
}: {
  block: Block
  container: string
  index: number
  count: number
  nested: boolean
}) {
  const { t } = useLingui()
  const d = useDesigner()
  const names = useBlockNames()
  const kind = names[block.type]
  const selected = d.selected === block.id
  const { setNodeRef, setActivatorNodeRef, listeners, attributes, isDragging } = useDraggable({
    id: block.id,
    data: { kind: "block", type: block.type, id: block.id },
    disabled: d.locked,
  })
  const { setNodeRef: setDropRef } = useDroppable({
    id: `over:${block.id}`,
    data: { kind: "block", container, index, id: block.id },
  })
  const issues = d.issues.get(block.id) ?? []
  const errors = issues.filter((i) => !isWarning(i))
  const [top, right, bottom, left] = block.padding ?? defaultPadding(block, nested)
  const hiddenHere = block.hide === (d.view.phone ? "mobile" : "desktop")
  const untranslated = d.missing.has(block.id)
  return (
    <div
      ref={(el) => {
        setNodeRef(el)
        setDropRef(el)
      }}
      className={cn(
        "relative outline-offset-[-1px] transition-[outline-color,opacity]",
        selected
          ? "z-10 outline-2 outline-brand"
          : errors.length
            ? "outline-2 outline-destructive/70 outline-dashed"
            : "outline-1 outline-transparent [&:hover:not(:has([data-block]:hover))]:outline-brand/50 [&:hover:not(:has([data-block]:hover))]:outline-dashed",
        "[&:hover:not(:has([data-block]:hover))>[data-tag]]:flex",
        isDragging && "opacity-35"
      )}
      style={{
        background: block.background,
        padding: `${top}px ${right}px ${bottom}px ${left}px`,
        borderRadius: block.type === "section" ? (block.border_radius ?? d.doc.theme.radius ?? 0) : undefined,
      }}
      onClick={(e) => {
        e.stopPropagation()
        d.select(block.id)
      }}
      data-block={block.id}
      data-type={block.type}
      data-selected={selected || undefined}
      data-testid="canvas-block"
    >
      {!selected && (
        <div
          data-tag
          className={cn(
            "absolute -top-3.5 start-1.5 z-30 items-center gap-0.5 rounded-full py-0.5 ps-0.5 pe-2 font-sans text-[11px] leading-4 font-medium shadow-sm",
            errors.length > 0 ? "flex bg-destructive text-white" : "hidden bg-card text-muted-foreground"
          )}
        >
          {!d.locked && (
            <button
              type="button"
              ref={setActivatorNodeRef}
              {...listeners}
              {...attributes}
              aria-label={t`Drag ${kind}`}
              className="grid size-5 cursor-grab touch-none place-items-center rounded-full hover:bg-black/10 active:cursor-grabbing"
              onClick={(e) => e.stopPropagation()}
            >
              <GripVerticalIcon className="size-3" />
            </button>
          )}
          <TagLabel block={block} />
        </div>
      )}
      {selected &&
        !d.readOnly &&
        d.toolSlot &&
        createPortal(
          <>
            <span
              className={cn(
                "flex h-7 items-center gap-1 rounded-full bg-brand py-0.5 pe-2.5 text-caption font-medium text-white",
                d.locked ? "ps-2.5" : "ps-0.5"
              )}
            >
              {!d.locked && (
                <button
                  type="button"
                  ref={setActivatorNodeRef}
                  {...listeners}
                  {...attributes}
                  aria-label={t`Drag ${kind}`}
                  className="grid size-6 cursor-grab touch-none place-items-center rounded-full hover:bg-black/10 active:cursor-grabbing"
                  onClick={(e) => e.stopPropagation()}
                  data-testid="block-handle"
                >
                  <GripVerticalIcon className="size-3.5" />
                </button>
              )}
              <TagLabel block={block} compact />
            </span>
            {!d.locked && <Tools block={block} container={container} index={index} count={count} />}
          </>,
          d.toolSlot
        )}
      {(hiddenHere || untranslated) && (
        <span className="absolute top-1 end-1.5 z-20 flex gap-1">
          {untranslated && (
            <span
              className="inline-flex items-center gap-1 rounded-full bg-warning px-2 py-0.5 font-sans text-[11px] leading-4 font-medium text-white shadow-sm"
              data-testid="untranslated-tag"
            >
              <LanguagesIcon className="size-3" />
              <Trans>Not translated</Trans>
            </span>
          )}
          {hiddenHere && (
            <span
              className="inline-flex items-center gap-1 rounded-full bg-card px-2 py-0.5 font-sans text-[11px] leading-4 font-medium text-muted-foreground shadow-sm"
              data-testid="hidden-tag"
            >
              <EyeOffIcon className="size-3" />
              {d.view.phone ? <Trans>Hidden on phones</Trans> : <Trans>Hidden on wide screens</Trans>}
            </span>
          )}
        </span>
      )}
      <div className={cn((hiddenHere || untranslated) && "opacity-45")}>
        <BlockBody block={block} />
      </div>
    </div>
  )
}

function TagLabel({ block, compact }: { block: Block; compact?: boolean }) {
  const { t } = useLingui()
  const names = useBlockNames()
  return (
    <>
      <BlockIcon type={block.type} className="size-3 shrink-0" />
      <span className={cn("whitespace-nowrap", compact && "phone:hidden")}>{names[block.type]}</span>
      {block.hide && block.hide !== "none" && (
        <EyeOffIcon
          className="ms-0.5 size-3 shrink-0"
          aria-label={block.hide === "mobile" ? t`Hidden on phones` : t`Hidden on wide screens`}
        />
      )}
    </>
  )
}

function BlockBody({ block }: { block: Block }) {
  switch (block.type) {
    case "section":
      return <SectionBody block={block} />
    case "columns":
      return <ColumnsBody block={block} />
    case "heading":
      return <HeadingBody block={block} />
    case "text":
      return <TextBody block={block} />
    case "image":
      return <ImageBody block={block} />
    case "button":
      return <ButtonBody block={block} />
    case "divider":
      return <DividerBody block={block} />
    case "spacer":
      return <SpacerBody block={block} />
    case "social":
      return <SocialBody block={block} />
    case "footer":
      return <FooterBody block={block} />
  }
}

function SectionBody({ block }: { block: BlockOf<"section"> }) {
  return <BlockList container={containerOf("section", block.id)} blocks={block.blocks as Block[]} nested />
}

function ColumnsBody({ block }: { block: BlockOf<"columns"> }) {
  const gap = block.gap ?? 16
  return (
    <div className="flex" style={{ gap }}>
      {block.columns.map((col, i) => (
        <div key={i} className="min-w-0" style={{ flex: `${col.width} 1 0%` }} data-testid="canvas-column">
          <BlockList container={containerOf("column", block.id, i)} blocks={col.blocks as Block[]} nested />
        </div>
      ))}
    </div>
  )
}

function HeadingBody({ block }: { block: BlockOf<"heading"> }) {
  const { t } = useLingui()
  const d = useDesigner()
  const theme = d.doc.theme
  const Tag = `h${block.level}` as const
  return (
    <Tag
      className={cn("m-0 font-bold", richClass)}
      style={{
        ...HEADING[block.level],
        fontFamily: FONTS[theme.heading_font ?? theme.font],
        color: block.color ?? d.view.colors.text,
        textAlign: alignOf(block),
      }}
    >
      <RichEditor
        mode="inline"
        owner={block.id}
        value={block.text}
        onChange={(text) => d.setText(block.id, text, `text:${block.id}`)}
        placeholder={t`Write a heading`}
        label={t`Heading`}
        readOnly={d.readOnly}
        testId="heading-editor"
        keepMarks={!!d.locale}
      />
    </Tag>
  )
}

function TextBody({ block }: { block: BlockOf<"text"> }) {
  const { t } = useLingui()
  const d = useDesigner()
  return (
    <div
      className={richClass}
      style={{
        fontSize: block.size ?? 16,
        lineHeight: 1.6,
        color: block.color ?? d.view.colors.text,
        textAlign: alignOf(block),
      }}
    >
      <RichEditor
        mode="rich"
        owner={block.id}
        value={block.html}
        onChange={(html) => d.setText(block.id, html, `text:${block.id}`)}
        placeholder={t`Write something`}
        label={t`Text`}
        readOnly={d.readOnly}
        testId="text-editor"
        keepMarks={!!d.locale}
      />
    </div>
  )
}

function FooterBody({ block }: { block: BlockOf<"footer"> }) {
  const { t } = useLingui()
  const d = useDesigner()
  return (
    <div
      className={richClass}
      style={{ fontSize: 13, lineHeight: 1.6, color: d.view.colors.muted, textAlign: alignOf(block) }}
    >
      <RichEditor
        mode="rich"
        owner={block.id}
        value={block.html}
        onChange={(html) => d.setText(block.id, html, `text:${block.id}`)}
        placeholder={t`Add your own lines above the required footer`}
        label={t`Footer text`}
        readOnly={d.readOnly}
        keepMarks={!!d.locale}
      />
    </div>
  )
}

function ImageBody({ block }: { block: BlockOf<"image"> }) {
  const d = useDesigner()
  const image = block.image_id ? d.images.get(block.image_id) : undefined
  const align = alignOf(block)
  const width = block.width ? `${block.width}px` : "100%"
  if (!image) {
    return (
      <button
        type="button"
        className="flex w-full flex-col items-center justify-center gap-2 rounded-md border border-dashed border-black/20 bg-black/[0.03] px-4 py-10 font-sans text-small text-black/55 transition-colors hover:border-brand hover:text-brand"
        style={{ maxWidth: width, ...margins(align) }}
        onClick={(e) => {
          e.stopPropagation()
          d.select(block.id)
          if (!d.locked) d.openLibrary(block.id)
        }}
        data-testid="image-placeholder"
      >
        <ImageIcon className="size-6" />
        {block.image_id ? <Trans>Loading the image…</Trans> : <Trans>Choose an image</Trans>}
      </button>
    )
  }
  return (
    <img
      src={image.url}
      alt={block.decorative ? "" : (block.alt ?? "")}
      width={block.width}
      className="block h-auto max-w-full"
      style={{ width, borderRadius: d.doc.theme.radius ?? 0, ...margins(align) }}
      onDoubleClick={() => !d.locked && d.openLibrary(block.id)}
      draggable={false}
      data-testid="canvas-image"
    />
  )
}

function ButtonBody({ block }: { block: BlockOf<"button"> }) {
  const { t } = useLingui()
  const d = useDesigner()
  const theme = d.doc.theme
  const bg = block.color ?? d.view.colors.accent
  return (
    <div style={{ textAlign: alignOf(block) }}>
      <span
        className={cn("inline-block font-semibold", block.full_width && "block w-full text-center")}
        style={{
          background: bg,
          color: textOn(bg, block.text_color),
          borderRadius: theme.radius ?? 0,
          padding: "12px 24px",
          fontSize: 16,
          lineHeight: "20px",
        }}
      >
        <RichEditor
          mode="plain"
          owner={block.id}
          value={block.label}
          onChange={(label) => d.setText(block.id, label, `text:${block.id}`)}
          placeholder={t`Button text`}
          label={t`Button text`}
          readOnly={d.readOnly}
          className="min-w-12"
          testId="button-editor"
        />
      </span>
    </div>
  )
}

function DividerBody({ block }: { block: BlockOf<"divider"> }) {
  const d = useDesigner()
  return (
    <div
      style={{
        borderTop: `${block.thickness ?? 1}px solid ${block.color ?? d.view.colors.border}`,
        width: `${block.width ?? 100}%`,
        ...margins(alignOf(block)),
      }}
    />
  )
}

function SpacerBody({ block }: { block: BlockOf<"spacer"> }) {
  const { selected } = useDesigner()
  return (
    <div
      className={cn(
        "grid place-items-center font-sans text-[11px] text-black/40",
        selected === block.id && "bg-[repeating-linear-gradient(45deg,rgb(0_0_0/0.04)_0_6px,transparent_6px_12px)]"
      )}
      style={{ height: block.height }}
    >
      {selected === block.id && `${block.height}px`}
    </div>
  )
}

function SocialIcon({ network, color }: { network: string; color: string }) {
  const [failed, setFailed] = useState(false)
  const file = color === "light" ? "light" : "dark"
  if (failed)
    return (
      <span className="grid size-6 place-items-center rounded-full bg-black/10 font-sans text-[10px] font-semibold uppercase">
        {network.slice(0, 2)}
      </span>
    )
  return (
    <img
      src={`/i/s/${network}-${file}.png`}
      alt=""
      width={24}
      height={24}
      className="size-6"
      onError={() => setFailed(true)}
      draggable={false}
    />
  )
}

function SocialBody({ block }: { block: BlockOf<"social"> }) {
  const d = useDesigner()
  const names = useNetworkNames()
  const align = alignOf(block)
  return (
    <div
      className="flex flex-wrap items-center gap-3"
      style={{ justifyContent: align === "center" ? "center" : align === "end" ? "flex-end" : "flex-start" }}
    >
      {block.items.map((it, i) =>
        block.style === "text" ? (
          <span key={i} className="text-[14px] underline underline-offset-2" style={{ color: d.view.colors.link }}>
            {names[it.network]}
          </span>
        ) : (
          <SocialIcon key={i} network={it.network} color={block.color ?? "dark"} />
        )
      )}
    </div>
  )
}

export function RequiredFooter() {
  const { doc, view, locale, original } = useDesigner()
  const version = locale ?? original.locale
  const english = !!version && versions(original).length > 0 && footerInEnglish(version)
  return (
    <div
      className="mx-auto mt-4 flex max-w-full items-start gap-2 px-6 text-center font-sans text-caption"
      style={{ width: view.phone ? PHONE_WIDTH : doc.theme.width, color: view.colors.muted }}
      data-testid="required-footer"
    >
      <LockIcon className="mt-0.5 size-3 shrink-0" />
      <p className="flex-1">
        <Trans>
          Added to every mail: your legal name and postal address, why the person gets the mail, and the unsubscribe and
          preference links.
        </Trans>
        {english && (
          <span className="mt-1 block text-warning" data-testid="footer-english">
            <Trans>This version gets it in English: Yuva has no footer words in its language.</Trans>
          </span>
        )}
      </p>
    </div>
  )
}

export function EmptyCanvas() {
  const { drop } = useDesigner()
  const { setNodeRef, isOver } = useDroppable({ id: `drop:${ROOT}`, data: { kind: "container", container: ROOT } })
  return (
    <div
      ref={setNodeRef}
      className={cn(
        "m-6 flex flex-col items-center gap-2 rounded-lg border-2 border-dashed px-6 py-14 text-center font-sans text-body",
        isOver || drop?.container === ROOT
          ? "border-brand bg-brand-wash/60 text-brand"
          : "border-black/15 text-black/45"
      )}
      data-testid="canvas-empty"
    >
      <PlusIcon className="size-6" />
      <Trans>Drag a block here, or pick one from the list.</Trans>
    </div>
  )
}

export function hasChildren(b: Block) {
  return childContainers(b).length > 0
}
