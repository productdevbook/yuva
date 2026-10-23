import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useNavigate } from "react-router"

import { OwnDomainCard } from "@/features/settings/OwnDomainCard"
import { CopyButton, toast, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useDeleteChangelog, useUpdateChangelog } from "@/features/changelog/queries"
import { ErrorLine } from "@/features/changelog/text"
import { Languages } from "@/features/help/SettingsTab"
import { useLocaleName } from "@/features/help/text"
import { Card, ChoiceSelect, Field, FormActions, FormCard, Section, ToggleRow } from "@/features/settings/ui"
import { validSlug } from "@/features/status/CreatePageDialog"
import type { Changelog, ChangelogUpdate } from "@/lib/api"

function General({ changelog }: { changelog: Changelog }) {
  const { t } = useLingui()
  const name = useLocaleName()
  const patch = useUpdateChangelog(changelog.id)
  const properties = useAnalyticsProperties().data?.items ?? []
  const [title, setTitle] = useState(changelog.name)
  const [description, setDescription] = useState(changelog.description)
  const [slug, setSlug] = useState(changelog.slug)
  const [main, setMain] = useState(changelog.default_locale)
  const [langs, setLangs] = useState(changelog.locales)
  const [property, setProperty] = useState(changelog.analytics_property_id ?? "")

  const body: ChangelogUpdate = {}
  if (title.trim() !== changelog.name) body.name = title.trim()
  if (description.trim() !== changelog.description) body.description = description.trim()
  if (slug !== changelog.slug) body.slug = slug
  if (main !== changelog.default_locale) body.default_locale = main
  if (langs.join() !== changelog.locales.join()) body.locales = langs
  if ((property || null) !== (changelog.analytics_property_id ?? null)) body.analytics_property_id = property || null
  const dirty = Object.keys(body).length > 0
  const ok = !!title.trim() && validSlug(slug) && langs.includes(main)
  const address = changelog.url.endsWith(changelog.slug)
    ? changelog.url.slice(0, changelog.url.length - changelog.slug.length) + slug
    : changelog.url

  return (
    <Section title={<Trans>Changelog</Trans>}>
      <FormCard
        onSubmit={() =>
          ok &&
          patch.mutate(body, {
            onSuccess: (saved) => {
              setTitle(saved.name)
              setSlug(saved.slug)
              setLangs(saved.locales)
              toast(t`Saved`)
            },
          })
        }
        footer={
          <FormActions
            pending={patch.isPending}
            saved={patch.isSuccess && !dirty}
            error={undefined}
            disabled={!dirty || !ok}
          />
        }
      >
        <Field label={<Trans>Name</Trans>} htmlFor="changelog-setting-name">
          <Input
            id="changelog-setting-name"
            value={title}
            maxLength={100}
            onChange={(e) => setTitle(e.target.value)}
            data-testid="changelog-setting-name"
          />
        </Field>
        <Field
          label={<Trans>Description</Trans>}
          htmlFor="changelog-setting-description"
          hint={<Trans>Shown under the name on the home page.</Trans>}
        >
          <Textarea
            id="changelog-setting-description"
            value={description}
            maxLength={1000}
            onChange={(e) => setDescription(e.target.value)}
            data-testid="changelog-setting-description"
          />
        </Field>
        <Field
          label={<Trans>Address</Trans>}
          htmlFor="changelog-setting-slug"
          hint={<Trans>The changelog is at {address}. The old address keeps redirecting if you change it.</Trans>}
          error={slug && !validSlug(slug) ? <Trans>Use 3 to 48 letters, numbers and inner hyphens.</Trans> : undefined}
        >
          <Input
            id="changelog-setting-slug"
            value={slug}
            maxLength={48}
            spellCheck={false}
            autoCapitalize="none"
            onChange={(e) => setSlug(e.target.value.toLowerCase())}
            data-testid="changelog-setting-slug"
          />
        </Field>
        <Field
          label={<Trans>Languages</Trans>}
          hint={
            <Trans>
              Entries can be written and published in these languages. Visitors see their own when it exists.
            </Trans>
          }
        >
          <Languages
            value={langs}
            main={main}
            onChange={(next) => {
              setLangs(next)
              if (!next.includes(main)) setMain(next[0])
            }}
          />
        </Field>
        <Field
          label={<Trans>Main language</Trans>}
          htmlFor="changelog-main-locale"
          hint={<Trans>Shown when an entry has no translation in the visitor’s language.</Trans>}
        >
          <ChoiceSelect
            id="changelog-main-locale"
            value={main}
            onChange={setMain}
            options={langs.map((l) => [l, name(l)] as const)}
            label={t`Main language`}
            className="w-64"
          />
        </Field>
        {properties.length > 0 && (
          <Field
            label={<Trans>Analytics property</Trans>}
            htmlFor="changelog-property"
            hint={<Trans>Counts page views of the changelog without cookies.</Trans>}
          >
            <ChoiceSelect
              id="changelog-property"
              value={property}
              onChange={setProperty}
              options={[["", t`None`] as const, ...properties.map((p) => [p.id, p.name] as const)]}
              label={t`Analytics property`}
              className="w-64"
            />
          </Field>
        )}
        <ErrorLine error={patch.error} />
      </FormCard>
    </Section>
  )
}

