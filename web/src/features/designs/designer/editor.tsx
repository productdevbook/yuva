import { Trans, useLingui } from "@lingui/react/macro"
import { Extension, mergeAttributes, Node, type Editor } from "@tiptap/core"
import type { Mark } from "@tiptap/pm/model"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { Placeholder } from "@tiptap/extensions"
import {
  EditorContent,
  NodeViewWrapper,
  ReactNodeViewRenderer,
  useEditor,
  useEditorState,
  type ReactNodeViewProps,
} from "@tiptap/react"
import { StarterKit } from "@tiptap/starter-kit"
import {
  BoldIcon,
  BracesIcon,
  CodeIcon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  RemoveFormattingIcon,
  StrikethroughIcon,
  UnderlineIcon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { validUrl } from "@/features/designs/doc"
import {
  ATTRIBUTE_PREFIX,
  canonical,
  knownField,
  MERGE_FIELDS,
  mergeToken,
  plainText,
  stripParagraph,
  textWithChips,
  withChips,
} from "@/features/designs/rich"
import { useMergeFieldNames } from "@/features/designs/text"
import { useLatest } from "@/hooks/use-latest"
import { mod } from "@/components/common/ShortcutSheet"
import { cn } from "@/lib/utils"

export type EditorMode = "rich" | "inline" | "plain"

type Active = { editor: Editor; mode: EditorMode; owner: string } | null

let active: Active = null
const listeners = new Set<() => void>()

function setActive(a: Active) {
  active = a
  listeners.forEach((l) => l())
}

export function clearActiveEditor(owner?: string) {
  if (!owner || active?.owner === owner) setActive(null)
}

function useActiveEditor() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => active
  )
}

function MergeChip({ node, updateAttributes, deleteNode, selected, editor }: ReactNodeViewProps) {
  const { t } = useLingui()
  const names = useMergeFieldNames()
  const field = String(node.attrs.field ?? "")
  const fallback = (node.attrs.fallback as string | null) ?? ""
  const known = knownField(field)
  const label = names[field] ?? field
  const [open, setOpen] = useState(false)
  return (
    <NodeViewWrapper as="span" className="inline">
      <Popover open={open && editor.isEditable} onOpenChange={setOpen}>
        <PopoverTrigger
          nativeButton={false}
          render={
            <span
              role="button"
              tabIndex={-1}
              contentEditable={false}
              className={cn(
                "mx-px inline-flex max-w-full cursor-pointer items-center gap-1 rounded-md border px-1.5 align-baseline font-sans text-[0.8em] leading-[1.5] font-medium whitespace-nowrap",
                known
                  ? "border-brand/30 bg-brand-wash text-brand"
                  : "border-destructive/40 bg-destructive/10 text-destructive",
                selected && "ring-2 ring-ring"
              )}
              title={known ? mergeToken(field, fallback) : t`There is no merge field called ${field}.`}
              data-testid="merge-chip"
            />
          }
        >
          <BracesIcon className="size-3 shrink-0" />
          <span className="truncate">{label}</span>
          {fallback && <span className="truncate font-normal opacity-75">· {fallback}</span>}
        </PopoverTrigger>
        <PopoverContent align="start" className="w-72 gap-3">
          <p className="text-small text-muted-foreground">
            {known ? (
              <code className="font-mono text-caption break-all">{field}</code>
            ) : (
              <span className="text-destructive">
                <Trans>There is no merge field called {field}.</Trans>
              </span>
            )}
          </p>
          <FallbackField
            value={fallback}
            onChange={(v) => updateAttributes({ fallback: v || null })}
            onDone={() => setOpen(false)}
          />
          <div className="flex justify-end">
            <Button variant="destructive" size="sm" onClick={() => deleteNode()}>
              <Trans>Remove</Trans>
            </Button>
          </div>
        </PopoverContent>
      </Popover>
    </NodeViewWrapper>
  )
}

