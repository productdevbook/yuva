import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { KeyRoundIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Field, PageTitle, Section } from "@/components/settings/SettingsLayout"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { locales } from "@/i18n"
import { api, unwrap, type Locale } from "@/lib/api"
import { keys } from "@/lib/keys"
import { passkeysSupported, registerPasskey } from "@/lib/passkey"
import { meKey, useSession } from "@/lib/session"

export function ProfileSettings() {
  const { me } = useSession()
  return (
    <>
      <PageTitle>
        <Trans>My profile</Trans>
      </PageTitle>
      <ProfileForm key={me.person.locale} />
      <Passkeys />
    </>
  )
}

function ProfileForm() {
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
    <Section title={<Trans>Profile</Trans>} description={me.person.email}>
      <form
        className="grid gap-4 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <Field label={<Trans>Name</Trans>} htmlFor="name" hint={<Trans>Shown to your team and to contacts.</Trans>}>
          <Input id="name" value={name} maxLength={200} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field
          label={<Trans>Language</Trans>}
          hint={<Trans>The language of the panel and of sign-in and notification e-mails.</Trans>}
        >
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
        <div className="flex items-center gap-3 sm:col-span-2">
          <Button type="submit" disabled={!dirty || save.isPending}>
            <Trans>Save</Trans>
          </Button>
          {save.isSuccess && !dirty && (
            <span className="text-sm text-muted-foreground">
              <Trans>Saved</Trans>
            </span>
          )}
          <ErrorLine error={save.error} />
        </div>
      </form>
    </Section>
  )
}

function Passkeys() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const [name, setName] = useState("")
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({
    queryKey: keys.passkeys,
    queryFn: () => unwrap(api.GET("/v1/me/passkeys")).then((r) => r.items),
  })
  const add = useMutation({
    mutationFn: () => registerPasskey(name.trim()),
    onSuccess: () => {
      setName("")
      void qc.invalidateQueries({ queryKey: keys.passkeys })
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/me/passkeys/{passkeyId}", { params: { path: { passkeyId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.passkeys }),
  })
  const supported = passkeysSupported()
  return (
    <Section
      title={<Trans>Passkeys</Trans>}
      description={<Trans>Sign in with your device's fingerprint, face or screen lock instead of a code.</Trans>}
    >
      {list.data && list.data.length > 0 ? (
        <ul className="flex flex-col divide-y rounded-lg border">
          {list.data.map((p) => {
            const created = formatDateTime(p.created_at, i18n.locale)
            const used = p.last_used_at ? formatDateTime(p.last_used_at, i18n.locale) : null
            return (
              <li key={p.id} className="flex items-center gap-3 px-3 py-2.5">
                <KeyRoundIcon className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{p.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {used ? <Trans>Added {created}, last used {used}</Trans> : <Trans>Added {created}</Trans>}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Remove ${p.name}`}
                  onClick={() =>
                    confirm({
                      title: <Trans>Remove this passkey?</Trans>,
                      description: <Trans>You will no longer be able to sign in with it.</Trans>,
                      confirm: <Trans>Remove</Trans>,
                      run: () => remove.mutate(p.id),
                    })
                  }
                >
                  <Trash2Icon />
                </Button>
              </li>
            )
          })}
        </ul>
      ) : (
        list.data && (
          <p className="text-sm text-muted-foreground">
            <Trans>You have no passkeys yet.</Trans>
          </p>
        )
      )}
      {supported ? (
        <form
          className="flex flex-col gap-2 sm:flex-row"
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate()
          }}
        >
          <Input
            value={name}
            maxLength={200}
            onChange={(e) => setName(e.target.value)}
            placeholder={t`Name, e.g. Work laptop`}
            aria-label={t`Passkey name`}
            className="sm:max-w-64"
          />
          <Button type="submit" variant="outline" disabled={add.isPending}>
            <PlusIcon />
            <Trans>Add a passkey</Trans>
          </Button>
        </form>
      ) : (
        <p className="text-sm text-muted-foreground">
          <Trans>This browser does not support passkeys.</Trans>
        </p>
      )}
      {add.error && (
        <p role="alert" className="text-sm text-destructive">
          <Trans>The passkey was not added. Try again.</Trans>
        </p>
      )}
      <ErrorLine error={remove.error} />
      {confirmDialog}
    </Section>
  )
}
