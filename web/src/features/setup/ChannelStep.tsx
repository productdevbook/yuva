import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronRightIcon } from "lucide-react"
import { useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { ChannelIcon } from "@/components/common"
import { useInbox } from "@/features/settings/inboxes/queries"
import { DEFAULT_COLOR, KindPreview, PreviewCaption } from "@/features/setup/previews"
import { SETUP_KINDS, type SetupKind, type SetupState } from "@/features/setup/setup"
import { SetupLayout, StepHeader } from "@/features/setup/SetupLayout"
import { cn } from "@/lib/utils"

export function useKindText() {
  const { t } = useLingui()
  return {
    chat: { title: t`Website chat`, hint: t`A chat bubble on your site, from one line of HTML` },
    email: { title: t`E-mail`, hint: t`Mail to your support address, answered from it` },
    app: { title: t`Mobile app`, hint: t`Help and feedback inside your iOS or Android app` },
  } satisfies Record<SetupKind, { title: string; hint: string }>
}

export function ChannelStep({ inboxId }: { inboxId: string }) {
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as SetupState
  const inbox = useInbox(inboxId).data
  const text = useKindText()
  const [focus, setFocus] = useState<SetupKind>("chat")
  const name = inbox?.name ?? ""
  const caption = {
    chat: <Trans>Visitors write from any page of your site; you answer here, live.</Trans>,
    email: <Trans>Customers keep writing to the address they know.</Trans>,
    app: <Trans>People write from inside your app, signed in as themselves.</Trans>,
  }[focus]
  return (
    <SetupLayout
      step={2}
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
      <div className="flex flex-col gap-2.5">
        {SETUP_KINDS.map((k) => (
          <button
            key={k}
            type="button"
            className={cn(
              "group flex w-full items-center gap-4 rounded-2xl border bg-card px-4 py-4 text-start transition-colors outline-none hover:border-faint focus-visible:ring-3 focus-visible:ring-ring/30",
              focus === k && "border-faint",
            )}
            onMouseEnter={() => setFocus(k)}
            onFocus={() => setFocus(k)}
            onClick={() => navigate(`/setup/${inboxId}/add/${k}`, { state })}
            data-testid={`setup-pick-${k}`}
          >
            <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-surface text-muted-foreground [&_svg]:size-[18px]">
              <ChannelIcon kind={k} />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block font-medium">{text[k].title}</span>
              <span className="mt-0.5 block text-small text-faint">{text[k].hint}</span>
            </span>
            <ChevronRightIcon className="size-4 shrink-0 text-faint transition-transform group-hover:translate-x-0.5 rtl:rotate-180" />
          </button>
        ))}
      </div>
    </SetupLayout>
  )
}
