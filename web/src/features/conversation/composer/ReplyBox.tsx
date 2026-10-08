import { Trans, useLingui } from "@lingui/react/macro"
import { PaperclipIcon, SlashIcon, SparklesIcon, XIcon } from "lucide-react"
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react"

import { BotAvatar, ErrorLine, Kbd, toast } from "@/components/common"
import { mod } from "@/components/common/ShortcutSheet"
import { formatBytes } from "@/components/common/text"
import { popupClass } from "@/components/ui/dropdown-menu"
import { firstName, readDraft, writeDraft, type QueueActions } from "@/features/conversation/actions"
import { CannedMenu, slashToken, useCannedMatches } from "@/features/conversation/composer/CannedMenu"
import { useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { useDraftActions } from "@/features/conversation/queries"
import type { CannedReply, Conversation, Message } from "@/lib/api"
import { useTypingSender } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useCannedReplies } from "@/lib/workspace"

export type ReplyHandle = {
  focus: (mode?: "message" | "note") => void
  attach: () => void
  discardSuggestion: () => void
}

const MAX_FILES = 10

export const pillButton = "inline-flex h-9 items-center gap-1.5 rounded-full border px-3.5 text-sm transition-colors disabled:opacity-50"

function Suggestion({ m, ctx, onUse, onDiscard }: { m: Message; ctx: ThreadContext; onUse: () => void; onDiscard: () => void }) {
  const { t } = useLingui()
  const author = useAuthorName(m, ctx)
  return (
    <div
      className="mx-2.5 mb-2 flex items-start gap-2.5 rounded-xl border border-dashed border-brand/35 bg-brand-wash/60 py-2.5 ps-3 pe-1.5 text-[13px] leading-normal text-muted-foreground transition-colors hover:border-solid hover:text-foreground"
      data-testid="suggestion"
    >
      <button type="button" onClick={onUse} className="flex min-w-0 flex-1 items-start gap-2.5 text-start" title={t`Use this reply`}>
        {m.author.type === "bot" ? (
          <BotAvatar name={author} url={m.author.avatar_url} className="mt-px size-4 text-[8px]" />
        ) : (
          <SparklesIcon className="mt-0.5 size-3.5 shrink-0 text-brand" />
        )}
        <span className="min-w-0 flex-1">
          <span className="block text-[11px] text-faint">
            <Trans>Suggested by {author}</Trans>
          </span>
          <span className="line-clamp-2">{m.body}</span>
        </span>
        <Kbd className="mt-0.5 bg-card">Tab</Kbd>
      </button>
      <button
        type="button"
        onClick={onDiscard}
        className="grid size-6 shrink-0 place-items-center rounded-md text-faint transition-colors hover:bg-card hover:text-foreground"
        aria-label={t`Discard the suggested reply`}
        title={t`Discard the suggested reply`}
      >
        <XIcon className="size-3.5" />
      </button>
    </div>
  )
}

export const ReplyBox = forwardRef<
  ReplyHandle,
  {
    c: Conversation
    contactName: string
    via?: string
    emailTo?: string
    undeliverable?: boolean
    suggestion?: Message
    ctx: ThreadContext
    actions: QueueActions
    autoFocus: boolean
  }
