import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { CHANNEL_KINDS, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { AppFields } from "@/features/settings/channels/app/AppFields"
import { appChannelInput, appForm, type AppForm } from "@/features/settings/channels/app/form"
import { AppInstall } from "@/features/settings/channels/app/AppInstall"
import { ChatFields } from "@/features/settings/channels/chat/ChatFields"
import { chatChannelInput, chatErrorField, chatForm, type ChatForm } from "@/features/settings/channels/chat/form"
import { ChatInstall } from "@/features/settings/channels/chat/ChatInstall"
import { EmailFields } from "@/features/settings/channels/email/EmailFields"
import { emailErrorField, emailForm, emailInput, useEmailFieldErrors, type EmailField, type EmailForm } from "@/features/settings/channels/email/form"
import { Field } from "@/features/settings/ui"
import { api, ApiError, unwrap, type Channel, type ChannelKind } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function parseSettings(raw: string): Record<string, unknown> | null {
  if (raw.trim() === "") return {}
  try {
    const v = JSON.parse(raw)
    return v && typeof v === "object" && !Array.isArray(v) ? v : null
  } catch {
    return null
  }
}

export function ChannelDialog({
  inboxId,
  channel,
  onClose,
  onCreated,
}: {
  inboxId: string
  channel: Channel | null
  onClose: () => void
  onCreated: (ch: Channel) => void
}) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const text = useEnumText()
  const fieldErrors = useEmailFieldErrors()
  const [kind, setKind] = useState<ChannelKind>(channel?.kind ?? "email")
  const [name, setName] = useState(channel?.name ?? "")
  const [raw, setRaw] = useState(channel ? JSON.stringify(channel.settings, null, 2) : "{}")
  const [email, setEmail] = useState<EmailForm>(() => emailForm(channel?.email))
  const [chat, setChat] = useState<ChatForm>(() => chatForm(channel?.chat))
  const [app, setApp] = useState<AppForm>(() => appForm(channel?.app))
  const settings = kind === "api" ? parseSettings(raw) : {}
  const chatInput = kind === "chat" ? chatChannelInput(chat) : null
  const appInput = kind === "app" ? appChannelInput(app) : null
  const valid = !!settings && (kind !== "chat" || chatInput?.ok === true) && (kind !== "app" || !!appInput)
  const save = useMutation({
    mutationFn: () => {
      const typed =
        kind === "email"
          ? { email: emailInput(email) }
          : kind === "chat" && chatInput?.ok
            ? { chat: chatInput.value }
            : kind === "app" && appInput
              ? { app: appInput }
              : { settings: settings ?? {} }
      return channel
        ? unwrap(api.PATCH("/v1/channels/{channelId}", { params: { path: { channelId: channel.id } }, body: { name, ...typed } }))
        : unwrap(api.POST("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId } }, body: { kind, name, ...typed } }))
    },
    onSuccess: (saved) => {
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inboxId) })
      void qc.invalidateQueries({ queryKey: ["ws", ws, "channel"] })
      if (channel) onClose()
      else onCreated(saved)
    },
  })
  const bad = emailErrorField(save.error)
  const badChat = chatErrorField(save.error)
  const taken = save.error instanceof ApiError && save.error.code === "email_address_taken"
  const fieldError = (f: EmailField) => (bad === f && !taken ? fieldErrors[f] : undefined)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] p-0 sm:max-w-lg">
        <form
          className="flex min-w-0 flex-col"
          onSubmit={(e) => {
            e.preventDefault()
            setApp((a) => ({ ...a, touched: true }))
            setChat((c) => ({ ...c, touched: true }))
            if (valid) save.mutate()
          }}
        >
          <div className="flex flex-col gap-5 p-6">
            <DialogHeader>
              <DialogTitle>{channel ? <Trans>Edit channel</Trans> : <Trans>Add channel</Trans>}</DialogTitle>
              <DialogDescription>
                <Trans>The kind cannot be changed later.</Trans>
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field label={<Trans>Kind</Trans>}>
                <Select value={kind} onValueChange={(v) => setKind(v as ChannelKind)} items={text.channel} disabled={!!channel}>
                  <SelectTrigger className="w-full" aria-label={t`Kind`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CHANNEL_KINDS.map((k) => (
                      <SelectItem key={k} value={k}>
                        {text.channel[k]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label={<Trans>Name</Trans>} htmlFor="channel-name">
                <Input
                  id="channel-name"
                  required
                  maxLength={200}
                  value={name}
                  placeholder={kind === "chat" ? t`Website chat` : kind === "app" ? t`iOS and Android app` : t`Support e-mail`}
                  onChange={(e) => setName(e.target.value)}
                />
              </Field>
            </div>
            {kind === "email" && <EmailFields f={email} set={(patch) => setEmail((f) => ({ ...f, ...patch }))} error={fieldError} taken={taken} />}
            {kind === "chat" && (
              <>
                <ChatFields
                  f={chat}
                  set={(patch) => setChat((f) => ({ ...f, ...patch }))}
                  invalid={chatInput && !chatInput.ok ? chatInput : null}
                  serverError={badChat}
                />
                {channel?.chat && <ChatInstall channel={channel} />}
              </>
            )}
            {kind === "app" && (
              <>
                <AppFields f={app} set={(patch) => setApp((f) => ({ ...f, ...patch }))} />
                {channel?.app && <AppInstall channel={channel} />}
              </>
            )}
            {kind === "api" && (
              <Field
                label={<Trans>Settings (JSON)</Trans>}
                htmlFor="channel-settings"
                error={settings ? undefined : <Trans>This is not a JSON object.</Trans>}
              >
                <Textarea
                  id="channel-settings"
                  rows={6}
                  spellCheck={false}
                  className="font-mono text-xs"
                  value={raw}
                  aria-invalid={!settings}
                  onChange={(e) => setRaw(e.target.value)}
                />
              </Field>
            )}
            {!bad && !badChat && <ErrorLine error={save.error} />}
          </div>
          <DialogFooter className="sticky bottom-0 border-t bg-card px-6 py-3">
            <Button type="submit" disabled={save.isPending || !settings}>
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
