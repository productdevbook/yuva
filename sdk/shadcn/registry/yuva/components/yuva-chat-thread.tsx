"use client"

import * as React from "react"
import type { ClientAttachment } from "useyuva"
import {
  ArrowDownIcon,
  CheckCheckIcon,
  CircleAlertIcon,
  FileIcon,
  ImageIcon,
  LockIcon,
  MessageSquareIcon,
  RotateCcwIcon,
  ThumbsDownIcon,
  ThumbsUpIcon,
  XIcon,
} from "lucide-react"
import {
  Attachment,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
  AttachmentTrigger,
} from "@/components/ui/attachment"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Bubble, BubbleContent, BubbleGroup } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker"
import { Message, MessageAvatar, MessageContent, MessageFooter, MessageHeader } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
  useMessageScrollerVisibility,
} from "@/components/ui/message-scroller"
import { Spinner } from "@/components/ui/spinner"
import type { YuvaChatMessage, YuvaChatState } from "@/hooks/use-yuva-chat"
import { cn } from "@/lib/utils"
import {
  dayKey,
  dayLabel,
  defaultYuvaChatLabels,
  initials,
  linkParts,
  sizeLabel,
  soleLink,
  timeLabel,
  type YuvaChatLabels,
} from "@/lib/yuva-chat-utils"

const GROUP_GAP_MS = 5 * 60 * 1000

type Author = YuvaChatMessage["author"]

type Entry =
  | { key: string; kind: "day"; at: string }
  | { key: string; kind: "event"; message: YuvaChatMessage }
  | { key: string; kind: "group"; mine: boolean; author: Author; messages: YuvaChatMessage[] }
  | { key: string; kind: "closed" }

function sameSide(a: Author, b: Author): boolean {
  return a.type === b.type && (a.name ?? "") === (b.name ?? "")
}

function buildEntries(messages: YuvaChatMessage[], closed: boolean): Entry[] {
  const entries: Entry[] = []
  let day = ""
  for (const message of messages) {
    const key = dayKey(message.createdAt)
    if (key !== day) {
      day = key
      entries.push({ key: `day:${key}`, kind: "day", at: message.createdAt })
    }
    if (message.author.type === "system") {
      entries.push({ key: message.id, kind: "event", message })
      continue
    }
    const mine = message.direction === "in"
    const previous = entries.at(-1)
    const last = previous?.kind === "group" ? previous.messages.at(-1) : undefined
    if (
      previous?.kind === "group" &&
      last &&
      previous.mine === mine &&
      sameSide(previous.author, message.author) &&
      Date.parse(message.createdAt) - Date.parse(last.createdAt) < GROUP_GAP_MS
    ) {
      previous.messages.push(message)
    } else {
      entries.push({ key: message.id, kind: "group", mine, author: message.author, messages: [message] })
    }
  }
  if (closed) entries.push({ key: "closed", kind: "closed" })
  return entries
}

function Linked({ body }: { body: string }) {
  return (
    <>
      {linkParts(body).map((part, index) =>
        part.href ? (
          <a
            key={index}
            href={part.href}
            target="_blank"
            rel="noopener noreferrer"
            className="underline underline-offset-3"
          >
            {part.text}
          </a>
        ) : (
          <React.Fragment key={index}>{part.text}</React.Fragment>
        )
      )}
    </>
  )
}

function BubbleText({ message, mine, labels }: { message: YuvaChatMessage; mine: boolean; labels: YuvaChatLabels }) {
  if (message.redacted) return <p className="text-sm text-muted-foreground italic">{labels.removed}</p>
  if (!message.body) return null
  const link = soleLink(message.body)
  const failed = message.status === "error"
  if (link && !mine) {
    return (
      <Bubble variant="tinted">
        <BubbleContent asChild>
          <a href={link.href} target="_blank" rel="noopener noreferrer">
            {link.hostname}
            {link.pathname === "/" ? "" : link.pathname}
          </a>
        </BubbleContent>
      </Bubble>
    )
  }
  const variant = failed ? "destructive" : mine ? "default" : "muted"
  return (
    <Bubble variant={variant}>
      <BubbleContent className={cn("whitespace-pre-wrap", !mine && !failed && "border-border")}>
        <Linked body={message.body} />
      </BubbleContent>
    </Bubble>
  )
}

