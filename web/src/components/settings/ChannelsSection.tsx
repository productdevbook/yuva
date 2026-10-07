import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CheckIcon, InfoIcon, PencilIcon, PlusIcon, RefreshCwIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { CopyButton, ErrorLine, useConfirm } from "@/components/common"
import { CHANNEL_KINDS, PLATFORMS, useEnumText } from "@/components/common/text"
import { Field, Section } from "@/components/settings/SettingsLayout"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import {
  api,
  ApiError,
  unwrap,
  type AppChannel,
  type AppChannelInput,
  type AppPlatform,
  type Channel,
  type ChannelKind,
  type ChatChannel,
  type ChatChannelInput,
  type ChatLauncherPosition,
  type EmailChannel,
  type EmailChannelInput,
  type Inbox,
  type SmtpTls,
} from "@/lib/api"
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

export function ChannelsSection({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const text = useEnumText()
  const [editing, setEditing] = useState<Channel | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const channels = useQuery({
    queryKey: keys.channels(ws, inbox.id),
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: inbox.id } } })).then(
        (r) => r.items,
      ),
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/channels/{channelId}", { params: { path: { channelId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.channels(ws, inbox.id) }),
  })
  return (
    <Section
      title={<Trans>Channels</Trans>}
      description={
        <Trans>How messages reach this inbox. E-mail channels receive and send mail, web chat channels power the chat widget, mobile app channels the iOS and Android SDKs, and API channels take feedback from your backend.</Trans>
      }
      action={
        canManage && (
          <Button variant="outline" size="sm" onClick={() => setEditing("new")}>
            <PlusIcon />
            <Trans>Add channel</Trans>
          </Button>
        )
      }
    >
      {channels.data && channels.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Trans>No channels yet.</Trans>
        </p>
      ) : (
        <ul className="flex flex-col divide-y rounded-lg border">
          {(channels.data ?? []).map((ch) => {
            const keyCount = Object.keys(ch.settings).length
            return (
              <li key={ch.id} className="flex items-center gap-3 px-3 py-2.5" data-testid="channel-row">
                <Badge variant="secondary">{text.channel[ch.kind]}</Badge>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{ch.name}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {ch.email ? (
                      <EmailSummary e={ch.email} />
                    ) : ch.chat ? (
                      <ChatSummary c={ch.chat} />
                    ) : ch.app ? (
                      <AppSummary c={ch.app} />
                    ) : keyCount === 0 ? (
                      <Trans>No settings</Trans>
                    ) : keyCount === 1 ? (
                      <Trans>1 setting</Trans>
                    ) : (
                      <Trans>{keyCount} settings</Trans>
                    )}
                  </p>
                </div>
                {canManage && (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={() => setEditing(ch)}>
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t`Remove`}
                      onClick={() =>
                        confirm({
                          title: <Trans>Remove this channel?</Trans>,
                          description: <Trans>Conversations that started on it keep their messages.</Trans>,
                          confirm: <Trans>Remove</Trans>,
                          run: () => remove.mutate(ch.id),
                        })
                      }
                    >
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </li>
            )
          })}
        </ul>
      )}
      <ErrorLine error={channels.error ?? remove.error} />
      {editing && (
        <ChannelDialog
          key={editing === "new" ? "new" : editing.id}
          inboxId={inbox.id}
          channel={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onCreated={(ch) => setEditing(ch.kind === "chat" || ch.kind === "app" ? ch : null)}
        />
      )}
      {confirmDialog}
    </Section>
  )
}

function EmailSummary({ e }: { e: EmailChannel }) {
  return (
    <>
      {e.address}
      {" · "}
      {e.smtp ? <Trans>sends through {e.smtp.host}</Trans> : <Trans>receives only, no SMTP account</Trans>}
    </>
  )
}

function ChatSummary({ c }: { c: ChatChannel }) {
  const first = c.allowed_origins[0] ?? ""
  const more = c.allowed_origins.length - 1
  return (
    <>
      {first}
      {more > 0 && <> +{more}</>}
      {" · "}
      {c.allow_anonymous ? <Trans>anonymous visitors allowed</Trans> : <Trans>signed-in users only</Trans>}
    </>
  )
}

function AppSummary({ c }: { c: AppChannel }) {
  const text = useEnumText()
  const platforms = c.platforms.map((p) => text.platform[p]).join(", ")
  return (
    <>
      {platforms}
      {" · "}
      {c.allow_anonymous ? <Trans>anonymous users allowed</Trans> : <Trans>signed-in users only</Trans>}
    </>
  )
}

