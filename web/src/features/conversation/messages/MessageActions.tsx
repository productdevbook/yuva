import { Trans, useLingui } from "@lingui/react/macro"
import { CheckCheckIcon, CheckIcon, CopyIcon, ForwardIcon, ReplyIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { MemberAvatar, toast } from "@/components/common"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import type { ThreadContext } from "@/features/conversation/messages/context"
import { useMediaQuery } from "@/hooks/use-media-query"
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
    <CheckCheckIcon className={cn("inline size-3.5 shrink-0 text-brand", className)} aria-label={t`Read`} data-testid="ticks-read" />
  ) : (
    <CheckIcon className={cn("inline size-3.5 shrink-0", className)} aria-label={t`Sent`} data-testid="ticks-sent" />
  )
}

export function MessageActions({ m, ctx, className }: { m: Message; ctx: ThreadContext; className?: string }) {
  const { t } = useLingui()
  const { membership } = useSession()
  const mates = useAssignableMembers(ctx.inboxId).filter((x) => x.id !== membership.member_id)
  const touch = useMediaQuery("(pointer: coarse)")
  const [tapped, setTapped] = useState(false)
  const row = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!touch) return
    const frame = row.current?.closest<HTMLElement>("[data-slot=message]")
    if (!frame) return
    const busy = () => !!row.current?.querySelector("[aria-expanded=true]")
    const down = (e: PointerEvent) => {
      if (!busy()) setTapped(frame.contains(e.target as Node))
    }
    const blur = () =>
      setTimeout(() => {
        const a = document.activeElement
        if (a instanceof HTMLIFrameElement && !busy()) setTapped(frame.contains(a))
      })
    document.addEventListener("pointerdown", down, true)
    window.addEventListener("blur", blur)
    return () => {
      document.removeEventListener("pointerdown", down, true)
      window.removeEventListener("blur", blur)
    }
  }, [touch])
  if (!ctx.onQuote && !ctx.onForward) return null
  const text = m.body.trim()
  const copy = () =>
    navigator.clipboard.writeText(text).then(
      () => toast(t`Copied`),
      () => toast(t`Could not copy`),
    )
  const btn = "size-7 rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
  return (
    <div
      ref={row}
      className={cn(
        !touch && "contents",
        touch && !tapped && "hidden",
        touch &&
          tapped &&
          "order-first flex basis-full group-data-[align=end]/message:justify-end group-data-[align=end]/message:pe-[34px] group-data-[align=start]/message:ps-[34px]",
      )}
    >
      <span
        className={cn(
          "z-10 flex shrink-0 items-center gap-0.5 self-center rounded-full border bg-background p-0.5 shadow-sm",
          !touch && "opacity-0 transition-opacity group-focus-within/message:opacity-100 group-[:hover]/message:opacity-100 has-aria-expanded:opacity-100",
          className,
        )}
        data-testid="message-actions"
      >
        {ctx.onQuote && text && (
          <Button variant="ghost" size="icon-xs" className={btn} onClick={() => ctx.onQuote?.(m)} aria-label={t`Reply with a quote`} title={t`Reply with a quote`} data-testid="message-quote">
            <ReplyIcon />
          </Button>
        )}
        {text && (
          <Button variant="ghost" size="icon-xs" className={btn} onClick={copy} aria-label={t`Copy`} title={t`Copy`} data-testid="message-copy">
            <CopyIcon />
          </Button>
        )}
        {ctx.onForward && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" size="icon-xs" className={btn} />}
              aria-label={t`Forward to a teammate`}
              title={t`Forward to a teammate`}
              data-testid="message-forward"
            >
              <ForwardIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="max-h-72 w-60 overflow-y-auto">
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
                    <MemberAvatar name={x.name || x.email} online={x.online} away={x.availability === "away"} className="size-5" />
                    <span className="truncate">{x.name || x.email}</span>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </span>
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
      </mark>,
    )
    from = at + q.length
  }
  parts.push(text.slice(from))
  return <>{parts}</>
}
