"use client"

import * as React from "react"
import type { IdentityTokenSource, YuvaClient } from "useyuva"
import { ArrowLeftIcon, PlusIcon } from "lucide-react"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { useYuvaChat } from "@/hooks/use-yuva-chat"
import { cn } from "@/lib/utils"
import { YuvaChatComposer } from "@/components/yuva-chat-composer"
import { YuvaChatConversations } from "@/components/yuva-chat-conversations"
import { YuvaChatEmail } from "@/components/yuva-chat-email"
import { YuvaChatThread } from "@/components/yuva-chat-thread"
import { chatErrorText, defaultYuvaChatLabels, initials, statusLine, type YuvaChatLabels } from "@/lib/yuva-chat-utils"

export interface YuvaChatProps {
  server?: string
  channel?: string
  client?: YuvaClient
  identityToken?: IdentityTokenSource | null
  helpArticleId?: string
  labels?: Partial<YuvaChatLabels>
  locale?: string
  confirmation?: React.ReactNode
  variant?: "default" | "lined"
  onCapture?: () => File | null | undefined | Promise<File | null | undefined>
  className?: string
}

function Cross({ className }: { className: string }) {
  return (
    <svg
      aria-hidden
      viewBox="0 0 13 13"
      className={cn("pointer-events-none absolute z-20 size-[13px] text-border", className)}
    >
      <path d="M6.5 0v13M0 6.5h13" stroke="currentColor" />
    </svg>
  )
}

function Crosses({ at }: { at: "top" | "bottom" }) {
  const edge = at === "top" ? "-top-[7px]" : "-bottom-[7px]"
  return (
    <>
      <Cross className={cn(edge, "start-3.5")} />
      <Cross className={cn(edge, "end-3.5")} />
    </>
  )
}

export function YuvaChat({
  server,
  channel,
  client,
  identityToken,
  helpArticleId,
  labels: custom,
  locale,
  confirmation,
  variant = "default",
  onCapture,
  className,
}: YuvaChatProps) {
  const lined = variant === "lined"
  const labels = React.useMemo(() => ({ ...defaultYuvaChatLabels, ...custom }), [custom])
  const chat = useYuvaChat({ server, channel, client, identityToken, helpArticleId })
  const [view, setView] = React.useState<"list" | "thread">("thread")
  const decided = React.useRef(false)
  const { loading, conversations, open, removed } = chat

  React.useEffect(() => {
    if (loading || decided.current) return
    decided.current = true
    const latest = conversations[0]
    if (latest && latest.status !== "closed") void open(latest.id)
    // oxlint-disable-next-line react/set-state-in-effect -- the first view depends on what the client loaded
    else setView(latest ? "list" : "thread")
  }, [loading, conversations, open])

  React.useEffect(() => {
    // oxlint-disable-next-line react/set-state-in-effect -- the open thread was removed, so the view follows the client
    if (removed) setView(conversations.length > 0 ? "list" : "thread")
  }, [removed, conversations.length])

  const inbox = chat.inbox
  const presence = inbox?.presence
  const status = statusLine(inbox, labels)
  const name = inbox?.name ?? labels.title

  return (
    <section
      aria-label={labels.title}
      className={cn(
        "relative flex h-[36rem] max-h-[80dvh] w-full max-w-md flex-col overflow-hidden",
        lined ? "bg-background text-foreground" : "rounded-xl border bg-card text-card-foreground",
        className
      )}
    >
      {lined && (
        <>
          <span aria-hidden className="pointer-events-none absolute inset-y-0 start-5 z-10 w-px bg-border" />
          <span aria-hidden className="pointer-events-none absolute inset-y-0 end-5 z-10 w-px bg-border" />
        </>
      )}
      <header
        className={cn(
          "relative z-20 flex h-16 flex-none items-center gap-1.5 border-b bg-inherit",
          lined ? "ps-5 pe-8" : "px-3"
        )}
      >
        {view === "thread" && conversations.length > 0 && (
          <Button variant="ghost" size="icon" aria-label={labels.back} onClick={() => setView("list")}>
            <ArrowLeftIcon className="rtl:rotate-180" />
          </Button>
        )}
        {view === "thread" && (
          <div className="relative ms-1 flex-none">
            <Avatar>
              {inbox?.branding.logo_url && <AvatarImage src={inbox.branding.logo_url} alt="" />}
              <AvatarFallback className="bg-accent font-semibold text-primary">
                {presence?.members[0]?.initials || initials(name)}
              </AvatarFallback>
            </Avatar>
            {presence && (
              <span
                role="img"
                aria-label={presence.available ? labels.online : labels.away}
                className={cn(
                  "absolute -end-px -bottom-px size-2.5 rounded-full border-2 border-background",
                  presence.available ? "bg-primary" : "bg-muted-foreground/50"
                )}
              />
            )}
          </div>
        )}
        <div className="ms-1.5 flex min-w-0 flex-1 flex-col">
          <h2 className="truncate text-sm leading-tight font-semibold">
            {view === "list" ? labels.conversations : name}
          </h2>
          {view === "thread" && status && <p className="truncate text-xs text-muted-foreground">{status}</p>}
        </div>
        {view === "list" && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              void chat.open(null)
              setView("thread")
            }}
          >
            <PlusIcon />
            {labels.newConversation}
          </Button>
        )}
        {lined && <Crosses at="bottom" />}
      </header>
      {chat.connection === "reconnecting" && (
        <p role="status" className="flex items-center gap-2 border-b bg-muted px-3 py-1 text-xs text-muted-foreground">
          <Spinner className="size-3" />
          {labels.reconnecting}
        </p>
      )}
      {removed && (
        <p role="status" className="border-b bg-muted px-3 py-1 text-xs text-muted-foreground">
          {labels.conversationRemoved}
        </p>
      )}
      {loading ? (
        <div className="flex flex-1 items-center justify-center">
          <Spinner aria-label={labels.loading} />
        </div>
      ) : chat.error !== null ? (
        <div role="alert" className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center text-sm">
          <p>{chat.error ? chatErrorText(chat.error, labels) : labels.loadError}</p>
        </div>
      ) : view === "list" ? (
        <YuvaChatConversations chat={chat} labels={labels} locale={locale} onOpen={() => setView("thread")} />
      ) : (
        <>
          <YuvaChatThread chat={chat} labels={labels} locale={locale} greeting={inbox?.chat.greeting} lined={lined} />
          <div className={cn("relative border-t bg-inherit", lined ? "px-8 pt-3 pb-3.5" : "p-3")}>
            {lined && <Crosses at="top" />}
            {confirmation !== undefined ? confirmation : <YuvaChatEmail chat={chat} labels={labels} className="mb-3" />}
            <YuvaChatComposer
              chat={chat}
              labels={labels}
              locale={locale}
              disabled={chat.threadLoading}
              onCapture={onCapture}
            />
          </div>
        </>
      )}
    </section>
  )
}