function FallbackField({
  value,
  onChange,
  onDone,
}: {
  value: string
  onChange: (v: string) => void
  onDone?: () => void
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor="merge-fallback">
        <Trans>When it is empty, write</Trans>
      </Label>
      <Input
        id="merge-fallback"
        value={value}
        maxLength={100}
        onChange={(e) => onChange(e.target.value.replace(/"/g, ""))}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault()
            onDone?.()
          }
        }}
        data-testid="merge-fallback"
      />
      <p className="text-caption text-faint">
        <Trans>A field without a fallback can leave a gap, like “Hi ,”.</Trans>
      </p>
    </div>
  )
}

const MergeField = Node.create({
  name: "mergeField",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,
  addAttributes() {
    return {
      field: {
        default: "",
        parseHTML: (el) => el.getAttribute("data-merge-field") ?? "",
        renderHTML: (a) => ({ "data-merge-field": a.field }),
      },
      fallback: {
        default: null,
        parseHTML: (el) => el.getAttribute("data-fallback"),
        renderHTML: (a) => (a.fallback ? { "data-fallback": a.fallback } : {}),
      },
    }
  },
  parseHTML() {
    return [{ tag: "span[data-merge-field]" }]
  },
  renderHTML({ HTMLAttributes, node }) {
    return ["span", mergeAttributes(HTMLAttributes), String(node.attrs.field)]
  },
  renderText({ node }) {
    return mergeToken(String(node.attrs.field), (node.attrs.fallback as string | null) ?? undefined)
  },
  addNodeView() {
    return ReactNodeViewRenderer(MergeChip, { as: "span" })
  },
})

const SingleLine = Node.create({ name: "doc", topNode: true, content: "paragraph" })

const keepMarksKey = new PluginKey<boolean>("keepMarks")

const KeepMarks = Extension.create({
  name: "keepMarks",
  addProseMirrorPlugins() {
    return [
      new Plugin<boolean>({
        key: keepMarksKey,
        state: {
          init: () => false,
          apply: (tr, on) => tr.getMeta(keepMarksKey) ?? on,
        },
        props: {
          handleTextInput(view, from, to, text) {
            if (from === to || !keepMarksKey.getState(view.state)) return false
            const { doc, schema } = view.state
            let marks: readonly Mark[] | null = null
            doc.nodesBetween(from, to, (node) => {
              if (!node.isText) return true
              marks = marks ? marks.filter((m) => m.isInSet(node.marks)) : node.marks
              return false
            })
            const kept: readonly Mark[] = marks ?? []
            if (!kept.some((m) => m.type.spec.inclusive === false)) return false
            view.dispatch(view.state.tr.replaceWith(from, to, schema.text(text, kept)).scrollIntoView())
            return true
          },
        },
      }),
    ]
  },
})

function extensionsFor(mode: EditorMode, placeholder: string) {
  const link = {
    openOnClick: false,
    autolink: false,
    linkOnPaste: true,
    HTMLAttributes: { target: null, rel: null, class: null },
    protocols: ["mailto", "tel"],
    isAllowedUri: (url: string) => validUrl(url),
  }
  const plain = mode === "plain"
  return [
    StarterKit.configure({
      document: mode === "rich" ? undefined : false,
      heading: false,
      blockquote: false,
      codeBlock: false,
      horizontalRule: false,
      undoRedo: false,
      dropcursor: false,
      gapcursor: false,
      trailingNode: false,
      bulletList: mode === "rich" ? undefined : false,
      orderedList: mode === "rich" ? undefined : false,
      listItem: mode === "rich" ? undefined : false,
      listKeymap: mode === "rich" ? undefined : false,
      hardBreak: plain ? false : undefined,
      bold: plain ? false : undefined,
      italic: plain ? false : undefined,
      underline: plain ? false : undefined,
      strike: plain ? false : undefined,
      code: plain ? false : undefined,
      link: plain ? false : link,
    }),
    ...(mode === "rich" ? [] : [SingleLine]),
    MergeField,
    Placeholder.configure({ placeholder }),
    KeepMarks,
  ]
}

