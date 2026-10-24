import { Navigate, useParams } from "react-router"

import { AddChannelStep } from "@/features/setup/AddChannelStep"
import { ChannelStep } from "@/features/setup/ChannelStep"
import { EmailAddressStep } from "@/features/setup/email/EmailAddressStep"
import { EmailDoneStep } from "@/features/setup/email/EmailDoneStep"
import { EmailRepliesStep } from "@/features/setup/email/EmailRepliesStep"
import { EmailTestStep } from "@/features/setup/email/EmailTestStep"
import { InstallStep } from "@/features/setup/InstallStep"
import { NameStep } from "@/features/setup/NameStep"
import { NewsletterDoneStep, NewsletterListStep } from "@/features/setup/NewsletterStep"
import { SETUP_KINDS, type SetupKind } from "@/features/setup/setup"
import { SetupHub } from "@/features/setup/SetupHub"
import { StartStep } from "@/features/setup/StartStep"
import { WaitStep } from "@/features/setup/WaitStep"
import { useSession } from "@/lib/session"

type Flow = "hub" | "mail" | "new" | "pick" | "add" | "start" | "newsletter" | "wait" | "replies" | "test" | "done"

function isKind(kind: string | undefined): kind is SetupKind {
  return SETUP_KINDS.includes(kind as SetupKind)
}

export function SetupPage({ flow }: { flow?: Flow }) {
  const { canManage } = useSession()
  const { inboxId: param, channelId, kind, listId } = useParams()
  const inboxId = flow === "pick" || flow === "add" ? "new" : flow === "replies" ? (param ?? "new") : param
  if (!canManage) return <Navigate to="/" replace />
  if (flow === "hub") return <SetupHub />
  if (flow === "mail") return <NameStep mail />
  if (flow === "newsletter")
    return listId ? <NewsletterDoneStep key={listId} listId={listId} /> : <NewsletterListStep />
  if (flow === "start") return isKind(kind) ? <StartStep key={kind} kind={kind} /> : <Navigate to="/setup" replace />
  if (flow === "new") {
    if (kind !== undefined && !isKind(kind)) return <Navigate to="/setup/new" replace />
    return <NameStep key={kind ?? ""} kind={kind} />
  }
  if (inboxId && flow === "replies") return <EmailRepliesStep inboxId={inboxId} />
  if (inboxId && kind) {
    if (!isKind(kind)) return <Navigate to={`/setup/${inboxId}`} replace />
    if (kind === "email") return <EmailAddressStep inboxId={inboxId} />
    return <AddChannelStep key={kind} inboxId={inboxId} kind={kind} />
  }
  if (inboxId && channelId) {
    if (flow === "test") return <EmailTestStep inboxId={inboxId} channelId={channelId} />
    if (flow === "done") return <EmailDoneStep inboxId={inboxId} channelId={channelId} />
    return flow === "wait" ? (
      <WaitStep inboxId={inboxId} channelId={channelId} />
    ) : (
      <InstallStep inboxId={inboxId} channelId={channelId} />
    )
  }
  if (inboxId) return <ChannelStep inboxId={inboxId} />
  return <Navigate to="/setup" replace />
}
