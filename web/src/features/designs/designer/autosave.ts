import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import type { DesignState } from "@/features/designs/designer/history"
import { useLatest } from "@/hooks/use-latest"
import { pruneTranslations } from "@/features/designs/doc"
import { conflictOf, issuesOf, saveDesign, useRefreshDesigns } from "@/features/designs/queries"
import { ApiError, type EmailDesign, type EmailDesignIssue } from "@/lib/api"

export type SaveStatus = "saved" | "dirty" | "saving" | "offline" | "invalid" | "conflict" | "error"

const DELAY = 1500

export function stateOf(d: EmailDesign): DesignState {
  return { name: d.name, inbox_id: d.inbox_id, document: d.document }
}

export function serialize(s: DesignState) {
  return JSON.stringify([s.name, s.inbox_id ?? null, s.document])
}

type Draft = { state: DesignState; base: number }

function draftKey(ws: string, id: string) {
  return `yuva.design.${ws}.${id}`
}

export function readDraft(ws: string, id: string): Draft | null {
  try {
    const raw = localStorage.getItem(draftKey(ws, id))
    return raw ? (JSON.parse(raw) as Draft) : null
  } catch {
    return null
  }
}

function writeDraft(ws: string, id: string, d: Draft | null) {
  try {
    if (d) localStorage.setItem(draftKey(ws, id), JSON.stringify(d))
    else localStorage.removeItem(draftKey(ws, id))
  } catch {
    return
  }
}

export function useAutosave({
  ws,
  design,
  base,
  state,
  enabled,
  initialConflict,
}: {
  ws: string
  design: EmailDesign
  base: number
  state: DesignState
  enabled: boolean
  initialConflict: EmailDesign | null
}) {
  const refresh = useRefreshDesigns()
  const json = useMemo(() => serialize(state), [state])
  const server = useRef({ version: base, json: serialize(stateOf(design)) })
  const latest = useLatest({ state, json })
  const id = design.id
  const fallbackName = useLatest(design.name)
  const flying = useRef(false)
  const [tick, setTick] = useState(0)
  const [status, setStatus] = useState<SaveStatus>(initialConflict ? "conflict" : "saved")
  const [errors, setErrors] = useState<EmailDesignIssue[]>([])
  const [problem, setProblem] = useState<ApiError | null>(null)
  const [warnings, setWarnings] = useState<EmailDesignIssue[]>(design.warnings)
  const [conflict, setConflict] = useState<EmailDesign | null>(initialConflict)
  const blocked = useRef(!!initialConflict)

  const flush = useCallback(
    async (snapshot = false) => {
      if (flying.current || blocked.current || !enabled) return
      const { state: s, json: sent } = latest.current
      if (sent === server.current.json && !snapshot) {
        setStatus("saved")
        return
      }
      flying.current = true
      setStatus("saving")
      try {
        const d = await saveDesign(id, {
          name: s.name.trim() || fallbackName.current,
          inbox_id: s.inbox_id,
          document: pruneTranslations(s.document),
          base_version: server.current.version,
          snapshot: snapshot || undefined,
        })
        server.current = { version: d.version, json: sent }
        setWarnings(d.warnings)
        setErrors([])
        setProblem(null)
        refresh(d)
        const clean = latest.current.json === sent
        if (clean) writeDraft(ws, id, null)
        setStatus(clean ? "saved" : "dirty")
      } catch (err) {
        const theirs = conflictOf(err)
        if (theirs) {
          blocked.current = true
          setConflict(theirs)
          setStatus("conflict")
        } else if (err instanceof ApiError && (err.status === 422 || err.status === 413)) {
          setErrors(issuesOf(err))
          setProblem(err)
          setStatus("invalid")
        } else if (!(err instanceof ApiError)) {
          setStatus("offline")
        } else {
          setProblem(err)
          setStatus("error")
        }
      } finally {
        flying.current = false
        setTick((n) => n + 1)
      }
    },
    [id, enabled, refresh, ws, latest, fallbackName]
  )

  useEffect(() => {
    if (!enabled || json === server.current.json) return
    writeDraft(ws, id, { state, base: server.current.version })
    if (blocked.current) return
    setStatus((s) => (s === "saving" || s === "invalid" || s === "offline" ? s : "dirty"))
    const timer = setTimeout(() => void flush(), DELAY)
    return () => clearTimeout(timer)
  }, [json, state, tick, enabled, flush, ws, id])

  useEffect(() => {
    const online = () => void flush()
    window.addEventListener("online", online)
    return () => window.removeEventListener("online", online)
  }, [flush])

  useEffect(() => () => void flush(), [flush])

  const resolve = {
    theirs: (d: EmailDesign) => {
      server.current = { version: d.version, json: serialize(stateOf(d)) }
      blocked.current = false
      setConflict(null)
      setWarnings(d.warnings)
      setErrors([])
      writeDraft(ws, id, null)
      setStatus("saved")
    },
    mine: (d: EmailDesign) => {
      server.current = { version: d.version, json: serialize(stateOf(d)) }
      blocked.current = false
      setConflict(null)
      setTick((n) => n + 1)
      void flush()
    },
    restored: (d: EmailDesign) => {
      server.current = { version: d.version, json: serialize(stateOf(d)) }
      setWarnings(d.warnings)
      setErrors([])
      refresh(d)
    },
  }

  const conflictWith = (d: EmailDesign) => {
    blocked.current = true
    setConflict(d)
    setStatus("conflict")
  }
  const version = () => server.current.version
  return { status, errors, problem, warnings, conflict, flush, resolve, version, conflictWith }
}
