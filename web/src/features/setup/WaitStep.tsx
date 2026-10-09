import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightIcon, MailIcon, PartyPopperIcon } from "lucide-react"
import { useNavigate } from "react-router"

import { Notice } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useConversations } from "@/features/inbox/queries"
import { useInbox } from "@/features/settings/inboxes/queries"
import { setupKind, useChannel } from "@/features/setup/InstallStep"
import { DEFAULT_COLOR, FirstConversation, MailMock, PhoneMock, PreviewCaption, SiteMock } from "@/features/setup/previews"
import { SetupLayout, StepHeader } from "@/features/setup/SetupLayout"
import type { Channel } from "@/lib/api"
import { useRealtimeStatus } from "@/lib/realtime"
import { cn } from "@/lib/utils"

function Listening() {
  const status = useRealtimeStatus()
  const live = status === "live"
  return (
    <p className="flex items-center gap-2.5 text-body text-muted-foreground" role="status" data-testid="setup-listening">
      <span className="relative flex size-2.5">
        {live && <span className="absolute inline-flex size-full animate-ping rounded-full bg-brand opacity-60" />}
        <span className={cn("relative inline-flex size-2.5 rounded-full", live ? "bg-brand" : "bg-faint")} />
      </span>
      {live ? <Trans>Listening for the first message…</Trans> : <Trans>Connecting…</Trans>}
    </p>
  )
}

function ChatPreview({ channel, name, color }: { channel: Channel; name: string; color: string }) {
  const chat = channel.chat!
  return (
    <>
      <SiteMock
        host={new URL(chat.allowed_origins[0]!).host}
        name={name}
        greeting={chat.greeting}
        color={chat.launcher.color ?? color}
        position={chat.launcher.position}
        waiting
      />
      <PreviewCaption>
        <Trans>It shows up here as soon as it reaches Yuva.</Trans>
      </PreviewCaption>
    </>
  )
}

export function WaitStep({ inboxId, channelId }: { inboxId: string; channelId: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const inbox = useInbox(inboxId).data
  const channel = useChannel(channelId).data
  const kind = channel ? setupKind(channel) : null
  const first = useConversations({ inbox_id: inboxId }).data?.pages[0]?.items[0]
  const name = inbox?.name ?? ""
  const color = inbox?.branding.color ?? DEFAULT_COLOR
  const step = first ? 5 : 4

  const preview = !channel ? null : first ? (
    <>
      <FirstConversation item={first} name={name} kind={kind ?? "chat"} />
      <PreviewCaption>
        <Trans>New conversations land in your queue, oldest first.</Trans>
      </PreviewCaption>
    </>
  ) : kind === "chat" ? (
    <ChatPreview channel={channel} name={name} color={color} />
  ) : kind === "email" ? (
    <>
      <MailMock name={name} to={channel.email!.address} waiting />
      <PreviewCaption>
        <Trans>It shows up here as soon as it reaches Yuva.</Trans>
      </PreviewCaption>
    </>
  ) : (
    <>
      <PhoneMock name={name} color={color} waiting />
      <PreviewCaption>
        <Trans>It shows up here as soon as it reaches Yuva.</Trans>
      </PreviewCaption>
    </>
  )

  return (
    <SetupLayout step={step} back={first ? undefined : `/setup/${inboxId}/${channelId}`} preview={preview} previewOpen={!!first}>
      {!channel || !kind ? (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-8 w-3/4" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : first ? (
        <div data-testid="setup-done">
          <StepHeader icon={<PartyPopperIcon />} title={<Trans>Your first conversation is here</Trans>}>
            <Trans>{name} is ready. Reply to it, and invite your team in Settings › Members when you like.</Trans>
          </StepHeader>
          <Button size="lg" onClick={() => navigate(`/conversations/${first.id}`)} data-testid="setup-open-conversation">
            <Trans>Open the conversation</Trans>
            <ArrowRightIcon className="rtl:rotate-180" />
          </Button>
        </div>
      ) : (
        <>
          <StepHeader
            title={
              kind === "chat" ? (
                <Trans>Say hello from your site</Trans>
              ) : kind === "email" ? (
                <Trans>Send a test e-mail</Trans>
              ) : (
                <Trans>Send a message from your app</Trans>
              )
            }
          >
            {kind === "chat" ? (
              <Trans>Open a page with the snippet and write in the chat, as a visitor would. The conversation appears here the moment it starts.</Trans>
            ) : kind === "email" ? (
              <Trans>Write to {channel.email!.address} from any other address. The conversation appears here the moment the mail arrives.</Trans>
            ) : (
              <Trans>Run your app with the SDK and send a message. The conversation appears here the moment it starts.</Trans>
            )}
          </StepHeader>
          <div className="flex flex-col gap-6">
            <Listening />
            {kind === "email" && (
              <Button
                variant="outline"
                size="lg"
                className="self-start"
                render={<a href={`mailto:${channel.email!.address}?subject=${encodeURIComponent(t`Hello from the setup`)}`} />}
              >
                <MailIcon />
                <Trans>Write a test e-mail</Trans>
              </Button>
            )}
            <Notice className="text-small">
              <Trans>You can leave this page: the conversation will wait for you in the queue.</Trans>
            </Notice>
          </div>
        </>
      )}
    </SetupLayout>
  )
}

