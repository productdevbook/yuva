import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ClockIcon } from "lucide-react"
import { useNavigate } from "react-router"

import { formatShort, useEnumText } from "@/components/common/text"
import type { ConversationListItem } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Item } from "@/components/ui/item"

const toggle = "justify-self-center text-faint hover:bg-card"

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
      <Button variant="ghost" size="sm" className={toggle} onClick={onToggle} aria-expanded={false} data-testid="history-toggle">
        <ClockIcon />
        <Plural value={n} one="# other conversation" other="# other conversations" />
      </Button>
    )
  }
  const first = name.split(" ")[0]
  return (
    <div className="mb-2 grid gap-2" data-testid="history">
      {others.map((o) => {
        const date = formatShort(o.last_activity_at, i18n.locale)
        const who = o.last_message?.author_type === "contact" ? first : t`Team`
        return (
          <Item render={<button type="button" />} variant="outline" size="sm" className="block border-dashed text-small text-muted-foreground hover:border-solid [button]:hover:bg-card"
            key={o.id}
            onClick={() => navigate(`/conversations/${o.id}`)}
          >
            <span className="mb-1.5 flex items-baseline gap-2">
              <b className="min-w-0 truncate font-medium text-foreground">{o.subject || <Trans>No subject</Trans>}</b>
              <span className="ms-auto shrink-0 text-caption text-faint">
                {date} · {text.status[o.status]}
              </span>
            </span>
            {o.last_message && (
              <span className="line-clamp-2">
                <span className="text-faint">{who}:</span> {o.last_message.text}
              </span>
            )}
          </Item>
        )
      })}
      <Button variant="ghost" size="sm" className={toggle} onClick={onToggle} aria-expanded data-testid="history-toggle">
        <Trans>Hide</Trans>
      </Button>
    </div>
  )
}