function fileIcon(type: string) {
  return type.startsWith("image/") ? <ImageIcon /> : <FileIcon />
}

function MessageAttachments({
  message,
  labels,
  locale,
  chat,
}: {
  message: YuvaChatMessage
  labels: YuvaChatLabels
  locale?: string
  chat: YuvaChatState
}) {
  if (message.attachments.length === 0 && message.files.length === 0) return null
  const state = message.status === "sending" ? "uploading" : message.status === "error" ? "error" : "done"
  return (
    <AttachmentGroup>
      {message.files.map((file, index) => (
        <Attachment key={`${file.name}:${index}`} state={state} size="sm">
          <AttachmentMedia>{state === "uploading" ? <Spinner /> : fileIcon(file.type)}</AttachmentMedia>
          <AttachmentContent>
            <AttachmentTitle>{file.name}</AttachmentTitle>
            <AttachmentDescription>
              {state === "error" ? labels.sendFailed : sizeLabel(file.size, locale)}
            </AttachmentDescription>
          </AttachmentContent>
        </Attachment>
      ))}
      {message.attachments.map((attachment: ClientAttachment) => (
        <Attachment key={attachment.id} size="sm">
          <AttachmentMedia>{fileIcon(attachment.content_type)}</AttachmentMedia>
          <AttachmentContent>
            <AttachmentTitle>{attachment.filename}</AttachmentTitle>
            <AttachmentDescription>{sizeLabel(attachment.size, locale)}</AttachmentDescription>
          </AttachmentContent>
          <AttachmentTrigger
            aria-label={`${labels.download}: ${attachment.filename}`}
            onClick={() => void chat.openAttachment(attachment).catch(() => undefined)}
          />
        </Attachment>
      ))}
    </AttachmentGroup>
  )
}

function Footer({
  group,
  chat,
  labels,
  locale,
  lastMine,
}: {
  group: Extract<Entry, { kind: "group" }>
  chat: YuvaChatState
  labels: YuvaChatLabels
  locale?: string
  lastMine: boolean
}) {
  const last = group.messages.at(-1)!
  const seen =
    group.mine &&
    lastMine &&
    last.status === "sent" &&
    !!chat.memberReadAt &&
    Date.parse(chat.memberReadAt) >= Date.parse(last.createdAt)
  return (
    <MessageFooter className="gap-1.5">
      {last.status === "sending" ? (
        <span>{labels.sending}</span>
      ) : last.status === "error" ? (
        <>
          <CircleAlertIcon className="size-3.5 text-destructive" />
          <span className="text-destructive">{labels.sendFailed}</span>
          <Button variant="ghost" size="xs" onClick={() => void chat.retry(last.id)}>
            <RotateCcwIcon />
            {labels.retry}
          </Button>
          <Button variant="ghost" size="xs" onClick={() => chat.discard(last.id)}>
            <XIcon />
            {labels.remove}
          </Button>
        </>
      ) : seen ? (
        <>
          <span>{labels.seen}</span>
          <time dateTime={chat.memberReadAt!}>{timeLabel(chat.memberReadAt!, locale)}</time>
          <CheckCheckIcon className="size-3.5 text-primary" aria-hidden />
        </>
      ) : (
        <time dateTime={last.createdAt}>{timeLabel(last.createdAt, locale)}</time>
      )}
    </MessageFooter>
  )
}

