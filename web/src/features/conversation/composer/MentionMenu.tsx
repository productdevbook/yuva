import { useLingui } from "@lingui/react/macro"

import { MemberAvatar } from "@/components/common"
import { Command, CommandItem, CommandList } from "@/components/ui/command"
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
    <Command
      value={items[active]?.id ?? ""}
      shouldFilter={false}
      label={t`Mention a teammate`}
      className={cn(popupClass, "absolute start-2.5 bottom-full z-20 mb-2 h-auto w-72 max-w-[calc(100%-20px)] rounded-xl! p-1.5")}
      data-testid="mention-menu"
    >
      <CommandList>
        {items.map((m) => (
          <CommandItem
            key={m.id}
            value={m.id}
            onSelect={() => onPick(m)}
            onMouseDown={(e) => e.preventDefault()}
            className="gap-2.5 rounded-lg px-2.5 py-1.5 data-selected:bg-muted"
          >
            <MemberAvatar name={m.name || m.email} online={m.online} away={m.availability === "away"} ring="ring-card" />
            <span className="min-w-0 flex-1">
              <span className="block truncate">{m.name || m.email}</span>
              <small className="block truncate text-xs text-faint">{m.email}</small>
            </span>
          </CommandItem>
        ))}
      </CommandList>
    </Command>
  )
}
