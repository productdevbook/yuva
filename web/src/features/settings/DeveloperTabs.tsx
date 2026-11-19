import { Trans } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { useLocation, useNavigate } from "react-router"

import { Button } from "@/components/ui/button"
import { useSettingsAbout, useSettingsNames } from "@/features/settings/SettingsFrame"
import { PageHeader } from "@/features/settings/ui"
import { useSession } from "@/lib/session"

type Tab = "api-keys" | "webhooks" | "connected-apps"

export function DeveloperHeader({ current, description }: { current: Tab; description?: React.ReactNode }) {
  const names = useSettingsNames()
  const about = useSettingsAbout()
  return (
    <PageHeader
      back={false}
      title={names.place[current]}
      description={description ?? about[current]}
      action={<CreateButton current={current} />}
    />
  )
}

function CreateButton({ current }: { current: Tab }) {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { canManage } = useSession()
  if (!canManage || current === "connected-apps") return null
  const state = { back: pathname }
  return current === "api-keys" ? (
    <Button onClick={() => navigate("/setup/api-key", { state })} data-testid="api-key-new">
      <PlusIcon />
      <Trans>New API key</Trans>
    </Button>
  ) : (
    <Button onClick={() => navigate("/setup/webhook", { state })} data-testid="webhook-add">
      <PlusIcon />
      <Trans>New webhook endpoint</Trans>
    </Button>
  )
}
