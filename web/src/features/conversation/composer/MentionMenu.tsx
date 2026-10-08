import { useLingui } from "@lingui/react/macro"

import { MemberAvatar } from "@/components/common"
import { popupClass } from "@/components/ui/dropdown-menu"
import type { Member } from "@/lib/api"
import { cn } from "@/lib/utils"

export function mentionToken(text: string, caret: number) {
  const m = /(^|\s)@([^\s@]*)$/.exec(text.slice(0, caret))
  if (!m) return null
  return { start: caret - m[2].length - 1, query: m[2] }
}

export function MentionMenu({ items, active, onPick }: { items: Member[]; active: number; onPick: (m: Member) => void }) {
  const { t } = useLingui()
  return (
    <ul role="listbox" aria-label={t`Mention a teammate`} className={cn(popupClass, "absolute start-2.5 bottom-full z-20 mb-2 w-72 max-w-[calc(100%-20px)]")} data-testid="mention-menu">
      {items.map((m, i) => (
        <li key={m.id} role="option" aria-selected={i === active}>
          <button
            type="button"
            onMouseDown={(e) => {
              e.preventDefault()
              onPick(m)
            }}
            className={cn("flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-start text-sm", i === active && "bg-muted")}
          >
            <MemberAvatar name={m.name || m.email} online={m.online} away={m.availability === "away"} ring="ring-card" />
            <span className="min-w-0 flex-1">
              <span className="block truncate">{m.name || m.email}</span>
              <small className="block truncate text-xs text-faint">{m.email}</small>
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}
