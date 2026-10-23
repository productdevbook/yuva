import { Trans, useLingui } from "@lingui/react/macro"
import { AlertTriangleIcon, CheckCircle2Icon, ClockIcon } from "lucide-react"
import { useState } from "react"

import { CodeLine, ErrorLine, Notice, useConfirm } from "@/components/common"
import { formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, Field, Section, StatusTag } from "@/features/settings/ui"
import { ApiError, type StatusCustomDomain } from "@/lib/api"

const HOST_RE = /^(?=.{4,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/

function normalize(v: string) {
  return v
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/\/.*$/, "")
    .replace(/\.$/, "")
}

function isOff(err: unknown) {
  return err instanceof ApiError && err.status === 400 && !!err.detail?.includes("YUVA_STATUS_DOMAINS")
}

type SaveDomain = {
  mutate(body: { custom_domain: string | null }, options?: { onSuccess?: () => void }): void
  isPending: boolean
  error: unknown
}

type Props = {
  domain: StatusCustomDomain | undefined
  enabled: boolean
  example: string
  tid: string
  save: SaveDomain
}

function useDomainError(example: string) {
  const { t } = useLingui()
  return (err: unknown) => {
    if (!(err instanceof ApiError)) return undefined
    if (isOff(err)) return t`This server does not offer own domains. Ask its operator to turn them on.`
    if (err.status === 400)
      return t`This is not a domain this page can use. Use a name like ${example} that is not one of this server’s own addresses.`
    return undefined
  }
}

function Record({
  tid,
  title,
  nameLabel,
  name,
  valueLabel,
  value,
  id,
  tag,
}: {
  tid: string
  tag?: React.ReactNode
  title: React.ReactNode
  nameLabel: React.ReactNode
  name: string
  valueLabel: React.ReactNode
  value: string
  id: string
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2.5 rounded-xl border p-3.5" data-testid={`${tid}-${id}`}>
      <p className="flex items-center justify-between gap-2 text-body font-medium">
        {title}
        {tag}
      </p>
      <div className="flex min-w-0 flex-col gap-1.5">
        <p className="text-small text-faint">{nameLabel}</p>
        <CodeLine value={name} />
      </div>
      <div className="flex min-w-0 flex-col gap-1.5">
        <p className="text-small text-faint">{valueLabel}</p>
        <CodeLine value={value} />
      </div>
    </div>
  )
}

function Explanation({ d, tid }: { d: StatusCustomDomain; tid: string }) {
  const { i18n } = useLingui()
  const missing = d.missing_since ? formatRelative(d.missing_since, i18n.locale) : null
  if (d.state === "pending")
    return (
      <Notice tone="warning" icon={ClockIcon} data-testid={`${tid}-explain`}>
        <p>
          <Trans>
            Waiting for the two DNS records below. Add them where you manage the domain; DNS changes can take a while to
            spread. Yuva looks every 10 minutes, and the claim ends if they are not found within 7 days.
          </Trans>
        </p>
        {d.problem && (
          <p className="text-small text-muted-foreground [overflow-wrap:anywhere]" data-testid={`${tid}-problem`}>
            <Trans>The last check saw: {d.problem}</Trans>
          </p>
        )}
      </Notice>
    )
  if (d.state === "verified")
    return (
      <Notice tone="info" icon={CheckCircle2Icon} data-testid={`${tid}-explain`}>
        <p>
          <Trans>Both records were found. Yuva checks them once a day; keep them in place.</Trans>
        </p>
        {missing && (
          <p className="text-warning" data-testid={`${tid}-missing`}>
            <Trans>
              The records have not been visible since {missing}. The domain is turned off after 3 days unless they come
              back.
            </Trans>
          </p>
        )}
      </Notice>
    )
  if (d.state === "off")
    return (
      <Notice icon={AlertTriangleIcon} data-testid={`${tid}-explain`}>
        <p>
          <Trans>
            The records were gone for 3 days, so the domain no longer shows this page and is not checked any more. Put
            the records back and start over.
          </Trans>
        </p>
      </Notice>
    )
  if (d.problem === "taken")
    return (
      <Notice tone="danger" icon={AlertTriangleIcon} data-testid={`${tid}-explain`}>
        <p>
          <Trans>
            Another page verified this domain first, and only a verified claim holds a name. Remove the domain, or start
            over once the other page lets it go.
          </Trans>
        </p>
      </Notice>
    )
  if (d.problem === "expired")
    return (
      <Notice tone="danger" icon={AlertTriangleIcon} data-testid={`${tid}-explain`}>
        <p>
          <Trans>
            The records were not found within 7 days, so the claim ended. Add the records below, then start over for 7
            more days.
          </Trans>
        </p>
      </Notice>
    )
  return (
    <Notice tone="danger" icon={AlertTriangleIcon} data-testid={`${tid}-explain`}>
      <p>
        <Trans>
          The records exist, but they do not point where Yuva needs them. Change them to match the ones below. Yuva
          looks again every 10 minutes.
        </Trans>
      </p>
      {d.problem && d.problem !== "taken" && d.problem !== "expired" && (
        <p className="text-small text-muted-foreground [overflow-wrap:anywhere]" data-testid={`${tid}-problem`}>
          <Trans>The last check saw: {d.problem}</Trans>
        </p>
      )}
    </Notice>
  )
}

function Progress({ d, tid }: { d: StatusCustomDomain; tid: string }) {
  const { t } = useLingui()
  const cf = d.cloudflare
  const hostnames: Record<string, string> = { active: t`Active`, pending: t`Pending`, blocked: t`Blocked` }
  const certs: Record<string, string> = {
    active: t`Active`,
    initializing: t`Waiting for validation`,
    pending_validation: t`Waiting for validation`,
  }
  const tone = (v?: string) => (v === "active" ? "success" : "warning")
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-xl border p-3.5" data-testid={`${tid}-cloudflare`}>
      <p className="text-body font-medium">
        <Trans>Hostname and certificate</Trans>
      </p>
      {!cf?.status ? (
        <p className="text-small text-faint">
          <Trans>The hostname is being set up. This takes a moment once the records are found.</Trans>
        </p>
      ) : (
        <div className="flex flex-wrap gap-x-6 gap-y-1.5">
          <span className="flex items-center gap-2 text-body">
            <Trans>Hostname</Trans>
            <StatusTag tone={tone(cf.status)} data-testid={`${tid}-hostname`}>
              {hostnames[cf.status] ?? cf.status}
            </StatusTag>
          </span>
          <span className="flex items-center gap-2 text-body">
            <Trans>Certificate</Trans>
            <StatusTag tone={tone(cf.ssl_status)} data-testid={`${tid}-certificate`}>
              {cf.ssl_status ? (certs[cf.ssl_status] ?? cf.ssl_status) : t`Pending`}
            </StatusTag>
          </span>
        </div>
      )}
      {cf?.errors?.map((e) => (
        <p key={e} className="text-small text-destructive [overflow-wrap:anywhere]">
          {e}
        </p>
      ))}
    </div>
  )
}