>(function ReplyBox({ c, contactName, via, emailTo, undeliverable, suggestion, ctx, actions, autoFocus }, ref) {
  const { t, i18n } = useLingui()
  const initial = useRef(readDraft(c.id)).current
  const [mode, setMode] = useState(initial.mode)
  const [body, setBody] = useState(initial.body)
  const [files, setFiles] = useState<File[]>(initial.files)
  const [linked, setLinked] = useState<string | null>(null)
  const [caret, setCaret] = useState(0)
  const [pick, setPick] = useState(0)
  const [dismissed, setDismissed] = useState<number | null>(null)
  const [cannedOpen, setCannedOpen] = useState(false)
  const [noting, setNoting] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const typing = useTypingSender(c.id)
  const { discard } = useDraftActions()
  const canned = useCannedReplies().data ?? []
  const note = mode === "note"
  const first = firstName(contactName)

  useEffect(() => writeDraft(c.id, { body, mode, files }), [c.id, body, mode, files])
  const stopTyping = typing.stop
  useEffect(() => {
    if (note) stopTyping()
  }, [note, stopTyping])
  useEffect(() => {
    if (autoFocus) textarea.current?.focus({ preventScroll: false })
  }, [autoFocus])

  const focus = (m?: "message" | "note") => {
    if (m) setMode(m)
    requestAnimationFrame(() => textarea.current?.focus())
  }
  const showSuggestion = !!suggestion && !note && linked !== suggestion.id
  const applySuggestion = () => {
    if (!suggestion) return
    setBody(suggestion.body)
    setLinked(suggestion.id)
    focus()
  }
  const dropSuggestion = () => {
    if (!suggestion) return
    if (linked === suggestion.id) setLinked(null)
    discard.mutate(suggestion, { onSuccess: () => toast(t`Suggestion discarded`), onError: (e) => setError(e) })
  }
  useImperativeHandle(ref, () => ({ focus, attach: () => fileInput.current?.click(), discardSuggestion: dropSuggestion }))

  const token = slashToken(body, caret)
  const matches = useCannedMatches(token?.query ?? null)
  const menuOpen = token !== null && matches.length > 0 && dismissed !== token.start

  const insertAt = (text: string, start: number, end: number) => {
    const next = body.slice(0, start) + text + body.slice(end)
    const pos = start + text.length
    setBody(next)
    setCaret(pos)
    requestAnimationFrame(() => {
      textarea.current?.setSelectionRange(pos, pos)
      textarea.current?.focus()
    })
  }
  const insertSlash = (r: CannedReply) => token && insertAt(r.body, token.start, caret)
  const insertCanned = (r: CannedReply) => {
    setCannedOpen(false)
    const at = textarea.current?.selectionStart ?? body.length
    const sep = at > 0 && !/\s$/.test(body.slice(0, at)) ? "\n" : ""
    insertAt(sep + r.body, at, at)
  }

  const addFiles = (more: File[]) => setFiles((f) => [...f, ...more].slice(0, MAX_FILES))
  const hasContent = body.trim() !== "" || files.length > 0
  const reset = () => {
    setBody("")
    setFiles([])
    setCaret(0)
    setLinked(null)
    setError(null)
  }
  const send = (close: boolean) => {
    if (!hasContent || note) return
    typing.stop()
    const linkedDraft = suggestion && linked === suggestion.id ? suggestion : undefined
    actions.send({ body, files, close, suggestion: linkedDraft })
    reset()
  }
  const addNote = () => {
    if (!hasContent || noting) return
    const saved = { body, files }
    setNoting(true)
    setError(null)
    setBody("")
    setFiles([])
    actions
      .note(saved.body, saved.files)
      .then(() => setMode("message"))
      .catch((e) => {
        setBody((b) => b || saved.body)
        setFiles((f) => (f.length ? f : saved.files))
        setError(e)
      })
      .finally(() => setNoting(false))
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (menuOpen) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault()
        const step = e.key === "ArrowDown" ? 1 : -1
        setPick((i) => (i + step + matches.length) % matches.length)
        return
      }
      if ((e.key === "Enter" && !e.metaKey && !e.ctrlKey) || e.key === "Tab") {
        e.preventDefault()
        insertSlash(matches[Math.min(pick, matches.length - 1)])
        return
      }
      if (e.key === "Escape") {
        e.preventDefault()
        setDismissed(token!.start)
        return
      }
    }
    if (e.key === "Tab" && !e.shiftKey && showSuggestion && body.trim() === "") {
      e.preventDefault()
      applySuggestion()
      return
    }
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      if (note) addNote()
      else send(!e.shiftKey)
      return
    }
    if (e.key === "Escape") {
      if (cannedOpen) setCannedOpen(false)
      else e.currentTarget.blur()
    }
  }

  const hint = note ? t`The customer does not see this` : emailTo ? t`To: ${emailTo}` : via ? t`Goes out through ${via}` : ""

  return (
    <div
      className={cn(
        "relative mt-6 rounded-[20px] border bg-card shadow-[0_1px_2px_rgb(0_0_0/0.04),0_8px_24px_-12px_rgb(0_0_0/0.08)] transition-colors focus-within:border-brand/45",
        note && "border-note-border bg-note focus-within:border-note-ink/40",
      )}
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault()
        addFiles(Array.from(e.dataTransfer.files))
      }}
      data-testid="reply-box"
    >
      <div className="flex items-center gap-0.5 px-2 pt-2" role="tablist" aria-label={t`Message type`}>
        {(["message", "note"] as const).map((m) => (
          <button
            key={m}
            type="button"
            role="tab"
            aria-selected={mode === m}
            onClick={() => focus(m)}
            className={cn(
              "rounded-lg px-2.5 py-[5px] text-[13px] text-faint transition-colors",
              mode === m && (note ? "bg-card font-medium text-note-ink" : "bg-background font-medium text-foreground"),
            )}
          >
            {m === "message" ? <Trans>Reply</Trans> : <Trans>Team note</Trans>}
          </button>
        ))}
        <span className="ms-auto min-w-0 truncate pe-2 text-xs text-faint" data-testid={!note && emailTo ? "composer-email-to" : undefined}>
          {hint}
        </span>
      </div>
      {!note && undeliverable && (
        <p className="mx-3 mt-2 rounded-lg bg-destructive/6 px-3 py-2 text-xs text-destructive" data-testid="composer-undeliverable">
          <Trans>This address is undeliverable. Clear it in the contact details before you reply.</Trans>
        </p>
      )}
      {menuOpen && <CannedMenu items={matches} active={pick} onPick={insertSlash} />}
      <textarea
        ref={textarea}
        value={body}
        rows={3}
        data-testid="composer-input"
        aria-label={note ? t`Team note` : t`Reply`}
        placeholder={note ? t`A note for the team…` : t`Write to ${first}…`}
        className="field-sizing-content block max-h-[50vh] min-h-[84px] w-full resize-none bg-transparent px-4 py-2.5 text-[15px] leading-[1.55] outline-none placeholder:text-faint focus-visible:outline-none"
        onChange={(e) => {
          setBody(e.target.value)
          setCaret(e.target.selectionStart)
          setPick(0)
          if (note) return
          if (e.target.value.trim() === "") typing.stop()
          else typing.typed()
        }}
        onBlur={typing.stop}
        onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
        onKeyDown={onKeyDown}
      />
      {showSuggestion && <Suggestion m={suggestion} ctx={ctx} onUse={applySuggestion} onDiscard={dropSuggestion} />}
      {files.length > 0 && (
        <ul className="flex flex-wrap gap-1.5 px-4 pb-2">
          {files.map((f, i) => (
            <li key={`${f.name}-${i}`} className="flex max-w-full items-center gap-1.5 rounded-lg border bg-background py-0.5 ps-2 pe-0.5 text-xs">
              <PaperclipIcon className="size-3 shrink-0 text-faint" />
              <span className="max-w-48 truncate">{f.name}</span>
              <span className="text-faint">{formatBytes(f.size, i18n.locale)}</span>
              <button
                type="button"
                className="grid size-5 place-items-center rounded text-faint hover:text-foreground"
                aria-label={t`Remove ${f.name}`}
                onClick={() => setFiles((all) => all.filter((_, j) => j !== i))}
              >
                <XIcon className="size-3" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <ErrorLine error={error} className="px-4 pb-2 text-xs" />
      <div className="relative flex items-center gap-1 px-2.5 pb-2.5">
        <input
          ref={fileInput}
          type="file"
          multiple
          hidden
          data-testid="composer-files"
          onChange={(e) => {
            addFiles(Array.from(e.target.files ?? []))
            e.target.value = ""
          }}
        />
        <button
          type="button"
          className="inline-flex h-8 min-w-8 items-center justify-center rounded-lg px-2 text-faint transition-colors hover:bg-background hover:text-foreground"
          aria-label={t`Attach files`}
          title={t`Attach files`}
          onClick={() => fileInput.current?.click()}
        >
          <PaperclipIcon className="size-4" />
        </button>
        <button
          type="button"
          className="inline-flex h-8 items-center gap-1.5 rounded-lg px-2 text-[13px] text-faint transition-colors hover:bg-background hover:text-foreground aria-expanded:bg-background aria-expanded:text-foreground"
          aria-expanded={cannedOpen}
          onClick={() => setCannedOpen((o) => !o)}
          title={t`Canned replies`}
          data-testid="canned-button"
        >
          <SlashIcon className="size-4" />
          <span className="phone:hidden">
            <Trans>Canned reply</Trans>
          </span>
        </button>
        {cannedOpen && (
          <div className={cn(popupClass, "absolute start-2.5 bottom-12 z-20 w-[min(320px,calc(100vw-48px))] p-1.5")} role="listbox" aria-label={t`Canned replies`}>
            {canned.length === 0 ? (
              <p className="px-3 py-6 text-center text-sm text-faint">
                <Trans>No canned replies yet. Add them in settings.</Trans>
              </p>
            ) : (
              canned.map((r) => (
                <button
                  key={r.id}
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => insertCanned(r)}
                  className="block w-full rounded-lg px-2.5 py-2 text-start text-sm hover:bg-background"
                >
                  {r.title}
                  <small className="block truncate text-xs text-faint">{r.body}</small>
                </button>
              ))
            )}
          </div>
        )}
        <span className="flex-1" />
        {note ? (
          <button
            type="button"
            className={cn(pillButton, "border-primary bg-primary text-white hover:brightness-105")}
            onClick={addNote}
            disabled={!hasContent || noting}
            data-testid="composer-note"
          >
            <Trans>Add note</Trans>
          </button>
        ) : (
          <>
            <span className="me-1.5 text-xs text-faint phone:hidden">{mod}↵</span>
            <button type="button" className={cn(pillButton, "bg-card hover:border-faint phone:px-3")} onClick={() => send(false)} disabled={!hasContent} data-testid="composer-send">
              <Trans>Send</Trans>
            </button>
            <button
              type="button"
              className={cn(pillButton, "border-primary bg-primary text-white hover:brightness-105 phone:px-3")}
              onClick={() => send(true)}
              disabled={!hasContent}
              data-testid="composer-send-close"
            >
              <Trans>Send and close</Trans>
            </button>
          </>
        )}
      </div>
    </div>
  )
})
