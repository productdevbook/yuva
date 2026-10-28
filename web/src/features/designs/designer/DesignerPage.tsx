import "@/features/designs/i18n"

import {
  DndContext,
  DragOverlay,
  PointerSensor,
  pointerWithin,
  rectIntersection,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragMoveEvent,
  type DragStartEvent,
} from "@dnd-kit/core"
import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArchiveIcon,
  ArrowLeftIcon,
  BlocksIcon,
  CheckIcon,
  CircleAlertIcon,
  CloudOffIcon,
  CopyIcon,
  DownloadIcon,
  EllipsisIcon,
  HistoryIcon,
  KeyboardIcon,
  LanguagesIcon,
  LayersIcon,
  LoaderIcon,
  MonitorIcon,
  MoonIcon,
  PaletteIcon,
  Redo2Icon,
  SaveIcon,
  SmartphoneIcon,
  TriangleAlertIcon,
  Undo2Icon,
  XIcon,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Link, Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import { useShell } from "@/app/shell"
import { ErrorLine, Kbd, Notice, Segmented, toast, useConfirm } from "@/components/common"
import { DESIGNER_KEYS, keyLabel, mod } from "@/components/common/ShortcutSheet"
import { formatBytes, useLanguageName } from "@/components/common/text"
import { keyHint } from "@/features/broadcasts/BroadcastParts"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Label } from "@/components/ui/label"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import {
  addVersion,
  children,
  countBlocks,
  duplicateBlock,
  hasMessageBlock,
  fileName,
  findBlock,
  imageIds,
  insertBlock,
  insertNear,
  localized,
  makeBlock,
  MAX_BLOCKS,
  missing,
  moveBlock,
  nudge,
  removeBlock,
  resolveDrop,
  setTranslatedText,
  updateBlock,
  versions,
  walk,
  type Block,
  type BlockType,
  type Doc,
} from "@/features/designs/doc"
import { derivedDark } from "@/features/designs/colors"
import { readDraft, serialize, stateOf, useAutosave, type SaveStatus } from "@/features/designs/designer/autosave"
import { BlockList, EmptyCanvas, FONTS, PHONE_WIDTH, RequiredFooter } from "@/features/designs/designer/Canvas"
import { ConflictDialog } from "@/features/designs/designer/ConflictDialog"
import { DesignerContext, type Designer, type DropTarget } from "@/features/designs/designer/context"
import { clearActiveEditor, TextToolbar } from "@/features/designs/designer/editor"
import { useHistory, type DesignState } from "@/features/designs/designer/history"
import { BlockIcon } from "@/features/designs/designer/icons"
import { ImageLibrary } from "@/features/designs/designer/ImageLibrary"
import { Inspector } from "@/features/designs/designer/Inspector"
import { AddLanguageDialog, LanguageBar } from "@/features/designs/designer/Languages"
import { BlockPalette, Layers } from "@/features/designs/designer/Panels"
import { Preview } from "@/features/designs/designer/Preview"
import { ThemePanel } from "@/features/designs/designer/ThemePanel"
import { Versions, VersionsPanel } from "@/features/designs/designer/Versions"
import {
  conflictOf,
  designLink,
  issuesOf,
  restoreVersion,
  useArchiveDesign,
  useCreateDesign,
  useDesign,
  useDuplicateDesign,
  useImageMap,
  useInvalidateVersions,
  useRender,
} from "@/features/designs/queries"
import { isWarning, useBlockNames, useIssueText } from "@/features/designs/text"
import { useDebounced } from "@/hooks/use-debounced"
import { useIsPhone } from "@/hooks/use-media-query"
import type { EmailDesign, EmailDesignIssue } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type RightTab = "block" | "design" | "versions"

const CLIP = 102 * 1024
const NEAR = 90 * 1024

const collision: CollisionDetection = (args) => {
  const hits = pointerWithin(args)
  if (hits.length === 0) return rectIntersection(args)
  const area = (id: string | number) => {
    const r = args.droppableRects.get(id)
    return r ? r.width * r.height : Infinity
  }
  return [...hits].sort((a, b) => area(a.id) - area(b.id))
}

function isTyping(el: EventTarget | null) {
  return el instanceof HTMLElement && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName))
}

function hasLayer() {
  return !!document.querySelector(
    ':is([role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]):not([data-closed])'
  )
}