function toEditor(value: string, mode: EditorMode) {
  if (mode === "plain") return textWithChips(value)
  if (mode === "inline") return `<p>${withChips(canonical(value, true))}</p>`
  return withChips(canonical(value)) || "<p></p>"
}

function fromEditor(html: string, mode: EditorMode) {
  if (mode === "plain") return plainText(html)
  if (mode === "inline") return stripParagraph(html)
  return canonical(html)
}

export function RichEditor({
  value,
  onChange,
  mode,
  owner,
  placeholder,
  label,
  className,
  style,
  readOnly,
  testId,
  keepMarks = false,
}: {
  value: string
  onChange: (v: string) => void
  mode: EditorMode
  owner: string
  placeholder: string
  label: string
  className?: string
  style?: React.CSSProperties
  readOnly?: boolean
  testId?: string
  keepMarks?: boolean
}) {
  const last = useRef(value)
  const change = useLatest(onChange)
  const extensions = useMemo(() => extensionsFor(mode, placeholder), [mode, placeholder])
  const [content] = useState(() => toEditor(value, mode))
  const editorProps = useMemo(
    () => ({
      attributes: { "aria-label": label, class: "outline-none", ...(testId ? { "data-testid": testId } : {}) },
    }),
    [label, testId]
  )
  const editor = useEditor({
    extensions,
    content,
    editable: !readOnly,
    immediatelyRender: true,
    shouldRerenderOnTransaction: false,
    editorProps,
    onUpdate: ({ editor }) => {
      const v = fromEditor(editor.getHTML(), mode)
      if (v === last.current) return
      last.current = v
      change.current(v)
    },
    onFocus: ({ editor }) => setActive({ editor, mode, owner }),
  })
  useEffect(() => {
    if (value === last.current) return
    last.current = value
    editor.commands.setContent(toEditor(value, mode), { emitUpdate: false })
  }, [editor, value, mode])
  useEffect(() => {
    editor.view.dispatch(editor.state.tr.setMeta(keepMarksKey, keepMarks))
  }, [editor, keepMarks])
  useEffect(() => {
    editor.setEditable(!readOnly, false)
  }, [editor, readOnly])
  useEffect(() => () => clearActiveEditor(owner), [owner])
  return (
    <EditorContent
      editor={editor}
      className={cn(
        "min-w-0 cursor-text [&_.is-editor-empty:first-child]:before:pointer-events-none [&_.is-editor-empty:first-child]:before:float-start [&_.is-editor-empty:first-child]:before:h-0 [&_.is-editor-empty:first-child]:before:opacity-45 [&_.is-editor-empty:first-child]:before:content-[attr(data-placeholder)]",
        className
      )}
      style={style}
    />
  )
}

function ToolButton({
  label,
  shortcut,
  pressed,
  disabled,
  onClick,
  children,
}: {
  label: string
  shortcut?: string
  pressed?: boolean
  disabled?: boolean
  onClick?: () => void
  children: React.ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            aria-pressed={pressed}
            disabled={disabled}
            className="aria-pressed:bg-muted aria-pressed:text-foreground"
            onMouseDown={(e) => e.preventDefault()}
            onClick={onClick}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>
        {label}
        {shortcut && <span className="opacity-60">{shortcut}</span>}
      </TooltipContent>
    </Tooltip>
  )
}

