import { useCallback, useMemo } from "react"
import { useSearchParams } from "react-router"

import { fromViewFilters } from "@/features/inbox/views/filters"
import type { ConversationFilters } from "@/lib/keys"

export type ActiveView = {
  id: string | null
  base: ConversationFilters | null
  q: string
  sla: boolean
  clear: (keepOrigin?: boolean) => void
}

export function useActiveView(): ActiveView {
  const [params, setParams] = useSearchParams()
  const id = params.get("view")
  const clear = useCallback(
    (keepOrigin?: boolean) => {
      setParams(
        (prev) => {
          const next = new URLSearchParams()
          const origin = prev.get("origin")
          if (keepOrigin && origin) next.set("origin", origin)
          return next
        },
        { replace: true }
      )
    },
    [setParams]
  )
  return useMemo(() => {
    if (!id) return { id: null, base: null, q: "", sla: false, clear }
    const rest = Object.fromEntries([...params].filter(([k]) => k !== "view"))
    const { q, sort, ...base } = fromViewFilters(rest)
    return { id, base, q: q ?? "", sla: sort === "sla_due", clear }
  }, [id, params, clear])
}
