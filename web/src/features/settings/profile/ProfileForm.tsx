import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Field, FormActions, FormCard, Section } from "@/features/settings/ui"
import { locales } from "@/i18n"
import { api, unwrap, type Locale } from "@/lib/api"
import { meKey, useSession } from "@/lib/session"

export function ProfileForm() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { me } = useSession()
  const [name, setName] = useState(me.person.name)
  const [locale, setLocale] = useState<Locale>(me.person.locale)
  const save = useMutation({
    mutationFn: () => unwrap(api.PATCH("/v1/me", { body: { name, locale } })),
    onSuccess: (data) => qc.setQueryData(meKey, data),
  })
  const dirty = name !== me.person.name || locale !== me.person.locale
  return (
    <Section title={<Trans>Profile</Trans>}>
      <FormCard
        onSubmit={() => save.mutate()}
        footer={<FormActions pending={save.isPending} disabled={!dirty} saved={save.isSuccess && !dirty} error={save.error} />}
      >
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={<Trans>Name</Trans>} htmlFor="name" hint={<Trans>Shown to your team and to contacts.</Trans>}>
            <Input id="name" value={name} maxLength={200} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label={<Trans>Language</Trans>} hint={<Trans>The language of the panel and of sign-in and notification e-mails.</Trans>}>
            <Select value={locale} onValueChange={(v) => setLocale(v as Locale)} items={locales}>
              <SelectTrigger className="w-full" aria-label={t`Language`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(locales).map(([k, v]) => (
                  <SelectItem key={k} value={k}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </div>
      </FormCard>
    </Section>
  )
}
