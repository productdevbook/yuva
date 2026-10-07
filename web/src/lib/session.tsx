import { useQuery, useQueryClient } from "@tanstack/react-query"
import { createContext, useContext, useMemo, useState } from "react"

import { api, ApiError, setWorkspace, unwrap, type Me, type Membership } from "@/lib/api"

const STORAGE_KEY = "workspace"

function storedWorkspace(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

function storeWorkspace(id: string | null) {
  try {
    if (id) localStorage.setItem(STORAGE_KEY, id)
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Storage can be blocked; the choice then lasts for this page only.
  }
}

export const meKey = ["me"] as const

export function useMe() {
  return useQuery({
    queryKey: meKey,
    queryFn: async () => {
      try {
        return await unwrap(api.GET("/v1/me"))
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null
        throw err
      }
    },
    staleTime: 60_000,
  })
}

export function pickMembership(me: Me, wanted: string | null): Membership | null {
  if (me.memberships.length === 1) return me.memberships[0]
  return me.memberships.find((m) => m.workspace.id === wanted) ?? null
}

type Session = {
  me: Me
  membership: Membership
  workspaceId: string
  canManage: boolean
  switchWorkspace: (id: string) => void
}

const SessionContext = createContext<Session | null>(null)

export function useSession(): Session {
  const s = useContext(SessionContext)
  if (!s) throw new Error("useSession outside SessionProvider")
  return s
}

export function useWorkspaceChoice() {
  const [chosen, setChosen] = useState(storedWorkspace)
  const choose = (id: string | null) => {
    storeWorkspace(id)
    setChosen(id)
  }
  return [chosen, choose] as const
}

export function SessionProvider({
  me,
  membership,
  onSwitch,
  children,
}: {
  me: Me
  membership: Membership
  onSwitch: (id: string) => void
  children: React.ReactNode
}) {
  setWorkspace(membership.workspace.id)
  const value = useMemo<Session>(
    () => ({
      me,
      membership,
      workspaceId: membership.workspace.id,
      canManage: membership.role === "owner" || membership.role === "admin",
      switchWorkspace: onSwitch,
    }),
    [me, membership, onSwitch],
  )
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}

export function useSignOut() {
  const qc = useQueryClient()
  return async () => {
    await api.POST("/v1/auth/sign-out")
    setWorkspace(null)
    qc.clear()
    qc.setQueryData(meKey, null)
  }
}
