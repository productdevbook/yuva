import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ClockIcon } from "lucide-react"
import { useNavigate } from "react-router"

import { formatShort, useEnumText } from "@/components/common/text"
import type { ConversationListItem } from "@/lib/api"

const toggle =
  "inline-flex items-center gap-1.5 justify-self-center rounded-full px-2.5 py-1 text-xs text-faint transition-colors hover:bg-card hover:text-foreground"

export function History({
  others,
  open,
  onToggle,
  name,
}: {
  others: ConversationListItem[]
  open: boolean
  onToggle: () => void
  name: string
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const navigate = useNavigate()
  if (others.length === 0) return null
  const n = others.length
  if (!open) {
    return (
      <button type="button" className={toggle} onClick={onToggle} aria-expanded={false} data-testid="history-toggle">
        <ClockIcon className="size-[13px]" />
        <Plural value={n} one="# other conversation" other="# other conversations" />
      </button>
    )
  }
  const first = name.split(" ")[0]
  return (
    <div className="mb-2 grid gap-2" data-testid="history">
      {others.map((o) => {
        const date = formatShort(o.last_activity_at, i18n.locale)
        const who = o.last_message?.author_type === "contact" ? first : t`Team`
        return (
          <button
            key={o.id}
            type="button"
            onClick={() => navigate(`/conversations/${o.id}`)}
            className="rounded-[14px] border border-dashed px-3.5 py-3 text-start text-[13px] text-muted-foreground transition-colors hover:border-solid hover:bg-card"
          >
            <span className="mb-1.5 flex items-baseline gap-2">
              <b className="min-w-0 truncate font-medium text-foreground">{o.subject || <Trans>No subject</Trans>}</b>
              <span className="ms-auto shrink-0 text-xs text-faint">
                {date} · {text.status[o.status]}
              </span>
            </span>
            {o.last_message && (
              <span className="line-clamp-2 leading-normal">
                <span className="text-faint">{who}:</span> {o.last_message.text}
              </span>
            )}
          </button>
        )
      })}
      <button type="button" className={toggle} onClick={onToggle} aria-expanded data-testid="history-toggle">
        <Trans>Hide</Trans>
      </button>
    </div>
  )
}
