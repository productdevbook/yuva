import { Trans, useLingui } from "@lingui/react/macro"
import { MailIcon, SmartphoneIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate } from "react-router"

import { ChannelIcon, Dot, ErrorLine, Segmented, toast } from "@/components/common"
import { formatDate, formatDateTime, formatShort, useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useClearUndeliverable, useCreateConversation, useUpdateContact } from "@/features/contact/queries"
import { attrValue } from "@/features/contact/Section"
import { currentRating, RatingMark } from "@/features/conversation/rating"
import { useConversations } from "@/features/inbox/queries"
import type { Contact, ContactSummary, ConversationListItem, ConversationStatus, UndeliverableEmail } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

export function languageName(tag: string, locale: string) {
  try {
    const name = new Intl.DisplayNames([locale], { type: "language" }).of(tag)
    return name && name !== tag ? name : tag
  } catch {
    return tag
  }
}

export function useContactConversations(contactId: string, status?: ConversationStatus) {
  const q = useConversations({ contact_id: contactId, status })
  const items = q.data?.pages.flatMap((p) => p.items) ?? []
  return { ...q, items }
}

export function SectionTitle({ children, className }: { children: React.ReactNode; className?: string }) {
  return <h2 className={cn("mb-2 text-small font-medium text-faint", className)}>{children}</h2>
}

function UndeliverableNote({ contactId, u }: { contactId: string; u: UndeliverableEmail }) {
  const { i18n } = useLingui()
  const clear = useClearUndeliverable(contactId)
  const since = formatDateTime(u.created_at, i18n.locale)
  return (
    <div className="flex flex-col gap-1 ps-[25px] text-caption" data-testid="undeliverable">
      <span className="text-destructive">
        {u.reason === "complaint" ? (
          <Trans>Undeliverable: marked as spam by the recipient, {since}</Trans>
        ) : (
          <Trans>Undeliverable: bounced, {since}</Trans>
        )}
      </span>
      {u.detail && <span className="break-words text-muted-foreground">{u.detail}</span>}
      <span className="flex flex-wrap items-center gap-2 text-muted-foreground">
        <Trans>Replies to this address are refused until you clear it.</Trans>
        <Button variant="outline" size="sm" onClick={() => clear.mutate(u.email)} disabled={clear.isPending} data-testid="clear-undeliverable">
          <Trans>Clear</Trans>
        </Button>
      </span>
      <ErrorLine error={clear.error} />
    </div>
  )
}

function AddAddress({ contact }: { contact: Contact }) {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState("")
  const update = useUpdateContact(contact.id)
  if (!open) {
    return (
      <Button variant="link" className="h-auto self-start p-0" onClick={() => setOpen(true)} data-testid="add-address">
        <Trans>+ Add address</Trans>
      </Button>
    )
  }
  const email = value.trim()
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (!email) return
        update.mutate(
          { emails: [...contact.emails, email] },
          {
            onSuccess: () => {
              setValue("")
              setOpen(false)
            },
          },
        )
      }}
    >
      <Label htmlFor={`add-address-${contact.id}`} className="sr-only">
        <Trans>E-mail address</Trans>
      </Label>
      <Input
        id={`add-address-${contact.id}`}
        type="email"
        autoFocus
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => e.key === "Escape" && (e.stopPropagation(), setOpen(false))}
        placeholder={t`name@example.com`}
        className="h-9"
        data-testid="add-address-input"
      />
      <ErrorLine error={update.error} />
      <span className="flex gap-2">
        <Button type="submit" size="sm" disabled={!email || update.isPending}>
          <Trans>Add</Trans>
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setOpen(false)}>
          <Trans>Cancel</Trans>
        </Button>
      </span>
    </form>
  )
}

