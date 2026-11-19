import { Trans } from "@lingui/react/macro"
import { ArrowRightIcon, PartyPopperIcon, SettingsIcon, UsersIcon } from "lucide-react"
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from "react-router"

import { Notice, StepHeader } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useConversations } from "@/features/inbox/queries"
import { useInbox } from "@/features/settings/inboxes/queries"
import { hostOfOrigin, setupKind, useChannel } from "@/features/setup/InstallStep"
import { LiveChat, useForgetPreviewVisitor } from "@/features/setup/LiveChat"
import { DEFAULT_COLOR, DocsMock, FirstConversation, PhoneMock, PreviewCaption } from "@/features/setup/previews"
import { isTrimmed, stepNumber, type SetupState } from "@/features/setup/setup"
import { BackHome } from "@/features/setup/BackHome"
import { SetupLayout } from "@/features/setup/SetupLayout"
import type { Channel } from "@/lib/api"
import { useRealtimeStatus } from "@/lib/realtime"
import { cn } from "@/lib/utils"

function Listening() {
  const status = useRealtimeStatus()
  const live = status === "live"
  return (
    <p
      className="flex items-center gap-2.5 text-body text-muted-foreground"
      role="status"
      data-testid="setup-listening"
    >
      <span className="relative flex size-2.5">
        {live && <span className="absolute inline-flex size-full animate-ping rounded-full bg-brand opacity-60" />}
        <span className={cn("relative inline-flex size-2.5 rounded-full", live ? "bg-brand" : "bg-faint")} />
      </span>
      {live ? <Trans>Listening for the first message…</Trans> : <Trans>Connecting…</Trans>}
    </p>
  )
}

function ChatPreview({ channel }: { channel: Channel }) {
  return (
    <>
      <LiveChat publicKey={channel.chat!.public_key} />
      <PreviewCaption>
        <Trans>Write here as a visitor would. The page moves on as soon as the message reaches Yuva.</Trans>
      </PreviewCaption>
    </>
  )
}

export function WaitStep({ inboxId, channelId }: { inboxId: string; channelId: string }) {
  const navigate = useNavigate()
  const inbox = useInbox(inboxId).data
  const channel = useChannel(channelId).data
  const docs = useSearchParams()[0].has("docs")
  const kind = channel ? setupKind(channel, docs) : null
  const recent = useConversations({ inbox_id: inboxId }).data?.pages[0]?.items
  const first = docs ? recent?.find((c) => c.channel_id === channelId && c.kind !== "conversation") : recent?.[0]
  const name = inbox?.name ?? ""
  const color = inbox?.branding.color ?? DEFAULT_COLOR
  const state = (useLocation().state ?? {}) as SetupState
  const trimmed = isTrimmed(kind, state.existing)
  const step = trimmed ? stepNumber("wait", true) : first ? 5 : 4
  useForgetPreviewVisitor(channel?.chat?.public_key, `/setup/${inboxId}/${channelId}`)

  if (channel?.email) return <Navigate to={`/setup/${inboxId}/${channelId}/test`} replace state={state} />

  const preview = !channel ? null : first ? (
    <>
      <FirstConversation item={first} name={name} kind={kind ?? "chat"} />
      <PreviewCaption>
        <Trans>New conversations show up at the top of your conversation list.</Trans>
      </PreviewCaption>
    </>
  ) : kind === "docs" ? (
    <>
      <DocsMock host={hostOfOrigin(channel.chat!.allowed_origins[0])} color={color} waiting />
      <PreviewCaption>
        <Trans>It shows up here as soon as it reaches Yuva.</Trans>
      </PreviewCaption>
    </>
  ) : kind === "chat" ? (
    <ChatPreview channel={channel} />
  ) : (
    <>
      <PhoneMock name={name} color={color} waiting />
      <PreviewCaption>
        <Trans>It shows up here as soon as it reaches Yuva.</Trans>
      </PreviewCaption>
    </>
  )

  return (
    <SetupLayout
      step={step}
      kind={kind}
      trimmed={trimmed}
      back={first ? undefined : `/setup/${inboxId}/${channelId}${docs ? "?docs" : ""}`}
      preview={preview}
      previewOpen={!!first || kind === "chat"}
    >
      {!channel || !kind ? (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-8 w-3/4" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : first ? (
        <div data-testid="setup-done">
          <StepHeader icon={<PartyPopperIcon />} title={<Trans>Your first conversation is here</Trans>}>
            <Trans>{name} is ready. Reply to it when you like.</Trans>
          </StepHeader>
          <Button
            size="lg"
            onClick={() => navigate(`/conversations/${first.id}`)}
            data-testid="setup-open-conversation"
          >
            <Trans>Open the conversation</Trans>
            <ArrowRightIcon className="rtl:rotate-180" />
          </Button>
          <div className="mt-6 flex flex-wrap gap-2">
            <BackHome variant="outline" />
            <Button variant="outline" render={<Link to="/settings/members" />} data-testid="setup-invite">
              <UsersIcon />
              <Trans>Invite your team</Trans>
            </Button>
            <Button
              variant="outline"
              render={<Link to={`/settings/inboxes/${inboxId}`} />}
              data-testid="setup-settings"
            >
              <SettingsIcon />
              <Trans>Open the inbox settings</Trans>
            </Button>
          </div>
        </div>
      ) : (
        <>
          <StepHeader
            title={
              kind === "docs" ? (
                <Trans>Send feedback from a page</Trans>
              ) : kind === "chat" ? (
                <Trans>Say hello from your site</Trans>
              ) : (
                <Trans>Send a message from your app</Trans>
              )
            }
          >
            {kind === "docs" ? (
              <Trans>
                Open a page with the elements, rate it and send feedback. A rating alone only counts; feedback appears
                here the moment it is sent. Discussions started there show up in Docs.
              </Trans>
            ) : kind === "chat" ? (
              <Trans>
                Write in the chat in the preview, or on a page with the snippet, as a visitor would. The conversation
                appears here the moment it starts.
              </Trans>
            ) : (
              <Trans>
                Run your app with the SDK and send a message. The conversation appears here the moment it starts.
              </Trans>
            )}
          </StepHeader>
          <div className="flex flex-col gap-6">
            <Listening />
            <Notice className="text-small">
              <Trans>You can leave this page: the conversation will wait for you in your conversation list.</Trans>
            </Notice>
          </div>
        </>
      )}
    </SetupLayout>
  )
}
