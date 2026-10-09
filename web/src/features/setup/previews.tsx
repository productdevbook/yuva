import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpIcon, CheckIcon, MessageCircleIcon, PaperclipIcon, ReplyIcon } from "lucide-react"

import { ChannelIcon } from "@/components/common"
import { TypingDots } from "@/components/common/TypingDots"
import type { SetupKind } from "@/features/setup/setup"
import type { ChatLauncherPosition, ConversationListItem } from "@/lib/api"
import { cn } from "@/lib/utils"

export const DEFAULT_COLOR = "#d4431c"
const lift = "shadow-[0_1px_2px_rgb(15_23_42/0.04),0_24px_60px_-28px_rgb(15_23_42/0.35)]"

function initial(name: string) {
  return (name.trim().charAt(0) || "Y").toLocaleUpperCase()
}

export function PreviewCaption({ children }: { children: React.ReactNode }) {
  return (
    <p className="mt-6 max-w-sm text-center text-small text-balance text-faint">
      <span className="me-2 inline-block size-1.5 rounded-full bg-brand align-middle" />
      {children}
    </p>
  )
}

export function ChatMock({
  name,
  greeting,
  color,
  waiting,
  className,
}: {
  name: string
  greeting?: string
  color: string
  waiting?: boolean
  className?: string
}) {
  const { t } = useLingui()
  return (
    <div className={cn("flex w-full max-w-[22rem] flex-col overflow-hidden rounded-2xl border bg-card", lift, className)} aria-hidden="true">
      <div className="flex items-center gap-3 px-4 py-3.5 text-white" style={{ background: color }}>
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-white/20 font-semibold">{initial(name)}</span>
        <span className="min-w-0">
          <span className="block truncate font-medium">{name.trim() || t`Your product`}</span>
          <span className="block text-caption text-white/80">
            <Trans>Usually replies within a day</Trans>
          </span>
        </span>
      </div>
      <div className="flex flex-1 flex-col gap-2.5 px-4 py-5">
        <p className="max-w-[85%] rounded-2xl rounded-ss-md bg-surface px-3.5 py-2.5 text-body">{greeting || t`Hi! How can we help?`}</p>
        {waiting ? (
          <span className="mt-1 flex items-center gap-2 text-caption text-faint">
            <TypingDots />
            <Trans>Waiting for the first message</Trans>
          </span>
        ) : (
          <p className="ms-auto max-w-[85%] rounded-2xl rounded-ee-md px-3.5 py-2.5 text-body text-white" style={{ background: color }}>
            <Trans>Is there a way to export my notes?</Trans>
          </p>
        )}
      </div>
      <div className="flex items-center gap-2 border-t px-3 py-2.5">
        <PaperclipIcon className="size-4 text-faint" />
        <span className="flex-1 text-body text-faint">
          <Trans>Write a message…</Trans>
        </span>
        <span className="grid size-7 place-items-center rounded-full text-white" style={{ background: color }}>
          <ArrowUpIcon className="size-3.5" />
        </span>
      </div>
    </div>
  )
}

function BrowserFrame({ host, children, className }: { host: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={cn("flex w-full max-w-[34rem] flex-col overflow-hidden rounded-2xl border bg-card", lift, className)}>
      <div className="flex items-center gap-3 border-b px-4 py-2.5" aria-hidden="true">
        <span className="flex gap-1.5">
          <span className="size-2.5 rounded-full bg-border" />
          <span className="size-2.5 rounded-full bg-border" />
          <span className="size-2.5 rounded-full bg-border" />
        </span>
        <span className="min-w-0 flex-1 truncate rounded-full bg-surface px-3 py-1 text-center font-mono text-caption text-muted-foreground">{host}</span>
      </div>
      <div className="relative min-h-0 flex-1">{children}</div>
    </div>
  )
}

function PageSkeleton() {
  return (
    <div className="flex flex-col gap-3 p-6" aria-hidden="true">
      <span className="h-3 w-24 rounded-full bg-muted" />
      <span className="mt-3 h-5 w-3/4 rounded-full bg-muted" />
      <span className="h-5 w-1/2 rounded-full bg-muted" />
      <span className="mt-2 h-2.5 w-5/6 rounded-full bg-muted/70" />
      <span className="h-2.5 w-4/6 rounded-full bg-muted/70" />
      <span className="h-2.5 w-3/6 rounded-full bg-muted/70" />
      <span className="mt-3 grid grid-cols-3 gap-2.5">
        <span className="h-16 rounded-xl bg-muted/70" />
        <span className="h-16 rounded-xl bg-muted/70" />
        <span className="h-16 rounded-xl bg-muted/70" />
      </span>
    </div>
  )
}

export function SiteMock({
  host,
  name,
  greeting,
  color,
  position = "right",
  waiting,
}: {
  host: string
  name: string
  greeting?: string
  color: string
  position?: ChatLauncherPosition
  waiting?: boolean
}) {
  return (
    <BrowserFrame host={host} className="h-[30rem]">
      <PageSkeleton />
      <div className={cn("absolute bottom-4 flex flex-col gap-3", position === "left" ? "start-4 items-start" : "end-4 items-end")}>
        <ChatMock name={name} greeting={greeting} color={color} waiting={waiting} className="w-[17rem] [&>div:nth-child(2)]:py-3.5" />
        <span className="grid size-11 place-items-center rounded-full text-white shadow-lg" style={{ background: color }} aria-hidden="true">
          <MessageCircleIcon className="size-5" />
        </span>
      </div>
    </BrowserFrame>
  )
}