function Group({
  group,
  chat,
  labels,
  locale,
  lastMine,
}: {
  group: Extract<Entry, { kind: "group" }>
  chat: YuvaChatState
  labels: YuvaChatLabels
  locale?: string
  lastMine: boolean
}) {
  const name = group.author.name || (group.author.type === "bot" ? labels.bot : labels.team)
  return (
    <Message align={group.mine ? "end" : "start"}>
      {!group.mine && (
        <MessageAvatar>
          <Avatar size="sm">
            {group.author.avatar_url && <AvatarImage src={group.author.avatar_url} alt="" />}
            <AvatarFallback className="bg-accent font-semibold text-primary">
              {group.author.initials || initials(name)}
            </AvatarFallback>
          </Avatar>
        </MessageAvatar>
      )}
      <MessageContent>
        {!group.mine && <MessageHeader>{name}</MessageHeader>}
        <BubbleGroup>
          {group.messages.map((message) => (
            <React.Fragment key={message.id}>
              <BubbleText message={message} mine={group.mine} labels={labels} />
              <MessageAttachments message={message} labels={labels} locale={locale} chat={chat} />
            </React.Fragment>
          ))}
        </BubbleGroup>
        <Footer group={group} chat={chat} labels={labels} locale={locale} lastMine={lastMine} />
      </MessageContent>
    </Message>
  )
}

function Typing({ chat, labels }: { chat: YuvaChatState; labels: YuvaChatLabels }) {
  const author = chat.typing
  if (!author) return null
  const name = author.name || (author.type === "bot" ? labels.bot : labels.team)
  return (
    <Message role="status" aria-label={author.name ? labels.typing(name) : labels.typingSomeone}>
      <MessageAvatar>
        <Avatar size="sm">
          {author.avatar_url && <AvatarImage src={author.avatar_url} alt="" />}
          <AvatarFallback className="bg-accent font-semibold text-primary">
            {author.initials || initials(name)}
          </AvatarFallback>
        </Avatar>
      </MessageAvatar>
      <MessageContent>
        <Bubble variant="muted">
          <BubbleContent className="flex items-center gap-1 border-border px-3.5 py-3">
            {[0, 150, 300].map((delay) => (
              <span
                key={delay}
                aria-hidden
                className="size-1.5 rounded-full bg-muted-foreground motion-safe:animate-[pulse_1.2s_ease-in-out_infinite]"
                style={{ animationDelay: `${delay}ms` }}
              />
            ))}
          </BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
  )
}

function JumpToLatest({ entries, labels }: { entries: Entry[]; labels: YuvaChatLabels }) {
  const { visibleMessageIds } = useMessageScrollerVisibility()
  const visible = new Set(visibleMessageIds)
  let lastVisible = -1
  entries.forEach((entry, index) => {
    if (visible.has(entry.key)) lastVisible = index
  })
  let unread = 0
  entries.forEach((entry, index) => {
    if (index > lastVisible && entry.kind === "group" && !entry.mine) unread += entry.messages.length
  })
  const text = labels.jumpToLatest(unread)
  return (
    <MessageScrollerButton
      size={unread > 0 ? "lg" : "icon-lg"}
      aria-label={text}
      className="rounded-full border border-input text-xs font-medium shadow-md"
    >
      <ArrowDownIcon />
      {unread > 0 && <span className="tabular-nums">{labels.newCount(unread)}</span>}
    </MessageScrollerButton>
  )
}

export interface YuvaChatThreadProps {
  chat: YuvaChatState
  labels?: Partial<YuvaChatLabels>
  locale?: string
  greeting?: string
  lined?: boolean
  className?: string
}

