import { Trans, useLingui } from "@lingui/react/macro"
import { useLocation, useMatch } from "react-router"

import { ContactAvatar } from "@/components/common"
import { useQueue } from "@/features/inbox/queue"
import { useIsPhone } from "@/hooks/use-media-query"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export function useShownId() {
  const { waiting, currentId } = useQueue()
  const { pathname } = useLocation()
  const match = useMatch("/conversations/:conversationId")
  if (match) return match.params.conversationId ?? null
  if (pathname !== "/") return null
  return currentId ?? waiting[0]?.id ?? null
}

export function WaitingPill() {
  const { t } = useLingui()
  const { waiting, show } = useQueue()
  const inboxes = useInboxes().data ?? []
  const shown = useShownId()
  const phone = useIsPhone()
  const max = phone ? 3 : 8
  const n = waiting.length
  if (n === 0) {
    return (
      <div className="justify-self-center rounded-full border bg-card px-3.5 py-1.5 text-sm text-muted-foreground" data-testid="waiting-pill">
        <Trans>Nobody is waiting</Trans>
      </div>
    )
  }
  const more = n - max
  return (
    <div
      className="flex min-w-0 items-center gap-2.5 justify-self-center rounded-full border bg-card py-1 ps-1 pe-3.5 text-sm text-muted-foreground phone:gap-2 phone:py-[3px] phone:ps-[3px] phone:pe-2.5"
      data-testid="waiting-pill"
    >
      <div className="flex shrink-0">
        {waiting.slice(0, max).map((c) => {
          const name = c.contact.name || c.contact.email || t`Unnamed contact`
          const inbox = inboxes.find((i) => i.id === c.inbox_id)?.name
          return (
            <button
              key={c.id}
              type="button"
              onClick={() => show(c.id)}
              title={inbox ? `${name} · ${inbox}` : name}
              aria-label={t`Open ${name}`}
              className={cn(
                "relative -ms-[3px] rounded-full ring-2 ring-card transition-transform first:ms-0 hover:z-10 hover:-translate-y-px",
                c.id === shown && "z-[1] ring-brand",
              )}
            >
              <ContactAvatar id={c.contact.id} name={name} className="size-7 text-[10px]" />
            </button>
          )
        })}
        {more > 0 && (
          <span className="relative -ms-[3px] grid size-7 place-items-center rounded-full bg-muted text-[10px] font-semibold text-muted-foreground ring-2 ring-card">
            +{more}
          </span>
        )}
      </div>
      <span className="truncate phone:text-[13px]">
        <Trans>
          <b className="font-semibold text-foreground">{n}</b> waiting for you
        </Trans>
      </span>
    </div>
  )
}
