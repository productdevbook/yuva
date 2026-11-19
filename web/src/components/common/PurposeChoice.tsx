import { Trans, useLingui } from "@lingui/react/macro"

import { RadioChoices } from "@/components/common/RadioChoices"
import { Field } from "@/features/settings/ui"
import type { InboxPurpose } from "@/lib/api"

export function PurposeChoice({ value, onChange }: { value: InboxPurpose; onChange: (p: InboxPurpose) => void }) {
  const { t } = useLingui()
  return (
    <Field label={<Trans>What is this inbox for?</Trans>}>
      <RadioChoices
        id="purpose"
        value={value}
        onChange={onChange}
        label={t`Purpose`}
        columns
        items={[
          {
            value: "support",
            title: <Trans>Support</Trans>,
            hint: <Trans>Customers write by chat, app or e-mail; ratings, assignment and SLA work here.</Trans>,
            testId: "purpose-support",
          },
          {
            value: "mail",
            title: <Trans>Team mail</Trans>,
            hint: <Trans>A shared mailbox like hello@ or billing@, kept out of support views and stats.</Trans>,
            testId: "purpose-mail",
          },
        ]}
      />
    </Field>
  )
}
