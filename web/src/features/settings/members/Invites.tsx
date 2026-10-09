import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { RoleSelect } from "@/features/settings/members/RoleSelect"
import { Card, Field, Row, Rows, RowText, Section } from "@/features/settings/ui"
import { locales } from "@/i18n"
import { api, unwrap, type Locale, type Role } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function InviteForm({ onDone }: { onDone: () => void }) {
  const { t, i18n } = useLingui()
  const { membership } = useSession()
  const [email, setEmail] = useState("")
  const [role, setRole] = useState<Role>("agent")
  const [locale, setLocale] = useState<Locale>(i18n.locale === "tr" ? "tr" : "en")
  const create = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/invites", { body: { email, role, locale } })),
    onSuccess: () => {
      setEmail("")
      onDone()
    },
  })
  return (
    <form
      className="flex w-full flex-col gap-3"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate()
      }}
    >
      <div className="grid gap-3 sm:grid-cols-[1fr_auto_auto_auto] sm:items-end">
        <Field label={<Trans>E-mail address</Trans>} htmlFor="invite-email">
          <Input id="invite-email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t`teammate@company.com`} />
        </Field>
        <Field label={<Trans>Role</Trans>}>
          <RoleSelect value={role} onChange={setRole} allowOwner={membership.role === "owner"} label={t`Role`} />
        </Field>
        <Field label={<Trans>Language</Trans>}>
          <Select value={locale} onValueChange={(v) => setLocale(v as Locale)} items={locales}>
            <SelectTrigger size="sm" className="w-28" aria-label={t`Invite language`}>
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
        <Button type="submit" size="sm" disabled={create.isPending}>
          <Trans>Invite</Trans>
        </Button>
      </div>
      <ErrorLine error={create.error} />
    </form>
  )
}

export function Invites() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const text = useEnumText()
  const { workspaceId: ws, canManage } = useSession()
  const invites = useQuery({ queryKey: keys.invites(ws), queryFn: () => unwrap(api.GET("/v1/invites")).then((r) => r.items) })
  const refresh = () => qc.invalidateQueries({ queryKey: keys.invites(ws) })
  const withdraw = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/invites/{inviteId}", { params: { path: { inviteId: id } } })),
    onSuccess: refresh,
  })
  return (
    <Section
      title={<Trans>Invites</Trans>}
      description={<Trans>An invite is accepted when the person signs in with a code sent to that address. It expires after 7 days.</Trans>}
    >
      <Card flush footer={canManage && <InviteForm onDone={() => void refresh()} />}>
        {invites.data && invites.data.length > 0 ? (
          <Rows>
            {invites.data.map((inv) => {
              const expires = formatDateTime(inv.expires_at, i18n.locale)
              return (
                <Row key={inv.id} data-testid="invite-row">
                  <RowText
                    title={inv.email}
                    detail={
                      <>
                        {text.role[inv.role]} · <Trans>expires {expires}</Trans>
                      </>
                    }
                  />
                  {canManage && (
                    <Button variant="ghost" size="sm" onClick={() => withdraw.mutate(inv.id)} aria-label={t`Withdraw the invite for ${inv.email}`}>
                      <Trans>Withdraw</Trans>
                    </Button>
                  )}
                </Row>
              )
            })}
          </Rows>
        ) : (
          invites.data && (
            <p className="px-5 py-4 text-body text-muted-foreground">
              <Trans>No pending invites.</Trans>
            </p>
          )
        )}
        <ErrorLine error={withdraw.error ?? invites.error} className="px-5 pb-4" />
      </Card>
    </Section>
  )
}
