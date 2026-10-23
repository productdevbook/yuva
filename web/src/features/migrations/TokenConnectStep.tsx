import { Plural, Trans } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import {
  ArrowRightIcon,
  ArrowUpRightIcon,
  CheckIcon,
  KeyRoundIcon,
  LoaderIcon,
  ShieldCheckIcon,
  XIcon,
} from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ErrorLine, Notice, StepHeader } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { FollowsNotice } from "@/features/migrations/parts"
import {
  useMigrationFolders,
  useMigrationStep,
  useSetCredentials,
  useTestConnection,
} from "@/features/migrations/queries"
import { guideUrl, useGuides, useTestCodeText } from "@/features/migrations/text"
import { WizardLayout } from "@/features/migrations/Wizard"
import { Field } from "@/features/settings/ui"
import type { Migration, MigrationCredentialsInput, MigrationPreset, MigrationTestResult } from "@/lib/api"

const SUBDOMAIN = /^[a-z0-9][a-z0-9-]{0,62}$/

const REGIONS = [
  { value: "us", label: "US · api.intercom.io" },
  { value: "eu", label: "EU · api.eu.intercom.io" },
  { value: "au", label: "AU · api.au.intercom.io" },
] as const

export function TokenConnectStep({ m, preset }: { m: Migration; preset: MigrationPreset }) {
  const guide = useGuides()[m.preset]
  const codeText = useTestCodeText()
  const provider = preset.name
  const fields = preset.token_fields ?? ["token"]
  const [token, setToken] = useState("")
  const [secret, setSecret] = useState("")
  const [subdomain, setSubdomain] = useState("")
  const [baseUrl, setBaseUrl] = useState("")
  const [account, setAccount] = useState("")
  const [region, setRegion] = useState<(typeof REGIONS)[number]["value"]>("us")
  const [result, setResult] = useState<MigrationTestResult | null>(null)
  const credentials = useSetCredentials(m.id)
  const test = useTestConnection(m.id)
  const resume = useMigrationStep(m.id, "resume")
  const reconnecting = m.state === "needs_attention"
  const canSet = m.actions.includes("set_credentials")
  const connected = !!m.credentials && (m.state === "connected" || m.state === "scanning" || m.state === "ready")
  const folders = useMigrationFolders(m.id, connected)
  const count = folders.data?.length ?? 0
  const host = m.credentials?.server.host ?? provider
  const sub = subdomain.trim().toLowerCase()
  const base = baseUrl.trim()
  const accountId = account.trim()
  const valid =
    token.trim() !== "" &&
    (!fields.includes("secret") || secret.trim() !== "") &&
    (!fields.includes("subdomain") || SUBDOMAIN.test(sub)) &&
    (!fields.includes("base_url") || base.startsWith("https://")) &&
    (!fields.includes("account") || /^[0-9]{1,18}$/.test(accountId))

  const run = useMutation({
    mutationFn: async () => {
      setResult(null)
      const body: MigrationCredentialsInput = {
        kind: "token",
        token: token.trim(),
        ...(fields.includes("secret") ? { secret: secret.trim() } : {}),
        ...(fields.includes("subdomain") ? { subdomain: sub } : {}),
        ...(fields.includes("base_url") ? { base_url: base } : {}),
        ...(fields.includes("account") ? { account: accountId } : {}),
        ...(fields.includes("region") ? { region } : {}),
      }
      await credentials.mutateAsync(body)
      const r = await test.mutateAsync()
      setResult(r)
      if (r.ok && reconnecting && r.migration.actions.includes("resume")) await resume.mutateAsync()
      return r
    },
  })
  const failed = result?.steps.find((s) => !s.ok)

  return (
    <WizardLayout step="connect" m={m} preset={preset}>
      <StepHeader title={reconnecting ? <Trans>Reconnect {provider}</Trans> : <Trans>Connect {provider}</Trans>}>
        {reconnecting ? (
          <Trans>
            {provider} stopped accepting the token. Enter a new token for the same account; the migration continues
            where it stopped.
          </Trans>
        ) : (
          <Trans>Yuva reads {provider} with an API token you create in your own account, and only reads.</Trans>
        )}
      </StepHeader>

      {!reconnecting && <FollowsNotice m={m} className="mb-6" />}

      {reconnecting && m.state_reason && !result?.ok && (
        <Notice tone="danger" icon={XIcon} className="mb-6">
          <p>{m.state_reason}</p>
        </Notice>
      )}

      {(connected || result?.ok) && (
        <Notice icon={CheckIcon} className="mb-6" role="status" data-testid="migration-connected">
          <p className="font-medium text-foreground">
            <Trans>Connected to {provider}</Trans>
            {folders.data && (
              <>
                {" · "}
                <Plural value={count} one="# folder" other="# folders" />
              </>
            )}
          </p>
          <p>
            <Trans>If an assistant sent you here, you can go back to it now, or choose what to bring yourself.</Trans>
          </p>
          <Button
            size="sm"
            className="self-start"
            render={<Link to={reconnecting ? "/settings/migrations" : `/migrations/${m.id}/choose`} />}
            data-testid="migration-connect-continue"
          >
            <Trans>Continue</Trans>
            <ArrowRightIcon className="rtl:rotate-180" />
          </Button>
        </Notice>
      )}

      <section className="mb-6 rounded-2xl border bg-card p-4" aria-labelledby="migration-guide">
        <h2 id="migration-guide" className="flex items-center gap-2 text-body font-medium">
          <KeyRoundIcon className="size-4 text-faint" />
          <Trans>Create an API token in {provider}</Trans>
        </h2>
        {guide && (
          <ol className="mt-3 flex list-decimal flex-col gap-2 ps-5 text-body text-muted-foreground marker:text-faint">
            {guide.steps.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ol>
        )}
        <div className="mt-4 flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            render={<a href={guideUrl(preset.guide)} target="_blank" rel="noreferrer" />}
            data-testid="migration-guide-link"
          >
            <Trans>Full guide</Trans>
            <ArrowUpRightIcon className="rtl:-scale-x-100" />
          </Button>
        </div>
      </section>

      {canSet && (
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (valid) run.mutate()
          }}
        >
          {connected && (
            <p className="text-small text-faint">
              <Trans>Enter a token again only to change it.</Trans>
            </p>
          )}
          {fields.includes("base_url") && (
            <Field
              label={<Trans>Address of your {provider}</Trans>}
              htmlFor="migration-base-url"
              hint={<Trans>The HTTPS address you open it at, such as https://help.example.com.</Trans>}
            >
              <Input
                id="migration-base-url"
                type="url"
                autoComplete="off"
                spellCheck={false}
                value={baseUrl}
                maxLength={2000}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder="https://help.example.com"
                aria-invalid={base !== "" && !base.startsWith("https://")}
                className="h-10 rounded-xl"
                data-testid="migration-base-url"
              />
            </Field>
          )}
          {fields.includes("account") && (
            <Field label={<Trans>Account number</Trans>} htmlFor="migration-account">
              <Input
                id="migration-account"
                inputMode="numeric"
                autoComplete="off"
                spellCheck={false}
                value={account}
                maxLength={18}
                onChange={(e) => setAccount(e.target.value)}
                placeholder="1"
                aria-invalid={account !== "" && !/^[0-9]{1,18}$/.test(accountId)}
                className="h-10 rounded-xl"
                data-testid="migration-account"
              />
            </Field>
          )}
          {fields.includes("subdomain") && (
            <Field label={<Trans>Subdomain</Trans>} htmlFor="migration-subdomain">
              <Input
                id="migration-subdomain"
                autoComplete="off"
                spellCheck={false}
                value={subdomain}
                maxLength={63}
                onChange={(e) => setSubdomain(e.target.value)}
                placeholder="acme"
                aria-invalid={subdomain !== "" && !SUBDOMAIN.test(sub)}
                className="h-10 rounded-xl"
                data-testid="migration-subdomain"
              />
            </Field>
          )}
          {fields.includes("region") && (
            <Field label={<Trans>Region</Trans>} htmlFor="migration-region">
              <NativeSelect
                id="migration-region"
                value={region}
                onChange={(e) => setRegion(e.target.value as typeof region)}
                data-testid="migration-region"
              >
                {REGIONS.map((r) => (
                  <NativeSelectOption key={r.value} value={r.value}>
                    {r.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </Field>
          )}
          <Field
            label={<Trans>API token</Trans>}
            htmlFor="migration-token"
            hint={<Trans>Stored encrypted and never shown again, not even to you.</Trans>}
          >
            <Input
              id="migration-token"
              type="password"
              autoComplete="new-password"
              value={token}
              maxLength={4000}
              onChange={(e) => setToken(e.target.value)}
              className="h-10 rounded-xl font-mono"
              data-testid="migration-token"
            />
          </Field>
          {fields.includes("secret") && (
            <Field label={<Trans>Second secret</Trans>} htmlFor="migration-secret">
              <Input
                id="migration-secret"
                type="password"
                autoComplete="new-password"
                value={secret}
                maxLength={4000}
                onChange={(e) => setSecret(e.target.value)}
                className="h-10 rounded-xl font-mono"
                data-testid="migration-secret"
              />
            </Field>
          )}

          <Notice icon={ShieldCheckIcon} data-testid="migration-read-only">
            <p>
              <Trans>
                Yuva only reads. Its connection sends {provider} nothing but read requests, and the token only to{" "}
                {provider}'s API, so no ticket, customer or setting changes.
              </Trans>
            </p>
            <p className="text-small">
              <Trans>Yuva deletes its token when the migration ends.</Trans>
            </p>
          </Notice>

          {failed?.code && (
            <Notice tone="danger" icon={XIcon} role="alert" data-testid="migration-test-error" data-code={failed.code}>
              <p>{codeText(failed.code, provider, host)}</p>
              {failed.detail && (
                <details className="text-small">
                  <summary className="cursor-pointer text-faint">
                    <Trans>Details</Trans>
                  </summary>
                  <code className="mt-1.5 block font-mono text-caption break-all">{failed.detail}</code>
                </details>
              )}
            </Notice>
          )}
          <ErrorLine error={run.error} />
          {!result?.ok && (
            <Button
              type="submit"
              size="lg"
              className="self-start"
              disabled={!valid || run.isPending}
              data-testid="migration-test"
            >
              {run.isPending && <LoaderIcon className="animate-spin" />}
              {result ? <Trans>Test again</Trans> : <Trans>Test connection</Trans>}
            </Button>
          )}
        </form>
      )}
    </WizardLayout>
  )
}
