import { Trans, useLingui } from "@lingui/react/macro"
import { CheckCheckIcon, CheckIcon } from "lucide-react"

import { Kbd, MemberAvatar, toast } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { ThreadContext } from "@/features/conversation/messages/context"
import type { Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useAssignableMembers } from "@/lib/workspace"

export function quoteOf(body: string) {
  const lines = body.trim().split("\n")
  const cut = lines.length > 8 ? [...lines.slice(0, 8), "…"] : lines
  return cut.map((l) => `> ${l}`).join("\n")
}

export function Ticks({ at, readAt, className }: { at: string; readAt?: string; className?: string }) {
  const { t } = useLingui()
  const read = !!readAt && Date.parse(at) <= Date.parse(readAt)
  return read ? (
    <CheckCheckIcon
      className={cn("inline size-3.5 shrink-0 text-brand", className)}
      aria-label={t`Read`}
      data-testid="ticks-read"
    />
  ) : (
    <CheckIcon className={cn("inline size-3.5 shrink-0", className)} aria-label={t`Sent`} data-testid="ticks-sent" />
  )
}

export function MessageActions({ m, ctx, className }: { m: Message; ctx: ThreadContext; className?: string }) {
  const { t } = useLingui()
  const { membership } = useSession()
  const mates = useAssignableMembers(ctx.inboxId).filter((x) => x.id !== membership.member_id)
  const removable = !!ctx.onRemove && !!ctx.canRemove?.(m)
  const mail = !!ctx.onForwardMail && m.kind === "message"
  if (!ctx.onQuote && !ctx.onForward && !mail && !removable) return null
  const text = m.body.trim()
  const copy = () =>
    navigator.clipboard.writeText(text).then(
      () => toast(t`Copied`),
      () => toast(t`Could not copy`)
    )
  const link =
    "rounded-sm text-caption text-faint underline-offset-2 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring aria-expanded:text-foreground"
  return (
    <div className={cn("flex items-center gap-3 px-1", className)} data-testid="message-actions">
      {ctx.onQuote && text && (
        <button
          type="button"
          className={link}
          onClick={() => ctx.onQuote?.(m)}
          title={t`Reply with a quote`}
          data-testid="message-quote"
        >
          <Trans>Quote</Trans>
        </button>
      )}
      {text && (
        <button type="button" className={link} onClick={copy} data-testid="message-copy">
          <Trans>Copy</Trans>
        </button>
      )}
      {removable && (
        <button
          type="button"
          className={cn(link, "hover:text-destructive")}
          onClick={() => ctx.onRemove?.(m)}
          title={t`Remove this message (Delete)`}
          data-testid="message-remove"
        >
          <Trans>Remove</Trans>
        </button>
      )}
      {(ctx.onForward || mail) && (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<button type="button" className={link} />}
            title={t`Forward`}
            data-testid="message-forward"
          >
            <Trans>Forward</Trans>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="max-h-72 w-60 overflow-y-auto">
            {mail && (
              <DropdownMenuGroup>
                <DropdownMenuItem onClick={() => ctx.onForwardMail?.(m)} data-testid="forward-mail">
                  <span className="flex-1">
                    <Trans>Forward by e-mail…</Trans>
                  </span>
                  <Kbd>{SHORTCUTS.forward}</Kbd>
                </DropdownMenuItem>
              </DropdownMenuGroup>
            )}
            {mail && ctx.onForward && <DropdownMenuSeparator />}
            {ctx.onForward && (
              <DropdownMenuGroup>
                <DropdownMenuLabel>
                  <Trans>Forward as a note mentioning</Trans>
                </DropdownMenuLabel>
                {mates.length === 0 && (
                  <DropdownMenuItem disabled data-testid="forward-none">
                    <Trans>No teammate can see this inbox</Trans>
                  </DropdownMenuItem>
                )}
                {mates.map((x) => (
                  <DropdownMenuItem key={x.id} onClick={() => ctx.onForward?.(m, x)} data-testid="forward-to">
                    <MemberAvatar
                      name={x.name || x.email}
                      online={x.online}
                      away={x.availability === "away"}
                      className="size-5"
                    />
                    <span className="truncate">{x.name || x.email}</span>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  )
}

const dotted = /[İIı]/

export function foldText(text: string) {
  let out = ""
  for (const ch of text) {
    const low = dotted.test(ch) ? "i" : ch.toLowerCase()
    out += low.length === ch.length ? low : ch
  }
  return out
}

export function Highlight({ text, query }: { text: string; query?: string }) {
  const q = query?.trim() ? foldText(query.trim()) : ""
  if (!q) return <>{text}</>
  const folded = foldText(text)
  const parts: React.ReactNode[] = []
  let from = 0
  for (let at = folded.indexOf(q); at >= 0; at = folded.indexOf(q, at + q.length)) {
    parts.push(text.slice(from, at))
    parts.push(
      <mark key={at} className="rounded-sm bg-warning/35 text-inherit" data-testid="find-hit">
        {text.slice(at, at + q.length)}
      </mark>
    )
    from = at + q.length
  }
  parts.push(text.slice(from))
  return <>{parts}</>
}
