import { useSyncExternalStore } from "react"

import { api, ApiError, currentWorkspace, unwrap } from "@/lib/api"

export type SsoPrompt = { kind: "required"; email: string } | { kind: "scope"; workspaceId: string }

let prompt: SsoPrompt | null = null
const listeners = new Set<() => void>()

function show(next: SsoPrompt | null) {
  prompt = next
  listeners.forEach((l) => l())
}

export function noteSsoRequired(err: unknown) {
  if (!(err instanceof ApiError) || err.status !== 403) return
  const body = err.body as { sso_email?: string; sso_workspace_id?: string } | undefined
  if (err.code === "sso_required" && body?.sso_email) {
    if (prompt?.kind !== "required" || prompt.email !== body.sso_email)
      show({ kind: "required", email: body.sso_email })
  } else if (
    err.code === "sso_session_scope" &&
    body?.sso_workspace_id &&
    body.sso_workspace_id !== currentWorkspace()
  ) {
    if (prompt?.kind !== "scope" || prompt.workspaceId !== body.sso_workspace_id) {
      show({ kind: "scope", workspaceId: body.sso_workspace_id })
    }
  }
}

export function clearSsoPrompt() {
  show(null)
}

export function useSsoPrompt() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => prompt
  )
}

export async function startSso(email: string, returnTo?: string) {
  return unwrap(api.POST("/v1/auth/sso", { body: { email, return_to: returnTo } }))
}
