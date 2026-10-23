import { useSyncExternalStore } from "react"

import { api, ApiError, unwrap } from "@/lib/api"

let required: string | null = null
const listeners = new Set<() => void>()

export function noteSsoRequired(err: unknown) {
  if (err instanceof ApiError && err.status === 403 && err.code === "sso_required") {
    const email = (err.body as { sso_email?: string } | undefined)?.sso_email
    if (email && email !== required) {
      required = email
      listeners.forEach((l) => l())
    }
  }
}

export function useSsoRequired() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => required
  )
}

export async function startSso(email: string, returnTo?: string) {
  return unwrap(api.POST("/v1/auth/sso", { body: { email, return_to: returnTo } }))
}
