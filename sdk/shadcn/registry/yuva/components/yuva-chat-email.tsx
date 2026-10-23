"use client"

import * as React from "react"
import { CheckIcon, CircleAlertIcon, MailIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { YuvaChatState } from "@/hooks/use-yuva-chat"
import { cn } from "@/lib/utils"
import { chatErrorText, defaultYuvaChatLabels, withAddress, type YuvaChatLabels } from "@/lib/yuva-chat-utils"

type Mode = "ask" | "sent" | "bad" | "limited" | "confirmed"

export interface YuvaChatEmailProps {
  chat: YuvaChatState
  labels?: Partial<YuvaChatLabels>
  className?: string
}

function Address({ template, address }: { template: string; address: string }) {
  return (
    <>
      {withAddress(template, address).map((part, index) =>
        typeof part === "string" ? (
          <React.Fragment key={index}>{part}</React.Fragment>
        ) : (
          <b key={index} className="font-medium text-foreground">
            {part.address}
          </b>
        )
      )}
    </>
  )
}

export function YuvaChatEmail({ chat, labels: custom, className }: YuvaChatEmailProps) {
  const labels = React.useMemo(() => ({ ...defaultYuvaChatLabels, ...custom }), [custom])
  const { contact, inbox } = chat
  const [editing, setEditing] = React.useState(false)
  const [busy, setBusy] = React.useState(false)
  const [error, setError] = React.useState<unknown>(null)
  const [resent, setResent] = React.useState(false)
  const [draft, setDraft] = React.useState<string | null>(null)
  const [now, setNow] = React.useState(() => Date.now())

  const confirmation = contact?.email_confirmation
  const expiresAt =
    confirmation?.state === "sent" && confirmation.expires_at ? Date.parse(confirmation.expires_at) : null
  React.useEffect(() => {
    if (expiresAt === null || expiresAt <= Date.now()) return
    const timer = setTimeout(() => setNow(Date.now()), Math.min(expiresAt - Date.now() + 100, 2 ** 31 - 1))
    return () => clearTimeout(timer)
  }, [expiresAt])

  const started = chat.activeId !== null || chat.messages.length > 0
  const verified = !!contact?.email
  const live = confirmation?.state === "sent" && (expiresAt === null || expiresAt > now)
  const nobody = !!inbox && inbox.mode === "live" && !inbox.presence?.available
  let mode: Mode | null = null
  if (started && contact) {
    if (confirmation?.state === "undeliverable" && !editing) mode = "bad"
    else if (confirmation?.state === "limited" && !editing) mode = "limited"
    else if (live && !editing) mode = "sent"
    else if (confirmation?.state === "confirmed" && !editing) mode = "confirmed"
    else if (editing || (confirmation?.state === "sent" && !verified)) mode = "ask"
    else if (inbox?.chat.ask_email_offline && nobody && !verified) mode = "ask"
  }
  if (!mode || !contact) return null

  const address = confirmation?.email ?? contact.typed_email ?? contact.email ?? ""

  const save = async (email: string) => {
    setBusy(true)
    setError(null)
    try {
      await chat.saveEmail(email)
      setEditing(false)
      setDraft(null)
    } catch (failure) {
      setError(failure)
    } finally {
      setBusy(false)
    }
  }

  const form = (
    <form
      noValidate
      className="flex gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        const value = (draft ?? address).trim()
        if (value) void save(value)
      }}
    >
      <Input
        type="email"
        autoComplete="email"
        aria-label={labels.emailLabel}
        placeholder={labels.emailPlaceholder}
        value={draft ?? address}
        onChange={(event) => setDraft(event.target.value)}
        className="h-10 min-w-0 flex-1"
      />
      <Button type="submit" size="lg" className="h-10 px-3.5" disabled={busy}>
        {labels.emailSave}
      </Button>
    </form>
  )
  const failure = error !== null && (
    <p role="alert" className="text-xs text-destructive">
      {chatErrorText(error, labels)}
    </p>
  )

  if (mode === "confirmed") {
    return (
      <p className={cn("flex items-center gap-2 text-[0.8125rem] text-muted-foreground", className)}>
        <CheckIcon className="size-4 shrink-0 text-primary" aria-hidden />
        <span>
          <Address template={labels.emailConfirmed} address={address} />
        </span>
      </p>
    )
  }
  if (mode === "limited") {
    return (
      <div
        role="status"
        className={cn("flex items-start gap-3 rounded-xl border border-dashed border-input p-3", className)}
      >
        <MailIcon className="mt-0.5 size-[1.125rem] shrink-0 text-primary" aria-hidden />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="font-medium">{labels.emailLimited}</span>
          <div className="mt-0.5 flex items-center gap-4 text-[0.8125rem]">
            <button
              type="button"
              className="min-h-7 cursor-pointer font-medium text-primary hover:underline"
              onClick={() => {
                setDraft(null)
                setEditing(true)
              }}
            >
              {labels.emailChange}
            </button>
            <button
              type="button"
              disabled={busy}
              className="min-h-7 cursor-pointer text-muted-foreground hover:text-foreground disabled:opacity-50"
              onClick={() => void save(address)}
            >
              {labels.emailResend}
            </button>
          </div>
          {failure}
        </div>
      </div>
    )
  }
  if (mode === "sent") {
    return (
      <div
        role="status"
        className={cn("flex items-start gap-3 rounded-xl border border-dashed border-input p-3", className)}
      >
        <MailIcon className="mt-0.5 size-[1.125rem] shrink-0 text-primary" aria-hidden />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="font-medium">{labels.emailSentTitle}</span>
          <span className="text-[0.8125rem] text-muted-foreground">
            <Address template={labels.emailSent} address={address} />
          </span>
          <div className="mt-0.5 flex items-center gap-4 text-[0.8125rem]">
            <button
              type="button"
              className="min-h-7 cursor-pointer font-medium text-primary hover:underline"
              onClick={() => {
                setDraft(null)
                setEditing(true)
              }}
            >
              {labels.emailChange}
            </button>
            <button
              type="button"
              disabled={busy}
              className="min-h-7 cursor-pointer text-muted-foreground hover:text-foreground disabled:opacity-50"
              onClick={() => {
                void save(address).then(() => {
                  setResent(true)
                  setTimeout(() => setResent(false), 4000)
                })
              }}
            >
              {resent ? labels.emailResent : labels.emailResend}
            </button>
          </div>
          {failure}
        </div>
      </div>
    )
  }
  const bad = mode === "bad"
  return (
    <div
      role={bad ? "alert" : undefined}
      className={cn(
        "flex flex-col gap-2 rounded-xl border p-3",
        bad ? "border-primary bg-accent text-accent-foreground" : "border-input",
        className
      )}
    >
      <div className="flex items-center gap-2">
        {bad ? (
          <CircleAlertIcon className="size-[1.125rem] shrink-0 text-primary" aria-hidden />
        ) : (
          <MailIcon className="size-[1.125rem] shrink-0 text-primary" aria-hidden />
        )}
        <span className="font-medium">
          {bad ? <Address template={labels.emailUndeliverableTitle} address={address} /> : labels.emailAskTitle}
        </span>
      </div>
      <span className="text-[0.8125rem] text-muted-foreground">
        {bad ? labels.emailUndeliverableHint : labels.emailAskHint}
      </span>
      {form}
      {failure}
    </div>
  )
}
