import { useQuery } from "@tanstack/react-query"
import { useCallback, useEffect, useRef, useState } from "react"

import { designFields, designPatch, pickKey, type DesignPick } from "@/components/common/designChoice"
import { useLatest } from "@/hooks/use-latest"
import { api, unwrap, type Message } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { newId } from "@/lib/utils"

export const AUTOSAVE_MS = 1500

export function isOwnDraft(m: Message, memberId: string) {
  return (
    m.draft && m.kind === "message" && m.author.type === "member" && m.author.member_id === memberId && !m.author.via
  )
}

export function isShownDraft(m: Message, memberId: string) {
  return isOwnDraft(m, memberId) && (!!m.scheduled_at || !!m.send_error)
}

type Saved = { id: string; unsigned: boolean; body: string; design: string }

const saved = new Map<string, Saved>()

export function savedDraftId(conversationId: string) {
  return saved.get(conversationId)?.id
}

export function takeSavedDraft(conversationId: string) {
  const rec = saved.get(conversationId)
  saved.delete(conversationId)
  return rec?.id
}

export function adoptDraft(conversationId: string, id: string, body: string, unsigned = false, design = "") {
  const now = saved.get(conversationId)
  if (now && now.id !== id) {
    void api.DELETE("/v1/messages/{messageId}", { params: { path: { messageId: id } } }).catch(() => undefined)
    return
  }
  saved.set(conversationId, { id, unsigned, body: body.trim(), design })
}

export function useOwnDrafts(conversationId: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.draftLists(ws), "conversation", conversationId] as const,
    queryFn: () =>
      unwrap(api.GET("/v1/drafts", { params: { query: { conversation_id: conversationId, mine: true, limit: 50 } } })),
    enabled,
  })
}

export type SaveState = "idle" | "saving" | "saved" | "failed"

const waiting = new Map<string, { timer: ReturnType<typeof setTimeout>; fire: () => void }>()

export function useReplyAutosave(
  conversationId: string,
  enabled: boolean,
  body: string,
  unsigned: boolean,
  design?: DesignPick
) {
  const [state, setState] = useState<SaveState>("idle")
  const designId = pickKey(design)
  const latest = useLatest({ body, unsigned, enabled, conversationId, design })
  const chain = useRef<Promise<unknown>>(Promise.resolve())

  const run = useCallback(async () => {
    const { body: raw, unsigned: noSig, conversationId: id, design: pick } = latest.current
    const dk = pickKey(pick)
    const text = raw.trim()
    const rec = saved.get(id)
    if (text === "") {
      if (!rec) return
      saved.delete(id)
      await api.DELETE("/v1/messages/{messageId}", { params: { path: { messageId: rec.id } } })
      return
    }
    if (rec && rec.unsigned === noSig) {
      if (rec.body === text && rec.design === dk) return
      await unwrap(
        api.PATCH("/v1/messages/{messageId}", {
          params: { path: { messageId: rec.id } },
          body: { body: text, ...(rec.design === dk ? {} : designPatch(pick)) },
        })
      )
      saved.set(id, { ...rec, body: text, design: dk })
      return
    }
    const created = await unwrap(
      api.POST("/v1/conversations/{conversationId}/messages", {
        params: { path: { conversationId: id } },
        body: {
          kind: "message",
          body: text,
          draft: true,
          client_id: newId(),
          signature: noSig ? false : undefined,
          ...designFields(pick),
        },
      })
    )
    saved.set(id, { id: created.id, unsigned: noSig, body: text, design: dk })
    if (rec) await api.DELETE("/v1/messages/{messageId}", { params: { path: { messageId: rec.id } } })
  }, [latest])

  const save = useCallback(() => {
    setState("saving")
    const next = chain.current
      .catch(() => undefined)
      .then(run)
      .then(
        () => setState(saved.has(latest.current.conversationId) ? "saved" : "idle"),
        (e: unknown) => {
          setState("failed")
          throw e
        }
      )
    chain.current = next
    return next
  }, [run, latest])

  useEffect(() => {
    if (!enabled) return
    const rec = saved.get(conversationId)
    if (body.trim() === "" && !rec) return
    if (rec && rec.body === body.trim() && rec.unsigned === unsigned && rec.design === designId) return
    const fire = () => {
      waiting.delete(conversationId)
      void save().catch(() => undefined)
    }
    const timer = setTimeout(fire, AUTOSAVE_MS)
    waiting.set(conversationId, { timer, fire })
    return () => clearTimeout(timer)
  }, [conversationId, enabled, body, unsigned, designId, save])

  useEffect(
    () => () => {
      const w = waiting.get(conversationId)
      if (!w) return
      clearTimeout(w.timer)
      w.fire()
    },
    [conversationId]
  )

  const flush = useCallback(async () => {
    const id = latest.current.conversationId
    const w = waiting.get(id)
    if (w) {
      clearTimeout(w.timer)
      waiting.delete(id)
    }
    await save()
    return saved.get(id)?.id
  }, [save, latest])

  return { state, flush }
}
