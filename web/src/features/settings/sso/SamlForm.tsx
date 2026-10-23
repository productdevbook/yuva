import { Trans, useLingui } from "@lingui/react/macro"
import { FileUpIcon } from "lucide-react"
import { useRef, useState } from "react"

import { CodeLine, Notice, toast, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { useSsoMutations } from "@/features/settings/sso/queries"
import { Card, Field, FormActions, FormCard, Pill, Section } from "@/features/settings/ui"
import type { SsoSamlCertificate, SsoSettings } from "@/lib/api"

const words = (v: string) => v.split(/[\s,]+/).filter(Boolean)
const SOON = 30 * 86_400_000
const now = () => Date.now()

function CertificateRow({ c }: { c: SsoSamlCertificate }) {
  const { i18n } = useLingui()
  const left = new Date(c.not_after).getTime() - now()
  const from = formatDateTime(c.not_before, i18n.locale)
  const to = formatDateTime(c.not_after, i18n.locale)
  return (
    <div className="flex flex-col gap-2 rounded-xl border bg-surface p-3.5" data-testid="sso-saml-cert">
      <div className="flex flex-wrap items-center gap-2">
        <span className="min-w-0 flex-1 basis-40 font-medium [overflow-wrap:anywhere]">{c.subject}</span>
        {left < 0 ? (
          <Pill tone="danger">
            <Trans>Expired</Trans>
          </Pill>
        ) : left <= SOON ? (
          <Pill tone="warning" data-testid="sso-saml-cert-soon">
            <Trans>Expires soon</Trans>
          </Pill>
        ) : (
          <Pill tone="success">
            <Trans>Valid</Trans>
          </Pill>
        )}
      </div>
      <p className="text-small text-muted-foreground">
        <Trans>
          Valid {from} to {to}
        </Trans>
      </p>
      <Field label={<Trans>SHA-256 fingerprint</Trans>}>
        <CodeLine value={c.sha256} />
      </Field>
    </div>
  )
}

function ProviderDetails({ sso, warnings }: { sso: SsoSettings; warnings: string[] }) {
  const { i18n } = useLingui()
  const s = sso.saml
  const soon = s?.certificates.some((c) => new Date(c.not_after).getTime() - now() <= SOON)
  const fetched = s?.metadata_fetched_at ? formatDateTime(s.metadata_fetched_at, i18n.locale) : null
  return (
    <>
      {warnings.length > 0 && (
        <Notice tone="warning" data-testid="sso-saml-warnings">
          <ul className="flex list-disc flex-col gap-1 ps-4">
            {warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        </Notice>
      )}
      {s && (
        <Section title={<Trans>Your identity provider</Trans>}>
          <Card>
            <Field label={<Trans>Entity ID</Trans>}>
              <CodeLine value={s.entity_id} testId="sso-saml-idp-entity" />
            </Field>
            <Field label={<Trans>Single sign-on URL</Trans>}>
              <CodeLine value={s.sso_url} testId="sso-saml-idp-url" />
            </Field>
            {s.metadata_url ? (
              <Field label={<Trans>Metadata</Trans>} hint={fetched ? <Trans>Last read {fetched}</Trans> : undefined}>
                <CodeLine value={s.metadata_url} />
              </Field>
            ) : (
              <p className="text-small text-muted-foreground">
                <Trans>The metadata was pasted. Paste it again when your provider renews its certificate.</Trans>
              </p>
            )}
            {soon && (
              <Notice tone="warning">
                <Trans>
                  A signing certificate expires within 30 days. Sign-ins stop working when it does, unless your provider
                  has published a newer one in its metadata.
                </Trans>
              </Notice>
            )}
            <div className="flex flex-col gap-2">
              {s.certificates.map((c) => (
                <CertificateRow key={c.sha256} c={c} />
              ))}
            </div>
          </Card>
        </Section>
      )}
    </>
  )
}

export function SamlForm({ sso }: { sso: SsoSettings }) {
  const { t } = useLingui()
  const { saveSaml, removeSaml } = useSsoMutations()
  const [confirm, confirmDialog] = useConfirm()
  const file = useRef<HTMLInputElement>(null)
  const s = sso.saml
  const sp = sso.saml_sp
  const [url, setUrl] = useState(s?.metadata_url ?? "")
  const [xml, setXml] = useState("")
  const [emailAttr, setEmailAttr] = useState(s?.email_attribute ?? "")
  const [groupsAttr, setGroupsAttr] = useState(s?.groups_attribute ?? "groups")
  const [groups, setGroups] = useState(s?.allowed_groups.join(", ") ?? "")
  const hasSource = !!xml.trim() || !!url.trim() || !!s
  const dirty =
    !s ||
    !!xml.trim() ||
    url.trim() !== (s.metadata_url ?? "") ||
    emailAttr.trim() !== (s.email_attribute ?? "") ||
    groupsAttr.trim() !== s.groups_attribute ||
    words(groups).join(" ") !== s.allowed_groups.join(" ")
  const submit = () =>
    saveSaml.mutate(
      {
        metadata_xml: xml.trim() || undefined,
        metadata_url: !xml.trim() && url.trim() && url.trim() !== (s?.metadata_url ?? "") ? url.trim() : undefined,
        email_attribute: emailAttr.trim(),
        groups_attribute: groupsAttr.trim() || undefined,
        allowed_groups: words(groups.replace(/,/g, " ")),
      },
      {
        onSuccess: () => {
          setXml("")
          toast(t`Provider saved. Its metadata was read.`)
        },
      }
    )
  return (
    <>
      <Section
        title={<Trans>Your service provider details</Trans>}
        description={<Trans>Paste these into your identity provider, or give it the metadata address.</Trans>}
      >
        <Card>
          <Field label={<Trans>Entity ID</Trans>} hint={<Trans>Also called audience or identifier.</Trans>}>
            <CodeLine value={sp.entity_id} testId="sso-saml-sp-entity" />
          </Field>
          <Field label={<Trans>ACS URL</Trans>} hint={<Trans>Also called reply URL. HTTP-POST binding.</Trans>}>
            <CodeLine value={sp.acs_url} testId="sso-saml-sp-acs" />
          </Field>
          <Field label={<Trans>Metadata URL</Trans>}>
            <CodeLine value={sp.metadata_url} testId="sso-saml-sp-metadata" />
          </Field>
          <Field label={<Trans>NameID format</Trans>}>
            <CodeLine value={sp.name_id_format} testId="sso-saml-sp-nameid" />
          </Field>
        </Card>
      </Section>
      <FormCard
        onSubmit={() => hasSource && submit()}
        footer={
          <>
            {s && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive"
                disabled={removeSaml.isPending}
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
                    run: () => removeSaml.mutate(undefined, { onSuccess: () => toast(t`Provider removed`) }),
                  })
                }
              >
                <Trans>Remove</Trans>
              </Button>
            )}
            <FormActions
              pending={saveSaml.isPending}
              saved={saveSaml.isSuccess && !dirty}
              error={saveSaml.error ?? removeSaml.error}
              disabled={!dirty || !hasSource}
              label={s ? <Trans>Save and test</Trans> : <Trans>Connect and test</Trans>}
            />
          </>
        }
      >
        <Field
          label={<Trans>Metadata URL</Trans>}
          htmlFor="sso-saml-url"
          hint={
            <Trans>
              Preferred: Yuva reads it again when the provider rotates its certificate. Saving with the same address
              reads it now.
            </Trans>
          }
        >
          <Input
            id="sso-saml-url"
            type="url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://idp.example.com/metadata"
            autoCapitalize="off"
            spellCheck={false}
            data-testid="sso-saml-metadata-url"
          />
        </Field>
        <Field
          label={<Trans>Or paste the metadata</Trans>}
          htmlFor="sso-saml-xml"
          hint={
            <Trans>For providers without a metadata address, such as Google Workspace. It replaces the address.</Trans>
          }
        >
          <Textarea
            id="sso-saml-xml"
            value={xml}
            onChange={(e) => setXml(e.target.value)}
            rows={4}
            className="font-mono text-caption"
            spellCheck={false}
            data-testid="sso-saml-metadata-xml"
          />
          <input
            ref={file}
            type="file"
            accept=".xml,text/xml,application/xml"
            className="hidden"
            onChange={async (e) => {
              const f = e.target.files?.[0]
              e.target.value = ""
              if (f) setXml(await f.text())
            }}
            data-testid="sso-saml-file-input"
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-2 self-start"
            onClick={() => file.current?.click()}
          >
            <FileUpIcon />
            <Trans>Upload a metadata file</Trans>
          </Button>
        </Field>
        <Field
          label={<Trans>E-mail attribute</Trans>}
          htmlFor="sso-saml-email"
          hint={<Trans>Leave empty when the NameID is the e-mail address.</Trans>}
        >
          <Input
            id="sso-saml-email"
            value={emailAttr}
            onChange={(e) => setEmailAttr(e.target.value)}
            autoCapitalize="off"
            spellCheck={false}
          />
        </Field>
        <Field
          label={<Trans>Groups attribute</Trans>}
          htmlFor="sso-saml-groups-attr"
          hint={<Trans>The attribute that lists a person's groups.</Trans>}
        >
          <Input
            id="sso-saml-groups-attr"
            value={groupsAttr}
            onChange={(e) => setGroupsAttr(e.target.value)}
            placeholder="groups"
            autoCapitalize="off"
            spellCheck={false}
          />
        </Field>
        <Field
          label={<Trans>Allowed groups</Trans>}
          htmlFor="sso-saml-groups"
          hint={<Trans>Separate with commas. Leave empty to let everyone in your domains sign in.</Trans>}
        >
          <Input
            id="sso-saml-groups"
            value={groups}
            onChange={(e) => setGroups(e.target.value)}
            autoCapitalize="off"
            spellCheck={false}
          />
        </Field>
      </FormCard>
      {confirmDialog}
      <ProviderDetails sso={sso} warnings={saveSaml.data?.warnings ?? []} />
    </>
  )
}
