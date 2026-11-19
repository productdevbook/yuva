import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { FileDownIcon, FilterIcon, ForwardIcon, PaperclipIcon, ReplyAllIcon, ReplyIcon } from "lucide-react"

import { BotAvatar, ContactAvatar, PersonAvatar } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { SignatureHtml } from "@/components/common/SignatureHtml"
import { formatDateTime, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { ArticleCards } from "@/features/conversation/messages/ArticleCards"
import { useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { DeliveryState } from "@/features/conversation/messages/DeliveryState"
import { DesignLine } from "@/features/conversation/messages/DesignLine"
import { MailBody, MailControls, QuotedToggle, useMail } from "@/features/conversation/messages/EmailBody"
import { Highlight, MessageActions, Ticks } from "@/features/conversation/messages/MessageActions"
import { rawMessageUrl, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

function useMailWho(m: Message, ctx: ThreadContext, contactName: string) {
  const { t } = useLingui()
  const author = useAuthorName(m, ctx)
  const by = useAuthorName({ ...m, author: m.sent_by ?? m.author }, ctx)
  if (m.rule) {
    const rule = m.rule.name
    return { who: t`by rule ${rule}`, by }
  }
  if (m.direction !== "out") {
    const from = m.email?.from?.toLowerCase()
    const other = !!from && !!ctx.contact && !ctx.contact.emails.some((e) => e.toLowerCase() === from)
    return { who: m.author.type === "contact" && !other ? contactName : author, by }
  }
  if (m.on_behalf_of_member_id) {
    const owner = ctx.members.get(m.on_behalf_of_member_id)
    const ownerName = owner ? owner.name || owner.email : t`Deleted member`
    return { who: t`${author} on behalf of ${ownerName}`, by }
  }
  return { who: m.sent_by ? t`${author}, sent by ${by}` : author, by }
}

function Face({ m, ctx, name, by }: { m: Message; ctx: ThreadContext; name: string; by: string }) {
  if (m.direction !== "out")
    return <ContactAvatar id={ctx.contact?.id ?? m.conversation_id} name={name} className="size-6 text-[10px]" />
  if (m.author.type === "bot" && !m.sent_by)
    return <BotAvatar name={by} url={m.author.avatar_url} className="size-6 text-[10px]" />
  return <PersonAvatar name={by} className="size-6 bg-muted-foreground text-[10px] text-background" />
}

export function MailFold({
  count: hidden,
  onOpen,
  shortcut = SHORTCUTS.expand,
}: {
  count: number
  onOpen: () => void
  shortcut?: string
}) {
  const { t } = useLingui()
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-expanded={false}
      title={t`Expand every message (${shortcut})`}
      className="flex w-full items-center gap-2.5 border-t border-line-soft px-[18px] py-[7px] text-small font-medium text-fg2 outline-none hover:bg-subtle focus-visible:bg-subtle phone:px-3"
      data-testid="mail-stack"
    >
      <span aria-hidden className="h-px flex-1 bg-border" />
      <Plural value={hidden} one="# earlier message" other="# earlier messages" />
      <span aria-hidden className="h-px flex-1 bg-border" />
    </button>
  )
}

export function MailRow({
  m,
  ctx,
  name,
  open,
  onToggle,
  subject,
  onFilter,
  hit,
  keys = { reply: SHORTCUTS.reply, replyAll: SHORTCUTS.replyAll, forward: SHORTCUTS.forward },
}: {
  m: Message
  ctx: ThreadContext
  name: string
  open: boolean
  onToggle: () => void
  subject?: string
  onFilter?: () => void
  hit?: boolean
  keys?: { reply: string; replyAll: string; forward: string }
}) {
  const { t, i18n } = useLingui()
  const { workspaceId } = useSession()
  const mail = useMail(m, ctx.expandQuoted)
  const { who, by } = useMailWho(m, ctx, name)
  const out = m.direction === "out"
  const e = m.email
  const time = (
    <time
      className="shrink-0 text-small text-muted-foreground"
      dateTime={m.created_at}
      title={formatDateTime(m.created_at, i18n.locale)}
    >
      {formatShort(m.created_at, i18n.locale)}
    </time>
  )
  if (!open) {
    const snippet = m.body.replace(/\s+/g, " ").trim()
    return (
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={false}
        className="flex w-full min-w-0 items-center gap-2.5 border-t border-line-soft px-[18px] py-2.5 text-start text-body outline-none hover:bg-subtle focus-visible:bg-subtle phone:px-3"
        data-testid="mail-message"
        data-collapsed
        data-message-id={m.id}
      >
        <Face m={m} ctx={ctx} name={name} by={by} />
        <b className="max-w-44 shrink-0 truncate font-semibold phone:max-w-28">{who}</b>
        <span className="min-w-0 flex-1 truncate text-muted-foreground">{snippet}</span>
        {m.attachments.length > 0 && <PaperclipIcon className="size-3.5 shrink-0 text-muted-foreground" />}
        {time}
      </button>
    )
  }
  const to = e?.to.join(", ") ?? ""
  const cc = e?.cc?.join(", ") ?? ""
  const bcc = e?.bcc?.join(", ") ?? ""
  const several = (e?.to.length ?? 0) + (e?.cc?.length ?? 0) > 1
  const tail = mail.listed.length > 0 || m.delivery || m.design || (m.cards?.length ?? 0) > 0
  const icon = "size-[26px] text-muted-foreground hover:text-foreground"
  return (
    <article
      tabIndex={-1}
      className={cn(
        "min-w-0 border-t border-line-soft px-[18px] pt-3.5 pb-4 outline-none phone:px-3",
        out && "bg-subtle",
        hit && "ring-2 ring-warning ring-inset"
      )}
      data-testid="mail-message"
      data-kind="email"
      data-message-id={m.id}
    >
      <div className="flex min-w-0 items-start gap-2.5">
        <Face m={m} ctx={ctx} name={name} by={by} />
        <button
          type="button"
          onClick={onToggle}
          aria-expanded
          title={t`Collapse`}
          className="min-w-0 flex-1 rounded-sm text-start text-body leading-[19px] outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <b className="font-semibold" data-testid="mail-from-name">
            {who}
          </b>
          {e?.from && (
            <span className="ms-1.5 text-small break-all text-muted-foreground" data-testid="mail-from">
              {e.from}
            </span>
          )}
          {(to || cc || bcc) && (
            <span className="block text-small break-words text-muted-foreground" data-testid="email-recipients">
              {to && (
                <span data-testid="mail-to">
                  {t`To`} {to}
                </span>
              )}
              {cc && (
                <span data-testid="mail-cc">
                  {to && " · "}
                  {t`Cc`} {cc}
                </span>
              )}
              {bcc && (
                <span data-testid="mail-bcc">
                  {(to || cc) && " · "}
                  {t`Bcc`} {bcc}
                </span>
              )}
            </span>
          )}
        </button>
        {time}
        {out && !m.delivery && <Ticks at={m.created_at} readAt={ctx.contactReadAt} className="mt-0.5" />}
        {ctx.onReply && (
          <span className="-me-1.5 -mt-[3px] flex shrink-0 gap-0.5">
            <Button
              variant="ghost"
              size="icon-xs"
              className={icon}
              onClick={() => ctx.onReply?.(m, false)}
              aria-label={t`Reply`}
              title={t`Reply (${keys.reply})`}
              data-testid="mail-row-reply"
            >
              <ReplyIcon />
            </Button>
            {several && (
              <Button
                variant="ghost"
                size="icon-xs"
                className={icon}
                onClick={() => ctx.onReply?.(m, true)}
                aria-label={t`Reply all`}
                title={t`Reply all (${keys.replyAll})`}
                data-testid="mail-row-reply-all"
              >
                <ReplyAllIcon />
              </Button>
            )}
            {ctx.onForwardMail && m.kind === "message" && (
              <Button
                variant="ghost"
                size="icon-xs"
                className={icon}
                onClick={() => ctx.onForwardMail?.(m)}
                aria-label={t`Forward`}
                title={t`Forward (${keys.forward})`}
                data-testid="mail-row-forward"
              >
                <ForwardIcon className="rtl:-scale-x-100" />
              </Button>
            )}
          </span>
        )}
      </div>
      {subject && (
        <div className="ms-[34px] mt-2 text-small font-semibold break-words phone:ms-0" data-testid="email-subject">
          {subject}
        </div>
      )}
      <div
        className={cn(
          "mt-2.5 ms-[34px] min-w-0 text-reading phone:ms-0",
          (mail.originalHtml || mail.waiting) && "overflow-hidden rounded-lg border"
        )}
      >
        {!mail.html && !mail.waiting && !mail.originalHtml ? (
          <div className="text-reading break-words whitespace-pre-wrap">
            <Highlight text={mail.text} query={ctx.find?.query} />
          </div>
        ) : (
          <MailBody mail={mail} />
        )}
      </div>
      <div className="ms-[34px] empty:hidden phone:ms-0">
        <QuotedToggle mail={mail} />
      </div>
      {m.signature_html && (
        <div className="mt-2 ms-[34px] border-t pt-2 opacity-80 phone:ms-0" data-testid="message-signature">
          <SignatureHtml html={m.signature_html} />
        </div>
      )}
      {tail && (
        <div className="mt-3 ms-[34px] flex flex-col gap-2 phone:ms-0">
          <DesignLine m={m} />
          <Attachments items={mail.listed} />
          <ArticleCards items={m.cards} outgoing={false} />
          {m.delivery && <DeliveryState d={m.delivery} />}
        </div>
      )}
      <div className="mt-2 ms-[30px] flex flex-wrap items-center gap-1 text-caption text-muted-foreground phone:ms-0">
        <MailControls mail={mail} quoted={false} />
        <MessageActions m={m} ctx={{ ...ctx, onForwardMail: undefined }} className="px-1" />
        <span className="flex-1" />
        {onFilter && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 gap-1 px-1.5 text-small font-normal text-muted-foreground"
            onClick={onFilter}
            data-testid="mail-filter-like"
          >
            <FilterIcon className="size-3.5" />
            <Trans>Filter messages like this</Trans>
          </Button>
        )}
        {e?.raw && (
          <Button
            variant="ghost"
            size="icon-xs"
            className="text-muted-foreground"
            render={<a href={rawMessageUrl(m.id, workspaceId)} download />}
            aria-label={t`Download the original e-mail (.eml)`}
            title={t`Download the original e-mail (.eml)`}
            data-testid="raw-download"
          >
            <FileDownIcon />
          </Button>
        )}
      </div>
    </article>
  )
}
