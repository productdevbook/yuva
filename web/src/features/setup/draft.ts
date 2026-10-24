import { useCallback } from "react"
import { useQueryClient } from "@tanstack/react-query"

import { useWizardDraft } from "@/hooks/use-wizard-draft"
import type { SetupKind } from "@/features/setup/setup"
import { browserLanguage, browserTimeZone } from "@/features/setup/setup"
import { slugify } from "@/features/settings/inboxes/slugify"
import { api, ApiError, unwrap, type Inbox, type InboxPurpose } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { timeZones } from "@/lib/utils"

export type InboxDraft = {
  name: string
  locale: string
  zone: string
  color?: string
  mode?: "live" | "async"
  purpose?: InboxPurpose
  inboxId?: string
  kind?: SetupKind
}

function readInboxDraft(ws: string): Partial<InboxDraft> & { idempotencyKey?: string } {
  try {
    const raw = sessionStorage.getItem(`yuva:wizard:${ws}:inbox:new`)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

export function useInboxDraft() {
  return useWizardDraft<InboxDraft>("inbox:new", { name: "", locale: browserLanguage(), zone: browserTimeZone() })
}

export function useLeaveSetup() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [, , clear] = useInboxDraft()
  return useCallback(() => {
    const created = readInboxDraft(ws).inboxId
    clear()
    if (!created) return
    const path = { inboxId: created }
    void unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path } }))
      .then((r) =>
        r.items.length === 0 ? unwrap(api.DELETE("/v1/inboxes/{inboxId}", { params: { path } })) : undefined
      )
      .catch(() => undefined)
      .finally(() => qc.invalidateQueries({ queryKey: keys.inboxes(ws) }))
  }, [clear, qc, ws])
}

export function idempotencyKey(key: string, part: string, body: unknown) {
  let h = 5381
  for (const c of JSON.stringify(body)) h = ((h * 33) ^ c.charCodeAt(0)) >>> 0
  return `${key}:${part}:${h.toString(36)}`
}

export function useEnsureInbox() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [draft, patch] = useInboxDraft()
  return useCallback(async (): Promise<{ id: string; secret?: string }> => {
    const d = { ...draft, ...readInboxDraft(ws) }
    if (d.inboxId) return { id: d.inboxId }
    const zones = timeZones()
    const base = slugify(d.name) || "inbox"
    const body = (slug: string) => ({
      name: d.name.trim(),
      slug,
      mode: d.mode ?? ("async" as const),
      ask_for_rating: false,
      ...(d.purpose === "mail" ? { purpose: "mail" as const } : {}),
      ...(d.color ? { branding: { color: d.color } } : {}),
      default_locale: d.locale,
      timezone: zones.includes(d.zone) ? d.zone : browserTimeZone(),
    })
    const post = (slug: string) =>
      unwrap(
        api.POST("/v1/inboxes", {
          params: { header: { "Idempotency-Key": idempotencyKey(d.idempotencyKey, "inbox", body(slug)) } },
          body: body(slug),
        })
      )
    let created
    try {
      created = await post(base)
    } catch (err) {
      if (!(err instanceof ApiError) || err.status !== 409 || err.code === "idempotency_key_in_use") throw err
      created = await post(`${base}-${Math.random().toString(36).slice(2, 6)}`)
    }
    patch({ inboxId: created.inbox.id })
    qc.setQueryData([...keys.inboxes(ws), created.inbox.id], created.inbox satisfies Inbox)
    void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
    return { id: created.inbox.id, secret: created.identity_secret }
  }, [draft, patch, qc, ws])
}
