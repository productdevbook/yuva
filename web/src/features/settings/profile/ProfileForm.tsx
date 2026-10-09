import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { Input } from "@/components/ui/input"
import { Card, ChoiceSelect, Rows, Section, SettingRow } from "@/features/settings/ui"
import { activate, locales } from "@/i18n"
import { api, unwrap, type Locale } from "@/lib/api"
import { meKey, useSession } from "@/lib/session"

export function ProfileForm() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const errorText = useErrorText()
  const { me } = useSession()
  const [name, setName] = useState(me.person.name)
  const save = useMutation({
    mutationFn: (body: { name?: string; locale?: Locale }) => unwrap(api.PATCH("/v1/me", { body })),
    onSuccess: (data) => {
      qc.setQueryData(meKey, data)
      activate(data.person.locale)
      toast(t`Saved`)
    },
    onError: (e) => toast(errorText(e)),
  })
  return (
    <Section>
      <Card flush>
        <Rows>
          <SettingRow title={<Trans>Name</Trans>} hint={<Trans>Shown to your team and to contacts</Trans>} htmlFor="name">
            <Input
              id="name"
              value={name}
              maxLength={200}
              className="h-9 w-60 rounded-[10px] bg-background phone:w-40"
              onChange={(e) => setName(e.target.value)}
              onBlur={() => name.trim() !== me.person.name && save.mutate({ name: name.trim() })}
            />
          </SettingRow>
          <SettingRow title={<Trans>E-mail</Trans>}>
            <span className="truncate text-body text-faint">{me.person.email}</span>
          </SettingRow>
          <SettingRow title={<Trans>Panel language</Trans>} hint={<Trans>Also for sign-in and notification e-mails</Trans>} htmlFor="locale">
            <ChoiceSelect<Locale>
              id="locale"
              label={t`Panel language`}
              value={me.person.locale}
              onChange={(v) => save.mutate({ locale: v })}
              options={Object.entries(locales) as [Locale, string][]}
            />
          </SettingRow>
        </Rows>
      </Card>
    </Section>
  )
}
