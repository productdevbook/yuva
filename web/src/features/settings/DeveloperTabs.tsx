import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { ChevronDownIcon } from "lucide-react"
import { useLocation, useNavigate } from "react-router"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { useGrants } from "@/features/settings/connected-apps/queries"
import { headButton, PageTabs, useFlatSettings } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

type Tab = "api-keys" | "webhooks" | "connected-apps"

export function DeveloperTitle() {
  const { canManage } = useSession()
  return canManage ? <Trans>API & MCP</Trans> : <Trans>Connected apps</Trans>
}

export function DeveloperTabs({ current }: { current: Tab }) {
  const { t } = useLingui()
  const { workspaceId: ws, canManage } = useSession()
  const apiKeys = useQuery({
    queryKey: keys.apiKeys(ws),
    queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items),
    enabled: canManage,
  })
  const grants = useGrants()
  const flat = useFlatSettings()
  const hooks = useQuery({
    queryKey: keys.webhookList(ws, ""),
    queryFn: () => unwrap(api.GET("/v1/webhooks", { params: { query: {} } })).then((r) => r.items),
    enabled: canManage,
  })
  if (!canManage || flat) return null
  return (
    <PageTabs
      label={t`API & MCP`}
      current={current}
      items={[
        {
          key: "api-keys",
          to: "/settings/api-keys",
          label: t`API keys`,
          count: apiKeys.data?.filter((k) => !k.revoked_at).length,
          testId: "developer-tab-api-keys",
        },
        {
          key: "webhooks",
          to: "/settings/webhooks",
          label: t`Webhooks`,
          count: hooks.data?.length,
          testId: "developer-tab-webhooks",
        },
        {
          key: "connected-apps",
          to: "/settings/connected-apps",
          label: t`Connected apps`,
          count: grants.data?.length,
          testId: "developer-tab-connected-apps",
        },
      ]}
    />
  )
}

export function CreateMenu({ current }: { current: Tab }) {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { canManage } = useSession()
  if (!canManage) return null
  const state = { back: pathname }
  const all = [
    {
      key: "api-keys",
      label: <Trans>New API key</Trans>,
      run: () => navigate("/setup/api-key", { state }),
      testId: "api-key-new",
    },
    {
      key: "webhooks",
      label: <Trans>New webhook endpoint</Trans>,
      run: () => navigate("/setup/webhook", { state }),
      testId: "webhook-add",
    },
    {
      key: "connected-apps",
      label: <Trans>Connect an assistant</Trans>,
      run: () => navigate("/settings/connected-apps"),
      testId: "assistant-connect",
    },
  ]
  const items = [...all.filter((x) => x.key === current), ...all.filter((x) => x.key !== current)]
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button className={headButton.primary} />} data-testid="developer-create">
        <Trans>Create</Trans>
        <ChevronDownIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        {items.map((x) => (
          <DropdownMenuItem key={x.key} onClick={x.run} data-testid={x.testId}>
            {x.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
