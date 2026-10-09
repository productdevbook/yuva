import { Navigate, useParams } from "react-router"

import { AddChannelStep } from "@/features/setup/AddChannelStep"
import { ChannelStep } from "@/features/setup/ChannelStep"
import { InstallStep } from "@/features/setup/InstallStep"
import { NameStep } from "@/features/setup/NameStep"
import { SETUP_KINDS, type SetupKind } from "@/features/setup/setup"
import { WaitStep } from "@/features/setup/WaitStep"
import { useSession } from "@/lib/session"

export function SetupPage({ wait }: { wait?: boolean }) {
  const { canManage } = useSession()
  const { inboxId, channelId, kind } = useParams()
  if (!canManage) return <Navigate to="/" replace />
  if (inboxId && kind) {
    if (!SETUP_KINDS.includes(kind as SetupKind)) return <Navigate to={`/setup/${inboxId}`} replace />
    return <AddChannelStep key={kind} inboxId={inboxId} kind={kind as SetupKind} />
  }
  if (inboxId && channelId) return wait ? <WaitStep inboxId={inboxId} channelId={channelId} /> : <InstallStep inboxId={inboxId} channelId={channelId} />
  if (inboxId) return <ChannelStep inboxId={inboxId} />
  return <NameStep />
}
