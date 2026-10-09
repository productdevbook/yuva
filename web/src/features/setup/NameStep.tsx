import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ArrowRightIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { languageName } from "@/features/contact/ContactDetails"
import { LANGS } from "@/features/settings/inboxes/InboxPage"
import { slugify } from "@/features/settings/inboxes/slugify"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { ChatMock, DEFAULT_COLOR, PreviewCaption } from "@/features/setup/previews"
import { browserLanguage, browserTimeZone, type SetupState } from "@/features/setup/setup"
import { SetupLayout, StepHeader } from "@/features/setup/SetupLayout"
import { api, ApiError, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function NameStep() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as SetupState
  const { workspaceId: ws } = useSession()
  const [name, setName] = useState("")
  const [locale, setLocale] = useState(browserLanguage)
  const [zone, setZone] = useState(browserTimeZone)
  const zones = useMemo(() => Intl.supportedValuesOf("timeZone"), [])
  const langs = LANGS.includes(locale) ? LANGS : [locale, ...LANGS]
  const create = useMutation({
    mutationFn: async () => {
      const base = slugify(name) || "inbox"
      const body = (slug: string) => ({
        name: name.trim(),
        slug,
        mode: "async" as const,
        ask_for_rating: false,
        default_locale: locale,
        timezone: zones.includes(zone) ? zone : browserTimeZone(),
      })
      try {
        return await unwrap(api.POST("/v1/inboxes", { body: body(base) }))
      } catch (err) {
        if (!(err instanceof ApiError) || err.status !== 409) throw err
        return unwrap(api.POST("/v1/inboxes", { body: body(`${base}-${Math.random().toString(36).slice(2, 6)}`) }))
      }
    },
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
      qc.setQueryData([...keys.inboxes(ws), r.inbox.id], r.inbox)
      navigate(`/setup/${r.inbox.id}`, { state: { ...state, secret: r.identity_secret } satisfies SetupState })
    },
  })
  return (
    <SetupLayout
      step={1}
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
        <Trans>An inbox collects the conversations of one product or brand. Name it after the product your customers know.</Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) create.mutate()
        }}
      >
        <Field label={<Trans>Product name</Trans>} htmlFor="setup-name">
          <Input
            id="setup-name"
            autoFocus
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={200}
            placeholder={t`For example Fieldnote`}
            className="h-12 rounded-xl px-3.5 text-title md:text-title"
            data-testid="setup-name"
          />
        </Field>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={<Trans>Language</Trans>} htmlFor="setup-locale" hint={<Trans>For automatic messages</Trans>}>
            <ChoiceSelect
              id="setup-locale"
              label={t`Language`}
              value={locale}
              onChange={setLocale}
              options={langs.map((l) => [l, languageName(l, i18n.locale)] as const)}
              className="h-10 w-full rounded-xl"
            />
          </Field>
          <Field label={<Trans>Time zone</Trans>} htmlFor="setup-zone" hint={<Trans>For business hours</Trans>}>
            <Input id="setup-zone" list="setup-zones" className="h-10 rounded-xl" value={zone} onChange={(e) => setZone(e.target.value)} />
            <datalist id="setup-zones">
              {zones.map((z) => (
                <option key={z} value={z} />
              ))}
            </datalist>
          </Field>
        </div>
        <ErrorLine error={create.error} />
        <Button type="submit" size="lg" disabled={!name.trim() || create.isPending} className="self-start" data-testid="setup-name-continue">
          <Trans>Continue</Trans>
          <ArrowRightIcon className="rtl:rotate-180" />
        </Button>
      </form>
    </SetupLayout>
  )
}
