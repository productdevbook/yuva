import { useState } from "react"

import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useHotkeys } from "@/hooks/use-hotkeys"

export function useListCursor(
  rows: { id: string }[],
  currentId: string | null,
  onOpen: (id: string) => void,
  active: boolean,
  scroller: React.RefObject<HTMLElement | null>,
  arrows = true,
  browse = false
) {
  const [cursorId, setCursorId] = useState<string | null>(null)
  const reveal = (id: string) =>
    requestAnimationFrame(() =>
      scroller.current?.querySelector(`[data-conversation-id="${id}"]`)?.scrollIntoView({ block: "nearest" })
    )
  const index = (id: string | null) => (id ? rows.findIndex((c) => c.id === id) : -1)
  const moveCursor = (d: 1 | -1) => {
    if (rows.length === 0) return
    const from = index(cursorId) >= 0 ? index(cursorId) : index(currentId)
    const next = rows[Math.min(rows.length - 1, Math.max(0, from < 0 ? (d === 1 ? 0 : rows.length - 1) : from + d))]
    setCursorId(next.id)
    reveal(next.id)
  }
  const openCursor = () => {
    const id = cursorId && index(cursorId) >= 0 ? cursorId : rows[0]?.id
    if (!id) return
    setCursorId(null)
    onOpen(id)
  }
  const step = (d: 1 | -1) => {
    if (rows.length === 0) return
    const i = index(currentId)
    const next = rows[Math.min(rows.length - 1, Math.max(0, i < 0 ? 0 : i + d))]
    setCursorId(null)
    onOpen(next.id)
    reveal(next.id)
  }
  useHotkeys(
    {
      ...(arrows
        ? {
            ArrowDown: () => moveCursor(1),
            ArrowUp: () => moveCursor(-1),
            Enter: () => cursorId && openCursor(),
          }
        : {}),
      ...(browse
        ? {
            [SHORTCUTS.next]: () => moveCursor(1),
            [SHORTCUTS.previous]: () => moveCursor(-1),
            Enter: () => openCursor(),
            o: () => openCursor(),
          }
        : {
            [SHORTCUTS.next]: () => step(1),
            [SHORTCUTS.previous]: () => step(-1),
          }),
    },
    active
  )
  return { cursorId, setCursorId, moveCursor, openCursor }
}
