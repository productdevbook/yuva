import { Trans, useLingui } from "@lingui/react/macro"

import { Card, ChoiceSelect, PageHeader, Rows, Section, SettingRow } from "@/features/settings/ui"
import { setEmailView, useEmailView, type EmailView } from "@/lib/emailView"
import { setTheme, useTheme, type Theme } from "@/lib/theme"

export function AppearancePage() {
  const { t } = useLingui()
  const theme = useTheme()
  const emailView = useEmailView()
  return (
    <>
      <PageHeader title={<Trans>Appearance</Trans>} />
      <Section description={<Trans>These choices are kept on this device only.</Trans>}>
        <Card flush>
          <Rows>
            <SettingRow title={<Trans>Theme</Trans>} hint={<Trans>System follows your device</Trans>} htmlFor="theme">
              <ChoiceSelect<Theme>
                id="theme"
                label={t`Theme`}
                value={theme}
                onChange={setTheme}
                options={[
                  ["system", t`System`],
                  ["light", t`Light`],
                  ["dark", t`Dark`],
                ]}
              />
            </SettingRow>
            <SettingRow
              title={<Trans>E-mail view</Trans>}
              hint={<Trans>How HTML e-mails open. You can still switch each message.</Trans>}
              htmlFor="email-view"
            >
              <ChoiceSelect<EmailView>
                id="email-view"
                label={t`E-mail view`}
                value={emailView}
                onChange={setEmailView}
                options={[
                  ["original", t`Original`],
                  ["reading", t`Reading`],
                ]}
              />
            </SettingRow>
          </Rows>
        </Card>
      </Section>
    </>
  )
}
