import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpIcon, PaperclipIcon, XIcon } from "lucide-react"
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react"

import { ErrorLine, Kbd } from "@/components/common"
import { formatBytes } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { CannedMenu, slashToken, useCannedMatches } from "@/features/conversation/composer/CannedMenu"
import { useSendMessage } from "@/features/conversation/queries"
import type { CannedReply } from "@/lib/api"
import { useTypingSender } from "@/lib/typing"
import { cn } from "@/lib/utils"

export type ComposerHandle = {
  focus: (mode?: "message" | "note") => void
  attach: () => void
}

const mod = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl"
const MAX_FILES = 10

function ModeTab({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={on}
      onClick={onClick}
      className={cn(
        "h-7 rounded-full px-2.5 text-[0.8rem] transition-colors",
        on ? "bg-muted font-medium text-foreground" : "text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  )
}

export const Composer = forwardRef<ComposerHandle, { conversationId: string; emailTo?: string; undeliverable?: boolean }>(
  function Composer({ conversationId, emailTo, undeliverable }, ref) {
    const { t, i18n } = useLingui()
    const send = useSendMessage(conversationId)
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
    const note = mode === "note"

    useEffect(() => {
      if (note) stopTyping()
    }, [note, stopTyping])

    useImperativeHandle(ref, () => ({
      focus: (m) => {
        if (m) setMode(m)
        textarea.current?.focus()
      },
      attach: () => fileInput.current?.click(),
    }))

    const token = slashToken(body, caret)
    const matches = useCannedMatches(token?.query ?? null)
    const menuOpen = token !== null && matches.length > 0 && dismissed !== token.start

    const insert = (c: CannedReply) => {
      if (!token) return
      const pos = token.start + c.body.length
      setBody(body.slice(0, token.start) + c.body + body.slice(caret))
      setCaret(pos)
      requestAnimationFrame(() => {
        textarea.current?.setSelectionRange(pos, pos)
        textarea.current?.focus()
      })
    }

    const addFiles = (more: File[]) => setFiles((f) => [...f, ...more].slice(0, MAX_FILES))
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
    }

    return (
      <div className="mx-auto w-full max-w-3xl shrink-0 px-4 pb-4 sm:px-6" data-testid="composer">
        <div
          className={cn(
            "relative flex flex-col rounded-2xl border bg-card transition-colors focus-within:border-input",
            note && "border-dashed border-input bg-surface",
          )}
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault()
            addFiles(Array.from(e.dataTransfer.files))
          }}
        >
          <div className="flex items-center gap-1 px-2.5 pt-2.5" role="tablist" aria-label={t`Message type`}>
            <ModeTab on={!note} onClick={() => setMode("message")}>
              <Trans>Reply</Trans>
            </ModeTab>
            <ModeTab on={note} onClick={() => setMode("note")}>
              <Trans>Note</Trans>
            </ModeTab>
            <span className="ms-auto min-w-0 truncate ps-2 text-xs text-faint" data-testid={!note && emailTo ? "composer-email-to" : undefined}>
              {note ? (
                <Trans>Only members see notes</Trans>
              ) : emailTo ? (
                <Trans>Reply by e-mail to {emailTo}</Trans>
              ) : (
                <span className="hidden sm:inline">
                  <Trans>Type / for canned replies</Trans>
                </span>
              )}
            </span>
          </div>
          {!note && undeliverable && (
            <p className="mx-3 mt-2 rounded-lg bg-destructive/6 px-3 py-2 text-xs text-destructive" data-testid="composer-undeliverable">
              <Trans>This address is undeliverable. Clear it on the contact before you reply.</Trans>
            </p>
          )}
          {menuOpen && <CannedMenu items={matches} active={pick} onPick={insert} />}
          <textarea
            ref={textarea}
            value={body}
            rows={2}
            data-testid="composer-input"
            aria-label={note ? t`Note` : t`Reply`}
            placeholder={note ? t`Write a note for your team…` : t`Write a reply…`}
            className="field-sizing-content max-h-64 min-h-16 w-full resize-none bg-transparent px-4 py-2.5 text-base leading-relaxed outline-none placeholder:text-faint focus-visible:outline-none md:text-sm"
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
          {files.length > 0 && (
            <ul className="flex flex-wrap gap-1.5 px-3 pb-2">
              {files.map((f, i) => (
                <li key={`${f.name}-${i}`} className="flex max-w-full items-center gap-1.5 rounded-full border bg-background py-0.5 ps-2.5 pe-0.5 text-xs">
                  <span className="max-w-48 truncate">{f.name}</span>
                  <span className="text-faint">{formatBytes(f.size, i18n.locale)}</span>
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
          <div className="flex items-center gap-2 px-2.5 pb-2.5">
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
            <Button type="button" variant="ghost" size="icon-sm" aria-label={t`Attach files`} onClick={() => fileInput.current?.click()}>
              <PaperclipIcon />
            </Button>
            <ErrorLine error={send.error} className="line-clamp-3 min-w-0 flex-1 text-xs" />
            <span className="ms-auto hidden items-center gap-1 text-xs text-faint sm:flex">
              <Kbd>{mod}</Kbd>
              <Kbd>Enter</Kbd>
            </span>
            <Button
              className="ms-auto sm:ms-0"
              type="button"
              size="sm"
              variant={note ? "secondary" : "default"}
              onClick={submit}
              disabled={!canSend}
              data-testid="composer-send"
            >
              {note ? <Trans>Add note</Trans> : <Trans>Send</Trans>}
              <ArrowUpIcon />
            </Button>
          </div>
        </div>
      </div>
    )
  },
)
