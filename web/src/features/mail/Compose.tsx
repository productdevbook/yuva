import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { PaperclipIcon, XIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { Kbd, toast } from "@/components/common"
import { AddressChips } from "@/components/common/AddressChips"
import { mod } from "@/components/common/ShortcutSheet"
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
import { useIsPhone } from "@/hooks/use-media-query"
import { api, ApiError, unwrap } from "@/lib/api"
import { cn, newId } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

const MAX_RECIPIENTS = 50
const EMAIL = /[^\s@,;<>"()]+@[^\s@,;<>"()]+\.[^\s@,;<>"()]+/

type Draft = {
  inboxId: string
  to: string[]
  cc: string[]
  bcc: string[]
  subject: string
  body: string
  files: File[]
  signature: boolean
  draft: boolean
  clientId: string
}

function post({ draft, files, ...d }: Draft) {
  const body = {
    inbox_id: d.inboxId,
    to: d.to,
    cc: d.cc,
    bcc: d.bcc,
    subject: d.subject.trim(),
    body: d.body.trim() || undefined,
    signature: d.signature ? undefined : false,
    draft: draft || undefined,
    client_id: d.clientId,
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
  const detail = error instanceof ApiError && error.code === "validation_failed" ? error.detail : undefined
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

export function Compose({ box, onClose }: { box: string; onClose: () => void }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const inboxes = useInboxes().data
  const channels = useChannelMap()
  const options = useMemo(() => {
    const withEmail = new Set([...channels.values()].filter((c) => c.kind === "email").map((c) => c.inbox_id))
    const usable = (inboxes ?? []).filter((i) => withEmail.has(i.id))
    return [...usable.filter((i) => i.purpose === "mail"), ...usable.filter((i) => i.purpose !== "mail")]
  }, [inboxes, channels])
  const [picked, setPicked] = useState<string>()
  const inboxId = picked ?? (options.find((i) => i.id === box) ?? options[0])?.id
  const [to, setTo] = useState<string[]>([])
  const [cc, setCc] = useState<string[]>([])
  const [bcc, setBcc] = useState<string[]>([])
  const [ccOpen, setCcOpen] = useState(false)
  const [bccOpen, setBccOpen] = useState(false)
  const [subject, setSubject] = useState("")
  const [body, setBody] = useState("")
  const [files, setFiles] = useState<File[]>([])
  const [noSignature, setNoSignature] = useState(false)
  const [asking, setAsking] = useState(false)
  const toHints = useSuggestions()
  const ccHints = useSuggestions()
  const bccHints = useSuggestions()
  const clientId = useRef(newId())
  const fileInput = useRef<HTMLInputElement>(null)
  const toInput = useRef<HTMLInputElement>(null)
  const signature = useMySignatures().data?.find((s) => s.inbox_id === inboxId)?.html

  const submit = useMutation({ mutationFn: post })
  const pending = submit.isPending
  const hasContent =
    to.length + cc.length + bcc.length > 0 || subject.trim() !== "" || body.trim() !== "" || files.length > 0
  const ready = !!inboxId && to.length > 0 && subject.trim() !== "" && !pending
  const canSend = ready && (body.trim() !== "" || files.length > 0)

  useEffect(() => toInput.current?.focus(), [])

  const run = (draft: boolean) => {
    if (!inboxId || (draft ? !ready : !canSend)) return
    submit.mutate(
      { inboxId, to, cc, bcc, subject, body, files, signature: !noSignature, draft, clientId: clientId.current },
      {
        onSuccess: (data) => {
          onClose()
          if (draft) toast(t`Draft saved`)
          else
            toast(
              t`Mail sent`,
              () => navigate(mailPath(data.conversation.inbox_id, "sent", data.conversation.id)),
              t`View`
            )
        },
      }
    )
  }
  const close = () => {
    if (pending) return
    if (hasContent && !asking) setAsking(true)
    else onClose()
  }

  const flagged = failedAddress(submit.error)
  const addFiles = (more: File[]) => setFiles((f) => [...f, ...more].slice(0, MAX_FILES))
  const sendKey = `${mod}↵`

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
          close()
        } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault()
          run(false)
        }
      }}
    >
      <div className="flex items-center gap-2 border-b bg-surface px-3 py-2">
        <h2 className="flex-1 text-body font-medium">
          <Trans>New mail</Trans>
        </h2>
        <Button variant="ghost" size="icon-sm" aria-label={t`Close`} onClick={close} data-testid="compose-close">
          <XIcon />
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto py-2">
        <Row label={<Trans>From</Trans>}>
          <NativeSelect
            className="w-full"
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
        </Row>
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
        <Textarea
          value={body}
          maxLength={65536}
          aria-label={t`Message`}
          placeholder={t`Write your mail…`}
          onChange={(e) => setBody(e.target.value)}
          onPaste={(e) => {
            const pasted = Array.from(e.clipboardData.files)
            if (pasted.length === 0) return
            e.preventDefault()
            addFiles(pasted)
          }}
          className="min-h-40 flex-1 resize-none rounded-none border-0 bg-transparent px-4 py-2 text-reading shadow-none hover:border-0 focus-visible:ring-0 md:text-reading dark:bg-transparent"
          data-testid="compose-body"
        />
        {signature && (
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
        {files.length > 0 && (
          <ul className="flex flex-wrap gap-1.5 px-4">
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
        <ComposeError error={submit.error} />
        {asking ? (
          <div className="flex flex-wrap items-center gap-2" data-testid="compose-ask">
            <span className="flex-1 text-small phone:basis-full">
              <Trans>Keep this mail as a draft?</Trans>
            </span>
            <Button variant="ghost" size="sm" onClick={() => setAsking(false)}>
              <Trans>Keep writing</Trans>
            </Button>
            <Button variant="outline" size="sm" onClick={onClose} data-testid="compose-discard">
              <Trans>Discard</Trans>
            </Button>
            <Button size="sm" disabled={!ready} onClick={() => run(true)} data-testid="compose-ask-save">
              <Trans>Save draft</Trans>
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
            <span className="flex-1" />
            <Button
              variant="outline"
              className={look.btn}
              disabled={!ready}
              onClick={() => run(true)}
              data-testid="compose-draft"
            >
              <Trans>Save draft</Trans>
            </Button>
            <Button className={look.btnInk} disabled={!canSend} onClick={() => run(false)} data-testid="compose-send">
              <Trans>Send</Trans>
              <Kbd className="phone:hidden bg-background/15 text-background">{sendKey}</Kbd>
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
