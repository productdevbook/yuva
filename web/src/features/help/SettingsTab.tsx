import { Trans, useLingui } from "@lingui/react/macro"
import { XIcon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import { OwnDomainCard } from "@/features/settings/OwnDomainCard"
import { CopyButton, toast, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useInboxChannels } from "@/features/broadcasts/queries"
import { usePublishToggle } from "@/features/help/CenterBar"
import { ImportCard } from "@/features/help/ImportCard"
import { useDeleteHelpCenter, useUpdateHelpCenter } from "@/features/help/queries"
import { ErrorLine, useLocaleName } from "@/features/help/text"
import { Card, ChoiceSelect, Field, FormActions, FormCard, Section, ToggleRow } from "@/features/settings/ui"
import { validSlug } from "@/features/status/CreatePageDialog"
import { locales as panelLocales } from "@/i18n"
import type { HelpCenter, HelpCenterUpdate } from "@/lib/api"

const COLOR_RE = /^#[0-9a-fA-F]{6}$/

export function Languages({
  value,
  main,
  onChange,
}: {
  value: string[]
  main: string
  onChange: (value: string[]) => void
}) {
  const { t } = useLingui()
  const name = useLocaleName()
  const addable = Object.keys(panelLocales).filter((l) => !value.includes(l))
  return (
    <div className="flex flex-col gap-2.5">
      <ul className="flex flex-wrap gap-1.5" data-testid="help-setting-locales">
        {value.map((l) => (
          <li
            key={l}
            className="flex items-center gap-1 rounded-lg border bg-background py-0.5 ps-2.5 pe-0.5 text-body"
          >
            {name(l)}
            <span className="text-caption text-faint uppercase">{l}</span>
            {l === main ? (
              <span className="px-1.5 text-caption text-faint">
                <Trans>Main</Trans>
              </span>
            ) : (
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                className="text-faint"
                aria-label={t`Remove ${name(l)}`}
                onClick={() => onChange(value.filter((x) => x !== l))}
              >
                <XIcon />
              </Button>
            )}
          </li>
        ))}
      </ul>
      {addable.length > 0 && (
        <ChoiceSelect
          value=""
          placeholder={t`Add a language`}
          onChange={(l) => onChange([...value, l])}
          options={addable.map((l) => [l, `${panelLocales[l as keyof typeof panelLocales]} (${l})`] as const)}
          label={t`Add a language`}
          className="w-64"
        />
      )}
    </div>
  )
}

function General({ center }: { center: HelpCenter }) {
  const { t } = useLingui()
  const name = useLocaleName()
  const patch = useUpdateHelpCenter(center.id)
  const channels = useInboxChannels(center.inbox_id).data ?? []
  const properties = useAnalyticsProperties().data?.items ?? []
  const [title, setTitle] = useState(center.name)
  const [description, setDescription] = useState(center.description)
  const [slug, setSlug] = useState(center.slug)
  const [main, setMain] = useState(center.default_locale)
  const [langs, setLangs] = useState(center.locales)
  const [logo, setLogo] = useState(center.logo_url ?? "")
  const [color, setColor] = useState(center.accent_color ?? "")
  const [site, setSite] = useState(center.site_url ?? "")
  const [channel, setChannel] = useState(center.chat_channel_id ?? "")
  const [property, setProperty] = useState(center.analytics_property_id ?? "")

  const link = (v: string) => (v.trim() ? v.trim() : null)
  const body: HelpCenterUpdate = {}
  if (title.trim() !== center.name) body.name = title.trim()
  if (description.trim() !== center.description) body.description = description.trim()
  if (slug !== center.slug) body.slug = slug
  if (main !== center.default_locale) body.default_locale = main
  if (langs.join() !== center.locales.join()) body.locales = langs
  if (link(logo) !== (center.logo_url ?? null)) body.logo_url = link(logo)
  if (link(color) !== (center.accent_color ?? null)) body.accent_color = link(color)
  if (link(site) !== (center.site_url ?? null)) body.site_url = link(site)
  if ((channel || null) !== (center.chat_channel_id ?? null)) body.chat_channel_id = channel || null
  if ((property || null) !== (center.analytics_property_id ?? null)) body.analytics_property_id = property || null
  const dirty = Object.keys(body).length > 0
  const colorOk = !color || COLOR_RE.test(color)
  const ok = !!title.trim() && validSlug(slug) && colorOk && langs.includes(main)
  const address = center.url.endsWith(center.slug)
    ? center.url.slice(0, center.url.length - center.slug.length) + slug
    : center.url

  return (
    <Section title={<Trans>Help center</Trans>}>
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
        <Field label={<Trans>Name</Trans>} htmlFor="help-name">
          <Input
            id="help-name"
            value={title}
            maxLength={100}
            onChange={(e) => setTitle(e.target.value)}
            data-testid="help-setting-name"
          />
        </Field>
        <Field
          label={<Trans>Description</Trans>}
          htmlFor="help-description"
          hint={<Trans>Shown under the name on the home page.</Trans>}
        >
          <Textarea
            id="help-description"
            value={description}
            maxLength={1000}
            onChange={(e) => setDescription(e.target.value)}
            data-testid="help-setting-description"
          />
        </Field>
        <Field
          label={<Trans>Address</Trans>}
          htmlFor="help-slug"
          hint={<Trans>The help center is at {address}. The old address keeps redirecting if you change it.</Trans>}
          error={slug && !validSlug(slug) ? <Trans>Use 3 to 48 letters, numbers and inner hyphens.</Trans> : undefined}
        >
          <Input
            id="help-slug"
            value={slug}
            maxLength={48}
            spellCheck={false}
            autoCapitalize="none"
            onChange={(e) => setSlug(e.target.value.toLowerCase())}
            data-testid="help-setting-slug"
          />
        </Field>
        <Field
          label={<Trans>Languages</Trans>}
          hint={
            <Trans>
              Articles can be written and published in these languages. Visitors see their own when it exists.
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
          htmlFor="help-main-locale"
          hint={<Trans>Shown when an article has no translation in the visitor’s language.</Trans>}
        >
          <ChoiceSelect
            id="help-main-locale"
            value={main}
            onChange={setMain}
            options={langs.map((l) => [l, name(l)] as const)}
            label={t`Main language`}
            className="w-64"
          />
        </Field>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={<Trans>Logo URL</Trans>} htmlFor="help-logo" hint={<Trans>An https image.</Trans>}>
            <Input
              id="help-logo"
              type="url"
              value={logo}
              placeholder="https://"
              onChange={(e) => setLogo(e.target.value)}
              data-testid="help-setting-logo"
            />
          </Field>
          <Field
            label={<Trans>Accent colour</Trans>}
            htmlFor="help-color"
            error={!colorOk ? <Trans>Use a colour like #2563eb.</Trans> : undefined}
          >
            <div className="flex items-center gap-2">
              <input
                type="color"
                aria-label={t`Pick a colour`}
                value={COLOR_RE.test(color) ? color : "#2563eb"}
                onChange={(e) => setColor(e.target.value)}
                className="size-9 shrink-0 cursor-pointer rounded-xl border border-input bg-background p-1"
              />
              <Input
                id="help-color"
                value={color}
                maxLength={7}
                placeholder="#2563eb"
                spellCheck={false}
                onChange={(e) => setColor(e.target.value)}
                data-testid="help-setting-color"
              />
            </div>
          </Field>
          <Field label={<Trans>Site link</Trans>} htmlFor="help-site" hint={<Trans>Your product.</Trans>}>
            <Input
              id="help-site"
              type="url"
              value={site}
              placeholder="https://"
              onChange={(e) => setSite(e.target.value)}
              data-testid="help-setting-site"
            />
          </Field>
          <Field
            label={<Trans>Chat channel</Trans>}
            htmlFor="help-channel"
            hint={<Trans>The pages offer this chat under “Still need help?”.</Trans>}
          >
            <ChoiceSelect
              id="help-channel"
              value={channel}
              onChange={setChannel}
              options={[
                ["", t`None`] as const,
                ...channels.filter((c) => c.kind === "chat").map((c) => [c.id, c.name] as const),
              ]}
              label={t`Chat channel`}
            />
          </Field>
          {properties.length > 0 && (
            <Field
              label={<Trans>Analytics property</Trans>}
              htmlFor="help-property"
              hint={<Trans>Counts page views of the help center without cookies.</Trans>}
            >
              <ChoiceSelect
                id="help-property"
                value={property}
                onChange={setProperty}
                options={[["", t`None`] as const, ...properties.map((p) => [p.id, p.name] as const)]}
                label={t`Analytics property`}
              />
            </Field>
          )}
        </div>
        <ErrorLine error={patch.error} />
      </FormCard>
    </Section>
  )
}

function Publishing({ center }: { center: HelpCenter }) {
  const patch = useUpdateHelpCenter(center.id)
  const pub = usePublishToggle(center.id, center.published)
  return (
    <Section title={<Trans>Publishing</Trans>}>
      <Card>
        <ToggleRow
          title={<Trans>Published</Trans>}
          hint={
            <Trans>On: anyone with the address can read the help center. Off: only your team can preview it.</Trans>
          }
        >
          <Switch
            checked={center.published}
            disabled={pub.pending}
            onCheckedChange={pub.toggle}
            data-testid="help-published-switch"
          />
        </ToggleRow>
        <ToggleRow
          title={<Trans>Agents may publish</Trans>}
          hint={
            <Trans>
              Off: agents write drafts and submit them, and owners and admins publish. On: agents with access to the
              inbox publish and unpublish too.
            </Trans>
          }
        >
          <Switch
            checked={center.agents_may_publish}
            disabled={patch.isPending}
            onCheckedChange={(v) => patch.mutate({ agents_may_publish: v })}
            data-testid="help-agents-publish"
          />
        </ToggleRow>
        <ErrorLine error={pub.error ?? patch.error} />
      </Card>
      {pub.dialog}
    </Section>
  )
}

function Address({ center }: { center: HelpCenter }) {
  return (
    <Section title={<Trans>Public address</Trans>}>
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          {center.published ? (
            <a
              href={center.url}
              target="_blank"
              rel="noreferrer noopener"
              className="min-w-0 flex-1 break-all text-body text-brand underline-offset-4 hover:underline"
              data-testid="help-public-url"
            >
              {center.url}
            </a>
          ) : (
            <span className="min-w-0 flex-1 break-all text-body text-muted-foreground" data-testid="help-public-url">
              {center.url}
            </span>
          )}
          <CopyButton value={center.url} />
        </div>
        {!center.published && (
          <p className="text-small text-faint">
            <Trans>The address answers “not found” until you publish the help center.</Trans>
          </p>
        )}
      </Card>
    </Section>
  )
}

function Danger({ center }: { center: HelpCenter }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [confirm, dialog] = useConfirm()
  const remove = useDeleteHelpCenter(center.id)
  const centerName = center.name
  return (
    <>
      <Section title={<Trans>Delete</Trans>}>
        <Card tone="danger">
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-0 flex-1 text-body">
              <Trans>
                Deletes the help center with its collections, articles, revisions and counters. The public address stops
                working.
              </Trans>
            </p>
            <Button
              variant="destructive"
              size="sm"
              disabled={remove.isPending}
              onClick={() =>
                confirm({
                  title: <Trans>Delete {centerName}?</Trans>,
                  description: <Trans>Collections, articles, revisions and counters are deleted with it.</Trans>,
                  confirm: <Trans>Delete help center</Trans>,
                  run: () =>
                    remove.mutate(undefined, {
                      onSuccess: () => {
                        toast(t`Help center deleted`)
                        navigate("/knowledge")
                      },
                    }),
                })
              }
              data-testid="help-delete"
            >
              <Trans>Delete help center</Trans>
            </Button>
          </div>
          <ErrorLine error={remove.error} />
        </Card>
      </Section>
      {dialog}
    </>
  )
}

export function SettingsTab({ center }: { center: HelpCenter }) {
  const domainSave = useUpdateHelpCenter(center.id)
  return (
    <div className="flex flex-col gap-8" data-testid="help-settings">
      <Address center={center} />
      <General center={center} />
      <Publishing center={center} />
      <ImportCard center={center} />
      <OwnDomainCard
        domain={center.custom_domain}
        enabled={!!center.own_domains}
        example="help.example.com"
        tid="help-domain"
        save={domainSave}
      />
      <Danger center={center} />
    </div>
  )
}
