import { Trans, useLingui } from "@lingui/react/macro"
import { LifeBuoyIcon, PaperclipIcon, SlashIcon, SmileIcon, SparklesIcon, XIcon } from "lucide-react"
import { forwardRef, lazy, Suspense, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react"

import { BotAvatar, ErrorLine, Kbd, toast } from "@/components/common"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatBytes } from "@/components/common/text"
import { popupClass } from "@/components/ui/dropdown-menu"
import { firstName, readDraft, writeDraft, type ConversationActions } from "@/features/conversation/actions"
import { ArticlePicker, type HelpPick } from "@/features/conversation/composer/ArticlePicker"
import {
  CannedMenu,
  slashToken,
  useCannedMatches,
  useReplyChoices,
  type ReplyChoice,
} from "@/features/conversation/composer/CannedMenu"
import { Recipients } from "@/features/conversation/composer/Recipients"
import { MacroChips } from "@/features/conversation/composer/MacroChips"
import type { MacroPick } from "@/features/conversation/macros"
import { MentionMenu, mentionToken } from "@/features/conversation/composer/MentionMenu"
import { useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { useDraftActions, useParticipants } from "@/features/conversation/queries"
import { useHelpCenters } from "@/features/help/queries"
import type { Conversation, Member, Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useTypingSender } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useAssignableMembers } from "@/lib/workspace"
import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { look } from "@/features/inbox/PageFrame"

const EmojiPanel = lazy(() => import("@/features/conversation/composer/EmojiPanel"))

export type ReplyHandle = {
  focus: (mode?: "message" | "note") => void
  attach: () => void
  discardSuggestion: () => void
  quote: (text: string) => void
  article: () => void
  addFiles: (files: File[]) => void
  insert: (choice: ReplyChoice) => void
  recipients: () => void
}

const MAX_FILES = 10

function FileThumb({ file }: { file: File }) {
  const preview = useCallback(
    (img: HTMLImageElement | null) => {
      if (!img) return
      const url = URL.createObjectURL(file)
      img.src = url
      return () => URL.revokeObjectURL(url)
    },
    [file]
  )
  if (!file.type.startsWith("image/") || file.type === "image/svg+xml")
    return <PaperclipIcon className="size-3 shrink-0 text-faint" />
  return <img ref={preview} alt="" className="size-6 shrink-0 rounded object-cover" />
}

