import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { MailIcon, Trash2Icon, UserPlusIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, PersonAvatar, useConfirm } from "@/components/common"
import { formatDateTime, ROLES, useEnumText } from "@/components/common/text"
import { Field, PageTitle, Section } from "@/components/settings/SettingsLayout"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { locales } from "@/i18n"
import { api, unwrap, type Locale, type Member, type Role } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useMembers } from "@/lib/queries"
import { useSession } from "@/lib/session"

function RoleSelect({
  value,
  onChange,
  disabled,
  allowOwner,
  label,
}: {
  value: Role
  onChange: (r: Role) => void
  disabled?: boolean
  allowOwner: boolean
  label: string
}) {
  const text = useEnumText()
  return (
    <Select value={value} onValueChange={(v) => onChange(v as Role)} items={text.role} disabled={disabled}>
      <SelectTrigger size="sm" className="w-32" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {ROLES.filter((r) => allowOwner || r !== "owner" || r === value).map((r) => (
          <SelectItem key={r} value={r} disabled={!allowOwner && r === "owner"}>
            {text.role[r]}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function MembersSettings() {
  return (
    <>
      <PageTitle>
        <Trans>Members</Trans>
      </PageTitle>
      <MemberList />
      <Invites />
    </>
  )
}

function MemberList() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, membership, canManage } = useSession()
  const members = useMembers()
  const [confirm, confirmDialog] = useConfirm()
  const isOwner = membership.role === "owner"
  const refresh = () => qc.invalidateQueries({ queryKey: keys.members(ws) })
  const setRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) =>
      unwrap(api.PATCH("/v1/members/{memberId}", { params: { path: { memberId: id } }, body: { role } })),
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/members/{memberId}", { params: { path: { memberId: id } } })),
    onSuccess: refresh,
  })
  const text = useEnumText()
  const editable = (m: Member) => canManage && (isOwner || m.role !== "owner")
  return (
    <Section
      title={<Trans>Workspace members</Trans>}
      description={<Trans>Owners and admins see every inbox; agents see the inboxes they were given.</Trans>}
    >
      <ul className="flex flex-col divide-y rounded-lg border">
        {(members.data ?? []).map((m) => {
          const name = m.name || m.email
          return (
            <li key={m.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5" data-testid="member-row">
              <PersonAvatar name={name} />
              <div className="min-w-40 flex-1">
                <p className="truncate text-sm font-medium">
                  {name}
                  {m.id === membership.member_id && (
                    <span className="font-normal text-muted-foreground">
                      {" "}
                      <Trans>(you)</Trans>
                    </span>
                  )}
                </p>
                <p className="truncate text-xs text-muted-foreground">{m.email}</p>
              </div>
              {editable(m) ? (
                <RoleSelect
                  value={m.role}
                  allowOwner={isOwner}
                  label={t`Role of ${name}`}
                  onChange={(role) => setRole.mutate({ id: m.id, role })}
                />
              ) : (
                <span className="w-32 text-sm text-muted-foreground">{text.role[m.role]}</span>
              )}
              {editable(m) && m.id !== membership.member_id ? (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Remove ${name}`}
                  onClick={() =>
                    confirm({
                      title: <Trans>Remove {name} from the workspace?</Trans>,
                      description: <Trans>Their conversations stay; they lose access at once.</Trans>,
                      confirm: <Trans>Remove</Trans>,
                      run: () => remove.mutate(m.id),
                    })
                  }
                >
                  <Trash2Icon />
                </Button>
              ) : (
                <span className="size-8" />
              )}
            </li>
          )
        })}
      </ul>
      <ErrorLine error={members.error ?? setRole.error ?? remove.error} />
      {confirmDialog}
    </Section>
  )
}

function Invites() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, membership, canManage } = useSession()
  const text = useEnumText()
  const [email, setEmail] = useState("")
  const [role, setRole] = useState<Role>("agent")
  const [locale, setLocale] = useState<Locale>(i18n.locale === "tr" ? "tr" : "en")
  const invites = useQuery({
    queryKey: keys.invites(ws),
    queryFn: () => unwrap(api.GET("/v1/invites")).then((r) => r.items),
  })
  const refresh = () => qc.invalidateQueries({ queryKey: keys.invites(ws) })
  const create = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/invites", { body: { email, role, locale } })),
    onSuccess: () => {
      setEmail("")
      void refresh()
    },
  })
  const withdraw = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/invites/{inviteId}", { params: { path: { inviteId: id } } })),
    onSuccess: refresh,
  })
  return (
    <Section
      title={<Trans>Invites</Trans>}
      description={<Trans>An invite is accepted when the person signs in with a code sent to that address. It expires after 7 days.</Trans>}
    >
      {canManage && (
        <form
          className="grid gap-3 sm:grid-cols-[1fr_auto_auto_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate()
          }}
        >
          <Field label={<Trans>E-mail address</Trans>} htmlFor="invite-email">
            <Input
              id="invite-email"
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={t`teammate@company.com`}
            />
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
          <Button type="submit" disabled={create.isPending}>
            <UserPlusIcon />
            <Trans>Invite</Trans>
          </Button>
        </form>
      )}
      <ErrorLine error={create.error ?? withdraw.error} />
      {invites.data && invites.data.length > 0 ? (
        <ul className="flex flex-col divide-y rounded-lg border">
          {invites.data.map((inv) => {
            const expires = formatDateTime(inv.expires_at, i18n.locale)
            return (
              <li key={inv.id} className="flex items-center gap-3 px-3 py-2.5" data-testid="invite-row">
                <MailIcon className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{inv.email}</p>
                  <p className="text-xs text-muted-foreground">
                    {text.role[inv.role]} · <Trans>expires {expires}</Trans>
                  </p>
                </div>
                {canManage && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => withdraw.mutate(inv.id)}
                    aria-label={t`Withdraw the invite for ${inv.email}`}
                  >
                    <Trans>Withdraw</Trans>
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      ) : (
        invites.data && (
          <p className="text-sm text-muted-foreground">
            <Trans>No pending invites.</Trans>
          </p>
        )
      )}
    </Section>
  )
}
