import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { MemberSelect } from "@/components/common/MemberSelect"
import { RadioChoices } from "@/components/common/RadioChoices"
import { Field } from "@/features/settings/ui"
import { useSession } from "@/lib/session"
import { useMembers } from "@/lib/workspace"

type Kind = "team" | "me" | "member"

export function MailboxOwnerChoice({
  value,
  onChange,
}: {
  value: string | undefined
  onChange: (ownerMemberId: string | undefined) => void
}) {
  const { t } = useLingui()
  const { canManage, membership } = useSession()
  const me = membership.member_id
  const members = useMembers().data ?? []
  const [kind, setKind] = useState<Kind>(!canManage || value === me ? "me" : value === undefined ? "team" : "member")
  const [other, setOther] = useState(value && value !== me ? value : "")
  const person = members.find((m) => m.id === other)
  const who = person ? person.name || person.email : ""
  const items: { key: Kind; title: React.ReactNode; hint: React.ReactNode; run: () => void }[] = [
    ...(canManage
      ? [
          {
            key: "team" as const,
            title: <Trans>Shared with the team</Trans>,
            hint: <Trans>Everyone with access to the inbox reads it.</Trans>,
            run: () => onChange(undefined),
          },
        ]
      : []),
    {
      key: "me",
      title: <Trans>Personal (only you)</Trans>,
      hint: <Trans>Nobody else reads it unless you let them. Owners and admins see only that it exists.</Trans>,
      run: () => onChange(me),
    },
    ...(canManage && members.length > 1
      ? [
          {
            key: "member" as const,
            title: who ? <Trans>Personal for {who}</Trans> : <Trans>Personal for a member</Trans>,
            hint: <Trans>Only that member reads it. You see only that it exists.</Trans>,
            run: () => onChange(other),
          },
        ]
      : []),
  ]
  return (
    <Field label={<Trans>Who reads it?</Trans>}>
      <RadioChoices
        id="owner"
        value={kind}
        onChange={(k) => {
          setKind(k)
          items.find((it) => it.key === k)?.run()
        }}
        label={t`Who reads it?`}
        items={items.map((it) => ({ value: it.key, title: it.title, hint: it.hint, testId: `owner-${it.key}` }))}
      />
      {canManage && kind === "member" && (
        <MemberSelect
          value={other}
          onChange={(id) => {
            setOther(id)
            onChange(id)
          }}
          exclude={[me]}
          label={t`Member`}
          placeholder={t`Pick a member`}
          className="w-full"
          testId="owner-member"
        />
      )}
    </Field>
  )
}
