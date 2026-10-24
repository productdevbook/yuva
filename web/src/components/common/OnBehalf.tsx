import { useLingui } from "@lingui/react/macro"
import { UserRoundIcon } from "lucide-react"

import { useAllInboxes, useMemberMap } from "@/lib/workspace"

export function OnBehalfNote({ inboxId }: { inboxId: string | undefined }) {
  const { t } = useLingui()
  const inbox = useAllInboxes().data?.find((i) => i.id === inboxId)
  const members = useMemberMap()
  if (inbox?.mailbox?.access !== "send" || !inbox.owner_member_id) return null
  const owner = members.get(inbox.owner_member_id)
  const name = owner ? owner.name || owner.email : t`the owner`
  return (
    <p className="mx-3 mb-1.5 flex items-center gap-1.5 text-caption text-faint" data-testid="on-behalf-note">
      <UserRoundIcon className="size-3.5 shrink-0" />
      {t`Sending on behalf of ${name}`}
    </p>
  )
}