export function Identities({ contact, editable, className }: { contact: Contact; editable?: boolean; className?: string }) {
  const inboxes = useInboxes().data ?? []
  const rows = contact.emails.length + contact.external_ids.length
  return (
    <div className={cn("flex flex-col gap-2", className)} data-testid="identities">
      {rows > 0 && (
        <ul className="flex flex-col divide-y rounded-xl border">
          {contact.emails.map((e) => {
            const u = contact.undeliverable.find((x) => x.email === e)
            return (
              <li key={e} className="flex flex-col gap-1.5 px-3 py-2.5">
                <span className="flex items-center gap-2.5">
                  <MailIcon className="size-[15px] shrink-0 text-faint" aria-hidden />
                  <span className="min-w-0 flex-1 truncate" title={e}>
                    {e}
                  </span>
                  {u && (
                    <span className="shrink-0 text-caption text-destructive">
                      <Trans>Undeliverable</Trans>
                    </span>
                  )}
                </span>
                {u && <UndeliverableNote contactId={contact.id} u={u} />}
              </li>
            )
          })}
          {contact.external_ids.map((x) => (
            <li key={`${x.inbox_id}:${x.external_id}`} className="flex items-center gap-2.5 px-3 py-2.5">
              <SmartphoneIcon className="size-[15px] shrink-0 text-faint" aria-hidden />
              <span className="min-w-0 flex-1 truncate">{inboxes.find((i) => i.id === x.inbox_id)?.name ?? "?"}</span>
              <span className="truncate font-mono text-caption text-muted-foreground" title={x.external_id}>
                {x.external_id}
              </span>
            </li>
          ))}
        </ul>
      )}
      {editable && <AddAddress contact={contact} />}
    </div>
  )
}

export function Attributes({ contact }: { contact: Contact }) {
  const { t, i18n } = useLingui()
  const entries: [string, string][] = Object.entries(contact.attributes ?? {}).map(([k, v]) => [k, attrValue(v)])
  if (contact.locale) entries.unshift([t`Language`, languageName(contact.locale, i18n.locale)])
  if (entries.length === 0) return null
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-2.5" data-testid="attributes">
      {entries.map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="truncate text-caption text-faint" lang="">
            {k}
          </dt>
          <dd className="mt-0.5 break-words" title={v}>
            {v}
          </dd>
        </div>
      ))}
    </dl>
  )
}