function Added({ d, tid, save: patch }: { d: StatusCustomDomain; tid: string; save: SaveDomain }) {
  const { t, i18n } = useLingui()
  const [confirm, dialog] = useConfirm()
  const ended = d.state === "off" || (d.state === "failed" && (d.problem === "taken" || d.problem === "expired"))
  const cf = d.cloudflare
  const ready =
    d.state === "verified" && (d.certificates === "ask" || (cf?.status === "active" && cf?.ssl_status === "active"))
  const last = d.checked_at ? formatRelative(d.checked_at, i18n.locale) : null
  const next = !ended && d.next_check_at ? formatRelative(d.next_check_at, i18n.locale) : null
  const found =
    d.state === "verified" ? (
      <StatusTag tone="success">{t`Found`}</StatusTag>
    ) : d.state === "failed" ? (
      <StatusTag tone="danger">{t`Not found`}</StatusTag>
    ) : (
      <StatusTag tone="warning">{t`Waiting`}</StatusTag>
    )
  const tags = {
    pending: <StatusTag tone="warning">{t`Waiting for DNS`}</StatusTag>,
    verified: <StatusTag tone="success">{t`Verified`}</StatusTag>,
    failed: <StatusTag tone="danger">{t`Needs attention`}</StatusTag>,
    off: <StatusTag>{t`Turned off`}</StatusTag>,
  }
  return (
    <>
      <Card
        data-testid={`${tid}`}
        footer={
          <>
            <ErrorLine error={patch.error} className="min-w-0 flex-1" />
            <div className="ms-auto flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={patch.isPending}
                onClick={() => patch.mutate({ custom_domain: d.domain })}
                data-testid={`${tid}-check`}
              >
                {ended ? <Trans>Start over</Trans> : <Trans>Check now</Trans>}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={patch.isPending}
                onClick={() =>
                  confirm({
                    title: <Trans>Remove {d.domain}?</Trans>,
                    description: (
                      <Trans>The page stops answering on this domain. Its address on this server keeps working.</Trans>
                    ),
                    confirm: <Trans>Remove domain</Trans>,
                    run: () => patch.mutate({ custom_domain: null }),
                  })
                }
                data-testid={`${tid}-remove`}
              >
                <Trans>Remove</Trans>
              </Button>
            </div>
          </>
        }
      >
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <p className="min-w-0 break-all text-body font-medium" data-testid={`${tid}-name`}>
            {d.domain}
          </p>
          <span data-testid={`${tid}-state`} data-state={d.state}>
            {tags[d.state]}
          </span>
        </div>
        <Explanation d={d} tid={tid} />
        {ready && (
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-small text-faint">
              <Trans>The page answers at</Trans>
            </p>
            <a
              href={`https://${d.domain}`}
              target="_blank"
              rel="noreferrer noopener"
              className="min-w-0 break-all text-body text-brand underline-offset-4 hover:underline"
              data-testid={`${tid}-live`}
            >
              {`https://${d.domain}`}
            </a>
          </div>
        )}
        {d.state === "verified" && !ready && (
          <Notice icon={ClockIcon} data-testid={`${tid}-waiting-cert`}>
            <p>
              <Trans>
                Links and feeds keep using the address on this server until the hostname and its certificate are both
                active.
              </Trans>
            </p>
          </Notice>
        )}
        {d.state === "verified" && d.certificates === "cloudflare" && <Progress d={d} tid={tid} />}
        {d.state !== "off" && !(d.state === "failed" && d.problem === "taken") && (
          <>
            <Record
              tid={tid}
              id="cname"
              tag={found}
              title={<Trans>CNAME record</Trans>}
              nameLabel={<Trans>Name</Trans>}
              name={d.domain}
              valueLabel={<Trans>Points to</Trans>}
              value={d.cname_target}
            />
            <Record
              tid={tid}
              id="txt"
              tag={found}
              title={<Trans>TXT record</Trans>}
              nameLabel={<Trans>Name</Trans>}
              name={d.txt_name}
              valueLabel={<Trans>Value</Trans>}
              value={d.txt_value}
            />
          </>
        )}
        {(last || next) && (
          <p className="text-small text-faint" data-testid={`${tid}-times`}>
            {last && <Trans>Last checked {last}.</Trans>} {next && <Trans>Next check {next}.</Trans>}
          </p>
        )}
      </Card>
      {dialog}
    </>
  )
}

