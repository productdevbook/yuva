import { Trans, useLingui } from "@lingui/react/macro"
import { GlobeIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { Navigate } from "react-router"

import { CodeLine, ErrorLine, Notice, toast, useConfirm } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { SamlForm } from "@/features/settings/sso/SamlForm"
import { useSso, useSsoMutations } from "@/features/settings/sso/queries"
import {
  Card,
  ChoiceSelect,
  Field,
  FormActions,
  FormCard,
  PageHeader,
  Pill,
  Section,
  ToggleRow,
} from "@/features/settings/ui"
import type { SsoDomain, SsoJitRole, SsoProtocol, SsoSettings } from "@/lib/api"
import { useSession } from "@/lib/session"

const words = (v: string) => v.split(/[\s,]+/).filter(Boolean)

function ProviderForm({ sso }: { sso: SsoSettings }) {
  const { t } = useLingui()
  const { saveProvider, removeProvider } = useSsoMutations()
  const [confirm, confirmDialog] = useConfirm()
  const o = sso.oidc
  const [issuer, setIssuer] = useState(o?.issuer ?? "")
  const [clientId, setClientId] = useState(o?.client_id ?? "")
  const [secret, setSecret] = useState("")
  const [replacing, setReplacing] = useState(!o)
  const [scopes, setScopes] = useState(o?.scopes.join(" ") ?? "")
  const [groupsClaim, setGroupsClaim] = useState(o?.groups_claim ?? "groups")
  const [groups, setGroups] = useState(o?.allowed_groups.join(", ") ?? "")
  const secretNeeded = replacing && !secret.trim()
  const dirty =
    !o ||
    issuer.trim() !== o.issuer ||
    clientId.trim() !== o.client_id ||
    secret.trim() !== "" ||
    words(scopes).join(" ") !== o.scopes.join(" ") ||
    groupsClaim.trim() !== o.groups_claim ||
    words(groups).join(" ") !== o.allowed_groups.join(" ")
  const submit = () =>
    saveProvider.mutate(
      {
        issuer: issuer.trim(),
        client_id: clientId.trim(),
        client_secret: secret.trim() || undefined,
        scopes: words(scopes),
        groups_claim: groupsClaim.trim() || undefined,
        allowed_groups: words(groups.replace(/,/g, " ")),
      },
      {
        onSuccess: () => {
          setSecret("")
          setReplacing(false)
          toast(t`Provider saved. Its discovery document was read.`)
        },
      }
    )
  return (
    <>
      <FormCard
        onSubmit={() => !secretNeeded && submit()}
        footer={
          <>
            {o && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive"
                disabled={removeProvider.isPending}
                data-testid="sso-remove-provider"
                onClick={() =>
                  confirm({
                    title: <Trans>Remove the provider?</Trans>,
                    description: (
                      <Trans>
                        People sign in with a code or a passkey again, and single sign-on stops being required. Your
                        domains stay.
                      </Trans>
                    ),
                    confirm: <Trans>Remove</Trans>,
                    run: () => removeProvider.mutate(undefined, { onSuccess: () => toast(t`Provider removed`) }),
                  })
                }
              >
                <Trans>Remove</Trans>
              </Button>
            )}
            <FormActions
              pending={saveProvider.isPending}
              saved={saveProvider.isSuccess && !dirty}
              error={saveProvider.error ?? removeProvider.error}
              disabled={!dirty || secretNeeded || !issuer.trim() || !clientId.trim()}
              label={o ? <Trans>Save and test</Trans> : <Trans>Connect and test</Trans>}
            />
          </>
        }
      >
        <Field
          label={<Trans>Redirect URI</Trans>}
          hint={<Trans>Register this address at your provider as the sign-in redirect.</Trans>}
        >
          <CodeLine value={sso.redirect_uri} testId="sso-redirect-uri" />
        </Field>
        <Field
          label={<Trans>Issuer</Trans>}
          htmlFor="sso-issuer"
          hint={
            <Trans>The provider's address, exactly as its discovery document names it. Yuva reads it on save.</Trans>
          }
        >
          <Input
            id="sso-issuer"
            type="url"
            required
            value={issuer}
            onChange={(e) => setIssuer(e.target.value)}
            placeholder="https://accounts.example.com"
            autoCapitalize="off"
            spellCheck={false}
            data-testid="sso-issuer"
          />
        </Field>
        <Field label={<Trans>Client ID</Trans>} htmlFor="sso-client-id">
          <Input
            id="sso-client-id"
            required
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
            autoCapitalize="off"
            autoComplete="off"
            spellCheck={false}
            data-testid="sso-client-id"
          />
        </Field>
        <Field
          label={<Trans>Client secret</Trans>}
          htmlFor="sso-secret"
          hint={<Trans>Stored encrypted and never shown again.</Trans>}
        >
          {replacing ? (
            <div className="flex gap-2">
              <Input
                id="sso-secret"
                type="password"
                autoComplete="off"
                spellCheck={false}
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                data-testid="sso-secret"
              />
              {o && (
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => {
                    setSecret("")
                    setReplacing(false)
                  }}
                >
                  <Trans>Cancel</Trans>
                </Button>
              )}
            </div>
          ) : (
            <div className="flex items-center gap-3">
              <Pill tone="success" data-testid="sso-secret-set">
                <Trans>Secret set</Trans>
              </Pill>
              <Button type="button" variant="outline" size="sm" onClick={() => setReplacing(true)}>
                <Trans>Replace</Trans>
              </Button>
            </div>
          )}
        </Field>
        <Field
          label={<Trans>Extra scopes</Trans>}
          htmlFor="sso-scopes"
          hint={<Trans>Besides openid, email and profile. Okta needs groups for group checks.</Trans>}
        >
          <Input
            id="sso-scopes"
            value={scopes}
            onChange={(e) => setScopes(e.target.value)}
            placeholder="groups"
            autoCapitalize="off"
            spellCheck={false}
          />
        </Field>
        <Field
          label={<Trans>Groups claim</Trans>}
          htmlFor="sso-groups-claim"
          hint={<Trans>The ID token claim that lists a person's groups.</Trans>}
        >
          <Input
            id="sso-groups-claim"
            value={groupsClaim}
            onChange={(e) => setGroupsClaim(e.target.value)}
            placeholder="groups"
            autoCapitalize="off"
            spellCheck={false}
          />
        </Field>
        <Field
          label={<Trans>Allowed groups</Trans>}
          htmlFor="sso-groups"
          hint={<Trans>Separate with commas. Leave empty to let everyone in your domains sign in.</Trans>}
        >
          <Input
            id="sso-groups"
            value={groups}
            onChange={(e) => setGroups(e.target.value)}
            autoCapitalize="off"
            spellCheck={false}
          />
        </Field>
      </FormCard>
      {confirmDialog}
    </>
  )
}

