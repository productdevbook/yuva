import { useLingui } from "@lingui/react/macro"
import { useMemo } from "react"

import { popupClass } from "@/components/ui/dropdown-menu"
import type { CannedReply } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useCannedReplies } from "@/lib/workspace"

export function slashToken(text: string, caret: number) {
  const m = /(^|\s)\/([a-z0-9-]*)$/.exec(text.slice(0, caret))
  if (!m) return null
  return { start: caret - m[2].length - 1, query: m[2] }
}

export function useCannedMatches(query: string | null) {
  const canned = useCannedReplies().data ?? []
  return useMemo(() => {
    if (query === null) return []
    const q = query.toLowerCase()
    return canned.filter((c) => c.shortcut.startsWith(q) || c.title.toLowerCase().includes(q)).slice(0, 8)
  }, [canned, query])
}

export function CannedMenu({ items, active, onPick }: { items: CannedReply[]; active: number; onPick: (c: CannedReply) => void }) {
  const { t } = useLingui()
  return (
    <ul role="listbox" aria-label={t`Canned replies`} className={cn(popupClass, "absolute start-0 bottom-full z-20 mb-2 max-h-64 w-full max-w-md")}>
      {items.map((c, i) => (
        <li key={c.id} role="option" aria-selected={i === active}>
          <button
            type="button"
            onMouseDown={(e) => {
              e.preventDefault()
              onPick(c)
            }}
            className={cn("flex w-full flex-col items-start gap-0.5 rounded-lg px-2.5 py-1.5 text-start text-sm", i === active && "bg-muted")}
          >
            <span className="flex w-full items-center gap-2">
              <span className="font-mono text-xs text-faint">/{c.shortcut}</span>
              <span className="truncate font-medium">{c.title}</span>
            </span>
            <span className="line-clamp-1 text-xs text-muted-foreground">{c.body}</span>
          </button>
        </li>
      ))}
    </ul>
  )
}