function download(state: DesignState) {
  const blob = new Blob([JSON.stringify(state.document, null, 2) + "\n"], { type: "application/json" })
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = fileName(state.name)
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export default function DesignerPage() {
  const { t } = useLingui()
  const { designId = "" } = useParams()
  const { canManage, workspaceId } = useSession()
  const design = useDesign(designId)
  if (!canManage) return <Navigate to="/broadcasts" replace />
  if (design.isPending)
    return (
      <div className="flex h-svh flex-col" data-testid="designer-loading">
        <div className="flex h-14 items-center gap-3 border-b px-4">
          <Skeleton className="h-7 w-48" />
        </div>
        <div className="flex flex-1 gap-0">
          <Skeleton className="m-4 w-60 rounded-xl phone:hidden" />
          <Skeleton className="m-4 flex-1 rounded-xl" />
          <Skeleton className="m-4 w-72 rounded-xl phone:hidden" />
        </div>
      </div>
    )
  if (!design.data)
    return (
      <div className="mx-auto flex max-w-lg flex-col gap-4 px-6 py-16">
        <Link
          to="/broadcasts/designs"
          className="inline-flex items-center gap-1.5 text-body text-faint hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
          {t`Designs`}
        </Link>
        <ErrorLine error={design.error} />
      </div>
    )
  return <DesignerScreen key={designId} initial={design.data} ws={workspaceId} />
}

function bootState(initial: EmailDesign, ws: string) {
  const server = stateOf(initial)
  const draft = readDraft(ws, initial.id)
  if (draft && serialize(draft.state) !== serialize(server)) {
    if (draft.base === initial.version) return { state: draft.state, base: initial.version, conflict: null }
    return { state: draft.state, base: draft.base, conflict: initial }
  }
  return { state: server, base: initial.version, conflict: null as EmailDesign | null }
}

function versionOf(i: EmailDesignIssue) {
  const m = /^translations\.([^.]+)/.exec(i.field ?? "")
  return m ? m[1] : null
}

function StatusLine({ status, onRetry, onChecks }: { status: SaveStatus; onRetry: () => void; onChecks: () => void }) {
  const { t } = useLingui()
  const base = "inline-flex shrink-0 items-center gap-1.5 text-small whitespace-nowrap"
  switch (status) {
    case "saved":
      return (
        <span className={cn(base, "text-faint")} role="status" data-testid="save-status" data-status={status}>
          <CheckIcon className="size-3.5" />
          <span className="phone:hidden">
            <Trans>Saved</Trans>
          </span>
        </span>
      )
    case "dirty":
    case "saving":
      return (
        <span className={cn(base, "text-faint")} role="status" data-testid="save-status" data-status={status}>
          <LoaderIcon className={cn("size-3.5", status === "saving" && "animate-spin")} />
          <span className="phone:hidden">
            <Trans>Saving…</Trans>
          </span>
        </span>
      )
    case "offline":
      return (
        <span
          className={cn(base, "text-warning")}
          role="status"
          title={t`Offline. Your changes are kept on this device and saved when you are back.`}
          data-testid="save-status"
          data-status={status}
        >
          <CloudOffIcon className="size-3.5" />
          <span className="phone:hidden">
            <Trans>Offline, kept on this device</Trans>
          </span>
        </span>
      )
    case "invalid":
      return (
        <button
          type="button"
          className={cn(base, "text-destructive hover:underline")}
          onClick={onChecks}
          data-testid="save-status"
          data-status={status}
        >
          <CircleAlertIcon className="size-3.5" />
          <span className="phone:hidden">
            <Trans>Not saved: fix the marked blocks</Trans>
          </span>
        </button>
      )
    case "conflict":
      return (
        <span className={cn(base, "text-warning")} role="status" data-testid="save-status" data-status={status}>
          <TriangleAlertIcon className="size-3.5" />
          <span className="phone:hidden">
            <Trans>Changed elsewhere</Trans>
          </span>
        </span>
      )
    case "error":
      return (
        <button
          type="button"
          className={cn(base, "text-destructive hover:underline")}
          onClick={onRetry}
          data-testid="save-status"
          data-status={status}
        >
          <CircleAlertIcon className="size-3.5" />
          <Trans>Not saved. Try again</Trans>
        </button>
      )
  }
}

function SizeMeter({ size }: { size: number | undefined }) {
  const { i18n } = useLingui()
  if (size === undefined) return null
  const pct = Math.min(100, (size / CLIP) * 100)
  const tone = size > CLIP ? "bg-destructive" : size > NEAR ? "bg-warning" : "bg-success"
  const used = formatBytes(size, i18n.locale)
  const limit = formatBytes(CLIP, i18n.locale)
  return (
    <div className="flex flex-col gap-1.5" data-testid="size-meter">
      <div className="flex items-center justify-between text-small">
        <span className="text-muted-foreground">
          <Trans>Size</Trans>
        </span>
        <span className="tabular-nums">
          <Trans>
            {used} of {limit}
          </Trans>
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted">
        <div className={cn("h-full rounded-full", tone)} style={{ width: `${pct}%` }} />
      </div>
      <p className="text-caption text-faint">
        <Trans>Gmail cuts mails off at about 102 KB of HTML, hiding the unsubscribe link.</Trans>
      </p>
    </div>
  )
}

type Level = "ok" | "warn" | "bad"

function levelOf(issues: EmailDesignIssue[], size: number | undefined): Level {
  if (issues.some((i) => !isWarning(i)) || (size ?? 0) > CLIP) return "bad"
  return issues.length > 0 || (size ?? 0) > NEAR ? "warn" : "ok"
}

const LEVEL_TONE: Record<Level, string> = {
  ok: "bg-success text-white",
  warn: "bg-warning text-white",
  bad: "bg-destructive text-white",
}

function ChecksButton({
  count,
  level,
  open,
  onClick,
}: {
  count: number
  level: Level
  open: boolean
  onClick: () => void
}) {
  return (
    <Button
      variant="ghost"
      size="sm"
      className={cn("gap-1.5 px-2.5", open && "bg-muted")}
      aria-expanded={open}
      onClick={onClick}
      data-testid="checks"
    >
      <Trans>Checks</Trans>
      <span
        className={cn(
          "inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full px-1.5 text-[11px] font-semibold tabular-nums",
          LEVEL_TONE[level]
        )}
        data-testid="checks-count"
      >
        {count}
      </span>
    </Button>
  )
}

function ChecksStrip({
  size,
  issues,
  doc,
  onShow,
  onClose,
}: {
  size: number | undefined
  issues: EmailDesignIssue[]
  doc: Doc
  onShow: (issue: EmailDesignIssue, block: Block | null) => void
  onClose: () => void
}) {
  const { t } = useLingui()
  const text = useIssueText()
  const names = useBlockNames()
  return (
    <section
      className="flex max-h-[40svh] shrink-0 flex-col gap-3 overflow-y-auto border-b bg-surface px-4 py-3"
      aria-label={t`Checks`}
      data-testid="checks-strip"
    >
      <div className="flex items-start gap-3">
        <div className="max-w-md flex-1">
          <SizeMeter size={size} />
        </div>
        <Button variant="ghost" size="icon-sm" aria-label={t`Close`} onClick={onClose}>
          <XIcon />
        </Button>
      </div>
      {issues.length === 0 ? (
        <p className="flex items-center gap-2 text-small text-success">
          <CheckIcon className="size-4" />
          <Trans>No problems found.</Trans>
        </p>
      ) : (
        <ul className="flex flex-col gap-1.5" data-testid="checks-list">
          {issues.map((i, n) => {
            const b = i.block_id ? findBlock(doc, i.block_id) : null
            const error = !isWarning(i)
            return (
              <li key={n} className="flex items-center gap-3 rounded-lg bg-background px-2.5 py-2 text-small">
                <span
                  className={cn(
                    "inline-flex min-w-12 shrink-0 justify-center rounded-full px-2 py-px text-caption font-semibold whitespace-nowrap",
                    error ? "bg-destructive/12 text-destructive" : "bg-warning/15 text-warning"
                  )}
                >
                  {error ? <Trans>Fix</Trans> : <Trans>Check</Trans>}
                </span>
                <span className="min-w-0 flex-1">
                  {b && <span className="font-medium text-faint">{names[b.type]}: </span>}
                  {text(i)}
                </span>
                <Button variant="outline" size="sm" onClick={() => onShow(i, b)}>
                  <Trans>Show</Trans>
                </Button>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

function IconButton({
  label,
  keys,
  onClick,
  disabled,
  children,
  testId,
}: {
  label: string
  keys?: string[]
  onClick: () => void
  disabled?: boolean
  children: React.ReactNode
  testId?: string
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            disabled={disabled}
            onClick={onClick}
            data-testid={testId}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>
        {label}
        {keys && <span className="opacity-60">{keys.join(" ")}</span>}
      </TooltipContent>
    </Tooltip>
  )
}

function DesignerScreen({ initial, ws }: { initial: EmailDesign; ws: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const shell = useShell()
  const phone = useIsPhone()
  const names = useBlockNames()
  const [boot] = useState(() => bootState(initial, ws))
  const history = useHistory(boot.state)
  const commit = history.commit
  const state = history.state
  const original = state.document
  const readOnly = original.schema !== 1
  const [params] = useSearchParams()
  const [langRaw, setLang] = useState<string | null>(() => params.get("lang"))
  const tags = versions(original)
  const locale = langRaw && tags.slice(1).includes(langRaw) ? langRaw : null
  const doc = useMemo(() => localized(original, locale), [original, locale])
  const gaps = useMemo(() => new Set(locale ? missing(original, locale) : []), [original, locale])
  const locked = readOnly || !!locale
  const [adding, setAdding] = useState(false)
  const inboxes = useInboxes().data ?? []
  const languageName = useLanguageName()
  const save = useAutosave({
    ws,
    design: initial,
    base: boot.base,
    state,
    enabled: !readOnly,
    initialConflict: boot.conflict,
  })
  const [selectedRaw, setSelected] = useState<string | null>(null)
  const selected = selectedRaw && findBlock(doc, selectedRaw) ? selectedRaw : null
  const selectedBlock = selected ? findBlock(doc, selected) : null
  const [preview, setPreview] = useState(false)
  const [phoneView, setPhoneView] = useState(false)
  const [dark, setDark] = useState(false)
  const [rtab, setRtab] = useState<RightTab>("block")
  const [left, setLeft] = useState<"blocks" | "layers">("blocks")
  const [sheet, setSheet] = useState<null | "blocks" | "layers" | "style">(null)
  const [library, setLibrary] = useState<string | null>(null)
  const altFocus = useRef<string | null>(null)
  const [toolSlot, setToolSlot] = useState<HTMLElement | null>(null)
  const [versionsOpen, setVersionsOpen] = useState(false)
  const [checksOpen, setChecksOpen] = useState(false)
  const [drop, setDrop] = useState<DropTarget | null>(null)
  const [dragging, setDragging] = useState<{ id: string; type: BlockType } | null>(null)
  const [restoring, setRestoring] = useState(false)
  const [restoreError, setRestoreError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const images = useImageMap(useMemo(() => imageIds(doc), [doc]))
  const create = useCreateDesign()
  const duplicate = useDuplicateDesign()
  const archive = useArchiveDesign()
  const invalidateVersions = useInvalidateVersions(initial.id)
  const [confirm, confirmDialog] = useConfirm()
  const palette = useRef<HTMLDivElement>(null)

  const renderLocale = tags.length > 0 ? (locale ?? tags[0]) : undefined
  const renderInput = useMemo(
    () => ({
      document: original,
      inbox_id: state.inbox_id,
      dark: (preview && dark) || undefined,
      locale: renderLocale,
    }),
    [original, state.inbox_id, preview, dark, renderLocale]
  )
  const debounced = useDebounced(renderInput, preview ? 400 : 2000)
  const render = useRender(debounced)
  const stale = debounced !== renderInput || render.isPlaceholderData || render.isFetching

  const defaults = {
    heading: t`A clear heading`,
    text: t`Write what you want to tell your readers.`,
    button: t`Read more`,
    footer: t`You can reply to this mail; a person reads every answer.`,
    columnText: t`Some text for this column.`,
  }

  const commitDoc = useCallback(
    (fn: (d: Doc) => Doc, key?: string) => commit((s) => ({ ...s, document: fn(s.document) }), key),
    [commit]
  )

  const select = useCallback((id: string | null) => {
    setSelected(id)
    if (!id) clearActiveEditor()
  }, [])

  const [seen, setSeen] = useState(selected)
  if (selected !== seen) {
    setSeen(selected)
    if (selected) setRtab("block")
  }

  const layoutHint = () => {
    const language = languageName(original.locale ?? "")
    toast(t`Layout is shared by all languages. Switch to ${language} to change it.`)
  }

  const add = (type: BlockType, at?: DropTarget) => {
    if (readOnly) return
    if (locked) return layoutHint()
    if (countBlocks(doc) >= MAX_BLOCKS) {
      toast(t`A design can hold at most 200 blocks.`)
      return
    }
    if (type === "message" && hasMessageBlock(doc)) {
      toast(t`A design can hold one message block.`)
      return
    }
    const block = makeBlock(type, defaults)
    const next = at ? insertBlock(doc, at.container, at.index, block) : insertNear(doc, selected, block)
    history.commit({ ...state, document: next })
    setSelected(block.id)
    setSheet(null)
    if (type === "image") setLibrary(block.id)
    requestAnimationFrame(() =>
      document.querySelector(`[data-block="${block.id}"]`)?.scrollIntoView({ block: "nearest", behavior: "smooth" })
    )
  }

  const colors = useMemo(
    () => (dark ? { ...derivedDark(doc.theme.colors), ...doc.theme.dark } : doc.theme.colors),
    [dark, doc.theme.colors, doc.theme.dark]
  )

  const setText = (id: string, value: string, key?: string) => {
    if (locale) commitDoc((d) => setTranslatedText(d, locale, id, value), key)
    else
      commitDoc((d) => {
        const b = findBlock(d, id)
        if (!b) return d
        const field =
          b.type === "heading" ? "text" : b.type === "button" ? "label" : b.type === "image" ? "alt" : "html"
        return updateBlock(d, id, (x: Block) => ({ ...x, [field]: value }) as Block)
      }, key)
  }

  const ctx: Designer = {
    doc,
    original,
    locale,
    missing: gaps,
    locked,
    setText,
    setLocale: (tag) => {
      setLang(tag)
      clearActiveEditor()
    },
    view: { phone: phoneView, dark, colors },
    selected,
    select,
    setDoc: commitDoc,
    update: (id, patch, key) =>
      commitDoc((d) => updateBlock(d, id, patch as Partial<Block> | ((b: Block) => Block)), key),
    remove: (id) => {
      if (locale) return layoutHint()
      const entries = walk(doc)
      const i = entries.findIndex((e) => e.block.id === id)
      const next = entries.slice(i + 1).find((e) => !findBlock({ ...doc, blocks: [entries[i].block] }, e.block.id))
      commitDoc((d) => removeBlock(d, id))
      setSelected(next?.block.id ?? entries[i - 1]?.block.id ?? null)
    },
    duplicate: (id) => {
      if (locale) return layoutHint()
      if (countBlocks(doc) >= MAX_BLOCKS) return toast(t`A design can hold at most 200 blocks.`)
      const source = findBlock(doc, id)
      if (source && hasMessageBlock({ ...doc, blocks: [source] })) return toast(t`A design can hold one message block.`)
      const r = duplicateBlock(doc, id)
      history.commit({ ...state, document: r.doc })
      setSelected(r.id)
    },
    nudge: (id, by) => (locale ? layoutHint() : commitDoc((d) => nudge(d, id, by))),
    images,
    issues: new Map(),
    readOnly,
    openLibrary: (id) => setLibrary(id),
    drop,
    dragging: dragging?.id ?? null,
    toolSlot,
  }

  const renderIssues = render.data && !render.isPlaceholderData ? render.data.warnings : null
  const allIssues = useMemo(() => {
    const list = [...save.errors, ...issuesOf(render.error)]
    const warnings = renderIssues ?? save.warnings
    const seen = new Set(list.map((i) => `${i.code}:${i.block_id}:${i.field}`))
    for (const w of warnings) if (!seen.has(`${w.code}:${w.block_id}:${w.field}`)) list.push(w)
    return list.filter((i) => i.code !== "translation_missing")
  }, [save.errors, save.warnings, render.error, renderIssues])
  for (const i of allIssues) {
    if (!i.block_id) continue
    const of = versionOf(i)
    if (of && of !== locale) continue
    const list = ctx.issues.get(i.block_id) ?? []
    list.push(i)
    ctx.issues.set(i.block_id, list)
  }

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))

  const targetOf = (e: DragMoveEvent): DropTarget | null => {
    const over = e.over
    const a = e.active.data.current as { kind: "new" | "block"; type: BlockType; id?: string } | undefined
    if (!over || !a) return null
    const data = over.data.current as { kind: "block" | "container"; container: string; index?: number; id?: string }
    const moving = a.kind === "block" ? a.id : undefined
    if (data.kind === "block") {
      if (moving && data.id === moving) return null
      const start = e.activatorEvent as PointerEvent
      const y = start.clientY + e.delta.y
      const after = y > over.rect.top + over.rect.height / 2
      return resolveDrop(doc, a.type, { container: data.container, index: (data.index ?? 0) + (after ? 1 : 0) }, moving)
    }
    return resolveDrop(doc, a.type, { container: data.container, index: children(doc, data.container).length }, moving)
  }

  const onDragStart = (e: DragStartEvent) => {
    const a = e.active.data.current as { type: BlockType; id?: string }
    setDragging({ id: String(e.active.id), type: a.type })
  }
  const onDragMove = (e: DragMoveEvent) => {
    const next = targetOf(e)
    setDrop((d) => (d?.container === next?.container && d?.index === next?.index ? d : next))
  }
  const onDragEnd = (e: DragEndEvent) => {
    const target = targetOf(e)
    const a = e.active.data.current as { kind: "new" | "block"; type: BlockType; id?: string }
    setDrop(null)
    setDragging(null)
    if (!target) return
    if (a.kind === "new") add(a.type, target)
    else if (a.id) {
      commitDoc((d) => moveBlock(d, a.id!, target.container, target.index))
      setSelected(a.id)
    }
  }

  const flushNow = () => {
    void save.flush(true).then(() => invalidateVersions())
  }

  const leave = () => {
    void save.flush()
    navigate("/broadcasts/designs")
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const typing = isTyping(e.target)
      const meta = e.metaKey || e.ctrlKey
      const key = e.key.toLowerCase()
      if (meta && !e.altKey && (key === "z" || key === "y")) {
        if (hasLayer() && !document.querySelector("[data-testid=designer]")?.contains(e.target as Node)) return
        e.preventDefault()
        if (key === "y" || e.shiftKey) history.redo()
        else history.undo()
        return
      }
      if (meta && key === "s") {
        e.preventDefault()
        flushNow()
        toast(t`Saved, with a version kept`)
        return
      }
      if (hasLayer() || readOnly) return
      if (e.key === "Escape" && typing && (e.target as HTMLElement).isContentEditable) {
        ;(e.target as HTMLElement).blur()
        return
      }
      if (typing) return
      if (meta && key === "d" && selected) {
        e.preventDefault()
        ctx.duplicate(selected)
        return
      }
      if (meta) return
      if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
        if (!selected) return
        e.preventDefault()
        ctx.nudge(selected, e.key === "ArrowUp" ? -1 : 1)
        return
      }
      if (e.altKey) return
      if (e.key === "ArrowUp" || e.key === "ArrowDown") {
        const list = walk(doc)
        if (list.length === 0) return
        e.preventDefault()
        const i = list.findIndex((x) => x.block.id === selected)
        const next =
          i < 0
            ? e.key === "ArrowDown"
              ? 0
              : list.length - 1
            : Math.max(0, Math.min(list.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))
        const id = list[next].block.id
        setSelected(id)
        document.querySelector(`[data-block="${id}"]`)?.scrollIntoView({ block: "nearest" })
        return
      }
      if ((e.key === "Backspace" || e.key === "Delete") && selected) {
        e.preventDefault()
        ctx.remove(selected)
        return
      }
      if (e.key === "Escape" && selected) {
        setSelected(null)
        return
      }
      if (e.key === DESIGNER_KEYS.edit) setPreview(false)
      else if (e.key === DESIGNER_KEYS.desktop) {
        setPreview(true)
        setPhoneView(false)
      } else if (e.key === DESIGNER_KEYS.phone) {
        setPreview(true)
        setPhoneView(true)
      } else if (e.key === DESIGNER_KEYS.dark) setDark((v) => !v)
      else if (e.key === DESIGNER_KEYS.add) {
        e.preventDefault()
        setPreview(false)
        setRtab("block")
        if (phone) setSheet("blocks")
        else {
          setLeft("blocks")
          requestAnimationFrame(() => palette.current?.querySelector<HTMLButtonElement>("button")?.focus())
        }
      } else if (e.key === DESIGNER_KEYS.theme) {
        setSelected(null)
        setRtab("design")
        if (phone) setSheet("style")
      } else if (e.key === DESIGNER_KEYS.checks) setChecksOpen((o) => !o)
      else if (e.key === DESIGNER_KEYS.versions) {
        if (phone) setVersionsOpen(true)
        else setRtab("versions")
      } else if (e.key === DESIGNER_KEYS.language && tags.length > 0) {
        const i = locale ? tags.indexOf(locale) : 0
        const next = tags[(i + 1) % tags.length]
        ctx.setLocale(next === tags[0] ? null : next)
      } else if (e.key === DESIGNER_KEYS.addLanguage) setAdding(true)
      else if (e.key === DESIGNER_KEYS.image && selectedBlock?.type === "image" && !locked) setLibrary(selectedBlock.id)
      else if (e.key === "?") shell.openShortcuts()
      else return
      e.preventDefault()
    }
    window.addEventListener("keydown", onKey, true)
    return () => window.removeEventListener("keydown", onKey, true)
  })

  const resolveTheirs = () => {
    const theirs = save.conflict
    if (!theirs) return
    save.resolve.theirs(theirs)
    history.commit(stateOf(theirs))
  }
  const resolveMine = () => {
    if (save.conflict) save.resolve.mine(save.conflict)
  }
  const resolveCopy = () => {
    const theirs = save.conflict
    if (!theirs) return
    const name = state.name
    setBusy(true)
    create.mutate(
      { name: t`${name} (copy)`, inbox_id: state.inbox_id, document: original },
      {
        onSuccess: (copy) => {
          save.resolve.theirs(theirs)
          history.reset(stateOf(theirs))
          navigate(designLink(copy.id))
        },
        onError: (err) => toast(err.message),
        onSettled: () => setBusy(false),
      }
    )
  }

  const restore = (version: number) => {
    setRestoring(true)
    setRestoreError(null)
    restoreVersion(initial.id, version, save.version())
      .then((d) => {
        save.resolve.restored(d)
        history.commit(stateOf(d))
        setVersionsOpen(false)
        invalidateVersions()
        toast(t`Version restored`, () => history.undo())
      })
      .catch((err) => {
        const theirs = conflictOf(err)
        if (theirs) {
          setVersionsOpen(false)
          save.conflictWith(theirs)
        } else setRestoreError(err)
      })
      .finally(() => setRestoring(false))
  }

  const issueCount = allIssues.length
  const size = render.data?.size_bytes
  const theme = doc.theme
  const level = levelOf(allIssues, size)
  const showLeft = !phone && !preview
  const canvasWidth = phoneView ? PHONE_WIDTH : theme.width
  const emptyBlock = (
    <p className="px-4 py-8 text-center text-small text-faint" data-testid="inspector-empty">
      <Trans>Select a block to change it. Design settings apply to the whole mail.</Trans>
    </p>
  )
  const design = (
    <ThemePanel
      inboxId={state.inbox_id}
      onInbox={(id) => history.commit({ ...state, inbox_id: id })}
      offered={!!state.for_conversations}
      onOffered={(v) => history.commit({ ...state, for_conversations: v })}
    />
  )
  const block = selectedBlock ? <Inspector key={selectedBlock.id} block={selectedBlock} /> : emptyBlock
  const right = selectedBlock ? block : design

  const showIssue = (issue: EmailDesignIssue, b: Block | null) => {
    setPreview(false)
    const of = versionOf(issue)
    if (of) setLang(of === original.locale ? null : of)
    if (phone) setChecksOpen(false)
    if (!b) {
      setSelected(null)
      setRtab("design")
      if (phone) setSheet("style")
      return
    }
    setSelected(b.id)
    requestAnimationFrame(() =>
      document.querySelector(`[data-block="${b.id}"]`)?.scrollIntoView({ block: "center", behavior: "smooth" })
    )
  }

  const canvas = (
    <div
      className="min-h-full px-6 py-8 phone:px-3 phone:py-5"
      style={{ background: colors.page }}
      onClick={() => select(null)}
      data-testid="canvas"
    >
      <div
        className="mx-auto max-w-full overflow-visible shadow-[0_1px_3px_rgb(0_0_0/0.06),0_12px_32px_-16px_rgb(0_0_0/0.18)]"
        style={
          {
            width: canvasWidth,
            background: colors.card,
            borderRadius: theme.radius ?? 0,
            fontFamily: FONTS[theme.font],
            color: colors.text,
            "--email-link": colors.link,
          } as React.CSSProperties
        }
        dir={theme.direction === "rtl" ? "rtl" : theme.direction === "ltr" ? "ltr" : undefined}
        data-testid="canvas-card"
      >
        {doc.blocks.length === 0 ? (
          <EmptyCanvas />
        ) : (
          <div className="py-3">
            <BlockList container="root" blocks={doc.blocks} nested={false} />
          </div>
        )}
      </div>
      <RequiredFooter />
    </div>
  )

  const viewControls = (
    <div className="flex min-w-0 shrink-0 items-center gap-2 phone:w-full phone:justify-between">
      <Segmented
        value={preview ? "preview" : "edit"}
        onChange={(v) => setPreview(v === "preview")}
        label={t`View`}
        items={[
          { value: "edit", label: t`Edit`, testId: "mode-edit" },
          { value: "preview", label: t`Preview`, testId: "mode-preview" },
        ]}
      />
      <Segmented
        value={phoneView ? "phone" : "desktop"}
        onChange={(v) => setPhoneView(v === "phone")}
        label={t`Preview size`}
        items={[
          {
            value: "desktop",
            label: (
              <>
                <MonitorIcon className="hidden size-4 phone:block" />
                <span className="phone:sr-only">
                  <Trans>Desktop</Trans>
                </span>
              </>
            ),
            testId: "mode-desktop",
          },
          {
            value: "phone",
            label: (
              <>
                <SmartphoneIcon className="hidden size-4 phone:block" />
                <span className="phone:sr-only">
                  <Trans>Phone</Trans>
                </span>
              </>
            ),
            testId: "mode-phone",
          },
        ]}
      />
      <Label className="gap-2 font-normal">
        <MoonIcon className="size-4 text-muted-foreground" />
        <span className="phone:sr-only">
          <Trans>Dark</Trans>
        </span>
        <Switch checked={dark} onCheckedChange={setDark} data-testid="mode-dark" />
      </Label>
    </div>
  )

  return (
    <DesignerContext.Provider value={ctx}>
      <DndContext
        sensors={sensors}
        collisionDetection={collision}
        onDragStart={onDragStart}
        onDragMove={onDragMove}
        onDragEnd={onDragEnd}
        onDragCancel={() => {
          setDrop(null)
          setDragging(null)
        }}
      >
        <div className="flex h-svh flex-col bg-background" data-testid="designer">
          <header className="flex h-14 shrink-0 items-center gap-2 border-b px-3 phone:gap-1 phone:px-2">
            <IconButton label={t`Back to designs`} onClick={leave} testId="designer-back">
              <ArrowLeftIcon className="rtl:rotate-180" />
            </IconButton>
            <input
              value={state.name}
              maxLength={200}
              onChange={(e) => history.commit({ ...state, name: e.target.value }, "name")}
              onBlur={() => !state.name.trim() && history.commit({ ...state, name: initial.name })}
              aria-label={t`Design name`}
              readOnly={readOnly}
              className="h-8 min-w-0 flex-1 rounded-lg bg-transparent px-2 text-body font-semibold outline-none hover:bg-muted focus:bg-muted sm:max-w-64 sm:flex-none sm:basis-56"
              data-testid="design-name"
            />
            <StatusLine status={save.status} onRetry={() => void save.flush()} onChecks={() => setChecksOpen(true)} />
            <div className="flex-1 phone:hidden" />
            {!phone && viewControls}
            <div className="flex-1 phone:hidden" />
            <IconButton
              label={t`Undo`}
              keys={[mod, "Z"]}
              onClick={history.undo}
              disabled={!history.canUndo || readOnly}
              testId="undo"
            >
              <Undo2Icon />
            </IconButton>
            <IconButton
              label={t`Redo`}
              keys={["⇧", mod, "Z"]}
              onClick={history.redo}
              disabled={!history.canRedo || readOnly}
              testId="redo"
            >
              <Redo2Icon />
            </IconButton>
            {!phone && tags.length === 0 && !readOnly && (
              <IconButton
                label={t`Add a language`}
                keys={keyLabel(DESIGNER_KEYS.addLanguage)}
                onClick={() => setAdding(true)}
                testId="language-add-first"
              >
                <LanguagesIcon />
              </IconButton>
            )}
            {!phone && (
              <ChecksButton
                count={issueCount}
                level={level}
                open={checksOpen}
                onClick={() => setChecksOpen((o) => !o)}
              />
            )}
            {!phone && (
              <Button
                variant="ghost"
                size="sm"
                className={cn("gap-1.5 px-2.5", rtab === "versions" && "bg-muted")}
                title={keyHint(t`Versions`, DESIGNER_KEYS.versions)}
                onClick={() => setRtab("versions")}
                data-testid="versions-open"
              >
                <Trans>Versions</Trans>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" size="icon-sm" />}
                aria-label={t`More actions`}
                data-testid="designer-menu"
              >
                <EllipsisIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-64">
                {phone && (
                  <>
                    <DropdownMenuItem onClick={() => setChecksOpen((o) => !o)}>
                      <TriangleAlertIcon />
                      <span className="flex-1">
                        <Trans>Checks</Trans>
                      </span>
                      {issueCount > 0 && <span className="text-faint tabular-nums">{issueCount}</span>}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setVersionsOpen(true)}>
                      <HistoryIcon />
                      <Trans>Versions</Trans>
                    </DropdownMenuItem>
                    {tags.length === 0 && !readOnly && (
                      <DropdownMenuItem onClick={() => setAdding(true)}>
                        <LanguagesIcon />
                        <Trans>Add a language</Trans>
                      </DropdownMenuItem>
                    )}
                  </>
                )}
                <DropdownMenuItem onClick={flushNow} disabled={readOnly} data-testid="save-version">
                  <SaveIcon />
                  <span className="flex-1">
                    <Trans>Save now and keep a version</Trans>
                  </span>
                  <span className="flex gap-0.5">
                    <Kbd>{mod}</Kbd>
                    <Kbd>S</Kbd>
                  </span>
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => download(state)} data-testid="download-json">
                  <DownloadIcon />
                  <Trans>Download JSON</Trans>
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() =>
                    duplicate.mutate(initial.id, {
                      onSuccess: (d) => {
                        toast(t`Copy made`)
                        navigate(designLink(d.id))
                      },
                      onError: (err) => toast(err.message),
                    })
                  }
                >
                  <CopyIcon />
                  <Trans>Make a copy</Trans>
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => shell.openShortcuts()}>
                  <KeyboardIcon />
                  <span className="flex-1">
                    <Trans>Keyboard shortcuts</Trans>
                  </span>
                  <Kbd>?</Kbd>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() =>
                    confirm({
                      title: t`Archive this design?`,
                      description: t`It leaves the list of designs. Its versions are kept. Drafts that use it still send, and you can restore it from the archive.`,
                      confirm: t`Archive`,
                      run: () =>
                        archive.mutate(initial.id, {
                          onSuccess: () => {
                            toast(t`Design archived`)
                            navigate("/broadcasts/designs")
                          },
                        }),
                    })
                  }
                  data-testid="archive-design"
                >
                  <ArchiveIcon />
                  <Trans>Archive</Trans>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            {!phone && (
              <Button size="sm" onClick={leave} className="ms-1" data-testid="designer-done">
                <Trans>Done</Trans>
              </Button>
            )}
          </header>
          {phone && <div className="border-b px-2 py-1.5">{viewControls}</div>}
          {tags.length > 0 && (
            <LanguageBar
              doc={original}
              current={locale}
              onSelect={ctx.setLocale}
              onAdd={() => setAdding(true)}
              onChange={(fn) => commitDoc(fn)}
              readOnly={readOnly}
            />
          )}
          {checksOpen && (
            <ChecksStrip
              size={size}
              issues={allIssues}
              doc={doc}
              onShow={showIssue}
              onClose={() => setChecksOpen(false)}
            />
          )}
          {readOnly && (
            <Notice tone="warning" icon={TriangleAlertIcon} className="m-3 mb-0" data-testid="designer-read-only">
              <p>
                <Trans>A newer version of Yuva made this design. You can look at it here, but not change it.</Trans>
              </p>
            </Notice>
          )}
          {save.problem && save.status === "invalid" && save.errors.length === 0 && (
            <Notice tone="danger" icon={CircleAlertIcon} className="m-3 mb-0">
              <ErrorLine error={save.problem} />
            </Notice>
          )}
          <div className="flex min-h-0 flex-1">
            {showLeft && (
              <aside
                className="flex w-64 shrink-0 flex-col border-e max-[1100px]:w-56"
                aria-label={t`Blocks and layers`}
              >
                <div className="px-3 pt-3">
                  <Segmented
                    value={left}
                    onChange={setLeft}
                    label={t`Left panel`}
                    className="w-full"
                    itemClassName="flex-1"
                    items={[
                      { value: "blocks", label: t`Blocks`, testId: "tab-blocks" },
                      { value: "layers", label: t`Layers`, testId: "tab-layers" },
                    ]}
                  />
                </div>
                <div className="min-h-0 flex-1 overflow-y-auto p-3" ref={palette}>
                  {left === "blocks" ? <BlockPalette onAdd={(type) => add(type)} /> : <Layers />}
                </div>
              </aside>
            )}
            <main className="flex min-w-0 flex-1 flex-col">
              {!preview ? (
                <>
                  {!readOnly && <TextToolbar owner={selected} slot={setToolSlot} />}
                  <div className="min-h-0 flex-1 overflow-y-auto">{canvas}</div>
                </>
              ) : (
                <div className="min-h-0 flex-1 overflow-y-auto">
                  <Preview
                    phone={phoneView}
                    dark={dark}
                    html={render.data?.html}
                    loading={render.isPending}
                    error={render.error}
                    stale={stale}
                  />
                </div>
              )}
              {phone && (
                <nav className="flex h-14 shrink-0 items-stretch border-t bg-card" aria-label={t`Designer`}>
                  {(
                    [
                      ["blocks", BlocksIcon, t`Add`],
                      ["layers", LayersIcon, t`Layers`],
                      ["style", PaletteIcon, selectedBlock ? names[selectedBlock.type] : t`Design`],
                    ] as const
                  ).map(([key, Icon, label]) => (
                    <button
                      key={key}
                      type="button"
                      onClick={() => setSheet(key)}
                      className="flex flex-1 flex-col items-center justify-center gap-0.5 text-caption text-muted-foreground"
                      data-testid={`phone-${key}`}
                    >
                      {key === "style" && selectedBlock ? (
                        <BlockIcon type={selectedBlock.type} className="size-5" />
                      ) : (
                        <Icon className="size-5" />
                      )}
                      <span className="max-w-full truncate px-1">{label}</span>
                    </button>
                  ))}
                </nav>
              )}
            </main>
            {!phone && (
              <aside className="flex w-80 shrink-0 flex-col border-s max-[1100px]:w-72" aria-label={t`Designer`}>
                <Tabs value={rtab} onValueChange={(v) => setRtab(v as RightTab)} className="min-h-0 flex-1 gap-0">
                  <TabsList variant="line" className="h-10 w-full shrink-0 gap-0 border-b px-1">
                    <TabsTrigger value="block" className="px-1 text-small" data-testid="tab-block">
                      <Trans>Block settings</Trans>
                    </TabsTrigger>
                    <TabsTrigger value="design" className="px-1 text-small" data-testid="tab-design">
                      <Trans>Design settings</Trans>
                    </TabsTrigger>
                    <TabsTrigger value="versions" className="px-1 text-small" data-testid="tab-versions">
                      <Trans>Versions</Trans>
                    </TabsTrigger>
                  </TabsList>
                  <TabsContent value="block" className="min-h-0 flex-1 overflow-y-auto">
                    {block}
                  </TabsContent>
                  <TabsContent value="design" className="min-h-0 flex-1 overflow-y-auto">
                    {design}
                  </TabsContent>
                  <TabsContent value="versions" className="min-h-0 flex-1 overflow-y-auto">
                    <VersionsPanel
                      designId={initial.id}
                      inboxId={state.inbox_id}
                      current={save.version()}
                      onRestore={restore}
                      restoring={restoring}
                      error={restoreError}
                    />
                  </TabsContent>
                </Tabs>
              </aside>
            )}
          </div>
        </div>
        <DragOverlay dropAnimation={null}>
          {dragging && (
            <div className="inline-flex items-center gap-2 rounded-full bg-brand px-3 py-1.5 text-small font-medium text-white shadow-lg">
              <BlockIcon type={dragging.type} className="size-4" />
              {names[dragging.type]}
            </div>
          )}
        </DragOverlay>
      </DndContext>
      {phone && (
        <Sheet open={sheet !== null} onOpenChange={(o) => !o && setSheet(null)}>
          <SheetContent className="w-full" data-testid="designer-sheet">
            <SheetHeader>
              <SheetTitle className="text-title">
                {sheet === "blocks" ? (
                  <Trans>Add a block</Trans>
                ) : sheet === "layers" ? (
                  <Trans>Layers</Trans>
                ) : selectedBlock ? (
                  names[selectedBlock.type]
                ) : (
                  <Trans>Design</Trans>
                )}
              </SheetTitle>
            </SheetHeader>
            <div className="min-h-0 flex-1 overflow-y-auto">
              {sheet === "blocks" && (
                <div className="p-4">
                  <BlockPalette onAdd={(type) => add(type)} />
                </div>
              )}
              {sheet === "layers" && (
                <div className="p-2" onClickCapture={() => setTimeout(() => setSheet(null), 0)}>
                  <Layers />
                </div>
              )}
              {sheet === "style" && right}
            </div>
          </SheetContent>
        </Sheet>
      )}
      <ImageLibrary
        open={library !== null}
        onOpenChange={(o) => !o && setLibrary(null)}
        current={library ? (findBlock(doc, library) as { image_id?: string } | null)?.image_id : undefined}
        onPick={(img) => {
          const block = library ? findBlock(doc, library) : null
          if (block?.type === "image" && block.image_id !== img.id) {
            ctx.update(block.id, block.decorative ? { image_id: img.id } : { image_id: img.id, alt: "" })
            if (!block.decorative) altFocus.current = block.id
          }
          setLibrary(null)
        }}
        finalFocus={() => {
          const id = altFocus.current
          altFocus.current = null
          return (id && document.getElementById(`alt-${id}`)) || true
        }}
      />
      <Versions
        open={phone && versionsOpen}
        onOpenChange={setVersionsOpen}
        designId={initial.id}
        inboxId={state.inbox_id}
        current={save.version()}
        onRestore={restore}
        restoring={restoring}
        error={restoreError}
      />
      <ConflictDialog
        theirs={save.conflict}
        busy={busy}
        onTheirs={resolveTheirs}
        onMine={resolveMine}
        onCopy={resolveCopy}
        onDownload={() => download(state)}
      />
      {confirmDialog}
      <AddLanguageDialog
        open={adding}
        onOpenChange={setAdding}
        doc={original}
        suggestedBase={
          inboxes.find((i) => i.id === state.inbox_id)?.default_locale ?? inboxes[0]?.default_locale ?? "en"
        }
        onAdd={(tag, base) => {
          commitDoc((d) => addVersion(d, tag, base))
          setLang(tag)
          setAdding(false)
        }}
      />
      <span className="sr-only" aria-live="polite">
        {save.status === "saved" ? t`Saved` : ""}
      </span>
    </DesignerContext.Provider>
  )
}
