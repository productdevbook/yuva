import { Trans, useLingui } from "@lingui/react/macro"
import {
  CircleAlertIcon,
  CircleCheckIcon,
  CircleDashedIcon,
  LoaderIcon,
  RefreshCwIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { useState } from "react"

import { CodeBlock, CodeLine, ErrorLine, Notice, useConfirm } from "@/components/common"
import { formatDateTime, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  isChecking,
  RateLimited,
  useCreateDkim,
  useDeleteDkim,
  useDeliverability,
  useRunCheck,
  useSetDkim,
} from "@/features/settings/channels/email/deliverability"
import { PageSection } from "@/features/settings/channels/sections"
import { Card, Field, StatusTag } from "@/features/settings/ui"
import { ApiError, type Channel, type DeliverabilityCheck } from "@/lib/api"
import { useSession } from "@/lib/session"

type Tone = "ok" | "warn" | "bad" | "unknown"

const icons: Record<Tone, React.ReactNode> = {
  ok: <CircleCheckIcon className="mt-0.5 size-4 shrink-0 text-success" />,
  warn: <TriangleAlertIcon className="mt-0.5 size-4 shrink-0 text-warning" />,
  bad: <CircleAlertIcon className="mt-0.5 size-4 shrink-0 text-destructive" />,
  unknown: <CircleDashedIcon className="mt-0.5 size-4 shrink-0 text-faint" />,
}

type Line = { key: string; title: string; tone: Tone; text: string; record?: string }

function useLines(c: DeliverabilityCheck): Line[] {
  const { t } = useLingui()
  const read = c.state === "received"
  const from = c.domain
  const lines: Line[] = []
  const running = c.state === "queued" || c.state === "sent"
  const dnsRead = Boolean(c.mx) && !running
  const readingDns = t`Reading DNS records…`

  const spf = c.spf
  const spfDomain = spf.domain ?? from
  const spfLine = (): Pick<Line, "tone" | "text"> => {
    if (read && spf.status === "pass") {
      return spf.aligned
        ? { tone: "ok", text: t`SPF passes for ${spfDomain}.` }
        : {
            tone: "warn",
            text: t`SPF passes for ${spfDomain}, not for the sending domain ${from}, so DMARC relies on DKIM alone. Many providers let you set a custom return-path on your domain.`,
          }
    }
    if (read && (spf.status === "fail" || spf.status === "softfail" || spf.status === "permerror")) {
      return {
        tone: "bad",
        text: t`SPF fails for ${spfDomain}. Add your SMTP provider to its SPF record, and keep it to one valid record.`,
      }
    }
    if (!dnsRead) return { tone: "unknown", text: readingDns }
    if (!spf.record)
      return { tone: "warn", text: t`${from} has no SPF record. Publish one that includes your SMTP provider.` }
    return read
      ? {
          tone: "warn",
          text: t`The receiving server did not confirm SPF. Check that the record includes your SMTP provider.`,
        }
      : { tone: "unknown", text: t`${from} has an SPF record. Whether it passes is known once the test mail is read.` }
  }
  lines.push({ key: "spf", title: "SPF", record: spf.record, ...spfLine() })

  const dkim = c.dkim
  const dkimDomain = dkim.domain ?? ""
  if (!read) {
    lines.push({
      key: "dkim",
      title: "DKIM",
      tone: "unknown",
      text: t`Known only from the receiving server’s results of the test mail.`,
    })
  } else if (dkim.status !== "pass") {
    lines.push({
      key: "dkim",
      title: "DKIM",
      tone: "bad",
      text: t`No DKIM signature passed, so broadcasts cannot be sent. Set up DKIM with your SMTP provider, or turn on Yuva DKIM below.`,
    })
  } else if (!dkim.aligned) {
    lines.push({
      key: "dkim",
      title: "DKIM",
      tone: "warn",
      text: t`DKIM passes for ${dkimDomain}, not for the sending domain ${from}. Set up DKIM for ${from} with your provider, or turn on Yuva DKIM below.`,
    })
  } else if (!dkim.covers_list_unsubscribe) {
    lines.push({
      key: "dkim",
      title: "DKIM",
      tone: "warn",
      text: t`DKIM passes for ${dkimDomain}, but the signature does not cover the unsubscribe headers, which Gmail and Yahoo ask for. Turn on Yuva DKIM below, or ask your provider.`,
    })
  } else {
    lines.push({
      key: "dkim",
      title: "DKIM",
      tone: "ok",
      text: t`DKIM passes for ${dkimDomain} and signs the unsubscribe headers.`,
    })
  }

  const dmarc = c.dmarc
  if (!dnsRead) {
    lines.push({ key: "dmarc", title: "DMARC", tone: "unknown", text: readingDns })
  } else if (!dmarc.record) {
    lines.push({
      key: "dmarc",
      title: "DMARC",
      tone: "warn",
      text: t`${from} has no DMARC record. Publish one at _dmarc.${from}, at least v=DMARC1; p=none.`,
    })
  } else if (read && dmarc.status === "fail") {
    lines.push({
      key: "dmarc",
      title: "DMARC",
      tone: "bad",
      text: t`DMARC fails: neither SPF nor DKIM passes for ${from}. Fix the lines above.`,
      record: dmarc.record,
    })
  } else if (dmarc.policy === "none" || !dmarc.policy) {
    lines.push({
      key: "dmarc",
      title: "DMARC",
      tone: "warn",
      text: t`The policy is none: receivers only report. Move to quarantine once all your mail passes.`,
      record: dmarc.record,
    })
  } else {
    lines.push({
      key: "dmarc",
      title: "DMARC",
      tone: "ok",
      text:
        dmarc.policy === "reject"
          ? t`The policy is reject: mail that fails is refused.`
          : t`The policy is quarantine: mail that fails goes to spam.`,
      record: dmarc.record,
    })
  }

  if (c.mx && !running) {
    const mxDomain = c.mx.domain
    const hosts = c.mx.hosts.join(", ")
    lines.push(
      c.mx.hosts.length
        ? { key: "mx", title: "MX", tone: "ok", text: t`Replies to ${mxDomain} are received by ${hosts}.` }
        : {
            key: "mx",
            title: "MX",
            tone: "bad",
            text: t`${mxDomain} has no MX record, so replies to broadcasts cannot arrive. Publish the MX records of your mail service.`,
          }
    )
  }

  const yd = c.yuva_dkim
  if (yd) {
    const name = yd.dns_name
    lines.push(
      yd.published
        ? { key: "yuva_dkim", title: t`Yuva DKIM`, tone: "ok", text: t`The record at ${name} is published.` }
        : {
            key: "yuva_dkim",
            title: t`Yuva DKIM`,
            tone: "warn",
            text: t`DNS does not answer with the key at ${name} yet. Publish the record below, then check again.`,
          }
    )
  }
  return lines
}

export function Lines({ check }: { check: DeliverabilityCheck }) {
  const lines = useLines(check)
  return (
    <ul className="flex flex-col divide-y" data-testid="deliverability-lines">
      {lines.map((l) => (
        <li
          key={l.key}
          className="flex gap-2.5 py-2.5"
          data-testid="deliverability-line"
          data-key={l.key}
          data-tone={l.tone}
        >
          {icons[l.tone]}
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="text-body font-medium">{l.title}</span>
            <span className="text-small text-muted-foreground">{l.text}</span>
            {l.record && (
              <code className="mt-0.5 block font-mono text-caption break-all text-faint" dir="ltr">
                {l.record}
              </code>
            )}
          </div>
        </li>
      ))}
    </ul>
  )
}

export function StateNotice({ check, address }: { check: DeliverabilityCheck; address: string }) {
  const reason = check.error ?? ""
  switch (check.state) {
    case "queued":
    case "sent":
      return (
        <Notice
          icon={LoaderIcon}
          className="text-small [&>svg]:animate-spin"
          role="status"
          data-testid="deliverability-checking"
        >
          <p>
            <Trans>
              A test mail is on its way to {address}. The result shows here when it arrives, usually within a minute and
              at most 10 minutes.
            </Trans>
          </p>
        </Notice>
      )
    case "failed":
      return (
        <Notice tone="danger" icon={CircleAlertIcon} role="alert" data-testid="deliverability-failed">
          <p>
            <Trans>
              The test mail could not be sent through the channel’s broadcast account. Check its SMTP settings.
            </Trans>
          </p>
          {reason && (
            <p className="font-mono text-caption break-all text-muted-foreground" dir="ltr">
              {reason}
            </p>
          )}
        </Notice>
      )
    case "timeout":
      return (
        <Notice tone="warning" icon={TriangleAlertIcon} data-testid="deliverability-timeout">
          <p>
            <Trans>
              The test mail did not come back within 10 minutes, so only DNS was checked. Make sure mail to {address}{" "}
              reaches Yuva (its forwarding or MX), then check again.
            </Trans>
          </p>
        </Notice>
      )
    case "unverifiable":
      return (
        <Notice tone="warning" icon={TriangleAlertIcon} data-testid="deliverability-unverifiable">
          <p>
            <Trans>
              The test mail arrived without results from a receiving server this Yuva trusts, so SPF, DKIM and DMARC
              could not be read and only DNS was checked. Ask the server’s operator to set the trusted receiving server.
            </Trans>
          </p>
        </Notice>
      )
    case "received":
      return null
  }
}

export function DeliverabilityCard({ channel }: { channel: Channel }) {
  const { i18n } = useLingui()
  const { canManage } = useSession()
  const check = useDeliverability(channel.id)
  const run = useRunCheck(channel.id)
  const c = check.data
  const address = channel.email?.address ?? ""
  const sends = !!channel.email?.smtp || !!channel.email?.broadcast_smtp
  const checking = isChecking(c)
  const limited = run.error instanceof RateLimited ? run.error : null
  const retry = limited ? formatShort(new Date(limited.retryAt).toISOString(), i18n.locale) : ""
  const when = c ? formatDateTime(c.checked_at ?? c.created_at, i18n.locale) : ""
  return (
    <PageSection
      id="deliverability"
      title={<Trans>Deliverability</Trans>}
      action={
        canManage && (
          <Button
            size="sm"
            variant="outline"
            className="bg-card"
            onClick={() => run.mutate()}
            disabled={run.isPending || checking || !sends}
            data-testid="deliverability-run"
          >
            {run.isPending || checking ? <LoaderIcon className="animate-spin" /> : <RefreshCwIcon />}
            {c ? <Trans>Check again</Trans> : <Trans>Run a check</Trans>}
          </Button>
        )
      }
      description={
        <Trans>
          A check sends a test mail through the broadcast account to the channel’s own address and reads how the
          receiving server judged it, then looks up the domain’s DNS. Up to six checks an hour.
        </Trans>
      }
    >
      <div id="deliverability" className="scroll-mt-4" />
      <Card data-testid="deliverability-card">
        {check.isPending ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : c === null ? (
          <p className="text-body text-muted-foreground" data-testid="deliverability-never">
            {sends ? (
              <Trans>Never checked. Run a check before the first broadcast; the review warns until you do.</Trans>
            ) : (
              <Trans>This channel has no SMTP account, so it cannot send broadcasts or a test mail.</Trans>
            )}
          </p>
        ) : c ? (
          <>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <span className="min-w-0 flex-1 text-body font-medium" dir="ltr">
                {c.domain}
              </span>
              <span className="text-small text-faint">
                {c.checked_at ? <Trans>Checked {when}</Trans> : <Trans>Started {when}</Trans>}
              </span>
            </div>
            <StateNotice check={c} address={address} />
            {c.state !== "failed" && <Lines check={c} />}
          </>
        ) : (
          <ErrorLine error={check.error} />
        )}
        {limited ? (
          <p role="alert" className="text-body text-destructive" data-testid="deliverability-limited">
            <Trans>That was the sixth check this hour. You can check again at {retry}.</Trans>
          </p>
        ) : (
          <ErrorLine error={run.error} />
        )}
      </Card>
    </PageSection>
  )
}

const SELECTOR = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

export function DkimSection({ channel }: { channel: Channel }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const dkim = channel.email?.dkim
  const [selector, setSelector] = useState("yuva")
  const create = useCreateDkim(channel)
  const toggle = useSetDkim(channel)
  const remove = useDeleteDkim(channel)
  const [confirm, confirmDialog] = useConfirm()
  const validSelector = SELECTOR.test(selector)
  const missing = toggle.error instanceof ApiError && toggle.error.code === "dkim_dns_missing"
  const relay =
    !!dkim?.unsupported_reason || (toggle.error instanceof ApiError && toggle.error.code === "dkim_relay_unsupported")
  return (
    <PageSection
      id="dkim-key"
      title={<Trans>Yuva DKIM</Trans>}
      description={
        <Trans>
          Yuva signs this channel’s broadcasts, tests, confirmation and welcome mails with its own key, over the
          unsubscribe headers. Your provider’s signature stays next to it; support replies are not changed.
        </Trans>
      }
    >
      <div id="dkim" className="scroll-mt-4" />
      <Card data-testid="dkim-card">
        {!dkim ? (
          canManage ? (
            <form
              className="flex flex-col gap-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (validSelector) create.mutate(selector)
              }}
            >
              <p className="text-body text-muted-foreground">
                <Trans>
                  Create a key when your SMTP provider cannot sign with your domain or leaves the unsubscribe headers
                  unsigned. Only the DNS record is shown; the private key never leaves the server.
                </Trans>
              </p>
              <div className="flex flex-wrap items-end gap-3">
                <Field
                  label={<Trans>Selector</Trans>}
                  htmlFor="dkim-selector"
                  className="min-w-40 flex-1 sm:max-w-60"
                  error={
                    validSelector ? undefined : (
                      <Trans>Lowercase letters, digits and dashes, not at the start or end.</Trans>
                    )
                  }
                >
                  <Input
                    id="dkim-selector"
                    value={selector}
                    maxLength={63}
                    onChange={(e) => setSelector(e.target.value)}
                    aria-invalid={!validSelector || undefined}
                    dir="ltr"
                    className="bg-background"
                    data-testid="dkim-selector"
                  />
                </Field>
                <Button type="submit" disabled={create.isPending || !validSelector} data-testid="dkim-create">
                  {create.isPending && <LoaderIcon className="animate-spin" />}
                  <Trans>Create key</Trans>
                </Button>
              </div>
              <ErrorLine error={create.error} />
            </form>
          ) : (
            <p className="text-body text-muted-foreground">
              <Trans>This channel has no Yuva DKIM key.</Trans>
            </p>
          )
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <span className="min-w-0 flex-1 text-body font-medium">
                <Trans>Selector</Trans> <code className="font-mono">{dkim.selector}</code>
              </span>
              {relay ? (
                <StatusTag tone="warning" data-testid="dkim-state">
                  <Trans>Not signing</Trans>
                </StatusTag>
              ) : dkim.enabled ? (
                <StatusTag tone="success" data-testid="dkim-state">
                  <Trans>Signing</Trans>
                </StatusTag>
              ) : (
                <StatusTag tone="warning" data-testid="dkim-state">
                  <Trans>Not signing yet</Trans>
                </StatusTag>
              )}
            </div>
            <div className="flex flex-col gap-3">
              <p className="text-body">
                <Trans>Publish this TXT record in your domain’s DNS:</Trans>
              </p>
              <Field label={<Trans>Name</Trans>}>
                <CodeLine value={dkim.dns_name} testId="dkim-name" />
              </Field>
              <Field
                label={<Trans>Value</Trans>}
                hint={
                  <Trans>
                    If your DNS provider limits a TXT string to 255 characters, split the value into several strings of
                    one record.
                  </Trans>
                }
              >
                <CodeBlock value={dkim.dns_value} testId="dkim-value" />
              </Field>
            </div>
            {missing && (
              <Notice tone="warning" icon={TriangleAlertIcon} role="alert" data-testid="dkim-dns-missing">
                <p>
                  <Trans>
                    DNS does not answer with this key at the name above yet. Publish the record, wait a few minutes
                    (some providers take longer) and try again.
                  </Trans>
                </p>
              </Notice>
            )}
            {relay && (
              <Notice tone="warning" icon={TriangleAlertIcon} role="status" data-testid="dkim-relay-unsupported">
                <p>
                  <Trans>
                    This channel’s broadcasts go through Amazon SES, which signs your mail with your domain through Easy
                    DKIM. Yuva DKIM is for other relays; set up Easy DKIM for your domain in SES instead.
                  </Trans>
                </p>
              </Notice>
            )}
            {!missing && !relay && <ErrorLine error={toggle.error ?? remove.error} />}
            {relay && <ErrorLine error={remove.error} />}
            {canManage && (
              <div className="flex flex-wrap items-center gap-2 border-t pt-4">
                {relay ? null : dkim.enabled ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => toggle.mutate(false)}
                    disabled={toggle.isPending}
                    data-testid="dkim-disable"
                  >
                    <Trans>Stop signing</Trans>
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    onClick={() => toggle.mutate(true)}
                    disabled={toggle.isPending}
                    data-testid="dkim-enable"
                  >
                    {toggle.isPending && <LoaderIcon className="animate-spin" />}
                    <Trans>Check DNS and start signing</Trans>
                  </Button>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  className="ms-auto text-destructive hover:text-destructive"
                  disabled={remove.isPending}
                  onClick={() =>
                    confirm({
                      title: <Trans>Delete this DKIM key?</Trans>,
                      description: (
                        <Trans>
                          Yuva stops signing with it at once. Remove the DNS record afterwards; a new key needs a new
                          record.
                        </Trans>
                      ),
                      confirm: <Trans>Delete key</Trans>,
                      run: () => remove.mutate(),
                    })
                  }
                  data-testid="dkim-delete"
                >
                  <Trans>Delete key</Trans>
                </Button>
              </div>
            )}
            {!dkim.enabled && !relay && !canManage && (
              <p className="text-small text-faint">{t`An owner or admin turns signing on once the record is published.`}</p>
            )}
          </>
        )}
      </Card>
      {confirmDialog}
    </PageSection>
  )
}
