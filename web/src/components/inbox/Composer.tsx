import { Trans, useLingui } from "@lingui/react/macro"
import { AlertCircleIcon, LockIcon, MailIcon, MessageSquareIcon, PaperclipIcon, SendIcon, XIcon } from "lucide-react"
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react"

import { ErrorLine, Kbd } from "@/components/common"
import { formatBytes } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { CannedReply } from "@/lib/api"
import { useCannedReplies, useSendMessage } from "@/lib/queries"
import { useTypingSender } from "@/lib/typing"
import { cn } from "@/lib/utils"

export type ComposerHandle = {
  focus: (mode?: "message" | "note") => void
  attach: () => void
}

const mod = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl"

function slashToken(text: string, caret: number) {
  const m = /(^|\s)\/([a-z0-9-]*)$/.exec(text.slice(0, caret))
  if (!m) return null
  return { start: caret - m[2].length - 1, query: m[2] }
}

export const Composer = forwardRef<
  ComposerHandle,
  { conversationId: string; emailTo?: string; undeliverable?: boolean }
>(function Composer({ conversationId, emailTo, undeliverable }, ref) {
  const { t, i18n } = useLingui()
  const send = useSendMessage(conversationId)
  const canned = useCannedReplies().data ?? []
  const [mode, setMode] = useState<"message" | "note">("message")
  const [body, setBody] = useState("")
  const [files, setFiles] = useState<File[]>([])
  const [caret, setCaret] = useState(0)
  const [pick, setPick] = useState(0)
  const [dismissed, setDismissed] = useState<number | null>(null)
  const clientId = useRef(crypto.randomUUID())
  const textarea = useRef<HTMLTextAreaElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const typing = useTypingSender(conversationId)
  const stopTyping = typing.stop

  useEffect(() => {
    if (mode === "note") stopTyping()
  }, [mode, stopTyping])

  useImperativeHandle(ref, () => ({
    focus: (m) => {
      if (m) setMode(m)
      textarea.current?.focus()
    },
    attach: () => fileInput.current?.click(),
  }))

  const token = slashToken(body, caret)
  const matches = useMemo(() => {
    if (!token) return []
    const q = token.query.toLowerCase()
    return canned
      .filter((c) => c.shortcut.startsWith(q) || c.title.toLowerCase().includes(q))
      .slice(0, 8)
  }, [canned, token])
  const menuOpen = token !== null && matches.length > 0 && dismissed !== token.start

  const insert = (c: CannedReply) => {
    if (!token) return
    const next = body.slice(0, token.start) + c.body + body.slice(caret)
    const pos = token.start + c.body.length
    setBody(next)
    setCaret(pos)
    requestAnimationFrame(() => {
      textarea.current?.setSelectionRange(pos, pos)
      textarea.current?.focus()
    })
  }

  const canSend = (body.trim() !== "" || files.length > 0) && !send.isPending
  const submit = () => {
    if (!canSend) return
    typing.stop()
    const sent = { body, files, clientId: clientId.current, mode }
    setBody("")
    setFiles([])
    setCaret(0)
    clientId.current = crypto.randomUUID()
    send.mutate(
      { kind: sent.mode, body: sent.body.trim() || undefined, client_id: sent.clientId, files: sent.files },
      {
        onError: () => {
          setBody((b) => (b === "" ? sent.body : b))
          setFiles((f) => (f.length === 0 ? sent.files : f))
          clientId.current = sent.clientId
        },
      },
    )
  }

  const note = mode === "note"
  return (
    <div className="shrink-0 border-t bg-background p-3" data-testid="composer">
      <div
        className={cn(
          "relative flex flex-col rounded-lg border shadow-xs focus-within:ring-3 focus-within:ring-ring/30",
          note && "border-amber-300 bg-amber-50/70 dark:border-amber-800 dark:bg-amber-950/30",
        )}
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault()
          setFiles((f) => [...f, ...Array.from(e.dataTransfer.files)].slice(0, 10))
        }}
      >
        <div className="flex items-center gap-1 border-b px-2 py-1" role="tablist" aria-label={t`Message type`}>
          <Button
            type="button"
            role="tab"
            aria-selected={!note}
            variant={note ? "ghost" : "secondary"}
            size="xs"
            onClick={() => setMode("message")}
          >
            <MessageSquareIcon />
            <Trans>Reply</Trans>
          </Button>
          <Button
            type="button"
            role="tab"
            aria-selected={note}
            variant={note ? "secondary" : "ghost"}
            size="xs"
            onClick={() => setMode("note")}
            className={cn(note && "bg-amber-200/70 text-amber-900 dark:bg-amber-900/60 dark:text-amber-100")}
          >
            <LockIcon />
            <Trans>Note</Trans>
          </Button>
          {!note && emailTo ? (
            <span
              className="ml-auto flex min-w-0 items-center gap-1 text-xs text-muted-foreground"
              data-testid="composer-email-to"
            >
              <MailIcon className="size-3.5 shrink-0" />
              <span className="truncate" title={t`Reply by e-mail to ${emailTo}`}>
                <Trans>Reply by e-mail to {emailTo}</Trans>
              </span>
            </span>
          ) : (
            <span className="ml-auto hidden text-xs text-muted-foreground sm:inline">
              {note ? <Trans>Only members see notes</Trans> : <Trans>Type / for canned replies</Trans>}
            </span>
          )}
        </div>
        {!note && undeliverable && (
          <p
            className="flex items-start gap-1.5 border-b bg-destructive/5 px-3 py-1.5 text-xs text-destructive"
            data-testid="composer-undeliverable"
          >
            <AlertCircleIcon className="mt-px size-3.5 shrink-0" />
            <span>
              <Trans>This address is undeliverable. Clear it on the contact before you reply.</Trans>
            </span>
          </p>
        )}
        {menuOpen && (
          <ul
            role="listbox"
            aria-label={t`Canned replies`}
            className="absolute bottom-full left-0 z-20 mb-1 max-h-64 w-full max-w-md overflow-y-auto rounded-lg border bg-popover p-1 shadow-md"
          >
            {matches.map((c, i) => (
              <li key={c.id} role="option" aria-selected={i === pick}>
                <button
                  type="button"
                  onMouseDown={(e) => {
                    e.preventDefault()
                    insert(c)
                  }}
                  className={cn(
                    "flex w-full flex-col items-start rounded-md px-2 py-1.5 text-left text-sm",
                    i === pick && "bg-accent text-accent-foreground",
                  )}
                >
                  <span className="flex w-full items-center gap-2">
                    <span className="font-mono text-xs text-muted-foreground">/{c.shortcut}</span>
                    <span className="truncate font-medium">{c.title}</span>
                  </span>
                  <span className="line-clamp-1 text-xs text-muted-foreground">{c.body}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        <Textarea
          ref={textarea}
          value={body}
          rows={3}
          data-testid="composer-input"
          aria-label={note ? t`Note` : t`Reply`}
          placeholder={note ? t`Write a note for your team…` : t`Write a reply…`}
          className="max-h-60 min-h-20 resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 dark:bg-transparent"
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
          onKeyDown={(e) => {
            if (menuOpen) {
              if (e.key === "ArrowDown") {
                e.preventDefault()
                setPick((i) => (i + 1) % matches.length)
                return
              }
              if (e.key === "ArrowUp") {
                e.preventDefault()
                setPick((i) => (i - 1 + matches.length) % matches.length)
                return
              }
              if ((e.key === "Enter" && !e.metaKey && !e.ctrlKey) || e.key === "Tab") {
                e.preventDefault()
                insert(matches[Math.min(pick, matches.length - 1)])
                return
              }
              if (e.key === "Escape") {
                e.preventDefault()
                setDismissed(token!.start)
                return
              }
            }
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              submit()
              return
            }
            if (e.key === "Escape") e.currentTarget.blur()
          }}
        />
        {files.length > 0 && (
          <ul className="flex flex-wrap gap-1.5 px-2 pb-2">
            {files.map((f, i) => (
              <li
                key={`${f.name}-${i}`}
                className="flex max-w-full items-center gap-1.5 rounded-md border bg-background py-0.5 pr-0.5 pl-2 text-xs"
              >
                <PaperclipIcon className="size-3 shrink-0 text-muted-foreground" />
                <span className="max-w-48 truncate">{f.name}</span>
                <span className="text-muted-foreground">{formatBytes(f.size, i18n.locale)}</span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  aria-label={t`Remove ${f.name}`}
                  onClick={() => setFiles((all) => all.filter((_, j) => j !== i))}
                >
                  <XIcon />
                </Button>
              </li>
            ))}
          </ul>
        )}
        <div className="flex items-center gap-2 px-2 pb-2">
          <input
            ref={fileInput}
            type="file"
            multiple
            hidden
            data-testid="composer-files"
            onChange={(e) => {
              const picked = Array.from(e.target.files ?? [])
              setFiles((f) => [...f, ...picked].slice(0, 10))
              e.target.value = ""
            }}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t`Attach files`}
            onClick={() => fileInput.current?.click()}
          >
            <PaperclipIcon />
          </Button>
          <ErrorLine error={send.error} className="line-clamp-3 min-w-0 flex-1 text-xs" />
          <span className="ml-auto hidden items-center gap-1 text-xs text-muted-foreground sm:flex">
            <Kbd>{mod}</Kbd>
            <Kbd>Enter</Kbd>
          </span>
          <Button
            type="button"
            size="sm"
            onClick={submit}
            disabled={!canSend}
            data-testid="composer-send"
            className={cn(note && "bg-amber-600 text-white hover:bg-amber-600/85")}
          >
            <SendIcon />
            {note ? <Trans>Add note</Trans> : <Trans>Send</Trans>}
          </Button>
        </div>
      </div>
    </div>
  )
})
