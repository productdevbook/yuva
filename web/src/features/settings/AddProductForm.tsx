import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useNavigate } from "react-router"

import { FormCard } from "@/components/common/FormCard"
import { MailboxOwnerChoice } from "@/components/common/MailboxOwnerChoice"
import { PurposeChoice } from "@/components/common/PurposeChoice"
import { RadioChoices } from "@/components/common/RadioChoices"
import { Input } from "@/components/ui/input"
import { ColorPicker, PRODUCT_COLORS } from "@/features/settings/inboxes/colors"
import { Field } from "@/features/settings/ui"
import { useInboxDraft } from "@/features/setup/draft"
import type { InboxPurpose } from "@/lib/api"

export function AddProductForm({ onClose }: { onClose: () => void }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [, patch] = useInboxDraft()
  const [name, setName] = useState("")
  const [color, setColor] = useState(PRODUCT_COLORS[5])
  const [mode, setMode] = useState<"live" | "async">("live")
  const [purpose, setPurpose] = useState<InboxPurpose>("support")
  const [owner, setOwner] = useState<string | undefined>(undefined)
  const ready = !!name.trim() && !(purpose === "mail" && owner === "")
  return (
    <FormCard
      title={<Trans>Add a product</Trans>}
      description={<Trans>The next step adds its first channel: website chat, e-mail or a mobile app.</Trans>}
      submit={<Trans>Create and add a channel</Trans>}
      ready={ready}
      onCancel={onClose}
      onSubmit={() => {
        patch({ name: name.trim(), color, mode, purpose, ownerMemberId: purpose === "mail" ? owner : undefined })
        navigate(purpose === "mail" ? "/setup/new/add/email" : "/setup/new/pick", { state: { back: "/settings" } })
      }}
      testId="add-product"
    >
      <Field label={<Trans>Product name</Trans>} htmlFor="add-product-name">
        <Input
          id="add-product-name"
          autoFocus
          required
          maxLength={200}
          value={name}
          placeholder={t`For example Fieldnote`}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <Field label={<Trans>Colour</Trans>}>
        <ColorPicker value={color} onChange={setColor} />
      </Field>
      <PurposeChoice
        value={purpose}
        onChange={(p) => {
          setPurpose(p)
          setOwner(undefined)
        }}
      />
      {purpose === "mail" && <MailboxOwnerChoice value={owner} onChange={setOwner} />}
      {purpose === "support" && (
        <Field label={<Trans>How do you answer?</Trans>}>
          <RadioChoices
            id="add-product-mode"
            value={mode}
            onChange={setMode}
            label={t`How do you answer?`}
            columns
            items={[
              {
                value: "live",
                title: <Trans>Live chat</Trans>,
                hint: <Trans>The chat shows who is available and typing</Trans>,
              },
              {
                value: "async",
                title: <Trans>Async</Trans>,
                hint: <Trans>The chat shows when you usually reply</Trans>,
              },
            ]}
          />
        </Field>
      )}
    </FormCard>
  )
}
