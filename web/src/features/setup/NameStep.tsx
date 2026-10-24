import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightIcon } from "lucide-react"
import { useEffect, useMemo } from "react"
import { useLocation, useNavigate } from "react-router"

import { StepHeader } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { languageName } from "@/features/contact/ContactDetails"
import { LANGS } from "@/features/settings/inboxes/InboxPage"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { MailboxOwnerChoice } from "@/components/common/MailboxOwnerChoice"
import { PurposeChoice } from "@/components/common/PurposeChoice"
import { useInboxDraft, useLeaveSetup } from "@/features/setup/draft"
import { ChatMock, DEFAULT_COLOR, PreviewCaption } from "@/features/setup/previews"
import { type SetupKind, type SetupState } from "@/features/setup/setup"
import { SetupLayout } from "@/features/setup/SetupLayout"
import { useSession } from "@/lib/session"
import { timeZones } from "@/lib/utils"

export function NameStep({ kind, mail: forMail }: { kind?: SetupKind; mail?: boolean }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as SetupState
  const [draft, patch] = useInboxDraft()
  const leave = useLeaveSetup()
  const { name, locale, zone } = draft
  const { canManage, membership } = useSession()
  const mail = draft.purpose === "mail"
  const pending = mail && draft.ownerMemberId === ""
  useEffect(() => {
    if (forMail || !canManage)
      patch({ purpose: "mail", ownerMemberId: draft.ownerMemberId ?? (canManage ? undefined : membership.member_id) })
  }, [forMail, canManage, membership.member_id, draft.ownerMemberId, patch])
  const zones = useMemo(() => timeZones(), [])
  const langs = LANGS.includes(locale) ? LANGS : [locale, ...LANGS]
  const next = mail ? "/setup/new/add/email" : kind ? `/setup/new/add/${kind}` : "/setup/new/pick"
  return (
    <SetupLayout
      step={1}
      onLeave={leave}
      preview={
        <>
          <ChatMock name={name} color={DEFAULT_COLOR} />
          <PreviewCaption>
            <Trans>Customers see this name in the chat, in your apps and on e-mails.</Trans>
          </PreviewCaption>
        </>
      }
    >
      <StepHeader title={<Trans>What should customers see?</Trans>}>
        <Trans>
          An inbox collects the conversations of one product or brand. Name it after the product your customers know.
        </Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim() && !pending) navigate(next, { state })
        }}
      >
        <Field label={<Trans>Product name</Trans>} htmlFor="setup-name">
          <Input
            id="setup-name"
            autoFocus
            required
            value={name}
            onChange={(e) => patch({ name: e.target.value })}
            maxLength={200}
            placeholder={t`For example Fieldnote`}
            className="h-12 rounded-xl px-3.5 text-title md:text-title"
            data-testid="setup-name"
          />
        </Field>
        {canManage && (
          <PurposeChoice
            value={draft.purpose ?? "support"}
            onChange={(purpose) => patch({ purpose, ownerMemberId: undefined })}
          />
        )}
        {mail && (
          <MailboxOwnerChoice
            key={draft.purpose}
            value={draft.ownerMemberId}
            onChange={(ownerMemberId) => patch({ ownerMemberId })}
          />
        )}
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={<Trans>Language</Trans>} htmlFor="setup-locale" hint={<Trans>For automatic messages</Trans>}>
            <ChoiceSelect
              id="setup-locale"
              label={t`Language`}
              value={locale}
              onChange={(v) => patch({ locale: v })}
              options={langs.map((l) => [l, languageName(l, i18n.locale)] as const)}
              className="h-10 w-full rounded-xl"
            />
          </Field>
          <Field label={<Trans>Time zone</Trans>} htmlFor="setup-zone" hint={<Trans>For business hours</Trans>}>
            <Input
              id="setup-zone"
              list="setup-zones"
              className="h-10 rounded-xl"
              value={zone}
              onChange={(e) => patch({ zone: e.target.value })}
            />
            <datalist id="setup-zones">
              {zones.map((z) => (
                <option key={z} value={z} />
              ))}
            </datalist>
          </Field>
        </div>
        <Button
          type="submit"
          size="lg"
          disabled={!name.trim() || pending}
          className="self-start"
          data-testid="setup-name-continue"
        >
          <Trans>Continue</Trans>
          <ArrowRightIcon className="rtl:rotate-180" />
        </Button>
      </form>
    </SetupLayout>
  )
}
