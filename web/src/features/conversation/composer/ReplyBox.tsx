import { Trans, useLingui } from "@lingui/react/macro"
import { PaperclipIcon, SlashIcon, SparklesIcon, XIcon } from "lucide-react"
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react"

import { BotAvatar, ErrorLine, Kbd, toast } from "@/components/common"
import { mod } from "@/components/common/ShortcutSheet"
import { formatBytes } from "@/components/common/text"
import { popupClass } from "@/components/ui/dropdown-menu"
import { firstName, readDraft, writeDraft, type QueueActions } from "@/features/conversation/actions"
import { CannedMenu, slashToken, useCannedMatches } from "@/features/conversation/composer/CannedMenu"
import { MentionMenu, mentionToken } from "@/features/conversation/composer/MentionMenu"
import { useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { useDraftActions } from "@/features/conversation/queries"
import type { CannedReply, Conversation, Member, Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useTypingSender } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useAssignableMembers, useCannedReplies } from "@/lib/workspace"
import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"

export type ReplyHandle = {
  focus: (mode?: "message" | "note") => void
  attach: () => void
  discardSuggestion: () => void
}

const MAX_FILES = 10

const pill = "px-3.5 font-normal phone:px-3"

function Suggestion({ m, ctx, onUse, onDiscard }: { m: Message; ctx: ThreadContext; onUse: () => void; onDiscard: () => void }) {
  const { t } = useLingui()
  const author = useAuthorName(m, ctx)
  return (
    <div
      className="mx-2.5 mb-2 flex items-start gap-2.5 rounded-xl border border-dashed border-brand/35 bg-brand-wash/60 py-2.5 ps-3 pe-1.5 text-[13px] leading-normal text-muted-foreground transition-colors hover:border-solid hover:text-foreground"
      data-testid="suggestion"
    >
      <Button variant="plain" size="auto" onClick={onUse} className="flex min-w-0 flex-1 items-start gap-2.5 rounded-md text-[13px] leading-normal text-inherit" title={t`Use this reply`}>
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
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        onClick={onDiscard}
        className="rounded-md text-faint hover:bg-card"
        aria-label={t`Discard the suggested reply`}
        title={t`Discard the suggested reply`}
      >
        <XIcon />
      </Button>
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
  const [mentioned, setMentioned] = useState<{ id: string; label: string }[]>([])
  const [error, setError] = useState<unknown>(null)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const typing = useTypingSender(c.id)
  const { discard } = useDraftActions()
  const { membership } = useSession()
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

  const at = note ? mentionToken(body, caret) : null
  const people = useAssignableMembers(c.inbox_id).filter((m) => m.id !== membership.member_id)
  const query = at?.query.toLocaleLowerCase() ?? ""
  const mentionMatches = at
    ? people.filter((m) => (m.name || m.email).toLocaleLowerCase().split(/\s+/).some((w) => w.startsWith(query)) || m.email.startsWith(query)).slice(0, 6)
    : []
  const mentionOpen = at !== null && mentionMatches.length > 0 && dismissed !== at.start
  const token = mentionOpen ? null : slashToken(body, caret)
  const matches = useCannedMatches(token?.query ?? null)
  const menuOpen = token !== null && matches.length > 0 && dismissed !== token.start
  const listLength = mentionOpen ? mentionMatches.length : matches.length

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
  const insertMention = (m: Member) => {
    if (!at) return
    const label = m.name || m.email
    setMentioned((all) => (all.some((x) => x.id === m.id) ? all : [...all, { id: m.id, label }]))
    insertAt(`@${label} `, at.start, caret)
  }
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
    const tagged = mentioned.filter((x) => body.includes(`@${x.label}`))
    setNoting(true)
    setError(null)
    setBody("")
    setFiles([])
    actions
      .note(
        saved.body,
        saved.files,
        tagged.map((x) => x.id),
      )
      .then(() => {
        setMode("message")
        setMentioned([])
        if (tagged.length) {
          const names = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(tagged.map((x) => firstName(x.label)))
          toast(t`${names} will be notified`)
        }
      })
      .catch((e) => {
        setBody((b) => b || saved.body)
        setFiles((f) => (f.length ? f : saved.files))
        setError(e)
      })
      .finally(() => setNoting(false))
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (menuOpen || mentionOpen) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault()
        const step = e.key === "ArrowDown" ? 1 : -1
        setPick((i) => (i + step + listLength) % listLength)
        return
      }
      if ((e.key === "Enter" && !e.metaKey && !e.ctrlKey) || e.key === "Tab") {
        e.preventDefault()
        const i = Math.min(pick, listLength - 1)
        if (mentionOpen) insertMention(mentionMatches[i])
        else insertSlash(matches[i])
        return
      }
      if (e.key === "Escape") {
        e.preventDefault()
        setDismissed((mentionOpen ? at : token)!.start)
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
      <div className="flex items-center gap-0.5 px-2 pt-2">
        <Tabs value={mode} onValueChange={(v) => focus(v as "message" | "note")} className="gap-0">
          <TabsList aria-label={t`Message type`} className="h-auto gap-0.5 bg-transparent p-0">
            {(["message", "note"] as const).map((m) => (
              <TabsTrigger
                key={m}
                value={m}
                className={cn(
                  "h-auto flex-none rounded-lg px-2.5 py-[5px] text-[13px] font-normal text-faint data-active:font-medium data-active:shadow-none dark:data-active:border-transparent",
                  note ? "data-active:bg-card data-active:text-note-ink dark:data-active:bg-card" : "data-active:bg-background data-active:text-foreground dark:data-active:bg-background",
                )}
              >
                {m === "message" ? <Trans>Reply</Trans> : <Trans>Team note</Trans>}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
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
      {mentionOpen && <MentionMenu items={mentionMatches} active={pick} onPick={insertMention} />}
      <Textarea
        ref={textarea}
        value={body}
        rows={3}
        data-testid="composer-input"
        aria-label={note ? t`Team note` : t`Reply`}
        placeholder={note ? t`A note for the team… type @ to mention a teammate` : t`Write to ${first}…`}
        className="max-h-[50vh] min-h-[84px] resize-none rounded-none border-0 bg-transparent px-4 py-2.5 text-[15px] leading-[1.55] shadow-none hover:border-0 focus-visible:ring-0 md:text-[15px] dark:bg-transparent"
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
              <Button
                variant="ghost"
                size="icon-xs"
                className="size-5 rounded text-faint [&_svg]:size-3"
                aria-label={t`Remove ${f.name}`}
                onClick={() => setFiles((all) => all.filter((_, j) => j !== i))}
              >
                <XIcon />
              </Button>
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
        <Button
          variant="ghost"
          size="icon-sm"
          className="rounded-lg text-faint hover:bg-background"
          aria-label={t`Attach files`}
          title={t`Attach files`}
          onClick={() => fileInput.current?.click()}
        >
          <PaperclipIcon />
        </Button>
        <Popover open={cannedOpen} onOpenChange={setCannedOpen}>
          <PopoverTrigger
            render={<Button variant="ghost" size="sm" className="rounded-lg px-2 text-[13px] font-normal text-faint hover:bg-background aria-expanded:bg-background" />}
            title={t`Canned replies`}
            data-testid="canned-button"
          >
            <SlashIcon />
            <span className="phone:hidden">
              <Trans>Canned reply</Trans>
            </span>
          </PopoverTrigger>
          <PopoverContent side="top" align="start" className={cn(popupClass, "w-[min(320px,calc(100vw-48px))] gap-0 rounded-xl p-0 ring-0")}>
            <Command className="bg-transparent p-1.5" label={t`Canned replies`}>
              <CommandList className="max-h-72">
                <CommandEmpty className="px-3 py-6 text-faint">
                  <Trans>No canned replies yet. Add them in settings.</Trans>
                </CommandEmpty>
                {canned.map((r) => (
                  <CommandItem key={r.id} value={`${r.title} ${r.shortcut} ${r.id}`} onSelect={() => insertCanned(r)} className="block rounded-lg px-2.5 py-2 data-selected:bg-muted">
                    {r.title}
                    <small className="block truncate text-xs text-faint">{r.body}</small>
                  </CommandItem>
                ))}
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
        <span className="flex-1" />
        {note ? (
          <Button className={pill} onClick={addNote} disabled={!hasContent || noting} data-testid="composer-note">
            <Trans>Add note</Trans>
          </Button>
        ) : (
          <>
            <span className="me-1.5 text-xs text-faint phone:hidden">{mod}↵</span>
            <Button variant="outline" className={cn(pill, "border-border bg-card hover:border-faint")} onClick={() => send(false)} disabled={!hasContent} data-testid="composer-send">
              <Trans>Send</Trans>
            </Button>
            <Button className={pill} onClick={() => send(true)} disabled={!hasContent} data-testid="composer-send-close">
              <Trans>Send and close</Trans>
            </Button>
          </>
        )}
      </div>
    </div>
  )
})