function Access({ changelog }: { changelog: Changelog }) {
  const patch = useUpdateChangelog(changelog.id)
  return (
    <Section title={<Trans>Who can publish</Trans>}>
      <Card>
        <ToggleRow
          title={<Trans>Agents may publish</Trans>}
          hint={
            <Trans>
              Off: agents write drafts and submit them, and owners and admins publish. On: agents with access to the
              inbox publish, schedule and unpublish too.
            </Trans>
          }
        >
          <Switch
            checked={changelog.agents_may_publish}
            disabled={patch.isPending}
            onCheckedChange={(v) => patch.mutate({ agents_may_publish: v })}
            data-testid="changelog-agents-publish"
          />
        </ToggleRow>
        <ErrorLine error={patch.error} />
      </Card>
    </Section>
  )
}

function Address({ changelog }: { changelog: Changelog }) {
  return (
    <Section title={<Trans>Public address</Trans>}>
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          {changelog.published ? (
            <a
              href={changelog.url}
              target="_blank"
              rel="noreferrer noopener"
              className="min-w-0 flex-1 break-all text-body text-brand underline-offset-4 hover:underline"
              data-testid="changelog-public-url"
            >
              {changelog.url}
            </a>
          ) : (
            <span
              className="min-w-0 flex-1 break-all text-body text-muted-foreground"
              data-testid="changelog-public-url"
            >
              {changelog.url}
            </span>
          )}
          <CopyButton value={changelog.url} />
        </div>
        {!changelog.published && (
          <p className="text-small text-faint">
            <Trans>The address answers “not found” until you publish the changelog.</Trans>
          </p>
        )}
      </Card>
    </Section>
  )
}

function Danger({ changelog }: { changelog: Changelog }) {
  const navigate = useNavigate()
  const [confirm, dialog] = useConfirm()
  const remove = useDeleteChangelog(changelog.id)
  const changelogName = changelog.name
  return (
    <>
      <Section title={<Trans>Delete</Trans>}>
        <Card tone="danger">
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-0 flex-1 text-body">
              <Trans>
                Deletes the changelog with its entries and translations. The public address stops working. Broadcasts
                made from its entries stay.
              </Trans>
            </p>
            <Button
              variant="destructive"
              size="sm"
              disabled={remove.isPending}
              onClick={() =>
                confirm({
                  title: <Trans>Delete {changelogName}?</Trans>,
                  description: <Trans>Entries and translations are deleted with it.</Trans>,
                  confirm: <Trans>Delete changelog</Trans>,
                  run: () =>
                    remove.mutate(undefined, {
                      onSuccess: () => {
                        navigate("/updates")
                      },
                    }),
                })
              }
              data-testid="changelog-delete"
            >
              <Trans>Delete changelog</Trans>
            </Button>
          </div>
          <ErrorLine error={remove.error} />
        </Card>
      </Section>
      {dialog}
    </>
  )
}

export function SettingsTab({ changelog }: { changelog: Changelog }) {
  const domainSave = useUpdateChangelog(changelog.id)
  return (
    <div className="flex flex-col gap-8" data-testid="changelog-settings">
      <Address changelog={changelog} />
      <General changelog={changelog} />
      <Access changelog={changelog} />
      <OwnDomainCard
        domain={changelog.custom_domain}
        enabled={!!changelog.own_domains}
        example="news.example.com"
        tid="changelog-domain"
        save={domainSave}
      />
      <Danger changelog={changelog} />
    </div>
  )
}
