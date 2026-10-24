import { useRef } from "react"

import { useResetState } from "@/hooks/use-reset-state"

export function useSelection(ids: string[], scope: unknown = "") {
  const [selecting, setSelecting] = useResetState(false, scope)
  const [picked, setPicked] = useResetState<ReadonlySet<string>>(new Set(), scope)
  const anchor = useRef<string | null>(null)
  const chosen = ids.filter((id) => picked.has(id))
  const all = ids.length > 0 && chosen.length === ids.length
  return {
    selecting,
    chosen,
    count: chosen.length,
    all,
    has: (id: string) => picked.has(id),
    start: () => setSelecting(true),
    stop: () => {
      setSelecting(false)
      setPicked(new Set())
      anchor.current = null
    },
    toggle: (id: string, range = false) => {
      setSelecting(true)
      const from = anchor.current ? ids.indexOf(anchor.current) : -1
      const to = ids.indexOf(id)
      setPicked((cur) => {
        const next = new Set(cur)
        if (range && from >= 0 && to >= 0) {
          for (let i = Math.min(from, to); i <= Math.max(from, to); i++) next.add(ids[i])
        } else if (!next.delete(id)) next.add(id)
        return next
      })
      anchor.current = id
    },
    toggleAll: () => setPicked(all ? new Set() : new Set(ids)),
  }
}

export type Selection = ReturnType<typeof useSelection>