export function YuvaChatThread({ chat, labels: custom, locale, greeting, lined, className }: YuvaChatThreadProps) {
  const labels = React.useMemo(() => ({ ...defaultYuvaChatLabels, ...custom }), [custom])
  const closed = chat.conversation?.status === "closed"
  const entries = React.useMemo(() => buildEntries(chat.messages, closed), [chat.messages, closed])
  const lastMine = React.useMemo(() => chat.messages.findLast((item) => item.direction === "in")?.id, [chat.messages])

  if (chat.threadLoading && chat.messages.length === 0) {
    return (
      <div className={cn("flex min-h-0 flex-1 items-center justify-center", className)}>
        <Spinner aria-label={labels.loading} />
      </div>
    )
  }
  if (chat.messages.length === 0) {
    return (
      <Empty className={cn("flex-1", className)}>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <MessageSquareIcon />
          </EmptyMedia>
          <EmptyTitle>{labels.emptyTitle}</EmptyTitle>
          <EmptyDescription>{greeting || labels.emptyDescription}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <MessageScrollerProvider defaultScrollPosition="end">
      <MessageScroller className={cn("flex-1", className)}>
        <div
          aria-hidden
          className={cn(
            "pointer-events-none absolute top-0 z-10 h-9 bg-gradient-to-b from-background to-transparent",
            lined ? "inset-x-[calc(1.25rem+1px)]" : "inset-x-0"
          )}
        />
        <MessageScrollerViewport preserveScrollOnPrepend aria-label={labels.messages}>
          <MessageScrollerContent className={cn("gap-4 py-4", lined ? "px-8" : "px-4")}>
            {chat.hasEarlier && (
              <Button variant="ghost" size="sm" className="self-center" onClick={() => void chat.loadEarlier()}>
                {labels.loadEarlier}
              </Button>
            )}
            {entries.map((entry) => (
              <MessageScrollerItem key={entry.key} messageId={entry.key}>
                {entry.kind === "day" ? (
                  <Marker variant="separator">
                    <MarkerContent className={cn(lined && "font-mono text-[0.6875rem] tracking-[0.08em] uppercase")}>
                      {dayLabel(entry.at, labels, locale)}
                    </MarkerContent>
                  </Marker>
                ) : entry.kind === "event" ? (
                  <Marker className="justify-center">
                    <MarkerIcon>
                      <MessageSquareIcon />
                    </MarkerIcon>
                    <MarkerContent>{entry.message.body}</MarkerContent>
                  </Marker>
                ) : entry.kind === "closed" ? (
                  <ClosedNotice chat={chat} labels={labels} />
                ) : (
                  <Group
                    group={entry}
                    chat={chat}
                    labels={labels}
                    locale={locale}
                    lastMine={entry.messages.some((item) => item.id === lastMine)}
                  />
                )}
              </MessageScrollerItem>
            ))}
            <Typing chat={chat} labels={labels} />
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <JumpToLatest entries={entries} labels={labels} />
      </MessageScroller>
    </MessageScrollerProvider>
  )
}

function ClosedNotice({ chat, labels }: { chat: YuvaChatState; labels: YuvaChatLabels }) {
  const conversation = chat.conversation
  const [busy, setBusy] = React.useState(false)
  const rate = (rating: "good" | "bad") => {
    setBusy(true)
    chat
      .rate(rating)
      .catch(() => undefined)
      .finally(() => setBusy(false))
  }
  return (
    <div className="flex flex-col items-center gap-2">
      <Marker className="justify-center">
        <MarkerIcon>
          <LockIcon />
        </MarkerIcon>
        <MarkerContent>{labels.closed}</MarkerContent>
      </Marker>
      {conversation?.rating ? (
        <p className="text-xs text-muted-foreground">{labels.rated}</p>
      ) : (
        conversation?.can_rate && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span>{labels.rateQuestion}</span>
            <Button variant="outline" size="sm" disabled={busy} onClick={() => rate("good")}>
              <ThumbsUpIcon />
              {labels.rateGood}
            </Button>
            <Button variant="outline" size="sm" disabled={busy} onClick={() => rate("bad")}>
              <ThumbsDownIcon />
              {labels.rateBad}
            </Button>
          </div>
        )
      )}
    </div>
  )
}
