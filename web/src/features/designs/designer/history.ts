import { useCallback, useReducer } from "react"

import type { Doc } from "@/features/designs/doc"

export type DesignState = { name: string; inbox_id?: string; for_conversations?: boolean; document: Doc }

export type Change = DesignState | ((s: DesignState) => DesignState)

const LIMIT = 100
const COALESCE_MS = 1000

type History = {
  past: DesignState[]
  present: DesignState
  future: DesignState[]
  key: string | null
  at: number
}

type Action =
  | { type: "commit"; next: Change; key?: string; at: number }
  | { type: "undo" }
  | { type: "redo" }
  | { type: "reset"; state: DesignState }

function reduce(h: History, a: Action): History {
  switch (a.type) {
    case "commit": {
      const next = typeof a.next === "function" ? a.next(h.present) : a.next
      if (next === h.present) return h
      const coalesce = !!a.key && a.key === h.key && a.at - h.at < COALESCE_MS
      return {
        past: coalesce ? h.past : [...h.past, h.present].slice(-LIMIT),
        present: next,
        future: [],
        key: a.key ?? null,
        at: coalesce ? h.at : a.at,
      }
    }
    case "undo": {
      const prev = h.past.at(-1)
      if (!prev) return h
      return { past: h.past.slice(0, -1), present: prev, future: [h.present, ...h.future], key: null, at: 0 }
    }
    case "redo": {
      const next = h.future[0]
      if (!next) return h
      return { past: [...h.past, h.present], present: next, future: h.future.slice(1), key: null, at: 0 }
    }
    case "reset":
      return { past: [], present: a.state, future: [], key: null, at: 0 }
  }
}

export function useHistory(initial: DesignState) {
  const [h, dispatch] = useReducer(reduce, undefined, () => ({
    past: [],
    present: initial,
    future: [],
    key: null,
    at: 0,
  }))
  const commit = useCallback(
    (next: Change, key?: string) => dispatch({ type: "commit", next, key, at: performance.now() }),
    []
  )
  const undo = useCallback(() => dispatch({ type: "undo" }), [])
  const redo = useCallback(() => dispatch({ type: "redo" }), [])
  const reset = useCallback((state: DesignState) => dispatch({ type: "reset", state }), [])
  return {
    state: h.present,
    commit,
    undo,
    redo,
    reset,
    canUndo: h.past.length > 0,
    canRedo: h.future.length > 0,
  }
}
