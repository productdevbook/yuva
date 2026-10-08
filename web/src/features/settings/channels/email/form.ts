import { useLingui } from "@lingui/react/macro"

import { ApiError, type EmailChannel, type EmailChannelInput, type SmtpTls } from "@/lib/api"

export const TLS: SmtpTls[] = ["starttls", "tls", "none"]

export type EmailForm = {
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

export function emailForm(e?: EmailChannel): EmailForm {
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

export function emailInput(f: EmailForm): EmailChannelInput {
  const out: EmailChannelInput = {
    address: f.address.trim(),
    display_name: f.displayName.trim(),
    auto_reply: { enabled: f.autoReply, text: f.autoReplyText.trim(), interval_hours: Number(f.interval) || 24 },
  }
  if (f.fromAddress.trim()) out.from_address = f.fromAddress.trim()
  if (f.host.trim()) {
    out.smtp = { host: f.host.trim(), username: f.username.trim(), tls: f.tls }
    if (f.port.trim()) out.smtp.port = Number(f.port)
    if (f.password !== null) out.smtp.password = f.password
  }
  return out
}

export type EmailField =
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

export function emailErrorField(err: unknown): EmailField | null {
  if (!(err instanceof ApiError)) return null
  if (err.code === "email_address_taken") return "address"
  if (err.status !== 400 || !err.detail) return null
  const m = /\bemail\.([a-z_]+(?:\.[a-z_]+)?)/.exec(err.detail)
  return (m?.[1] as EmailField | undefined) ?? null
}

export function useEmailFieldErrors() {
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