function DomainCard({ d }: { d: SsoDomain }) {
  const { t } = useLingui()
  const { verifyDomain, removeDomain } = useSsoMutations()
  const [confirm, confirmDialog] = useConfirm()
  const verified = d.state === "verified"
  const name = d.domain
  return (
    <Card data-testid="sso-domain" data-state={d.state}>
      <div className="flex flex-wrap items-center gap-3">
        <GlobeIcon className="size-4 shrink-0 text-faint" />
        <span className="min-w-0 flex-1 basis-40 truncate font-medium">{d.domain}</span>
        {verified ? (
          <Pill tone="success">
            <Trans>Verified</Trans>
          </Pill>
        ) : (
          <Pill tone="warning">
            <Trans>Waiting for the record</Trans>
          </Pill>
        )}
        {!verified && (
          <Button
            size="sm"
            disabled={verifyDomain.isPending}
            data-testid="sso-verify"
            onClick={() => verifyDomain.mutate(d)}
          >
            <Trans>Verify</Trans>
          </Button>
        )}
        <Button
          variant="outline"
          size="sm"
          className="text-destructive"
          disabled={removeDomain.isPending}
          data-testid="sso-domain-remove"
          onClick={() =>
            confirm({
              title: <Trans>Remove {name}?</Trans>,
              description: (
                <Trans>
                  People with this domain stop signing in through your provider. Removing the last verified domain turns
                  off required single sign-on.
                </Trans>
              ),
              confirm: <Trans>Remove</Trans>,
              run: () => removeDomain.mutate(d, { onSuccess: () => toast(t`Removed ${name}`) }),
            })
          }
        >
          <Trans>Remove</Trans>
        </Button>
      </div>
      {!verified && (
        <div className="flex flex-col gap-3 rounded-xl border bg-surface p-3.5">
          <p className="text-small text-muted-foreground">
            <Trans>
              Create this TXT record at your DNS provider, then press Verify. It can take a while to appear.
            </Trans>
          </p>
          <Field label={<Trans>Name</Trans>}>
            <CodeLine value={d.txt_name} testId="sso-txt-name" />
          </Field>
          <Field label={<Trans>Value</Trans>}>
            <CodeLine value={d.txt_value} testId="sso-txt-value" />
          </Field>
          {d.problem && (
            <p className="text-small text-destructive" role="status" data-testid="sso-problem">
              {d.problem}
            </p>
          )}
        </div>
      )}
      <ErrorLine error={verifyDomain.error ?? removeDomain.error} className="text-small" />
      {confirmDialog}
    </Card>
  )
}

function DomainsSection({ sso }: { sso: SsoSettings }) {
  const { addDomain } = useSsoMutations()
  const [domain, setDomain] = useState("")
  const add = () =>
    addDomain.mutate(domain.trim().toLowerCase(), {
      onSuccess: () => setDomain(""),
    })
  return (
    <Section
      title={<Trans>E-mail domains</Trans>}
      description={
        <Trans>
          Only the exact domain is claimed, not its subdomains. Add eu.example.com on its own if people use such
          addresses.
        </Trans>
      }
    >
      <div className="flex flex-col gap-4">
        {sso.domains.length === 0 && (
          <EmptyBox icon={GlobeIcon} title={<Trans>No domain claimed</Trans>} testId="sso-domains-empty">
            <Trans>Claim your company's e-mail domain to send its people to your provider.</Trans>
          </EmptyBox>
        )}
        {sso.domains.map((d) => (
          <DomainCard key={d.id} d={d} />
        ))}
        <form
          className="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (domain.trim() && !addDomain.isPending) add()
          }}
        >
          <Field label={<Trans>Add a domain</Trans>} htmlFor="sso-domain">
            <div className="flex gap-2">
              <Input
                id="sso-domain"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                placeholder="example.com"
                autoCapitalize="off"
                autoComplete="off"
                spellCheck={false}
                data-testid="sso-domain-input"
              />
              <Button type="submit" variant="outline" disabled={!domain.trim() || addDomain.isPending}>
                <PlusIcon />
                <Trans>Add</Trans>
              </Button>
            </div>
          </Field>
          <ErrorLine error={addDomain.error} className="text-small" />
        </form>
      </div>
    </Section>
  )
}

