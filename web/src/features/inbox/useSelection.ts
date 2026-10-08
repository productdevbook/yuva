import { useRef, useState } from "react"

export function useSelection(ids: string[]) {
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set())
  const anchor = useRef<string | null>(null)
  const selection = ids.filter((id) => picked.has(id))

  const toggle = (id: string, on: boolean, range: boolean) => {
    const from = range && anchor.current ? ids.indexOf(anchor.current) : -1
    const to = ids.indexOf(id)
    const span = from === -1 ? [id] : ids.slice(Math.min(from, to), Math.max(from, to) + 1)
    setPicked((prev) => {
      const next = new Set(prev)
      for (const x of span) {
        if (on) next.add(x)
        else next.delete(x)
      }
      return next
    })
    anchor.current = id
  }
  const set = (next: Iterable<string>) => {
    setPicked(new Set(next))
    anchor.current = null
  }
  return { picked, selection, toggle, set }
}