export function Satisfaction({ summary }: { summary?: ContactSummary }) {
  if (!summary) return null
  const { good, bad } = summary.ratings
  if (good + bad === 0) return null
  return (
    <div className="flex gap-2 text-small" data-testid="satisfaction">
      <span className="rounded-full bg-success/10 px-2.5 py-1 text-success">
        <Trans>{good} good</Trans>
      </span>
      <span className={cn("rounded-full px-2.5 py-1", bad ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground")}>
        <Trans>{bad} bad</Trans>
      </span>
    </div>
  )
}

export type ConversationFilter = "all" | "open" | "done"

export const filterStatus: Record<ConversationFilter, ConversationStatus | undefined> = { all: undefined, open: "open", done: "closed" }

export function ConversationFilters({
  value,
  onChange,
  contact,
}: {
  value: ConversationFilter
  onChange: (f: ConversationFilter) => void
  contact: Contact
}) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const a = contact.activity
  const count = (n?: number) => n !== undefined && <span className="text-faint tabular-nums">{fmt.format(n)}</span>
  return (
    <Segmented<ConversationFilter>
      label={t`Conversations`}
      value={value}
      onChange={onChange}
      items={[
        { value: "all", testId: "contact-filter-all", label: <>{t`All`} {count(a?.conversations)}</> },
        { value: "open", testId: "contact-filter-open", label: <>{t`Open`} {count(a?.open_conversations)}</> },
        { value: "done", testId: "contact-filter-done", label: t`Done` },
      ]}
      itemClassName="gap-1.5"
      className="self-start"
    />
  )
}

export function ConversationRows({
  items,
  currentId,
  roomy,
  pending,
}: {
  items: ConversationListItem[]
  currentId?: string
  roomy?: boolean
  pending?: boolean
}) {
  const { i18n } = useLingui()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const channels = useChannelMap()
  if (pending) return <Skeleton className="h-20 w-full rounded-xl" />
  if (items.length === 0) {
    return (
      <p className="rounded-xl border border-dashed px-4 py-6 text-center text-small text-muted-foreground">
        <Trans>No conversations here.</Trans>
      </p>
    )
  }
  return (
    <ul className={cn("flex flex-col", roomy ? "divide-y overflow-hidden rounded-2xl border" : "gap-2")} data-testid="contact-conversations">
      {items.map((c) => {
        const inbox = inboxes.find((i) => i.id === c.inbox_id)
        const channel = c.channel_id ? channels.get(c.channel_id) : undefined
        const rating = currentRating(c)
        const open = c.status !== "closed"
        const title = c.subject || c.last_message?.text
        return (
          <li key={c.id}>
            <Link
              to={`/conversations/${c.id}`}
              className={cn(
                "flex gap-3 transition-colors hover:bg-muted",
                roomy ? "px-4 py-3.5" : "rounded-xl border p-3",
                c.id === currentId && "bg-brand-wash hover:bg-brand-wash",
              )}
              data-testid="contact-conversation"
            >
              {roomy && (
                <span className="grid size-[34px] shrink-0 place-items-center rounded-[10px] border text-muted-foreground">
                  <ChannelIcon kind={channel?.kind ?? "api"} className="size-[15px]" />
                </span>
              )}
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="flex items-baseline gap-2">
                  <span className={cn("min-w-0 flex-1 truncate font-medium", !title && "text-muted-foreground italic")}>
                    {title || <Trans>No subject</Trans>}
                  </span>
                  <span className={cn("inline-flex shrink-0 items-center gap-1 text-caption", open ? "text-brand" : "text-faint")}>
                    {text.status[c.status]}
                    {rating && <RatingMark rating={rating.rating} comment={rating.comment} />}
                  </span>
                </span>
                {roomy && c.subject && c.last_message?.text && <span className="truncate text-small text-muted-foreground">{c.last_message.text}</span>}
                <span className="flex flex-wrap items-center gap-x-1.5 text-caption text-faint">
                  <span title={formatDateTime(c.last_activity_at, i18n.locale)}>{formatShort(c.last_activity_at, i18n.locale)}</span>
                  {inbox && (
                    <>
                      <span aria-hidden>·</span>
                      <span className="inline-flex items-center gap-1">
                        <Dot color={inbox.branding.color} className="size-1.5" />
                        {inbox.name}
                      </span>
                    </>
                  )}
                  {channel && (
                    <>
                      <span aria-hidden>·</span>
                      <span>{text.channel[channel.kind]}</span>
                    </>
                  )}
                </span>
              </span>
            </Link>
          </li>
        )
      })}
    </ul>
  )
}

export function sinceText(contact: Contact, locale: string) {
  return formatDate(contact.created_at, locale)
}

export function NewConversationDialog({ contact, open, onOpenChange }: { contact: Contact; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const navigate = useNavigate()
  const { membership } = useSession()
  const inboxes = useInboxes().data ?? []
  const channelMap = useChannelMap()
  const [inboxId, setInboxId] = useState("")
  const [channelId, setChannelId] = useState("")
  const [subject, setSubject] = useState("")
  const create = useCreateConversation()
  const inbox = inboxId || inboxes[0]?.id || ""
  const channels = [...channelMap.values()].filter((c) => c.inbox_id === inbox)
  const preferred = channels.find((c) => (contact.emails.length ? c.kind === "email" : c.kind !== "email")) ?? channels[0]
  const channel = channels.some((c) => c.id === channelId) ? channelId : (preferred?.id ?? "")
  const text = useEnumText()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="new-conversation">
        <DialogHeader>
          <DialogTitle>
            <Trans>New conversation</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>It opens assigned to you; write the first message there.</Trans>
          </DialogDescription>
        </DialogHeader>
        <form
          id="new-conversation-form"
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!inbox) return
            create.mutate(
              { inbox_id: inbox, contact_id: contact.id, channel_id: channel || undefined, subject: subject.trim() || undefined, assignee_id: membership.member_id },
              {
                onSuccess: (c) => {
                  onOpenChange(false)
                  navigate(`/conversations/${c.id}`)
                },
                onError: (err) => toast(errorText(err)),
              },
            )
          }}
        >
          <div className="grid grid-cols-2 gap-3 phone:grid-cols-1">
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label>
                <Trans>Inbox</Trans>
              </Label>
              <Select value={inbox} onValueChange={(v) => (setInboxId(String(v)), setChannelId(""))}>
                <SelectTrigger aria-label={t`Inbox`} className="w-full">
                  <SelectValue>{inboxes.find((i) => i.id === inbox)?.name}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {inboxes.map((i) => (
                    <SelectItem key={i.id} value={i.id}>
                      {i.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label>
                <Trans>Channel</Trans>
              </Label>
              <Select value={channel} onValueChange={(v) => setChannelId(String(v))} disabled={channels.length === 0}>
                <SelectTrigger aria-label={t`Channel`} className="w-full">
                  <SelectValue>
                    {(() => {
                      const c = channels.find((x) => x.id === channel)
                      return c ? `${c.name} · ${text.channel[c.kind]}` : t`No channel`
                    })()}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {channels.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name} · {text.channel[c.kind]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="new-conversation-subject">
              <Trans>Subject</Trans>
            </Label>
            <Input id="new-conversation-subject" value={subject} onChange={(e) => setSubject(e.target.value)} maxLength={500} className="h-9" />
          </div>
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            <Trans>Cancel</Trans>
          </Button>
          <Button type="submit" form="new-conversation-form" disabled={!inbox || create.isPending} data-testid="new-conversation-create">
            <Trans>Start conversation</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
