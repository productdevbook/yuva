import { Trans, useLingui } from "@lingui/react/macro"
import { useMemo } from "react"

import { Command, CommandItem, CommandList } from "@/components/ui/command"
import { popupClass } from "@/components/ui/dropdown-menu"
import type { CannedReply, Macro } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useCannedReplies, useMacros } from "@/lib/workspace"

export type ReplyChoice =
  | { kind: "canned"; id: string; shortcut: string; title: string; body: string }
  | { kind: "macro"; id: string; shortcut: string; title: string; body?: string; macro: Macro }

export function useReplyChoices(): ReplyChoice[] {
  const canned = useCannedReplies().data
  const macros = useMacros().data
  return useMemo(
    () => [
      ...(canned ?? []).map((r: CannedReply): ReplyChoice => ({ kind: "canned", ...r })),
      ...(macros ?? []).map((m): ReplyChoice => ({
        kind: "macro",
        id: m.id,
        shortcut: m.shortcut,
        title: m.name,
        body: m.body,
        macro: m,
      })),
    ],
    [canned, macros]
  )
}

export function slashToken(text: string, caret: number) {
  const m = /(^|\s)\/([a-z0-9-]*)$/.exec(text.slice(0, caret))
  if (!m) return null
  return { start: caret - m[2].length - 1, query: m[2] }
}

export function useCannedMatches(query: string | null) {
  const choices = useReplyChoices()
  return useMemo(() => {
    if (query === null) return []
    const q = query.toLowerCase()
    return choices.filter((c) => c.shortcut.startsWith(q) || c.title.toLowerCase().includes(q)).slice(0, 8)
  }, [choices, query])
}

export function CannedMenu({
  items,
  active,
  onPick,
}: {
  items: ReplyChoice[]
  active: number
  onPick: (c: ReplyChoice) => void
}) {
  const { t } = useLingui()
  return (
    <Command
      value={items[active] ? `${items[active].kind}:${items[active].id}` : ""}
      shouldFilter={false}
      label={t`Canned replies`}
      className={cn(
        popupClass,
        "absolute start-0 bottom-full z-20 mb-2 h-auto max-h-64 w-full max-w-md rounded-xl! p-1.5"
      )}
    >
      <CommandList className="max-h-60">
        {items.map((c) => (
          <CommandItem
            key={`${c.kind}:${c.id}`}
            value={`${c.kind}:${c.id}`}
            onSelect={() => onPick(c)}
            onMouseDown={(e) => e.preventDefault()}
            className="flex flex-col items-start gap-0.5 rounded-lg px-2.5 py-1.5 data-selected:bg-muted"
          >
            <span className="flex w-full items-center gap-2">
              <span className="font-mono text-caption text-faint">/{c.shortcut}</span>
              <span className="truncate font-medium">{c.title}</span>
              {c.kind === "macro" && (
                <span className="ms-auto shrink-0 rounded-md bg-muted px-1.5 text-caption text-muted-foreground">
                  <Trans>Macro</Trans>
                </span>
              )}
            </span>
            <span className="line-clamp-1 text-caption text-muted-foreground">
              {c.body ?? <Trans>Actions only</Trans>}
            </span>
          </CommandItem>
        ))}
      </CommandList>
    </Command>
  )
}