export function MailMock({ name, to, waiting }: { name: string; to: string; waiting?: boolean }) {
  const { t } = useLingui()
  return (
    <div className={cn("w-full max-w-[26rem] overflow-hidden rounded-2xl border bg-card", lift)} aria-hidden="true">
      <div className="flex items-center justify-between border-b px-5 py-3.5">
        <p className="text-body font-medium">
          <Trans>New message</Trans>
        </p>
        <ChannelIcon kind="email" className="size-4 text-faint" />
      </div>
      <dl className="divide-y text-body">
        <div className="flex gap-3 px-5 py-2.5">
          <dt className="w-14 shrink-0 text-faint">
            <Trans>From</Trans>
          </dt>
          <dd className="truncate">Lina Haddad &lt;lina@example.org&gt;</dd>
        </div>
        <div className="flex gap-3 px-5 py-2.5">
          <dt className="w-14 shrink-0 text-faint">
            <Trans>To</Trans>
          </dt>
          <dd className="truncate font-medium">{to || t`support@your-company.com`}</dd>
        </div>
        <div className="flex gap-3 px-5 py-2.5">
          <dt className="w-14 shrink-0 text-faint">
            <Trans>Subject</Trans>
          </dt>
          <dd className="truncate">
            <Trans>A question about {name}</Trans>
          </dd>
        </div>
      </dl>
      <div className="flex flex-col gap-2 border-t px-5 py-4 text-body text-muted-foreground">
        <span className="h-2.5 w-11/12 rounded-full bg-muted" />
        <span className="h-2.5 w-4/5 rounded-full bg-muted" />
        <span className="h-2.5 w-2/5 rounded-full bg-muted" />
      </div>
      <div className="flex items-center gap-2 border-t bg-surface/60 px-5 py-3 text-small text-muted-foreground">
        {waiting ? (
          <>
            <TypingDots />
            <Trans>Waiting for it to arrive</Trans>
          </>
        ) : (
          <>
            <ReplyIcon className="size-4" />
            <Trans>Your replies go out from this address</Trans>
          </>
        )}
      </div>
    </div>
  )
}

export function PhoneMock({ name, color, waiting }: { name: string; color: string; waiting?: boolean }) {
  return (
    <div className={cn("w-[17.5rem] rounded-[2.6rem] border bg-card p-2.5", lift)} aria-hidden="true">
      <div className="flex h-[32rem] flex-col overflow-hidden rounded-[2.1rem] bg-background">
        <div className="flex justify-center pt-2.5">
          <span className="h-5 w-24 rounded-full bg-foreground/90" />
        </div>
        <div className="flex items-center gap-2.5 border-b px-4 pt-4 pb-3">
          <span className="grid size-8 shrink-0 place-items-center rounded-full text-small font-semibold text-white" style={{ background: color }}>
            {initial(name)}
          </span>
          <span className="min-w-0">
            <span className="block truncate text-body font-medium">{name}</span>
            <span className="block text-caption text-faint">
              <Trans>Help and feedback</Trans>
            </span>
          </span>
        </div>
        <div className="flex flex-1 flex-col justify-end gap-2.5 px-4 py-4">
          <p className="max-w-[85%] rounded-2xl rounded-ss-md bg-surface px-3 py-2 text-small">
            <Trans>Hi! How can we help?</Trans>
          </p>
          {waiting ? (
            <span className="flex items-center gap-2 text-caption text-faint">
              <TypingDots />
              <Trans>Waiting for the first message</Trans>
            </span>
          ) : (
            <p className="ms-auto max-w-[85%] rounded-2xl rounded-ee-md px-3 py-2 text-small text-white" style={{ background: color }}>
              <Trans>The app logs me out after the update.</Trans>
            </p>
          )}
        </div>
        <div className="mx-3 mb-4 flex items-center gap-2 rounded-full border px-3 py-2">
          <span className="flex-1 text-small text-faint">
            <Trans>Message</Trans>
          </span>
          <ArrowUpIcon className="size-4 text-faint" />
        </div>
      </div>
    </div>
  )
}

export function KindPreview({ kind, name, color, host }: { kind: SetupKind; name: string; color: string; host: string }) {
  if (kind === "email") return <MailMock name={name} to="" />
  if (kind === "app") return <PhoneMock name={name} color={color} />
  return <SiteMock host={host} name={name} color={color} />
}

export function FirstConversation({ item, name, kind }: { item: ConversationListItem; name: string; kind: SetupKind }) {
  const { t } = useLingui()
  const who = item.contact.name || item.contact.email || t`Unnamed contact`
  return (
    <div className={cn("w-full max-w-md overflow-hidden rounded-2xl border bg-card", lift)} data-testid="setup-first-conversation">
      <div className="flex items-center justify-between border-b px-5 py-3.5">
        <p className="text-body font-medium">{name}</p>
        <span className="flex items-center gap-1.5 text-caption text-success">
          <CheckIcon className="size-3.5" />
          <Trans>Arrived</Trans>
        </span>
      </div>
      <div className="flex items-start gap-3.5 px-5 py-4">
        <span className="grid size-9 shrink-0 place-items-center rounded-full border text-muted-foreground">
          <ChannelIcon kind={kind} className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-body">
            <span className="truncate font-medium">{who}</span>
            <span className="ms-auto size-2 shrink-0 rounded-full bg-brand" />
          </div>
          <p className="mt-1 line-clamp-2 text-body text-muted-foreground">{item.last_message?.text || item.subject}</p>
        </div>
      </div>
    </div>
  )
}
