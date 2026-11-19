import { useQueries } from "@tanstack/react-query"
import { Outlet, useLocation } from "react-router"

import { ProductsHome } from "@/features/settings/ProductsPage"
import { SettingsLayout } from "@/features/settings/SettingsFrame"
import { Page } from "@/features/settings/ui"
import { useIsPhone } from "@/hooks/use-media-query"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

const WIDE = [
  "/settings/api-keys",
  "/settings/issue-trackers",
  "/settings/slack",
  "/settings/webhooks",
  "/settings/connected-apps",
]

function isWide(pathname: string) {
  if (WIDE.includes(pathname)) return true
  if (pathname.startsWith("/settings/connected-apps/")) return true
  return /^\/settings\/inboxes\/[^/]+$/.test(pathname)
}

export function SettingsPane() {
  const { pathname } = useLocation()
  const phone = useIsPhone()
  const own = pathname === "/settings/workspace" || /^\/settings\/inboxes\/[^/]+\/channels\/[^/]+$/.test(pathname)
  const body = own ? (
    <Outlet />
  ) : (
    <Page testId="settings" width={isWide(pathname) ? "full" : "form"}>
      <Outlet />
    </Page>
  )
  return phone ? body : <SettingsLayout>{body}</SettingsLayout>
}

export function SettingsHome() {
  const phone = useIsPhone()
  return phone ? (
    <ProductsHome />
  ) : (
    <SettingsLayout>
      <ProductsHome />
    </SettingsLayout>
  )
}

export function useInboxChannels() {
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data ?? []
  const results = useQueries({
    queries: inboxes.map((i) => ({
      queryKey: keys.channels(ws, i.id),
      queryFn: () =>
        unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: i.id } } })).then((r) => r.items),
      staleTime: 5 * 60_000,
    })),
  })
  return inboxes.map((inbox, i) => ({ inbox, channels: results[i]?.data }))
}
