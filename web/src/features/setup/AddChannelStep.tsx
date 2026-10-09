import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ArrowRightIcon } from "lucide-react"
import { useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { PLATFORMS, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { appChannelInput, appForm, type AppForm } from "@/features/settings/channels/app/form"
import { chatChannelInput, chatErrorField, chatForm, type ChatForm } from "@/features/settings/channels/chat/form"
import { Sending } from "@/features/settings/channels/email/EmailFields"
import { emailErrorField, emailForm, emailInput, useEmailFieldErrors, type EmailField, type EmailForm } from "@/features/settings/channels/email/form"
import { FieldError } from "@/features/settings/channels/parts"
import { useInbox } from "@/features/settings/inboxes/queries"
import { Field, ToggleRow } from "@/features/settings/ui"
import { useKindText } from "@/features/setup/ChannelStep"
import { DEFAULT_COLOR, MailMock, PhoneMock, PreviewCaption, SiteMock } from "@/features/setup/previews"
import type { SetupKind, SetupState } from "@/features/setup/setup"
import { SetupLayout, StepHeader } from "@/features/setup/SetupLayout"
import { api, ApiError, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function hostOf(origins: string) {
  for (const line of origins.split(/[\n,]+/)) {
    try {
      const u = new URL(line.trim())
      if (u.protocol === "https:" || u.protocol === "http:") return u.host
    } catch {
      continue
    }
  }
  return "www.example.com"
}

export function AddChannelStep({ inboxId, kind }: { inboxId: string; kind: SetupKind }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as SetupState
  const { workspaceId: ws } = useSession()
  const inbox = useInbox(inboxId).data
  const kindText = useKindText()
  const enumText = useEnumText()
  const fieldErrors = useEmailFieldErrors()
  const [chat, setChat] = useState<ChatForm>(() => ({ ...chatForm(), allowAnonymous: true }))
  const [email, setEmail] = useState<EmailForm>(() => emailForm())
  const [app, setApp] = useState<AppForm>(() => appForm())
  const name = inbox?.name ?? ""
  const color = inbox?.branding.color ?? DEFAULT_COLOR
  const chatInput = kind === "chat" ? chatChannelInput(chat) : null
  const appInput = kind === "app" ? appChannelInput(app) : null

  const save = useMutation({
    mutationFn: () => {
      const typed =
        kind === "email"
          ? { email: emailInput(email) }
          : kind === "chat" && chatInput?.ok
            ? { chat: chatInput.value }
            : { app: appInput! }
      const channelName = kind === "email" ? name || kindText.email.title : kindText[kind].title
      return unwrap(api.POST("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId } }, body: { kind, name: channelName, ...typed } }))
    },
    onSuccess: (ch) => {
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inboxId) })
      qc.setQueryData(keys.channel(ws, ch.id), ch)
      navigate(`/setup/${inboxId}/${ch.id}`, { state })
    },
  })

  const bad = emailErrorField(save.error)
  const badChat = chatErrorField(save.error)
  const taken = save.error instanceof ApiError && save.error.code === "email_address_taken"
  const emailError = (f: EmailField) => (bad === f && !taken ? fieldErrors[f] : undefined)
  const valid = kind === "chat" ? chatInput?.ok === true : kind === "app" ? !!appInput : email.address.trim() !== ""

  const notOrigins = chatInput && !chatInput.ok ? chatInput.bad.join(", ") : ""
  const originsError =
    badChat === "allowed_origins" ? (
      <Trans>Use origins such as https://www.example.com, without a path.</Trans>
    ) : chatInput && !chatInput.ok && (chat.touched || chatInput.bad.length > 0) ? (
      chatInput.bad.length > 0 ? (
        <Trans>Not an origin: {notOrigins}. Use scheme and host only, such as https://www.example.com.</Trans>
      ) : chatInput.tooMany ? (
        <Trans>List at most 20 origins.</Trans>
      ) : (
        <Trans>Add at least one origin.</Trans>
      )
    ) : null

  const preview =
    kind === "chat" ? (
      <>
        <SiteMock host={hostOf(chat.origins)} name={name} color={color} />
        <PreviewCaption>
          <Trans>The chat appears in the corner of every page that has the snippet.</Trans>
        </PreviewCaption>
      </>
    ) : kind === "email" ? (
      <>
        <MailMock name={name} to={email.address.trim()} />
        <PreviewCaption>
          <Trans>Each new e-mail starts a conversation; replies continue it.</Trans>
        </PreviewCaption>
      </>
    ) : (
      <>
        <PhoneMock name={name} color={color} />
        <PreviewCaption>
          <Trans>The SDKs bring ready-made screens, or a client for your own.</Trans>
        </PreviewCaption>
      </>
    )

  const title = {
    chat: <Trans>Which sites will show the chat?</Trans>,
    email: <Trans>Which address do customers write to?</Trans>,
    app: <Trans>Which apps will use it?</Trans>,
  }[kind]
  const lead = {
    chat: <Trans>Only pages on these addresses can open the chat. Enter scheme and host, without a path.</Trans>,
    email: <Trans>Use the support address you already have. Mail sent to it will arrive here.</Trans>,
    app: <Trans>Pick the platforms; the next step shows the lines to add to each app.</Trans>,
  }[kind]
  const submit = {
    chat: <Trans>Get the snippet</Trans>,
    email: <Trans>Continue</Trans>,
    app: <Trans>Get the SDK lines</Trans>,
  }[kind]

  return (
    <SetupLayout step={3} back={`/setup/${inboxId}`} preview={preview}>
      <StepHeader title={title}>{lead}</StepHeader>
      <form
        className="flex flex-col gap-6"
        onSubmit={(e) => {
          e.preventDefault()
          setApp((a) => ({ ...a, touched: true }))
          setChat((c) => ({ ...c, touched: true }))
          if (valid) save.mutate()
        }}
        data-testid={`setup-add-${kind}`}
      >
        {kind === "chat" && (
          <>
            <Field label={<Trans>Website addresses</Trans>} htmlFor="setup-origins" hint={<Trans>One per line, up to 20.</Trans>}>
              <Textarea
                id="setup-origins"
                autoFocus
                rows={3}
                spellCheck={false}
                className="rounded-xl font-mono text-body"
                value={chat.origins}
                placeholder={"https://www.example.com\nhttps://app.example.com"}
                onChange={(e) => setChat((c) => ({ ...c, origins: e.target.value }))}
                aria-invalid={!!originsError || undefined}
                aria-describedby={originsError ? "err-setup-origins" : undefined}
                data-testid="setup-origins"
              />
              <FieldError id="err-setup-origins">{originsError}</FieldError>
            </Field>
            <ToggleRow title={<Trans>Allow anonymous visitors</Trans>} hint={<Trans>Anyone on your site can write. Turn it off when only signed-in users may chat.</Trans>}>
              <Switch
                checked={chat.allowAnonymous}
                onCheckedChange={(allowAnonymous) => setChat((c) => ({ ...c, allowAnonymous }))}
                aria-label={t`Allow anonymous visitors`}
              />
            </ToggleRow>
          </>
        )}
        {kind === "email" && (
          <>
            <Field label={<Trans>Support address</Trans>} htmlFor="setup-address">
              <Input
                id="setup-address"
                type="email"
                autoFocus
                required
                maxLength={320}
                className="h-10 rounded-xl"
                value={email.address}
                placeholder="support@example.com"
                onChange={(e) => setEmail((f) => ({ ...f, address: e.target.value }))}
                aria-invalid={taken || bad === "address" || undefined}
                data-testid="setup-address"
              />
              <FieldError id="err-address">
                {taken ? <Trans>Another channel already receives mail at this address.</Trans> : emailError("address")}
              </FieldError>
            </Field>
            <Sending f={email} set={(patch) => setEmail((f) => ({ ...f, ...patch }))} error={emailError} taken={taken} />
          </>
        )}
        {kind === "app" && (
          <>
            <Field label={<Trans>Platforms</Trans>}>
              <div className="flex gap-5" role="group" aria-label={t`Platforms`}>
                {PLATFORMS.map((p) => (
                  <Label key={p} className="font-normal">
                    <Checkbox
                      checked={app.platforms.includes(p)}
                      onCheckedChange={(on) => setApp((a) => ({ ...a, platforms: on ? [...a.platforms, p] : a.platforms.filter((x) => x !== p) }))}
                      aria-invalid={(app.platforms.length === 0 && app.touched) || undefined}
                      data-testid={`setup-platform-${p}`}
                    />
                    {enumText.platform[p]}
                  </Label>
                ))}
              </div>
              <FieldError id="err-setup-platforms">{app.platforms.length === 0 && <Trans>Pick at least one platform.</Trans>}</FieldError>
            </Field>
            <ToggleRow
              title={<Trans>Allow anonymous users</Trans>}
              hint={<Trans>People who are not signed in to your app can write without an identity token from your backend.</Trans>}
            >
              <Switch
                checked={app.allowAnonymous}
                onCheckedChange={(allowAnonymous) => setApp((a) => ({ ...a, allowAnonymous }))}
                aria-label={t`Allow anonymous users`}
              />
            </ToggleRow>
          </>
        )}
        {!bad && !badChat && <ErrorLine error={save.error} />}
        <Button type="submit" size="lg" className="self-start" disabled={save.isPending} data-testid="setup-add-submit">
          {submit}
          <ArrowRightIcon className="rtl:rotate-180" />
        </Button>
      </form>
    </SetupLayout>
  )
}
