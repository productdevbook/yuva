import { Trans, useLingui } from "@lingui/react/macro"
import { ClockIcon, PaperclipIcon, XIcon } from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { Kbd, toast } from "@/components/common"
import { AddressChips } from "@/components/common/AddressChips"
import { FromPicker, sendingAddress, useOwnSenders } from "@/components/common/FromPicker"
import { formatWhen, SendLaterMenu } from "@/components/common/SendLater"
import { alt, mod } from "@/components/common/ShortcutSheet"
import { SignatureHtml } from "@/components/common/SignatureHtml"
import { formatBytes, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { Textarea } from "@/components/ui/textarea"
import { FileThumb, MAX_FILES } from "@/features/conversation/composer/ReplyBox"
import { useSuggestions } from "@/features/conversation/composer/Recipients"
import { look } from "@/features/inbox/PageFrame"
import { mailPath } from "@/features/mail/folders"
import { useMySignatures } from "@/features/settings/signature/queries"
import { useLatest } from "@/hooks/use-latest"
import { useIsPhone } from "@/hooks/use-media-query"
import { api, ApiError, unwrap, type DraftItem } from "@/lib/api"
import { cn, newId } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

const MAX_RECIPIENTS = 50
const EMAIL = /[^\s@,;<>"()]+@[^\s@,;<>"()]+\.[^\s@,;<>"()]+/
const AUTOSAVE_MS = 1500

type Draft = {
  inboxId: string
  to: string[]
  cc: string[]
  bcc: string[]
  subject: string
  body: string
  files: File[]
  signature: boolean
  clientId: string
  channelId?: string
}

function post({ files, ...d }: Draft) {
  const body = {
    inbox_id: d.inboxId,
    to: d.to,
    cc: d.cc,
    bcc: d.bcc,
    subject: d.subject.trim(),
    body: d.body.trim() || undefined,
    signature: d.signature ? undefined : false,
    draft: true,
    client_id: d.clientId,
    channel_id: d.channelId,
  }
  if (files.length === 0) return unwrap(api.POST("/v1/conversations/mail", { body }))
  return unwrap(
    api.POST("/v1/conversations/mail", {
      body: { ...body, files: [] },
      bodySerializer: () => {
        const form = new FormData()
        for (const [k, v] of Object.entries(body)) {
          if (Array.isArray(v)) for (const x of v) form.append(k, String(x))
          else if (v !== undefined) form.append(k, String(v))
        }
        for (const f of files) form.append("files", f, f.name)
        return form
      },
    })
  )
}

function failedAddress(error: unknown) {
  if (!(error instanceof ApiError) || error.code !== "email_undeliverable") return undefined
  return EMAIL.exec(error.detail ?? "")?.[0].toLowerCase()
}

function ComposeError({ error }: { error: unknown }) {
  const text = useErrorText()
  const address = failedAddress(error)
  if (!error) return null
  if (address)
    return (
      <p role="alert" className="text-caption text-destructive" data-testid="compose-error">
        <Trans>{address} bounced or complained before. Remove it, or clear it on its contact to write to it.</Trans>
      </p>
    )
  const detail =
    error instanceof ApiError && (error.code === "validation_failed" || error.code === "from_not_allowed")
      ? error.detail
      : undefined
  return (
    <p role="alert" className="text-caption text-destructive" data-testid="compose-error">
      {detail ?? text(error)}
    </p>
  )
}

const toggleClass =
  "rounded-md px-1.5 py-0.5 text-caption text-faint outline-none transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"

function Row({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-2 px-3">
      <span className="w-14 shrink-0 pt-1.5 text-caption text-faint">{label}</span>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}

type Saved = {
  id: string
  conversationId: string
  inboxId: string
  structure: string
  fields: string
  body: string
}

type SaveState = "idle" | "saving" | "saved" | "failed"

export function Compose({ box, onClose, draft }: { box: string; onClose: () => void; draft?: DraftItem }) {
  const { t, i18n } = useLingui()
  const errorText = useErrorText()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const inboxes = useInboxes().data
  const channels = useChannelMap()
  const reopened = !!draft
  const options = useMemo(() => {
    const withEmail = new Set([...channels.values()].filter((c) => c.kind === "email").map((c) => c.inbox_id))
    const usable = (inboxes ?? []).filter((i) => withEmail.has(i.id))
    return [...usable.filter((i) => i.purpose === "mail"), ...usable.filter((i) => i.purpose !== "mail")]
  }, [inboxes, channels])
  const [picked, setPicked] = useState<string | undefined>(draft?.conversation.inbox_id)
  const inboxId = picked ?? (options.find((i) => i.id === box) ?? options[0])?.id
  const [pickedFrom, setPickedFrom] = useState("")
  const inboxChannels = useMemo(
    () => [...channels.values()].filter((c) => c.kind === "email" && c.inbox_id === inboxId),
    [channels, inboxId]
  )
  const senders = useOwnSenders(inboxChannels.map((c) => c.id))
  const sender = senders.find((a) => a.address === pickedFrom)
  const senderChannel = sender && inboxChannels.find((c) => sender.usable_channel_ids.includes(c.id))
  const [to, setTo] = useState<string[]>(
    () => draft?.draft.recipients?.to ?? (draft?.conversation.contact.email ? [draft.conversation.contact.email] : [])
  )
  const [cc, setCc] = useState<string[]>(() => draft?.draft.recipients?.cc ?? [])
  const [bcc, setBcc] = useState<string[]>(() => draft?.draft.recipients?.bcc ?? [])
  const [ccOpen, setCcOpen] = useState(() => !!draft?.draft.recipients?.cc.length)
  const [bccOpen, setBccOpen] = useState(() => !!draft?.draft.recipients?.bcc.length)
  const [subject, setSubject] = useState(draft?.conversation.subject ?? "")
  const [body, setBody] = useState(draft?.draft.body ?? "")
  const [files, setFiles] = useState<File[]>([])
  const [noSignature, setNoSignature] = useState(false)
  const [asking, setAsking] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [state, setState] = useState<SaveState>(reopened ? "saved" : "idle")
  const [laterOpen, setLaterOpen] = useState(false)
  const toHints = useSuggestions()
  const ccHints = useSuggestions()
  const bccHints = useSuggestions()
  const fileInput = useRef<HTMLInputElement>(null)
  const toInput = useRef<HTMLInputElement>(null)
  const signature = useMySignatures().data?.find((s) => s.inbox_id === inboxId)?.html
  const kept = draft?.draft.attachments ?? []

  const saved = useRef<Saved | null>(
    draft
      ? {
          id: draft.draft.id,
          conversationId: draft.conversation.id,
          inboxId: draft.conversation.inbox_id,
          structure: JSON.stringify([draft.conversation.inbox_id, false, []]),
          fields: JSON.stringify([
            draft.draft.recipients?.to ?? [],
            draft.draft.recipients?.cc ?? [],
            draft.draft.recipients?.bcc ?? [],
            draft.conversation.subject.trim(),
          ]),
          body: draft.draft.body.trim(),
        }
      : null
  )
  const chain = useRef<Promise<unknown>>(Promise.resolve())
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const hasContent =
    to.length + cc.length + bcc.length > 0 || subject.trim() !== "" || body.trim() !== "" || files.length > 0
  const ready = !!inboxId && to.length > 0 && subject.trim() !== ""
  const saveable = ready && (body.trim() !== "" || files.length > 0 || kept.length > 0)
  const canSend = saveable && !busy
  const structure = JSON.stringify([inboxId, noSignature, files.map((f) => `${f.name}:${f.size}:${f.lastModified}`)])
  const fields = JSON.stringify([to, cc, bcc, subject.trim()])
  const latest = useLatest({
    inboxId,
    channelId: senderChannel?.id,
    to,
    cc,
    bcc,
    subject,
    body,
    files,
    noSignature,
    structure,
    fields,
    saveable,
  })

  const run = useCallback(async () => {
    const cur = latest.current
    if (!cur.saveable || !cur.inboxId) return
    const text = cur.body.trim()
    const rec = saved.current
    if (rec && rec.structure === cur.structure) {
      if (rec.body === text && rec.fields === cur.fields) return
      await unwrap(
        api.PATCH("/v1/messages/{messageId}", {
          params: { path: { messageId: rec.id } },
          body: { body: text, to: cur.to, cc: cur.cc, bcc: cur.bcc, subject: cur.subject.trim() },
        })
      )
      saved.current = { ...rec, body: text, fields: cur.fields }
      return
    }
    const made = await post({
      inboxId: cur.inboxId,
      to: cur.to,
      cc: cur.cc,
      bcc: cur.bcc,
      subject: cur.subject,
      body: cur.body,
      files: cur.files,
      signature: !cur.noSignature,
      clientId: newId(),
      channelId: cur.channelId,
    })
    saved.current = {
      id: made.message.id,
      conversationId: made.conversation.id,
      inboxId: cur.inboxId,
      structure: cur.structure,
      fields: cur.fields,
      body: text,
    }
    if (rec) await api.DELETE("/v1/messages/{messageId}", { params: { path: { messageId: rec.id } } })
  }, [latest])
  const save = useCallback(() => {
    setState("saving")
    const next = chain.current
      .catch(() => undefined)
      .then(run)
      .then(
        () => setState(saved.current ? "saved" : "idle"),
        (e: unknown) => {
          setState("failed")
          throw e
        }
      )
    chain.current = next
    return next
  }, [run, setState])

  useEffect(() => toInput.current?.focus(), [])
  useEffect(() => {
    if (!saveable) return
    const rec = saved.current
    if (rec && rec.structure === structure && rec.fields === fields && rec.body === body.trim()) return
    timer.current = setTimeout(() => {
      timer.current = undefined
      void save().catch(() => undefined)
    }, AUTOSAVE_MS)
    return () => clearTimeout(timer.current)
  }, [saveable, structure, fields, body, save])

  const go = async (at?: Date) => {
    if (!canSend) return
    setBusy(true)
    setError(null)
    try {
      clearTimeout(timer.current)
      timer.current = undefined
      await save()
      const rec = saved.current
      if (!rec) return
      const sent = await unwrap(
        api.POST("/v1/messages/{messageId}/send", {
          params: { path: { messageId: rec.id } },
          body: { from_address: sender?.address, send_at: at?.toISOString() },
        })
      )
      saved.current = null
      onClose()
      if (at) {
        const when = formatWhen(at, i18n.locale)
        toast(t`Scheduled for ${when}`, () =>
          unwrap(api.DELETE("/v1/messages/{messageId}/schedule", { params: { path: { messageId: sent.id } } })).then(
            () => toast(t`Schedule cancelled`),
            (e: unknown) => toast(errorText(e))
          )
        )
      } else toast(t`Mail sent`, () => navigate(mailPath(rec.inboxId, "sent", rec.conversationId)), t`View`)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const discard = async () => {
    clearTimeout(timer.current)
    timer.current = undefined
    setBusy(true)
    try {
      await chain.current.catch(() => undefined)
      const rec = saved.current
      if (rec) await api.DELETE("/v1/messages/{messageId}", { params: { path: { messageId: rec.id } } })
      saved.current = null
      onClose()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const close = async () => {
    if (busy) return
    setBusy(true)
    setError(null)
    try {
      clearTimeout(timer.current)
      timer.current = undefined
      if (saveable) await save()
      if (saved.current) {
        onClose()
        toast(t`Draft saved`)
      } else if (hasContent && !asking) setAsking(true)
      else onClose()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  const flagged = failedAddress(error)
  const addFiles = (more: File[]) => setFiles((f) => [...f, ...more].slice(0, MAX_FILES))
  const sendKey = `${mod}↵`
  const laterKey = `${alt}↵`
  const inboxName = options.find((i) => i.id === inboxId)?.name ?? ""

  return (
    <div
      role="dialog"
      aria-label={t`New mail`}
      className={cn(
        "fixed z-40 flex flex-col overflow-hidden border bg-card shadow-xl",
        phone
          ? "inset-0 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]"
          : "end-6 bottom-0 max-h-[min(720px,calc(100svh-96px))] w-[min(600px,calc(100vw-48px))] rounded-t-xl"
      )}
      data-testid="compose"
      onKeyDown={(e) => {
        if (e.key === "Escape" && !e.defaultPrevented) {
          e.stopPropagation()
          void close()
        } else if (e.key === "Enter" && e.altKey) {
          e.preventDefault()
          if (canSend) setLaterOpen(true)
        } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault()
          void go()
        }
      }}
    >
      <div className="flex items-center gap-2 border-b bg-surface px-3 py-2">
        <h2 className="flex-1 text-body font-medium">{reopened ? <Trans>Draft</Trans> : <Trans>New mail</Trans>}</h2>
        {state !== "idle" && (
          <span
            className={cn("text-caption", state === "failed" ? "text-destructive" : "text-faint")}
            aria-live="polite"
            data-testid="compose-saved"
          >
            {state === "saving" ? (
              <Trans>Saving…</Trans>
            ) : state === "saved" ? (
              <Trans>Saved</Trans>
            ) : (
              <Trans>Not saved</Trans>
            )}
          </span>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t`Close`}
          onClick={() => void close()}
          data-testid="compose-close"
        >
          <XIcon />
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto py-2">
        <Row label={<Trans>From</Trans>}>
          <div className="flex flex-wrap gap-1.5">
            {reopened ? (
              <span className="min-w-40 flex-1 truncate pt-1.5 text-body" data-testid="compose-from">
                {inboxName}
              </span>
            ) : (
              <NativeSelect
                className="min-w-40 flex-1"
                size="sm"
                value={inboxId ?? ""}
                aria-label={t`From`}
                onChange={(e) => setPicked(e.target.value)}
                data-testid="compose-from"
              >
                {options.map((i) => (
                  <NativeSelectOption key={i.id} value={i.id}>
                    {i.name}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            )}
            <FromPicker
              className="min-w-40 flex-1"
              channelAddress={sendingAddress(inboxChannels[0])}
              senders={senders}
              value={pickedFrom}
              onChange={setPickedFrom}
              testId="compose-send-as"
            />
          </div>
        </Row>
        <>
          <Row label={<Trans>To</Trans>}>
            <div className="flex items-start gap-1">
              <AddressChips
                ref={toInput}
                label={t`To`}
                value={to}
                onChange={setTo}
                max={MAX_RECIPIENTS}
                flagged={flagged ? [flagged] : []}
                below
                testId="compose-to"
                className="flex-1"
                {...toHints}
              />
              {!ccOpen && (
                <button
                  type="button"
                  className={toggleClass}
                  onClick={() => setCcOpen(true)}
                  data-testid="compose-cc-toggle"
                >
                  <Trans>Cc</Trans>
                </button>
              )}
              {!bccOpen && (
                <button
                  type="button"
                  className={toggleClass}
                  onClick={() => setBccOpen(true)}
                  data-testid="compose-bcc-toggle"
                >
                  <Trans>Bcc</Trans>
                </button>
              )}
            </div>
          </Row>
          {ccOpen && (
            <Row label={<Trans>Cc</Trans>}>
              <AddressChips
                label={t`Cc`}
                value={cc}
                onChange={setCc}
                max={MAX_RECIPIENTS}
                flagged={flagged ? [flagged] : []}
                below
                testId="compose-cc"
                {...ccHints}
              />
            </Row>
          )}
          {bccOpen && (
            <Row label={<Trans>Bcc</Trans>}>
              <AddressChips
                label={t`Bcc`}
                value={bcc}
                onChange={setBcc}
                max={MAX_RECIPIENTS}
                flagged={flagged ? [flagged] : []}
                below
                testId="compose-bcc"
                {...bccHints}
              />
              <p className="pt-1 text-caption text-faint">
                <Trans>Bcc recipients get the mail but are hidden from everyone else.</Trans>
              </p>
            </Row>
          )}
          <Row label={<Trans>Subject</Trans>}>
            <Input
              value={subject}
              maxLength={500}
              aria-label={t`Subject`}
              onChange={(e) => setSubject(e.target.value)}
              className="h-8"
              data-testid="compose-subject"
            />
          </Row>
        </>
        <Textarea
          value={body}
          maxLength={65536}
          aria-label={t`Message`}
          placeholder={t`Write your mail…`}
          onChange={(e) => setBody(e.target.value)}
          onPaste={(e) => {
            if (reopened) return
            const pasted = Array.from(e.clipboardData.files)
            if (pasted.length === 0) return
            e.preventDefault()
            addFiles(pasted)
          }}
          className="min-h-40 flex-1 resize-none rounded-none border-0 bg-transparent px-4 py-2 text-reading shadow-none hover:border-0 focus-visible:ring-0 md:text-reading dark:bg-transparent"
          data-testid="compose-body"
        />
        {signature && !reopened && (
          <div className="mx-4 flex items-start gap-3 border-t pt-2" data-testid="compose-signature">
            <div className={cn("min-w-0 flex-1 opacity-70", noSignature && "line-through opacity-40")}>
              <SignatureHtml html={signature} className="max-h-24 overflow-y-auto" />
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 shrink-0 px-2 text-caption text-faint hover:bg-muted"
              aria-pressed={noSignature}
              onClick={() => setNoSignature((x) => !x)}
              data-testid="compose-signature-toggle"
            >
              {noSignature ? <Trans>Add it back</Trans> : <Trans>Leave it out</Trans>}
            </Button>
          </div>
        )}
        {(kept.length > 0 || files.length > 0) && (
          <ul className="flex flex-wrap gap-1.5 px-4">
            {kept.map((a) => (
              <li
                key={a.id}
                className="flex max-w-full items-center gap-1.5 rounded-lg border bg-background py-0.5 ps-2 pe-2 text-caption"
              >
                <PaperclipIcon className="size-3 shrink-0 text-faint" />
                <span className="max-w-48 truncate">{a.filename}</span>
                <span className="text-faint">{formatBytes(a.size, i18n.locale)}</span>
              </li>
            ))}
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
      </div>
      <div className="flex flex-col gap-1.5 border-t px-3 py-2">
        <ComposeError error={error} />
        {asking ? (
          <div className="flex flex-wrap items-center gap-2" data-testid="compose-ask">
            <span className="flex-1 text-small phone:basis-full">
              <Trans>This mail needs a recipient, a subject and some text before it can be kept as a draft.</Trans>
            </span>
            <Button variant="ghost" size="sm" onClick={() => setAsking(false)}>
              <Trans>Keep writing</Trans>
            </Button>
            <Button variant="outline" size="sm" onClick={onClose} data-testid="compose-discard">
              <Trans>Discard</Trans>
            </Button>
          </div>
        ) : (
          <div className="flex items-center gap-1.5">
            <input
              ref={fileInput}
              type="file"
              multiple
              hidden
              data-testid="compose-files"
              onChange={(e) => {
                addFiles(Array.from(e.target.files ?? []))
                e.target.value = ""
              }}
            />
            {!reopened && (
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
            )}
            <Button
              variant="ghost"
              className={cn(look.btn, "text-faint hover:bg-muted")}
              disabled={busy || !hasContent}
              onClick={() => void discard()}
              data-testid="compose-discard"
            >
              <Trans>Discard</Trans>
            </Button>
            <span className="flex-1" />
            <SendLaterMenu
              open={laterOpen}
              onOpenChange={setLaterOpen}
              trigger={
                <Button
                  variant="outline"
                  className={cn(look.btn, "w-[34px] px-0")}
                  disabled={!canSend}
                  aria-label={t`Send later`}
                  title={t`Send later (${laterKey})`}
                  data-testid="compose-send-later"
                />
              }
              onPick={(at) => void go(at)}
            >
              <ClockIcon />
            </SendLaterMenu>
            <Button className={look.btnInk} disabled={!canSend} onClick={() => void go()} data-testid="compose-send">
              <Trans>Send</Trans>
              <Kbd className="phone:hidden bg-background/15 text-background">{sendKey}</Kbd>
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
