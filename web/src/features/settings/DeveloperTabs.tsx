import { Trans } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { useSettingsAbout, useSettingsNames } from "@/features/settings/SettingsFrame"
import { PageHeader } from "@/features/settings/ui"
import { useSession } from "@/lib/session"

type Tab = "api-keys" | "webhooks" | "connected-apps"

export function DeveloperHeader({
  current,
  description,
  onCreate,
  creating,
}: {
  current: Tab
  description?: React.ReactNode
  onCreate?: () => void
  creating?: boolean
}) {
  const names = useSettingsNames()
  const about = useSettingsAbout()
  return (
    <PageHeader
      back={false}
      title={names.place[current]}
      description={description ?? about[current]}
      action={<CreateButton current={current} onCreate={onCreate} creating={creating} />}
    />
  )
}

function CreateButton({ current, onCreate, creating }: { current: Tab; onCreate?: () => void; creating?: boolean }) {
  const { canManage } = useSession()
  if (!canManage || !onCreate || current === "connected-apps") return null
  return current === "api-keys" ? (
    <Button onClick={onCreate} disabled={creating} data-testid="api-key-new">
      <PlusIcon />
      <Trans>New API key</Trans>
    </Button>
  ) : (
    <Button onClick={onCreate} disabled={creating} data-testid="webhook-add">
      <PlusIcon />
      <Trans>New webhook endpoint</Trans>
    </Button>
  )
}
