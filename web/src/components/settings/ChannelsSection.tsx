import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CheckIcon, InfoIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { CopyButton, ErrorLine, useConfirm } from "@/components/common"
import { CHANNEL_KINDS, useEnumText } from "@/components/common/text"
import { Field, Section } from "@/components/settings/SettingsLayout"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
  type Channel,
  type ChannelKind,
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
        <Trans>How messages reach this inbox. E-mail channels receive and send mail; the other kinds store their settings for now.</Trans>
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
          inboxId={inbox.id}
          channel={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
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
}: {
  inboxId: string
  channel: Channel | null
  onClose: () => void
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
  const isEmail = kind === "email"
  const settings = isEmail ? {} : parseSettings(raw)
  const save = useMutation({
    mutationFn: () =>
      channel
        ? unwrap(
            api.PATCH("/v1/channels/{channelId}", {
              params: { path: { channelId: channel.id } },
              body: isEmail ? { name, email: emailInput(email) } : { name, settings: settings ?? {} },
            }),
          )
        : unwrap(
            api.POST("/v1/inboxes/{inboxId}/channels", {
              params: { path: { inboxId } },
              body: isEmail ? { kind, name, email: emailInput(email) } : { kind, name, settings: settings ?? {} },
            }),
          ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inboxId) })
      void qc.invalidateQueries({ queryKey: ["ws", ws, "channel"] })
      onClose()
    },
  })
  const bad = errorField(save.error)
  const taken = save.error instanceof ApiError && save.error.code === "email_address_taken"
  const fieldError = (f: EmailField) => (bad === f && !taken ? fieldErrors[f] : undefined)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
        <form
          className="flex min-w-0 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (settings) save.mutate()
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
              placeholder={t`Support e-mail`}
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
          {!bad && <ErrorLine error={save.error} />}
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