function LinkButton({ editor, disabled }: { editor: Editor | null; disabled: boolean }) {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const [href, setHref] = useState("")
  const ok = validUrl(href)
  const apply = () => {
    if (!editor) return
    if (!href) editor.chain().focus().extendMarkRange("link").unsetLink().run()
    else if (ok) editor.chain().focus().extendMarkRange("link").setLink({ href: href.trim() }).run()
    else return
    setOpen(false)
  }
  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        if (o && editor) setHref(String(editor.getAttributes("link").href ?? ""))
        setOpen(o)
      }}
    >
      <PopoverTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t`Link`}
            disabled={disabled}
            aria-pressed={!!editor?.isActive("link")}
            className="aria-pressed:bg-muted aria-pressed:text-foreground"
            onMouseDown={(e) => e.preventDefault()}
            data-testid="tool-link"
          />
        }
      >
        <LinkIcon />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 gap-3">
        <form
          className="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            apply()
          }}
        >
          <Label htmlFor="link-href">
            <Trans>Link address</Trans>
          </Label>
          <Input
            id="link-href"
            value={href}
            placeholder="https://"
            onChange={(e) => setHref(e.target.value)}
            aria-invalid={(href !== "" && !ok) || undefined}
            autoFocus
            data-testid="link-href"
          />
          {href !== "" && !ok && (
            <p className="text-small text-destructive">
              <Trans>Links start with https://, http://, mailto: or tel:.</Trans>
            </p>
          )}
          <div className="flex justify-end gap-2">
            {editor?.isActive("link") && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  editor.chain().focus().extendMarkRange("link").unsetLink().run()
                  setOpen(false)
                }}
              >
                <Trans>Remove link</Trans>
              </Button>
            )}
            <Button type="submit" size="sm" disabled={href !== "" && !ok}>
              <Trans>Apply</Trans>
            </Button>
          </div>
        </form>
      </PopoverContent>
    </Popover>
  )
}

export function MergePicker({
  onPick,
  disabled,
  trigger,
}: {
  onPick: (token: { field: string; fallback?: string }) => void
  disabled?: boolean
  trigger?: React.ReactElement
}) {
  const { t } = useLingui()
  const names = useMergeFieldNames()
  const [open, setOpen] = useState(false)
  const [field, setField] = useState<string>(MERGE_FIELDS[0])
  const [attribute, setAttribute] = useState("")
  const [fallback, setFallback] = useState("")
  const custom = field === ATTRIBUTE_PREFIX
  const chosen = custom ? `${ATTRIBUTE_PREFIX}${attribute.trim()}` : field
  const ok = knownField(chosen)
  const pick = () => {
    if (!ok) return
    onPick({ field: chosen, fallback: fallback || undefined })
    setOpen(false)
    setFallback("")
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          trigger ?? (
            <Button
              variant="ghost"
              size="sm"
              className="gap-1 px-2"
              disabled={disabled}
              onMouseDown={(e) => e.preventDefault()}
              data-testid="tool-merge"
            />
          )
        }
      >
        <BracesIcon />
        <span className="phone:hidden">
          <Trans>Merge field</Trans>
        </span>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 gap-3">
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            pick()
          }}
        >
          <p className="text-small text-muted-foreground">
            <Trans>Put each person’s own details into the mail.</Trans>
          </p>
          <div className="grid grid-cols-2 gap-1" role="radiogroup" aria-label={t`Merge field`}>
            {[...MERGE_FIELDS, ATTRIBUTE_PREFIX].map((f) => (
              <button
                key={f}
                type="button"
                role="radio"
                aria-checked={field === f}
                onClick={() => setField(f)}
                className="rounded-lg border px-2 py-1.5 text-start text-small transition-colors hover:bg-muted aria-checked:border-brand aria-checked:bg-brand-wash aria-checked:text-brand"
              >
                {f === ATTRIBUTE_PREFIX ? t`Contact attribute` : names[f]}
              </button>
            ))}
          </div>
          {custom && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="merge-attribute">
                <Trans>Attribute key</Trans>
              </Label>
              <Input
                id="merge-attribute"
                value={attribute}
                placeholder="plan"
                onChange={(e) => setAttribute(e.target.value)}
                aria-invalid={(attribute !== "" && !ok) || undefined}
              />
            </div>
          )}
          <FallbackField value={fallback} onChange={setFallback} />
          <div className="flex justify-end">
            <Button type="submit" size="sm" disabled={!ok} data-testid="merge-insert">
              <Trans>Insert</Trans>
            </Button>
          </div>
        </form>
      </PopoverContent>
    </Popover>
  )
}

