import { keepPreviousData, useQuery } from "@tanstack/react-query"

import { api, ApiError, unwrap, type ConversationDesignRender, type EmailDesignSummary, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"

export type DesignPick = { id: string; name: string; locale?: string }

export function pickKey(d: DesignPick | undefined) {
  return d ? `${d.id}|${d.locale ?? ""}` : ""
}

export function pickOf(m: Pick<Message, "design">): DesignPick | undefined {
  const d = m.design
  return d?.id ? { id: d.id, name: d.name, locale: d.locale } : undefined
}

export function designFields(d: DesignPick | undefined) {
  return d ? { design_id: d.id, design_locale: d.locale } : {}
}

export function designPatch(d: DesignPick | undefined) {
  return { design_id: d?.id ?? null, design_locale: d?.locale ?? null }
}

export function blocksSend(error: unknown) {
  return error instanceof ApiError && error.status >= 400 && error.status < 500 && error.status !== 429
}

const key = (ws: string, ...rest: unknown[]) => ["ws", ws, "designs", "conversation", ...rest] as const

export function useOfferedDesigns(inboxId: string, q: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: key(ws, "offered", inboxId, q),
    queryFn: () =>
      unwrap(
        api.GET("/v1/email-designs", {
          params: { query: { for_conversations: true, inbox_id: inboxId, q: q || undefined, limit: 50 } },
        })
      ).then((r) => r.items as EmailDesignSummary[]),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function previewDesign(
  conversationId: string,
  pick: DesignPick,
  body: string | undefined,
  dark: boolean,
  signal?: AbortSignal
): Promise<ConversationDesignRender> {
  return unwrap(
    api.POST("/v1/conversations/{conversationId}/design-preview", {
      params: { path: { conversationId } },
      body: { design_id: pick.id, design_locale: pick.locale, body: body || undefined, dark: dark || undefined },
      signal,
    })
  )
}

const retry = (n: number, err: unknown) =>
  n < 2 && (!(err instanceof ApiError) || err.status === 429 || err.status >= 500)

export function useDesignCheck(conversationId: string | undefined, pick: DesignPick | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: key(ws, "check", conversationId, pickKey(pick)),
    queryFn: ({ signal }) => previewDesign(conversationId!, pick!, undefined, false, signal),
    enabled: !!conversationId && !!pick,
    staleTime: 60_000,
    gcTime: 60_000,
    retry,
    retryDelay: (n, err) => (err instanceof ApiError && err.status === 429 ? 15_000 : 1000 * 2 ** n),
  })
}

export function useDesignPreview(
  conversationId: string | undefined,
  pick: DesignPick | undefined,
  body: string,
  dark: boolean,
  enabled: boolean
) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: key(ws, "preview", conversationId, pickKey(pick), body, dark),
    queryFn: ({ signal }) => previewDesign(conversationId!, pick!, body, dark, signal),
    enabled: enabled && !!conversationId && !!pick,
    placeholderData: keepPreviousData,
    staleTime: Infinity,
    gcTime: 60_000,
    retry,
    retryDelay: (n, err) => (err instanceof ApiError && err.status === 429 ? 15_000 : 1000 * 2 ** n),
  })
}

export function useDesignBlocked(conversationId: string | undefined, pick: DesignPick | undefined) {
  const check = useDesignCheck(conversationId, pick)
  return !!pick && check.isError && blocksSend(check.error)
}

const DESIGN_CODES = new Set([
  "email_opted_out",
  "sender_identity_missing",
  "design_locale_unknown",
  "design_unavailable",
  "design_not_emailed",
  "design_one_recipient",
  "invalid_design",
  "html_too_large",
  "broadcasts_approval_required",
  "broadcasts_suspended",
])

export function isDesignError(error: unknown) {
  return error instanceof ApiError && !!error.code && DESIGN_CODES.has(error.code)
}
