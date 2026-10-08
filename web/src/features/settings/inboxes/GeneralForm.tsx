import { Trans, useLingui } from "@lingui/react/macro"
import { useMemo, useState } from "react"

import { MODES, useEnumText } from "@/components/common/text"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { useInboxOutlet } from "@/features/settings/inboxes/InboxLayout"
import { useSaveInbox } from "@/features/settings/inboxes/queries"
import { Field, FormActions, FormCard, Section } from "@/features/settings/ui"
import type { InboxMode } from "@/lib/api"
import { useSession } from "@/lib/session"

export function GeneralForm() {
  const { t } = useLingui()
  const { inbox } = useInboxOutlet()
  const { canManage } = useSession()
  const text = useEnumText()
  const save = useSaveInbox(inbox)
  const [name, setName] = useState(inbox.name)
  const [slug, setSlug] = useState(inbox.slug)
  const [mode, setMode] = useState<InboxMode>(inbox.mode)
  const [minutes, setMinutes] = useState(inbox.expected_reply_minutes?.toString() ?? "")
  const [timezone, setTimezone] = useState(inbox.timezone)
  const [locale, setLocale] = useState(inbox.default_locale)
  const zones = useMemo(() => Intl.supportedValuesOf("timeZone"), [])
  const submit = () =>
    save.mutate({
      name,
      slug,
      mode,
      timezone,
      default_locale: locale,
      expected_reply_minutes: minutes === "" ? null : Number(minutes),
    })
  return (
      <Section title={<Trans>General</Trans>}>
        <FormCard onSubmit={submit} footer={canManage && <FormActions pending={save.isPending} saved={save.isSuccess} error={save.error} />}>
          <fieldset disabled={!canManage} className="grid gap-5 sm:grid-cols-2">
            <Field label={<Trans>Name</Trans>} htmlFor="name">
              <Input id="name" required maxLength={200} value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field label={<Trans>Slug</Trans>} htmlFor="slug">
              <Input id="slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxLength={64} value={slug} onChange={(e) => setSlug(e.target.value)} />
            </Field>
            <Field
              label={<Trans>Mode</Trans>}
              hint={
                mode === "live" ? (
                  <Trans>Contacts see who is available, typing and read receipts.</Trans>
                ) : (
                  <Trans>Contacts see the expected reply time instead of who is online.</Trans>
                )
              }
            >
              <Select value={mode} onValueChange={(v) => setMode(v as InboxMode)} items={text.mode} disabled={!canManage}>
                <SelectTrigger className="w-full" aria-label={t`Mode`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODES.map((m) => (
                    <SelectItem key={m} value={m}>
                      {text.mode[m]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field
              label={<Trans>Expected reply time (minutes)</Trans>}
              htmlFor="minutes"
              hint={<Trans>Shown to contacts in async mode. Leave empty to hide it.</Trans>}
            >
              <Input id="minutes" type="number" min={1} max={43200} value={minutes} onChange={(e) => setMinutes(e.target.value)} />
            </Field>
            <Field label={<Trans>Time zone</Trans>} htmlFor="timezone">
              <Input id="timezone" list="timezones" required maxLength={64} value={timezone} onChange={(e) => setTimezone(e.target.value)} />
              <datalist id="timezones">
                {zones.map((z) => (
                  <option key={z} value={z} />
                ))}
              </datalist>
            </Field>
            <Field label={<Trans>Default language</Trans>} htmlFor="locale" hint={<Trans>A language tag such as en or tr.</Trans>}>
              <Input id="locale" required pattern="[a-z]{2,3}(-[A-Za-z0-9]{2,8})*" value={locale} onChange={(e) => setLocale(e.target.value)} />
            </Field>
          </fieldset>
        </FormCard>
      </Section>
  )
}

export function BrandingForm() {
  const { t } = useLingui()
  const { inbox } = useInboxOutlet()
  const { canManage } = useSession()
  const save = useSaveInbox(inbox)
  const [color, setColor] = useState(inbox.branding.color ?? "#d4431c")
  const [greeting, setGreeting] = useState(inbox.branding.greeting ?? "")
  const [logo, setLogo] = useState(inbox.branding.logo_url ?? "")
  const submit = () => save.mutate({ branding: { color, greeting: greeting || undefined, logo_url: logo || undefined } })
  return (
      <Section title={<Trans>Branding</Trans>} description={<Trans>How the chat widget and the apps present this inbox.</Trans>}>
        <FormCard onSubmit={submit} footer={canManage && <FormActions pending={save.isPending} saved={save.isSuccess} error={save.error} />}>
          <fieldset disabled={!canManage} className="grid gap-5 sm:grid-cols-2">
            <Field label={<Trans>Brand color</Trans>} htmlFor="color">
              <div className="flex gap-2">
                <input
                  type="color"
                  value={color}
                  onChange={(e) => setColor(e.target.value)}
                  aria-label={t`Pick a color`}
                  className="h-9 w-12 shrink-0 cursor-pointer rounded-xl border bg-transparent p-1"
                />
                <Input id="color" pattern="#[0-9a-fA-F]{6}" value={color} onChange={(e) => setColor(e.target.value)} />
              </div>
            </Field>
            <Field label={<Trans>Logo URL</Trans>} htmlFor="logo">
              <Input id="logo" type="url" value={logo} onChange={(e) => setLogo(e.target.value)} />
            </Field>
            <Field label={<Trans>Greeting</Trans>} htmlFor="greeting" className="sm:col-span-2">
              <Textarea id="greeting" maxLength={500} rows={2} value={greeting} placeholder={t`Hi! Ask us anything.`} onChange={(e) => setGreeting(e.target.value)} />
            </Field>
          </fieldset>
        </FormCard>
      </Section>
  )
}

export function GeneralPage() {
  return (
    <>
      <GeneralForm />
      <BrandingForm />
    </>
  )
}