function Add({ example, tid, save: patch }: { example: string; tid: string; save: SaveDomain }) {
  const errorText = useDomainError(example)
  const [value, setValue] = useState("")
  const domain = normalize(value)
  const ok = HOST_RE.test(domain)
  const off = isOff(patch.error)
  return (
    <Card data-testid={`${tid}-add`}>
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (ok) patch.mutate({ custom_domain: domain }, { onSuccess: () => setValue("") })
        }}
      >
        <Field
          label={<Trans>Domain</Trans>}
          htmlFor={`${tid}-input`}
          hint={
            <Trans>
              A name you control, like {example}. Yuva then shows you two DNS records to add, and the page answers on
              the domain once it has seen them.
            </Trans>
          }
          error={value && !ok ? <Trans>Enter a domain like {example}.</Trans> : errorText(patch.error)}
        >
          <div className="flex flex-wrap gap-2">
            <Input
              id={`${tid}-input`}
              className="min-w-0 flex-1 basis-56"
              value={value}
              placeholder={example}
              spellCheck={false}
              autoCapitalize="none"
              onChange={(e) => setValue(e.target.value)}
              disabled={off}
              data-testid={`${tid}-input`}
            />
            <Button type="submit" size="sm" disabled={!ok || patch.isPending || off} data-testid={`${tid}-save`}>
              <Trans>Add domain</Trans>
            </Button>
          </div>
        </Field>
        {!errorText(patch.error) && <ErrorLine error={patch.error} />}
      </form>
    </Card>
  )
}

export function OwnDomainCard({ domain, enabled, example, tid, save }: Props) {
  if (!enabled) return null
  return (
    <Section
      title={<Trans>Own domain</Trans>}
      description={<Trans>Show this page on your own address instead of this server’s.</Trans>}
    >
      {domain ? <Added d={domain} tid={tid} save={save} /> : <Add example={example} tid={tid} save={save} />}
    </Section>
  )
}