const TLS: SmtpTls[] = ["starttls", "tls", "none"]

type EmailForm = {
  address: string
  displayName: string
  fromAddress: string
  host: string
  port: string
  username: string
  tls: SmtpTls
  password: string | null
  passwordSet: boolean
  clearing: boolean
  autoReply: boolean
  autoReplyText: string
  interval: string
}

function emailForm(e?: EmailChannel): EmailForm {
  return {
    address: e?.address ?? "",
    displayName: e?.display_name ?? "",
    fromAddress: e?.from_address ?? "",
    host: e?.smtp?.host ?? "",
    port: e?.smtp ? String(e.smtp.port) : "",
    username: e?.smtp?.username ?? "",
    tls: e?.smtp?.tls ?? "starttls",
    password: e?.smtp?.password_set ? null : "",
    passwordSet: !!e?.smtp?.password_set,
    clearing: false,
    autoReply: e?.auto_reply.enabled ?? false,
    autoReplyText: e?.auto_reply.text ?? "",
    interval: String(e?.auto_reply.interval_hours ?? 24),
  }
}

function emailInput(f: EmailForm): EmailChannelInput {
  const out: EmailChannelInput = {
    address: f.address.trim(),
    display_name: f.displayName.trim(),
    auto_reply: {
      enabled: f.autoReply,
      text: f.autoReplyText.trim(),
      interval_hours: Number(f.interval) || 24,
    },
  }
  if (f.fromAddress.trim()) out.from_address = f.fromAddress.trim()
  if (f.host.trim()) {
    out.smtp = { host: f.host.trim(), username: f.username.trim(), tls: f.tls }
    if (f.port.trim()) out.smtp.port = Number(f.port)
    if (f.password !== null) out.smtp.password = f.password
  }
  return out
}

type EmailField =
  | "address"
  | "display_name"
  | "from_address"
  | "smtp.host"
  | "smtp.port"
  | "smtp.username"
  | "smtp.password"
  | "smtp.tls"
  | "auto_reply.text"
  | "auto_reply.interval_hours"

function errorField(err: unknown): EmailField | null {
  if (!(err instanceof ApiError)) return null
  if (err.code === "email_address_taken") return "address"
  if (err.status !== 400 || !err.detail) return null
  const m = /\bemail\.([a-z_]+(?:\.[a-z_]+)?)/.exec(err.detail)
  return (m?.[1] as EmailField | undefined) ?? null
}

function useFieldErrors() {
  const { t } = useLingui()
  return {
    address: t`Enter a valid e-mail address.`,
    display_name: t`Use at most 200 characters.`,
    from_address: t`Enter a valid e-mail address, or leave it empty.`,
    "smtp.host": t`Enter a host name or IP address.`,
    "smtp.port": t`Use a port from 1 to 65535.`,
    "smtp.username": t`A username needs TLS or STARTTLS: the password is never sent unencrypted.`,
    "smtp.password": t`The password is too long.`,
    "smtp.tls": t`Pick a TLS mode.`,
    "auto_reply.text": t`Write the auto-reply, or turn it off.`,
    "auto_reply.interval_hours": t`Use 1 to 8760 hours.`,
  } satisfies Record<EmailField, string>
}