function RulesSection({ sso }: { sso: SsoSettings }) {
  const { t } = useLingui()
  const { rules } = useSsoMutations()
  const ready = !!(sso.oidc || sso.saml) && sso.domains.some((d) => d.state === "verified")
  return (
    <Section title={<Trans>Sign-in rules</Trans>}>
      <Card>
        <ToggleRow
          title={<Trans>Require single sign-on</Trans>}
          hint={
            <Trans>
              People of your verified domains work in this workspace only after signing in through your provider. Owners
              are exempt, so a broken provider never locks you out. Sessions that exist now stop working here, so
              covered members have to sign in again, and their connected apps need approving again.
            </Trans>
          }
        >
          <Switch
            checked={sso.sso_required}
            disabled={rules.isPending || (!ready && !sso.sso_required)}
            onCheckedChange={(v) => rules.mutate({ sso_required: v })}
            aria-label={t`Require single sign-on`}
            data-testid="sso-required"
          />
        </ToggleRow>
        {!ready && !sso.sso_required && (
          <p className="text-small text-faint">
            <Trans>Connect a provider and verify a domain first.</Trans>
          </p>
        )}
        <ToggleRow
          title={<Trans>Add people on their first sign-in</Trans>}
          hint={
            <Trans>
              Someone of your domains who signs in through the provider and is not a member joins with the role below.
            </Trans>
          }
        >
          <Switch
            checked={sso.jit}
            disabled={rules.isPending}
            onCheckedChange={(v) => rules.mutate({ jit: v })}
            aria-label={t`Add people on their first sign-in`}
            data-testid="sso-jit"
          />
        </ToggleRow>
        <Field label={<Trans>Role for new people</Trans>} htmlFor="sso-jit-role">
          <ChoiceSelect<SsoJitRole>
            id="sso-jit-role"
            value={sso.jit_role}
            onChange={(v) => rules.mutate({ jit_role: v })}
            options={[
              ["agent", t`Agent`],
              ["admin", t`Admin`],
            ]}
            disabled={!sso.jit || rules.isPending}
            label={t`Role for new people`}
            className="w-48"
          />
        </Field>
        <ErrorLine error={rules.error} className="text-small" />
      </Card>
    </Section>
  )
}

export function SsoPage() {
  const { t } = useLingui()
  const { isOwner } = useSession()
  const q = useSso(isOwner)
  const [picked, setPicked] = useState<SsoProtocol | null>(null)
  if (!isOwner) return <Navigate to="/settings/profile" replace />
  const sso = q.data
  const protocol: SsoProtocol = picked ?? sso?.protocol ?? "oidc"
  const switching = !!sso?.protocol && sso.protocol !== protocol
  return (
    <>
      <PageHeader
        title={<Trans>Single sign-on</Trans>}
        description={
          <Trans>
            Let your team sign in through your company's identity provider. Their e-mail address decides which provider
            they use.
          </Trans>
        }
      />
      {q.isPending ? (
        <Skeleton className="h-64 w-full rounded-xl" />
      ) : sso ? (
        <>
          <Section title={<Trans>Provider</Trans>}>
            <Card>
              <Field label={<Trans>Protocol</Trans>} htmlFor="sso-protocol">
                <ChoiceSelect<SsoProtocol>
                  id="sso-protocol"
                  value={protocol}
                  onChange={setPicked}
                  options={[
                    ["oidc", t`OpenID Connect`],
                    ["saml", t`SAML 2.0`],
                  ]}
                  label={t`Protocol`}
                  className="w-56"
                />
              </Field>
              {switching && (
                <Notice tone="warning" data-testid="sso-switch-warning">
                  {protocol === "saml" ? (
                    <Trans>Saving a SAML provider removes the OpenID Connect provider and its client secret.</Trans>
                  ) : (
                    <Trans>Saving an OpenID Connect provider removes the SAML provider and its certificates.</Trans>
                  )}
                </Notice>
              )}
            </Card>
            {protocol === "saml" ? (
              <SamlForm sso={sso} />
            ) : (
              <ProviderForm key={sso.oidc?.updated_at ?? "new"} sso={sso} />
            )}
          </Section>
          <DomainsSection sso={sso} />
          <RulesSection sso={sso} />
        </>
      ) : null}
      <ErrorLine error={q.error} />
    </>
  )
}
