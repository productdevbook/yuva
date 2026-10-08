import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon } from "lucide-react"

import { CodeLine, Notice } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { TLS, type EmailField, type EmailForm } from "@/features/settings/channels/email/form"
import { FieldError, FormBlock } from "@/features/settings/channels/parts"
import { Field } from "@/features/settings/ui"
import type { SmtpTls } from "@/lib/api"

type Props = {
  f: EmailForm
  set: (patch: Partial<EmailForm>) => void
  error: (field: EmailField) => string | undefined
  taken: boolean
}

function useInvalid({ error, taken }: Pick<Props, "error" | "taken">) {
  return (field: EmailField) => ({
    "aria-invalid": !!error(field) || (field === "address" && taken) || undefined,
    "aria-describedby": error(field) ? `err-${field}` : undefined,
  })
}

function Receiving({ f, set, error, taken }: Props) {
  const invalid = useInvalid({ error, taken })
  const ingress = `${window.location.origin}/ingress/email`
  return (
    <FormBlock title={<Trans>Receiving</Trans>}>
      <Field
        label={<Trans>Support address</Trans>}
        htmlFor="email-address"
        hint={
          <Trans>
            *@example.com receives every address of the domain that no other channel has; replies then go out from the
            address the customer wrote to.
          </Trans>
        }
      >
        <Input
          id="email-address"
          type="email"
          required
          maxLength={320}
          value={f.address}
          placeholder="support@example.com"
          onChange={(e) => set({ address: e.target.value })}
          {...invalid("address")}
        />
        <FieldError id="err-address">{taken ? <Trans>Another channel already receives mail at this address.</Trans> : error("address")}</FieldError>
      </Field>
      <Notice className="text-xs" data-testid="ingress-hint">
        <p>
          <Trans>
            Forward mail for this address to the Cloudflare Email Worker in edge/ of the Yuva repository, or to any mail
            server that posts each message to this server's ingress URL, signed with YUVA_INGRESS_SECRET:
          </Trans>
        </p>
        <CodeLine value={ingress} />
      </Notice>
    </FormBlock>
  )
}

function Password({ f, set, error, taken }: Props) {
  const { t } = useLingui()
  const invalid = useInvalid({ error, taken })
  return (
    <Field label={<Trans>Password</Trans>} htmlFor="smtp-password">
      {f.password === null ? (
        <div className="flex h-9 items-center gap-2" data-testid="smtp-password-set">
          <span className="inline-flex min-w-0 flex-1 items-center gap-1.5 text-sm">
            <CheckIcon className="size-4 shrink-0 text-success" />
            <Trans>Password set</Trans>
          </span>
          <Button type="button" variant="outline" size="sm" onClick={() => set({ password: "" })}>
            <Trans>Replace</Trans>
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => set({ password: "", passwordSet: false, clearing: true })}>
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
            {...invalid("smtp.password")}
          />
          {(f.passwordSet || f.clearing) && (
            <Button type="button" variant="ghost" size="sm" onClick={() => set({ password: null, passwordSet: true, clearing: false })}>
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
  )
}

function Sending(p: Props) {
  const { t } = useLingui()
  const { f, set, error } = p
  const invalid = useInvalid(p)
  const tlsText: Record<SmtpTls, string> = {
    starttls: t`STARTTLS (port 587)`,
    tls: t`TLS (port 465)`,
    none: t`None (local relays and test servers only)`,
  }
  return (
    <FormBlock title={<Trans>Sending</Trans>}>
      <div className="grid gap-5 sm:grid-cols-2">
        <Field label={<Trans>Display name</Trans>} htmlFor="email-display-name" hint={<Trans>The channel name when empty.</Trans>}>
          <Input
            id="email-display-name"
            maxLength={200}
            value={f.displayName}
            placeholder={t`Acme Support`}
            onChange={(e) => set({ displayName: e.target.value })}
            {...invalid("display_name")}
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
            {...invalid("from_address")}
          />
          <FieldError id="err-from_address">{error("from_address")}</FieldError>
        </Field>
      </div>
      <p className="text-xs text-muted-foreground">
        <Trans>Replies go out through your own SMTP account. Leave the host empty to only receive mail.</Trans>
      </p>
      <div className="grid gap-5 sm:grid-cols-[1fr_7rem]">
        <Field label={<Trans>SMTP host</Trans>} htmlFor="smtp-host">
          <Input
            id="smtp-host"
            maxLength={253}
            value={f.host}
            placeholder="smtp.example.com"
            autoComplete="off"
            onChange={(e) => set({ host: e.target.value })}
            {...invalid("smtp.host")}
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
            {...invalid("smtp.port")}
          />
          <FieldError id="err-smtp.port">{error("smtp.port")}</FieldError>
        </Field>
      </div>
      <Field label={<Trans>Encryption</Trans>}>
        <Select value={f.tls} onValueChange={(v) => set({ tls: v as SmtpTls })} items={tlsText}>
          <SelectTrigger className="w-full" aria-label={t`Encryption`} {...invalid("smtp.tls")}>
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
      <div className="grid gap-5 sm:grid-cols-2">
        <Field label={<Trans>Username</Trans>} htmlFor="smtp-username">
          <Input id="smtp-username" maxLength={320} value={f.username} autoComplete="off" onChange={(e) => set({ username: e.target.value })} {...invalid("smtp.username")} />
          <FieldError id="err-smtp.username">{error("smtp.username")}</FieldError>
        </Field>
        <Password {...p} />
      </div>
      <p className="text-xs text-muted-foreground">
        <Trans>The password is stored encrypted and never shown again.</Trans>
      </p>
    </FormBlock>
  )
}

function AutoReply(p: Props) {
  const { t } = useLingui()
  const { f, set, error } = p
  const invalid = useInvalid(p)
  return (
    <FormBlock
      title={<Trans>Auto-reply</Trans>}
      action={<Switch checked={f.autoReply} onCheckedChange={(autoReply) => set({ autoReply })} aria-label={t`Send an auto-reply`} />}
    >
      <p className="-mt-2 text-xs leading-relaxed text-muted-foreground">
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
              {...invalid("auto_reply.text")}
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
              {...invalid("auto_reply.interval_hours")}
            />
            <FieldError id="err-auto_reply.interval_hours">{error("auto_reply.interval_hours")}</FieldError>
          </Field>
        </>
      )}
    </FormBlock>
  )
}

export function EmailFields(p: Props) {
  return (
    <>
      <Receiving {...p} />
      <Sending {...p} />
      <AutoReply {...p} />
    </>
  )
}