function Suggestion({
  m,
  ctx,
  onUse,
  onDiscard,
}: {
  m: Message
  ctx: ThreadContext
  onUse: () => void
  onDiscard: () => void
}) {
  const author = useAuthorName(m, ctx)
  return (
    <div
      className="mx-2.5 mb-2 flex flex-col gap-2 rounded-xl border border-dashed border-brand/50 bg-brand-wash/60 px-3 py-2.5 text-small"
      data-testid="suggestion"
    >
      <span className="flex items-center gap-1.5 text-caption font-semibold text-brand">
        {m.author.type === "bot" ? (
          <BotAvatar name={author} url={m.author.avatar_url} className="size-4" />
        ) : (
          <SparklesIcon className="size-3.5 shrink-0" />
        )}
        <Trans>Draft by {author}</Trans>
      </span>
      <span className="line-clamp-3 whitespace-pre-wrap text-foreground">{m.body}</span>
      <span className="flex items-center gap-2">
        <Button size="sm" className="h-[30px] rounded-lg px-3" onClick={onUse} data-testid="suggestion-use">
          <Trans>Use this reply</Trans>
          <Kbd className="bg-card text-foreground">Tab</Kbd>
        </Button>
        <Button variant="outline" size="sm" className={look.btnSm} onClick={onDiscard} data-testid="suggestion-discard">
          <Trans>Discard</Trans>
        </Button>
      </span>
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
    actions: ConversationActions
    autoFocus: boolean
    className?: string
  }
>(function ReplyBox(
  { c, contactName, via, emailTo, undeliverable, suggestion, ctx, actions, autoFocus, className },
  ref
) {
  const { t, i18n } = useLingui()
  const [initial] = useState(() => readDraft(c.id))
  const [mode, setMode] = useState(initial.mode)
  const [body, setBody] = useState(initial.body)
  const [files, setFiles] = useState<File[]>(initial.files)
  const [articles, setArticles] = useState<HelpPick[]>(initial.articles)
  const [macro, setMacro] = useState<MacroPick | undefined>(initial.macro)
  const [articleOpen, setArticleOpen] = useState(false)
  const [linked, setLinked] = useState<string | null>(null)
  const [caret, setCaret] = useState(0)
  const [pick, setPick] = useState(0)
  const [dismissed, setDismissed] = useState<number | null>(null)
  const [cannedOpen, setCannedOpen] = useState(false)
  const [emojiOpen, setEmojiOpen] = useState(false)
  const [noting, setNoting] = useState(false)
  const [mentioned, setMentioned] = useState<{ id: string; label: string }[]>([])
  const [error, setError] = useState<unknown>(null)
  const [ccEdit, setCcEdit] = useState<string[] | undefined>(initial.cc)
  const [bcc, setBcc] = useState<string[]>(initial.bcc ?? [])
  const [ccOpen, setCcOpen] = useState(false)
  const [bccOpen, setBccOpen] = useState(false)
  const ccInput = useRef<HTMLInputElement>(null)
  const bccInput = useRef<HTMLInputElement>(null)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const typing = useTypingSender(c.id)
  const { discard } = useDraftActions()
  const { membership } = useSession()
  const choices = useReplyChoices()
  const helpCenter = (useHelpCenters().data ?? []).find((h) => h.inbox_id === c.inbox_id)
  const note = mode === "note"
  const participants = useParticipants(c.id, !!emailTo)
  const defaultCc = (participants.data?.items ?? []).filter((p) => p.status === "active").map((p) => p.email)
  const cc = ccEdit ?? defaultCc
  const blocked = !note && !!undeliverable
  const first = firstName(contactName)
  const articleKey = keyLabel(SHORTCUTS.article).join("")

  useEffect(
    () => writeDraft(c.id, { body, mode, files, articles, macro, cc: ccEdit, bcc }),
    [c.id, body, mode, files, articles, macro, ccEdit, bcc]
  )
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
  const showSuggestion = !!suggestion && !note && !blocked && linked !== suggestion.id
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
  const quote = (text: string) => {
    const next = body.trim() ? `${text}\n\n${body}` : `${text}\n\n`
    setBody(next)
    setCaret(next.length)
    requestAnimationFrame(() => {
      const el = textarea.current
      if (!el) return
      el.focus()
      el.setSelectionRange(next.length, next.length)
    })
  }

  const at = note ? mentionToken(body, caret) : null
  const people = useAssignableMembers(c.inbox_id).filter((m) => m.id !== membership.member_id)
  const query = at?.query.toLocaleLowerCase() ?? ""
  const mentionMatches = at
    ? people
        .filter(
          (m) =>
            (m.name || m.email)
              .toLocaleLowerCase()
              .split(/\s+/)
              .some((w) => w.startsWith(query)) || m.email.startsWith(query)
        )
        .slice(0, 6)
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
  const choose = (r: ReplyChoice) => {
    setMacro(r.kind === "macro" ? { macro: r.macro, dropped: [] } : macro)
    return r.body ?? ""
  }
  const insertSlash = (r: ReplyChoice) => token && insertAt(choose(r), token.start, caret)
  const insertMention = (m: Member) => {
    if (!at) return
    const label = m.name || m.email
    setMentioned((all) => (all.some((x) => x.id === m.id) ? all : [...all, { id: m.id, label }]))
    insertAt(`@${label} `, at.start, caret)
  }
  const insertEmoji = (emoji: string) => {
    setEmojiOpen(false)
    const start = textarea.current?.selectionStart ?? body.length
    const end = textarea.current?.selectionEnd ?? start
    insertAt(emoji, start, end)
  }
  const insertCanned = (r: ReplyChoice) => {
    setCannedOpen(false)
    const at = textarea.current?.selectionStart ?? body.length
    const text = choose(r)
    const sep = text && at > 0 && !/\s$/.test(body.slice(0, at)) ? "\n" : ""
    insertAt(sep + text, at, at)
  }

  const addFiles = (more: File[]) => setFiles((f) => [...f, ...more].slice(0, MAX_FILES))
  useImperativeHandle(ref, () => ({
    focus,
    attach: () => fileInput.current?.click(),
    discardSuggestion: dropSuggestion,
    quote,
    article: () => {
      if (helpCenter && !note) setArticleOpen(true)
    },
    addFiles,
    insert: (r) => {
      setMode("message")
      insertCanned(r)
    },
    recipients: () => {
      if (!emailTo || note || blocked) return
      setCcOpen(true)
      requestAnimationFrame(() => ccInput.current?.focus())
    },
  }))
  const hasContent = body.trim() !== "" || files.length > 0 || articles.length > 0
  const notified = mentioned.filter((x) => body.includes(`@${x.label}`))
  const names = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(
    notified.map((x) => firstName(x.label))
  )
  const reset = () => {
    setBody("")
    setFiles([])
    setArticles([])
    setMacro(undefined)
    setCaret(0)
    setLinked(null)
    setError(null)
    setCcEdit(undefined)
    setBcc([])
    setCcOpen(false)
    setBccOpen(false)
  }
  const send = (close: boolean) => {
    if (!hasContent || note || blocked) return
    typing.stop()
    const linkedDraft = suggestion && linked === suggestion.id ? suggestion : undefined
    const saved = { body, files, articles, macro }
    const recipients = emailTo ? { cc: ccEdit, bcc: bcc.length ? bcc : undefined } : {}
    actions.send({ body, files, close, suggestion: linkedDraft, articles, macro, ...recipients })?.catch(() => {
      setBody((b) => b || saved.body)
      setFiles((f) => (f.length ? f : saved.files))
      setArticles((a) => (a.length ? a : saved.articles))
      setMacro((m) => m ?? saved.macro)
    })
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
        tagged.map((x) => x.id)
      )
      .then(() => {
        setMode("message")
        setMentioned([])
        if (tagged.length) {
          const names = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(
            tagged.map((x) => firstName(x.label))
          )
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
    if (e.key === "Enter" && actions.chat && !e.shiftKey && !e.altKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      if (note) addNote()
      else send(false)
      return
    }
    if (e.key === "Escape") {
      if (cannedOpen) setCannedOpen(false)
      else e.currentTarget.blur()
    }
  }

  const recipientsShown = !!emailTo && !note && !blocked
  const hint = note
    ? t`The customer does not see this`
    : emailTo
      ? recipientsShown
        ? ""
        : t`To: ${emailTo}`
      : via
        ? t`Goes out through ${via}`
        : ""

  return (
    <div
      className={cn(
        "@container/composer relative rounded-xl border bg-card transition-colors focus-within:border-brand/45",
        note && "border-note-border bg-note focus-within:border-note-ink/40",
        className
      )}
      data-testid="reply-box"
    >
      <div className="flex items-center gap-0.5 px-2 pt-2">
        <Tabs value={mode} onValueChange={(v) => focus(v as "message" | "note")} className="gap-0">
          <TabsList aria-label={t`Message type`}>
            {(["message", "note"] as const).map((m) => (
              <TabsTrigger key={m} value={m} className={cn("flex-none px-3", note && "data-active:text-note-ink")}>
                {m === "message" ? <Trans>Reply</Trans> : <Trans>Note</Trans>}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <span
          className="ms-auto min-w-0 truncate pe-2 text-caption text-faint"
          data-testid={!note && emailTo && !recipientsShown ? "composer-email-to" : undefined}
        >
          {hint}
        </span>
      </div>
      {recipientsShown && (
        <Recipients
          to={emailTo}
          cc={cc}
          onCc={setCcEdit}
          bcc={bcc}
          onBcc={setBcc}
          ccOpen={ccOpen}
          bccOpen={bccOpen}
          setCcOpen={setCcOpen}
          setBccOpen={setBccOpen}
          ccRef={ccInput}
          bccRef={bccInput}
        />
      )}
      {blocked && (
        <p
          className="mx-3 my-2 rounded-lg bg-destructive/6 px-3 py-2.5 text-small text-destructive"
          data-testid="composer-undeliverable"
        >
          <Trans>This address is undeliverable. Clear it in the contact details before you reply.</Trans>
        </p>
      )}
      {menuOpen && <CannedMenu items={matches} active={pick} onPick={insertSlash} />}
      {mentionOpen && <MentionMenu items={mentionMatches} active={pick} onPick={insertMention} />}
      {!blocked && (
        <Textarea
          ref={textarea}
          name="reply-body"
          value={body}
          rows={2}
          data-testid="composer-input"
          aria-label={note ? t`Team note` : t`Reply`}
          placeholder={note ? t`A note for the team… type @ to mention a teammate` : t`Write to ${first}…`}
          className="max-h-[40vh] min-h-[64px] resize-none rounded-none border-0 bg-transparent px-4 py-2.5 text-reading shadow-none hover:border-0 focus-visible:ring-0 md:text-reading dark:bg-transparent"
          onChange={(e) => {
            setBody(e.target.value)
            setCaret(e.target.selectionStart)
            setPick(0)
            if (note) return
            if (e.target.value.trim() === "") typing.stop()
            else typing.typed()
          }}
          onBlur={typing.stop}
          onPaste={(e) => {
            const pasted = Array.from(e.clipboardData.files)
            if (pasted.length === 0) return
            e.preventDefault()
            addFiles(pasted)
          }}
          onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
          onKeyDown={onKeyDown}
        />
      )}
      {note && notified.length > 0 && (
        <p className="px-4 pb-1 text-caption text-note-ink" data-testid="composer-notify">
          <Trans>{names} will be notified</Trans>
        </p>
      )}
      {showSuggestion && <Suggestion m={suggestion} ctx={ctx} onUse={applySuggestion} onDiscard={dropSuggestion} />}
      {files.length > 0 && (
        <ul className="flex flex-wrap gap-1.5 px-4 pb-2">
          {files.map((f, i) => (
            <li
              key={`${f.name}-${i}`}
              className="flex max-w-full items-center gap-1.5 rounded-lg border bg-background py-0.5 ps-2 pe-0.5 text-caption"
            >
              <FileThumb file={f} />
              <span className="max-w-48 truncate">{f.name}</span>
              <span className="text-faint">{formatBytes(f.size, i18n.locale)}</span>
              <Button
                variant="ghost"
                size="icon-xs"
                className="text-faint"
                aria-label={t`Remove ${f.name}`}
                onClick={() => setFiles((all) => all.filter((_, j) => j !== i))}
              >
                <XIcon />
              </Button>
            </li>
          ))}
        </ul>
      )}
      {articles.length > 0 && !note && (
        <ul className="flex flex-wrap gap-1.5 px-4 pb-2" data-testid="composer-articles">
          {articles.map((a) => (
            <li
              key={a.id}
              className="flex max-w-full items-center gap-1.5 rounded-lg border bg-background py-0.5 ps-2 pe-0.5 text-caption"
            >
              <LifeBuoyIcon className="size-3 shrink-0 text-faint" />
              <span className="max-w-56 truncate">{a.title}</span>
              <Button
                variant="ghost"
                size="icon-xs"
                className="text-faint"
                aria-label={t`Remove ${a.title}`}
                onClick={() => setArticles((all) => all.filter((x) => x.id !== a.id))}
              >
                <XIcon />
              </Button>
            </li>
          ))}
        </ul>
      )}
      {macro && !note && <MacroChips pick={macro} onChange={setMacro} />}
      <ErrorLine error={error} className="px-4 pb-2 text-caption" />
      <div className="relative flex flex-wrap items-center gap-1 px-2.5 pb-2.5">
        <input
          ref={fileInput}
          type="file"
          name="reply-files"
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
          className="text-faint hover:bg-muted"
          aria-label={t`Attach files`}
          title={t`Attach files`}
          onClick={() => fileInput.current?.click()}
        >
          <PaperclipIcon />
        </Button>
        <Popover open={emojiOpen} onOpenChange={setEmojiOpen}>
          <PopoverTrigger
            render={<Button variant="ghost" size="sm" className="text-faint hover:bg-muted aria-expanded:bg-muted" />}
            title={t`Emoji`}
            data-testid="emoji-button"
          >
            <SmileIcon />
            <span className="@max-md/composer:hidden">
              <Trans>Emoji</Trans>
            </span>
          </PopoverTrigger>
          <PopoverContent
            side="top"
            align="start"
            className={cn(popupClass, "w-[min(320px,calc(100vw-24px))] gap-0 overflow-hidden rounded-xl p-0 ring-0")}
          >
            <Suspense fallback={<div className="h-[340px]" />}>
              {emojiOpen && <EmojiPanel onPick={insertEmoji} />}
            </Suspense>
          </PopoverContent>
        </Popover>
        <Popover open={cannedOpen} onOpenChange={setCannedOpen}>
          <PopoverTrigger
            render={<Button variant="ghost" size="sm" className="text-faint hover:bg-muted aria-expanded:bg-muted" />}
            title={t`Canned replies`}
            data-testid="canned-button"
          >
            <SlashIcon />
            <span className="@max-md/composer:hidden">
              <Trans>Canned reply</Trans>
            </span>
          </PopoverTrigger>
          <PopoverContent
            side="top"
            align="start"
            className={cn(popupClass, "w-[min(320px,calc(100vw-48px))] gap-0 rounded-xl p-0 ring-0")}
          >
            <Command className="bg-transparent p-1.5" label={t`Canned replies`}>
              <CommandList className="max-h-72">
                <CommandEmpty className="px-3 py-6 text-faint">
                  <Trans>No canned replies yet. Add them in settings.</Trans>
                </CommandEmpty>
                {choices.map((r) => (
                  <CommandItem
                    key={`${r.kind}:${r.id}`}
                    value={`${r.title} ${r.shortcut} ${r.kind}:${r.id}`}
                    onSelect={() => insertCanned(r)}
                    className="block rounded-lg px-2.5 py-2 data-selected:bg-muted"
                  >
                    {r.title}
                    {r.kind === "macro" && (
                      <span className="ms-2 rounded-md bg-muted px-1.5 text-caption text-muted-foreground">
                        <Trans>Macro</Trans>
                      </span>
                    )}
                    <small className="block truncate text-caption text-faint">
                      {r.body ?? <Trans>Actions only</Trans>}
                    </small>
                  </CommandItem>
                ))}
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
        {helpCenter && !note && (
          <Popover open={articleOpen} onOpenChange={setArticleOpen}>
            <PopoverTrigger
              render={<Button variant="ghost" size="sm" className="text-faint hover:bg-muted aria-expanded:bg-muted" />}
              title={t`Send a help article (${articleKey})`}
              data-testid="article-button"
            >
              <LifeBuoyIcon />
              <span className="@max-md/composer:hidden">
                <Trans>Help article</Trans>
              </span>
            </PopoverTrigger>
            <PopoverContent
              side="top"
              align="start"
              className={cn(popupClass, "w-[min(360px,calc(100vw-48px))] gap-0 rounded-xl p-0 ring-0")}
            >
              <ArticlePicker
                center={helpCenter}
                chosen={articles}
                onPick={(a) => {
                  setArticles((all) => (all.length < 3 && !all.some((x) => x.id === a.id) ? [...all, a] : all))
                  setArticleOpen(false)
                  requestAnimationFrame(() => textarea.current?.focus())
                }}
              />
            </PopoverContent>
          </Popover>
        )}
        <span className="flex-1" />
        <span className="me-1.5 text-caption text-faint @max-md/composer:hidden">{actions.chat ? "↵" : `${mod}↵`}</span>
        {note ? (
          <Button onClick={addNote} disabled={!hasContent || noting} data-testid="composer-note">
            <Trans>Add note</Trans>
          </Button>
        ) : (
          <>
            <Button
              variant={actions.chat ? "default" : "outline"}
              onClick={() => send(false)}
              disabled={!hasContent || blocked}
              data-testid="composer-send"
            >
              <Trans>Send</Trans>
            </Button>
            <Button
              variant={actions.chat ? "outline" : "default"}
              onClick={() => send(true)}
              disabled={!hasContent || blocked}
              data-testid="composer-send-close"
            >
              {c.kind === "feedback" ? <Trans>Reply and resolve</Trans> : <Trans>Send and close</Trans>}
            </Button>
          </>
        )}
      </div>
    </div>
  )
})
