import { Trans, useLingui } from "@lingui/react/macro"
import { BookOpenTextIcon } from "lucide-react"
import { useState } from "react"
import { Navigate, useLocation, useNavigate } from "react-router"

import { ChannelIcon, ChoiceCards, StepHeader } from "@/components/common"
import { useInbox } from "@/features/settings/inboxes/queries"
import { useInboxDraft, useLeaveSetup } from "@/features/setup/draft"
import { DEFAULT_COLOR, KindPreview, PreviewCaption } from "@/features/setup/previews"
import { SETUP_KINDS, type SetupKind, type SetupState } from "@/features/setup/setup"
import { SetupLayout } from "@/features/setup/SetupLayout"

export function useKindText() {
  const { t } = useLingui()
  return {
    chat: { title: t`Website chat`, hint: t`A chat bubble on your site, from one line of HTML` },
    email: { title: t`E-mail`, hint: t`Mail to your support address, answered from it` },
    app: { title: t`Mobile app`, hint: t`Help and feedback inside your iOS or Android app` },
    docs: { title: t`Documentation site`, hint: t`“Was this page helpful?”, feedback and discussions on your docs` },
  } satisfies Record<SetupKind, { title: string; hint: string }>
}

export function ChannelStep({ inboxId }: { inboxId: string }) {
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as SetupState
  const inbox = useInbox(inboxId).data
  const text = useKindText()
  const isNew = inboxId === "new"
  const [draft] = useInboxDraft()
  const leave = useLeaveSetup()
  const [focus, setFocus] = useState<SetupKind>("chat")
  const name = isNew ? draft.name : (inbox?.name ?? "")
  const mail = (isNew ? draft.purpose : inbox?.purpose) === "mail"
  const kinds = mail ? SETUP_KINDS.filter((k) => k === "email") : SETUP_KINDS
  if (isNew && !name.trim()) return <Navigate to="/setup/new" replace state={state} />
  const caption = {
    chat: <Trans>Visitors write from any page of your site; you answer here, live.</Trans>,
    email: <Trans>Customers keep writing to the address they know.</Trans>,
    app: <Trans>People write from inside your app, signed in as themselves.</Trans>,
    docs: <Trans>Readers rate each page, send feedback and discuss it in public; you answer and moderate here.</Trans>,
  }[focus]
  return (
    <SetupLayout
      step={2}
      onLeave={isNew ? leave : undefined}
      back={isNew ? "/setup/new" : undefined}
      preview={
        <>
          <KindPreview kind={focus} name={name} color={inbox?.branding.color ?? DEFAULT_COLOR} host="www.example.com" />
          <PreviewCaption>{caption}</PreviewCaption>
        </>
      }
    >
      <StepHeader title={<Trans>Where do {name} customers write from?</Trans>}>
        <Trans>Start with one. You can add the others later in the inbox settings.</Trans>
      </StepHeader>
      <ChoiceCards
        items={kinds.map((k) => ({
          value: k,
          icon: k === "docs" ? <BookOpenTextIcon /> : <ChannelIcon kind={k} />,
          title: text[k].title,
          hint: text[k].hint,
          active: focus === k,
          onActive: () => setFocus(k),
          onSelect: () => navigate(`/setup/${inboxId}/add/${k}`, { state }),
          testId: `setup-pick-${k}`,
        }))}
      />
    </SetupLayout>
  )
}