export function TextToolbar({ owner, slot }: { owner: string | null; slot?: (el: HTMLDivElement | null) => void }) {
  const { t } = useLingui()
  const current = useActiveEditor()
  const a = current && current.owner === owner ? current : null
  const editor = a?.editor ?? null
  const state = useEditorState({
    editor,
    selector: ({ editor: e }) =>
      e
        ? {
            bold: e.isActive("bold"),
            italic: e.isActive("italic"),
            underline: e.isActive("underline"),
            strike: e.isActive("strike"),
            code: e.isActive("code"),
            bullet: e.isActive("bulletList"),
            ordered: e.isActive("orderedList"),
          }
        : null,
  })
  const marks = !!a && a.mode !== "plain"
  const lists = a?.mode === "rich"
  const run = (fn: (e: Editor) => void) => () => editor && fn(editor)
  return (
    <div className="flex h-11 shrink-0 items-center border-b bg-card">
      <div
        className="flex h-full min-w-0 flex-1 items-center gap-0.5 overflow-x-auto px-2"
        role="toolbar"
        aria-label={t`Text formatting`}
        data-testid="text-toolbar"
      >
        <ToolButton
          label={t`Bold`}
          shortcut={`${mod} B`}
          pressed={state?.bold}
          disabled={!marks}
          onClick={run((e) => e.chain().focus().toggleBold().run())}
        >
          <BoldIcon />
        </ToolButton>
        <ToolButton
          label={t`Italic`}
          shortcut={`${mod} I`}
          pressed={state?.italic}
          disabled={!marks}
          onClick={run((e) => e.chain().focus().toggleItalic().run())}
        >
          <ItalicIcon />
        </ToolButton>
        <ToolButton
          label={t`Underline`}
          shortcut={`${mod} U`}
          pressed={state?.underline}
          disabled={!marks}
          onClick={run((e) => e.chain().focus().toggleUnderline().run())}
        >
          <UnderlineIcon />
        </ToolButton>
        <ToolButton
          label={t`Strikethrough`}
          pressed={state?.strike}
          disabled={!marks}
          onClick={run((e) => e.chain().focus().toggleStrike().run())}
        >
          <StrikethroughIcon />
        </ToolButton>
        <ToolButton
          label={t`Code`}
          pressed={state?.code}
          disabled={!marks}
          onClick={run((e) => e.chain().focus().toggleCode().run())}
        >
          <CodeIcon />
        </ToolButton>
        <LinkButton editor={editor} disabled={!marks} />
        <span className="mx-1 h-5 w-px shrink-0 bg-border" />
        <ToolButton
          label={t`Bulleted list`}
          pressed={state?.bullet}
          disabled={!lists}
          onClick={run((e) => e.chain().focus().toggleBulletList().run())}
        >
          <ListIcon />
        </ToolButton>
        <ToolButton
          label={t`Numbered list`}
          pressed={state?.ordered}
          disabled={!lists}
          onClick={run((e) => e.chain().focus().toggleOrderedList().run())}
        >
          <ListOrderedIcon />
        </ToolButton>
        <ToolButton
          label={t`Clear formatting`}
          disabled={!marks}
          onClick={run((e) => e.chain().focus().unsetAllMarks().run())}
        >
          <RemoveFormattingIcon />
        </ToolButton>
        <span className="mx-1 h-5 w-px shrink-0 bg-border" />
        <MergePicker
          disabled={!a}
          onPick={({ field, fallback }) =>
            editor
              ?.chain()
              .focus()
              .insertContent({ type: "mergeField", attrs: { field, fallback: fallback ?? null } })
              .run()
          }
        />
      </div>
      <div ref={slot} className="flex shrink-0 items-center gap-1 border-s px-2 empty:hidden" data-testid="block-bar" />
    </div>
  )
}
