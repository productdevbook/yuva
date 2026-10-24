import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { MemberSelect } from "@/components/common/MemberSelect"
import { Field } from "@/features/settings/ui"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
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
      <div className="flex flex-col gap-2" role="radiogroup" aria-label={t`Who reads it?`}>
        {items.map((it) => (
          <button
            key={it.key}
            type="button"
            role="radio"
            aria-checked={kind === it.key}
            onClick={() => {
              setKind(it.key)
              it.run()
            }}
            className={cn(
              "flex flex-col rounded-xl border px-3.5 py-2.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
              kind === it.key ? "border-foreground bg-surface" : "hover:border-faint"
            )}
            data-testid={`owner-${it.key}`}
          >
            <span className="font-medium">{it.title}</span>
            <span className="text-caption text-faint">{it.hint}</span>
          </button>
        ))}
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
      </div>
    </Field>
  )
}