function ChannelDialog({
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
  const fieldErrors = useFieldErrors()
  const [kind, setKind] = useState<ChannelKind>(channel?.kind ?? "email")
  const [name, setName] = useState(channel?.name ?? "")
  const [raw, setRaw] = useState(channel ? JSON.stringify(channel.settings, null, 2) : "{}")
  const [email, setEmail] = useState<EmailForm>(() => emailForm(channel?.email))
  const [chat, setChat] = useState<ChatForm>(() => chatForm(channel?.chat))
  const [app, setApp] = useState<AppForm>(() => appForm(channel?.app))
  const isEmail = kind === "email"
  const isChat = kind === "chat"
  const isApp = kind === "app"
  const settings = isEmail || isChat || isApp ? {} : parseSettings(raw)
  const chatInput = isChat ? chatChannelInput(chat) : null
  const appInput = isApp ? appChannelInput(app) : null
  const valid = !!settings && (!isChat || chatInput?.ok === true) && (!isApp || !!appInput)
  const save = useMutation({
    mutationFn: () => {
      const typed = isEmail
        ? { email: emailInput(email) }
        : isChat && chatInput?.ok
          ? { chat: chatInput.value }
          : isApp && appInput
            ? { app: appInput }
            : { settings: settings ?? {} }
      return channel
        ? unwrap(
            api.PATCH("/v1/channels/{channelId}", {
              params: { path: { channelId: channel.id } },
              body: { name, ...typed },
            }),
          )
        : unwrap(
            api.POST("/v1/inboxes/{inboxId}/channels", {
              params: { path: { inboxId } },
              body: { kind, name, ...typed },
            }),
          )
    },
    onSuccess: (saved) => {
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inboxId) })
      void qc.invalidateQueries({ queryKey: ["ws", ws, "channel"] })
      if (channel) onClose()
      else onCreated(saved)
    },
  })
  const bad = errorField(save.error)
  const badChat = chatErrorField(save.error)
  const taken = save.error instanceof ApiError && save.error.code === "email_address_taken"
  const fieldError = (f: EmailField) => (bad === f && !taken ? fieldErrors[f] : undefined)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
        <form
          className="flex min-w-0 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            setApp((a) => ({ ...a, touched: true }))
            if (valid) {
              setChat((c) => ({ ...c, touched: true }))
              save.mutate()
            } else if (isChat) setChat((c) => ({ ...c, touched: true }))
          }}
        >
          <DialogHeader>
            <DialogTitle>{channel ? <Trans>Edit channel</Trans> : <Trans>Add channel</Trans>}</DialogTitle>
            <DialogDescription>
              <Trans>The kind cannot be changed later.</Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Kind</Trans>}>
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as ChannelKind)}
              items={text.channel}
              disabled={!!channel}
            >
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
              placeholder={isChat ? t`Website chat` : isApp ? t`iOS and Android app` : t`Support e-mail`}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          {isEmail ? (
            <EmailFields
              f={email}
              set={(patch) => setEmail((f) => ({ ...f, ...patch }))}
              error={fieldError}
              taken={taken}
            />
          ) : isChat ? (
            <>
              <ChatFields
                f={chat}
                set={(patch) => setChat((f) => ({ ...f, ...patch }))}
                invalid={chatInput && !chatInput.ok ? chatInput : null}
                serverError={badChat}
              />
              {channel?.chat && <ChatEmbed channel={channel} />}
            </>
          ) : isApp ? (
            <>
              <AppFields f={app} set={(patch) => setApp((f) => ({ ...f, ...patch }))} />
              {channel?.app && <AppInstall channel={channel} />}
            </>
          ) : (
            <Field
              label={<Trans>Settings (JSON)</Trans>}
              htmlFor="channel-settings"
              hint={settings ? undefined : <span className="text-destructive"><Trans>This is not a JSON object.</Trans></span>}
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
          <DialogFooter className="sticky -bottom-6 z-10 -mx-6 -mb-6 border-t bg-popover px-6 py-3">
            <Button type="submit" disabled={save.isPending || !settings}>
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function FieldError({ children, id }: { children?: React.ReactNode; id: string }) {
  if (!children) return null
  return (
    <p id={id} role="alert" className="text-xs text-destructive">
      {children}
    </p>
  )
}

function EmailFields({
  f,
  set,
  error,
  taken,
}: {
  f: EmailForm
  set: (patch: Partial<EmailForm>) => void
  error: (field: EmailField) => string | undefined
  taken: boolean
}) {
  const { t } = useLingui()
  const ingress = `${window.location.origin}/ingress/email`
  const tlsText: Record<SmtpTls, string> = {
    starttls: t`STARTTLS (port 587)`,
    tls: t`TLS (port 465)`,
    none: t`None (local relays and test servers only)`,
  }
  const input = (field: EmailField) => ({
    "aria-invalid": !!error(field) || (field === "address" && taken) || undefined,
    "aria-describedby": error(field) ? `err-${field}` : undefined,
  })
  return (
    <>
      <fieldset className="flex min-w-0 flex-col gap-4">
        <legend className="mb-3 text-sm font-semibold">
          <Trans>Receiving</Trans>
        </legend>
        <Field label={<Trans>Support address</Trans>} htmlFor="email-address">
          <Input
            id="email-address"
            type="email"
            required
            maxLength={320}
            value={f.address}
            placeholder="support@example.com"
            onChange={(e) => set({ address: e.target.value })}
            {...input("address")}
          />
          <FieldError id="err-address">
            {taken ? <Trans>Another channel already receives mail at this address.</Trans> : error("address")}
          </FieldError>
        </Field>
        <div className="flex gap-2 rounded-lg border bg-muted/40 p-3 text-xs text-muted-foreground" data-testid="ingress-hint">
          <InfoIcon className="mt-px size-4 shrink-0" />
          <div className="flex min-w-0 flex-col gap-2">
            <p>
              <Trans>
                Forward mail for this address to the Cloudflare Email Worker in edge/ of the Yuva repository, or to any
                mail server that posts each message to this server's ingress URL, signed with YUVA_INGRESS_SECRET:
              </Trans>
            </p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded border bg-background px-2 py-1 font-mono">{ingress}</code>
              <CopyButton value={ingress} />
            </div>
          </div>
        </div>
      </fieldset>
      <fieldset className="flex min-w-0 flex-col gap-4">
        <legend className="mb-3 text-sm font-semibold">
          <Trans>Sending</Trans>
        </legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={<Trans>Display name</Trans>} htmlFor="email-display-name" hint={<Trans>The channel name when empty.</Trans>}>
            <Input
              id="email-display-name"
              maxLength={200}
              value={f.displayName}
              placeholder={t`Acme Support`}
              onChange={(e) => set({ displayName: e.target.value })}
              {...input("display_name")}
            />
            <FieldError id="err-display_name">{error("display_name")}</FieldError>
          </Field>
          <Field
            label={<Trans>From address (optional)</Trans>}
            htmlFor="email-from"
            hint={<Trans>When your provider sends from another address. Replies still come to the support address.</Trans>}
          >
            <Input
              id="email-from"
              type="email"
              maxLength={320}
              value={f.fromAddress}
              placeholder="no-reply@example.com"
              onChange={(e) => set({ fromAddress: e.target.value })}
              {...input("from_address")}
            />
            <FieldError id="err-from_address">{error("from_address")}</FieldError>
          </Field>
        </div>
        <p className="text-xs text-muted-foreground">
          <Trans>Replies go out through your own SMTP account. Leave the host empty to only receive mail.</Trans>
        </p>
        <div className="grid gap-4 sm:grid-cols-[1fr_7rem]">
          <Field label={<Trans>SMTP host</Trans>} htmlFor="smtp-host">
            <Input
              id="smtp-host"
              maxLength={253}
              value={f.host}
              placeholder="smtp.example.com"
              autoComplete="off"
              onChange={(e) => set({ host: e.target.value })}
              {...input("smtp.host")}
            />
            <FieldError id="err-smtp.host">{error("smtp.host")}</FieldError>
          </Field>
          <Field label={<Trans>Port</Trans>} htmlFor="smtp-port">
            <Input
              id="smtp-port"
              type="number"
              min={1}
              max={65535}
              value={f.port}
              placeholder={f.tls === "tls" ? "465" : "587"}
              onChange={(e) => set({ port: e.target.value })}
              {...input("smtp.port")}
            />
            <FieldError id="err-smtp.port">{error("smtp.port")}</FieldError>
          </Field>
        </div>
        <Field label={<Trans>Encryption</Trans>}>
          <Select value={f.tls} onValueChange={(v) => set({ tls: v as SmtpTls })} items={tlsText}>
            <SelectTrigger className="w-full" aria-label={t`Encryption`} {...input("smtp.tls")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TLS.map((x) => (
                <SelectItem key={x} value={x}>
                  {tlsText[x]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldError id="err-smtp.tls">{error("smtp.tls")}</FieldError>
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={<Trans>Username</Trans>} htmlFor="smtp-username">
            <Input
              id="smtp-username"
              maxLength={320}
              value={f.username}
              autoComplete="off"
              onChange={(e) => set({ username: e.target.value })}
              {...input("smtp.username")}
            />
            <FieldError id="err-smtp.username">{error("smtp.username")}</FieldError>
          </Field>
          <Field label={<Trans>Password</Trans>} htmlFor="smtp-password">
            {f.password === null ? (
              <div className="flex h-8 items-center gap-2" data-testid="smtp-password-set">
                <span className="inline-flex min-w-0 flex-1 items-center gap-1 text-sm">
                  <CheckIcon className="size-4 shrink-0 text-success" />
                  <Trans>Password set</Trans>
                </span>
                <Button type="button" variant="outline" size="sm" onClick={() => set({ password: "" })}>
                  <Trans>Replace</Trans>
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => set({ password: "", passwordSet: false, clearing: true })}
                >
                  <Trans>Remove</Trans>
                </Button>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <Input
                  id="smtp-password"
                  type="password"
                  maxLength={1024}
                  value={f.password}
                  autoComplete="new-password"
                  placeholder={f.passwordSet ? t`New password` : undefined}
                  onChange={(e) => set({ password: e.target.value })}
                  {...input("smtp.password")}
                />
                {(f.passwordSet || f.clearing) && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => set({ password: null, passwordSet: true, clearing: false })}
                  >
                    <Trans>Keep</Trans>
                  </Button>
                )}
              </div>
            )}
            {f.clearing && f.password === "" && (
              <p className="text-xs text-muted-foreground" data-testid="smtp-password-cleared">
                <Trans>Saving removes the stored password.</Trans>
              </p>
            )}
            <FieldError id="err-smtp.password">{error("smtp.password")}</FieldError>
          </Field>
        </div>
        <p className="text-xs text-muted-foreground">
          <Trans>The password is stored encrypted and never shown again.</Trans>
        </p>
      </fieldset>
      <fieldset className="flex min-w-0 flex-col gap-4">
        <legend className="mb-3 flex w-full items-center justify-between gap-3 text-sm font-semibold">
          <Trans>Auto-reply</Trans>
          <label className="flex items-center gap-2 text-sm font-normal">
            <Switch
              checked={f.autoReply}
              onCheckedChange={(autoReply) => set({ autoReply })}
              aria-label={t`Send an auto-reply`}
            />
            {f.autoReply ? <Trans>On</Trans> : <Trans>Off</Trans>}
          </label>
        </legend>
        <p className="-mt-2 text-xs text-muted-foreground">
          <Trans>Greets new conversations by e-mail, once per contact within the interval. Never sent to automatic mail or spam.</Trans>
        </p>
        {f.autoReply && (
          <>
            <Field label={<Trans>Text</Trans>} htmlFor="auto-reply-text">
              <Textarea
                id="auto-reply-text"
                rows={4}
                maxLength={5000}
                required
                value={f.autoReplyText}
                placeholder={t`Thanks for writing. We usually answer within a day.`}
                onChange={(e) => set({ autoReplyText: e.target.value })}
                {...input("auto_reply.text")}
              />
              <FieldError id="err-auto_reply.text">{error("auto_reply.text")}</FieldError>
            </Field>
            <Field label={<Trans>At most once per contact every (hours)</Trans>} htmlFor="auto-reply-interval">
              <Input
                id="auto-reply-interval"
                type="number"
                min={1}
                max={8760}
                className="w-32"
                value={f.interval}
                onChange={(e) => set({ interval: e.target.value })}
                {...input("auto_reply.interval_hours")}
              />
              <FieldError id="err-auto_reply.interval_hours">{error("auto_reply.interval_hours")}</FieldError>
            </Field>
          </>
        )}
      </fieldset>
    </>
  )
}

const MAX_ORIGINS = 20
const DEFAULT_COLOR = "#2563eb"
const POSITIONS: ChatLauncherPosition[] = ["right", "left"]

type ChatForm = {
  origins: string
  allowAnonymous: boolean
  askEmailOffline: boolean
  greeting: string
  position: ChatLauncherPosition
  customColor: boolean
  color: string
  touched: boolean
}

function chatForm(c?: ChatChannel): ChatForm {
  return {
    origins: c?.allowed_origins.join("\n") ?? "",
    allowAnonymous: c?.allow_anonymous ?? false,
    askEmailOffline: c?.ask_email_offline ?? true,
    greeting: c?.greeting ?? "",
    position: c?.launcher.position ?? "right",
    customColor: !!c?.launcher.color,
    color: c?.launcher.color ?? DEFAULT_COLOR,
    touched: false,
  }
}

function normalizeOrigin(raw: string): string | null {
  const v = raw.trim().replace(/\/$/, "")
  if (!/^https?:\/\/[^/?#\s]+$/i.test(v)) return null
  try {
    const u = new URL(v)
    if (u.username || u.password) return null
    return u.origin.length <= 300 ? u.origin : null
  } catch {
    return null
  }
}

type ChatInputResult =
  | { ok: true; value: ChatChannelInput }
  | { ok: false; empty: boolean; tooMany: boolean; bad: string[] }

function chatChannelInput(f: ChatForm): ChatInputResult {
  const lines = f.origins
    .split(/[\n,]+/)
    .map((x) => x.trim())
    .filter(Boolean)
  const bad = lines.filter((l) => !normalizeOrigin(l))
  const origins = [...new Set(lines.map(normalizeOrigin).filter((x): x is string => !!x))]
  if (bad.length > 0 || origins.length === 0 || origins.length > MAX_ORIGINS) {
    return { ok: false, empty: lines.length === 0, tooMany: origins.length > MAX_ORIGINS, bad }
  }
  return {
    ok: true,
    value: {
      allowed_origins: origins,
      allow_anonymous: f.allowAnonymous,
      ask_email_offline: f.askEmailOffline,
      greeting: f.greeting.trim(),
      launcher: { position: f.position, ...(f.customColor ? { color: f.color.toLowerCase() } : {}) },
    },
  }
}

type ChatField = "allowed_origins" | "greeting" | "launcher.color" | "launcher.position"

function chatErrorField(err: unknown): ChatField | null {
  if (!(err instanceof ApiError) || err.status !== 400 || !err.detail) return null
  const m = /\bchat\.([a-z_]+(?:\.[a-z_]+)?)/.exec(err.detail)
  return (m?.[1] as ChatField | undefined) ?? null
}

function ChatFields({
  f,
  set,
  invalid,
  serverError,
}: {
  f: ChatForm
  set: (patch: Partial<ChatForm>) => void
  invalid: Extract<ChatInputResult, { ok: false }> | null
  serverError: ChatField | null
}) {
  const { t } = useLingui()
  const positionText: Record<ChatLauncherPosition, string> = { right: t`Bottom right`, left: t`Bottom left` }
  const notOrigins = invalid?.bad.join(", ") ?? ""
  const originsError =
    serverError === "allowed_origins" ? (
      <Trans>Use origins such as https://www.example.com, without a path.</Trans>
    ) : invalid && (f.touched || invalid.bad.length > 0) ? (
      invalid.bad.length > 0 ? (
        <Trans>Not an origin: {notOrigins}. Use scheme and host only, such as https://www.example.com.</Trans>
      ) : invalid.tooMany ? (
        <Trans>List at most 20 origins.</Trans>
      ) : (
        <Trans>Add at least one origin.</Trans>
      )
    ) : null
  return (
    <>
      <fieldset className="flex min-w-0 flex-col gap-4">
        <legend className="mb-3 text-sm font-semibold">
          <Trans>Web chat</Trans>
        </legend>
        <Field
          label={<Trans>Allowed origins</Trans>}
          htmlFor="chat-origins"
          hint={<Trans>One per line, 1 to 20. Only pages on these origins can use the widget.</Trans>}
        >
          <Textarea
            id="chat-origins"
            rows={3}
            spellCheck={false}
            className="font-mono text-xs"
            value={f.origins}
            placeholder={"https://www.example.com\nhttps://app.example.com"}
            onChange={(e) => set({ origins: e.target.value })}
            aria-invalid={!!originsError || undefined}
            aria-describedby={originsError ? "err-chat-origins" : undefined}
            data-testid="chat-origins"
          />
          <FieldError id="err-chat-origins">{originsError}</FieldError>
        </Field>
        <label className="flex items-start justify-between gap-3 text-sm">
          <span className="flex flex-col gap-0.5">
            <Trans>Allow anonymous visitors</Trans>
            <span className="text-xs text-muted-foreground">
              <Trans>Visitors can chat without an identity token from your backend.</Trans>
            </span>
          </span>
          <Switch
            checked={f.allowAnonymous}
            onCheckedChange={(allowAnonymous) => set({ allowAnonymous })}
            aria-label={t`Allow anonymous visitors`}
          />
        </label>
        <label className="flex items-start justify-between gap-3 text-sm">
          <span className="flex flex-col gap-0.5">
            <Trans>Ask for an e-mail address when offline</Trans>
            <span className="text-xs text-muted-foreground">
              <Trans>When nobody is available, the widget asks where to send the reply.</Trans>
            </span>
          </span>
          <Switch
            checked={f.askEmailOffline}
            onCheckedChange={(askEmailOffline) => set({ askEmailOffline })}
            aria-label={t`Ask for an e-mail address when offline`}
          />
        </label>
        <Field
          label={<Trans>Greeting</Trans>}
          htmlFor="chat-greeting"
          hint={<Trans>Shown before the first message. The inbox greeting when empty.</Trans>}
        >
          <Textarea
            id="chat-greeting"
            rows={2}
            maxLength={500}
            value={f.greeting}
            placeholder={t`Hi! How can we help?`}
            onChange={(e) => set({ greeting: e.target.value })}
            aria-invalid={serverError === "greeting" || undefined}
          />
          <FieldError id="err-chat-greeting">
            {serverError === "greeting" && <Trans>Use at most 500 characters.</Trans>}
          </FieldError>
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={<Trans>Launcher position</Trans>}>
            <Select value={f.position} onValueChange={(v) => set({ position: v as ChatLauncherPosition })} items={positionText}>
              <SelectTrigger className="w-full" aria-label={t`Launcher position`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {POSITIONS.map((x) => (
                  <SelectItem key={x} value={x}>
                    {positionText[x]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label={<Trans>Launcher color</Trans>} htmlFor="chat-color">
            <div className="flex h-8 items-center gap-2">
              <Switch
                checked={f.customColor}
                onCheckedChange={(customColor) => set({ customColor })}
                aria-label={t`Use a custom launcher color`}
              />
              {f.customColor ? (
                <>
                  <input
                    id="chat-color"
                    type="color"
                    value={f.color}
                    onChange={(e) => set({ color: e.target.value })}
                    className="h-8 w-10 cursor-pointer rounded-md border bg-background p-0.5"
                    aria-label={t`Launcher color`}
                  />
                  <code className="font-mono text-xs text-muted-foreground">{f.color}</code>
                </>
              ) : (
                <span className="text-sm text-muted-foreground">
                  <Trans>Inbox color</Trans>
                </span>
              )}
            </div>
            <FieldError id="err-chat-color">
              {serverError === "launcher.color" && <Trans>Pick a color.</Trans>}
            </FieldError>
          </Field>
        </div>
      </fieldset>
    </>
  )
}

function embedSnippet(publicKey: string) {
  const origin = window.location.origin
  return `<script src="${origin}/yuva.js"></script>\n<yuva-chat channel="${publicKey}" server="${origin}"></yuva-chat>`
}

function PublicKeyField({
  channel,
  value,
  onRotated,
  hint,
  warning,
}: {
  channel: Channel
  value: string
  onRotated: (key: string) => void
  hint: React.ReactNode
  warning: React.ReactNode
}) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const rotate = useMutation({
    mutationFn: () =>
      unwrap(api.POST("/v1/channels/{channelId}/public-key", { params: { path: { channelId: channel.id } } })),
    onSuccess: (data) => {
      const key = data.chat?.public_key ?? data.app?.public_key
      if (key) onRotated(key)
      void qc.invalidateQueries({ queryKey: keys.channels(ws, channel.inbox_id) })
      qc.setQueryData(keys.channel(ws, channel.id), data)
    },
  })
  return (
    <Field label={<Trans>Public key</Trans>} hint={hint}>
      <div className="flex items-center gap-2">
        <code
          className="min-w-0 flex-1 truncate rounded-md border bg-muted px-2 py-1.5 font-mono text-xs"
          data-testid="channel-public-key"
        >
          {value}
        </code>
        <CopyButton value={value} />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={rotate.isPending}
          data-testid="rotate-public-key"
          onClick={() =>
            confirm({
              title: <Trans>Rotate the public key?</Trans>,
              description: warning,
              confirm: <Trans>Rotate</Trans>,
              run: () => rotate.mutate(),
            })
          }
        >
          <RefreshCwIcon />
          <Trans>Rotate</Trans>
        </Button>
      </div>
      <ErrorLine error={rotate.error} className="text-xs" />
      {confirmDialog}
    </Field>
  )
}

function ChatEmbed({ channel }: { channel: Channel }) {
  const [publicKey, setPublicKey] = useState(channel.chat!.public_key)
  const snippet = embedSnippet(publicKey)
  return (
    <fieldset className="flex min-w-0 flex-col gap-4" data-testid="chat-embed">
      <legend className="mb-3 text-sm font-semibold">
        <Trans>Install</Trans>
      </legend>
      <PublicKeyField
        channel={channel}
        value={publicKey}
        onRotated={setPublicKey}
        hint={<Trans>Not a secret: it is part of your web pages. Rotate it to stop old embeds from starting new chats.</Trans>}
        warning={
          <Trans>
            Pages that embed the old key can no longer start chats until you update the snippet. Visitors already
            chatting keep their session.
          </Trans>
        }
      />
      <Field
        label={<Trans>Embed snippet</Trans>}
        hint={<Trans>Paste it before the closing body tag of every page that should show the chat.</Trans>}
      >
        <div className="flex flex-col gap-2">
          <pre
            className="max-w-full rounded-md border bg-muted px-2 py-1.5 font-mono text-xs break-all whitespace-pre-wrap"
            data-testid="chat-snippet"
          >
            {snippet}
          </pre>
          <CopyButton value={snippet} className="self-start" />
        </div>
      </Field>
    </fieldset>
  )
}

type AppForm = { allowAnonymous: boolean; platforms: AppPlatform[]; touched: boolean }

function appForm(c?: AppChannel): AppForm {
  return { allowAnonymous: c?.allow_anonymous ?? false, platforms: c?.platforms ?? ["ios", "android"], touched: false }
}

function appChannelInput(f: AppForm): AppChannelInput | null {
  if (f.platforms.length === 0) return null
  return { allow_anonymous: f.allowAnonymous, platforms: PLATFORMS.filter((p) => f.platforms.includes(p)) }
}

function AppFields({ f, set }: { f: AppForm; set: (patch: Partial<AppForm>) => void }) {
  const { t } = useLingui()
  const text = useEnumText()
  const none = f.platforms.length === 0
  return (
    <fieldset className="flex min-w-0 flex-col gap-4">
      <legend className="mb-3 text-sm font-semibold">
        <Trans>Mobile app</Trans>
      </legend>
      <div className="flex flex-col gap-1.5">
        <span className="text-sm font-medium" id="app-platforms">
          <Trans>Platforms</Trans>
        </span>
        <div className="flex gap-4" role="group" aria-labelledby="app-platforms">
          {PLATFORMS.map((p) => (
            <label key={p} className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={f.platforms.includes(p)}
                onCheckedChange={(on) =>
                  set({ platforms: on ? [...f.platforms, p] : f.platforms.filter((x) => x !== p) })
                }
                aria-invalid={(none && f.touched) || undefined}
              />
              {text.platform[p]}
            </label>
          ))}
        </div>
        <FieldError id="err-app-platforms">{none && <Trans>Pick at least one platform.</Trans>}</FieldError>
      </div>
      <label className="flex items-start justify-between gap-3 text-sm">
        <span className="flex flex-col gap-0.5">
          <Trans>Allow anonymous users</Trans>
          <span className="text-xs text-muted-foreground">
            <Trans>People who are not signed in to your app can write without an identity token from your backend.</Trans>
          </span>
        </span>
        <Switch
          checked={f.allowAnonymous}
          onCheckedChange={(allowAnonymous) => set({ allowAnonymous })}
          aria-label={t`Allow anonymous users`}
        />
      </label>
    </fieldset>
  )
}

function AppInstall({ channel }: { channel: Channel }) {
  const [publicKey, setPublicKey] = useState(channel.app!.public_key)
  const server = window.location.origin
  return (
    <fieldset className="flex min-w-0 flex-col gap-4" data-testid="app-install">
      <legend className="mb-3 text-sm font-semibold">
        <Trans>Install</Trans>
      </legend>
      <Field label={<Trans>Server URL</Trans>}>
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded-md border bg-muted px-2 py-1.5 font-mono text-xs">{server}</code>
          <CopyButton value={server} />
        </div>
      </Field>
      <PublicKeyField
        channel={channel}
        value={publicKey}
        onRotated={setPublicKey}
        hint={<Trans>Not a secret: it ships inside your app. Native apps send no origin, so the key is not tied to one.</Trans>}
        warning={
          <Trans>
            App builds with the old key can no longer start sessions until you ship one with the new key. People already
            signed in keep their session.
          </Trans>
        }
      />
      <div className="flex gap-2 rounded-lg border bg-muted/40 p-3 text-xs text-muted-foreground" data-testid="app-hint">
        <InfoIcon className="mt-px size-4 shrink-0" />
        <ul className="flex min-w-0 flex-col gap-1.5">
          <li>
            <Trans>
              <span className="font-medium text-foreground">iOS:</span> add the Yuva repository as a Swift package and
              use the <code className="font-mono">YuvaKit</code> library; create the client with the server URL and
              this key.
            </Trans>
          </li>
          <li>
            <Trans>
              <span className="font-medium text-foreground">Android:</span> add the library from{" "}
              <code className="font-mono">sdk/kotlin</code> in the Yuva repository and configure it with the same two
              values.
            </Trans>
          </li>
          <li>
            <Trans>
              For signed-in users, your backend signs an identity token with the inbox's identity secret and the app
              passes it to the SDK.
            </Trans>
          </li>
        </ul>
      </div>
    </fieldset>
  )
}
