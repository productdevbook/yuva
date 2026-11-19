import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { ChannelIcon, toast } from "@/components/common"
import { FormCard } from "@/components/common/FormCard"
import { RadioChoices } from "@/components/common/RadioChoices"
import { ApiChannelForm } from "@/features/settings/channels/ApiChannelForm"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import type { ChannelKind, Inbox } from "@/lib/api"

type Kind = ChannelKind | "docs"

export function AddChannelForm({
  inboxes,
  initial,
  onClose,
  className,
}: {
  inboxes: Inbox[]
  initial?: string
  onClose: () => void
  className?: string
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { pathname, search } = useLocation()
  const [inboxId, setInboxId] = useState(
    initial && inboxes.some((i) => i.id === initial) ? initial : (inboxes[0]?.id ?? "")
  )
  const [picked, setKind] = useState<Kind>("chat")
  const [apiOpen, setApiOpen] = useState(false)
  const kinds: { value: Kind; label: string; icon: ChannelKind }[] = [
    { value: "chat", label: t`Website chat`, icon: "chat" },
    { value: "email", label: t`E-mail`, icon: "email" },
    { value: "app", label: t`Mobile app`, icon: "app" },
    { value: "docs", label: t`Documentation site`, icon: "chat" },
    { value: "api", label: t`API channel`, icon: "api" },
  ]
  const mail = inboxes.find((i) => i.id === inboxId)?.purpose === "mail"
  const kind: Kind = mail ? "email" : picked
  const shown = mail ? kinds.filter((k) => k.value === "email") : kinds
  if (apiOpen)
    return (
      <ApiChannelForm
        inboxId={inboxId}
        className={className}
        onClose={onClose}
        onCreated={() => {
          toast(t`Channel added`)
          onClose()
        }}
      />
    )
  return (
    <FormCard
      title={<Trans>Add a channel</Trans>}
      description={<Trans>Pick the product and how customers reach it. The next steps set it up.</Trans>}
      submit={<Trans>Continue</Trans>}
      ready={!!inboxId}
      onCancel={onClose}
      onSubmit={() => {
        if (kind === "api") return setApiOpen(true)
        navigate(`/setup/${inboxId}/add/${kind}`, { state: { back: `${pathname}${search}`, existing: true } })
      }}
      className={className}
      testId="add-channel"
    >
      <Field label={<Trans>Product</Trans>} htmlFor="add-channel-product">
        <ChoiceSelect
          id="add-channel-product"
          label={t`Product`}
          value={inboxId}
          onChange={setInboxId}
          options={inboxes.map((i) => [i.id, i.name] as const)}
          className="w-full sm:w-80"
        />
      </Field>
      <Field label={<Trans>Channel</Trans>}>
        <RadioChoices
          id="add-channel-kind"
          value={kind}
          onChange={setKind}
          label={t`Channel`}
          columns
          items={shown.map((k) => ({
            value: k.value,
            title: k.label,
            icon: <ChannelIcon kind={k.icon} />,
            testId: `add-channel-${k.value}`,
          }))}
        />
      </Field>
    </FormCard>
  )
}
