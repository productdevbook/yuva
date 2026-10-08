import { Trans } from "@lingui/react/macro"

import { Invites } from "@/features/settings/members/Invites"
import { MemberList } from "@/features/settings/members/MemberList"
import { PageHeader } from "@/features/settings/ui"

export function MembersPage() {
  return (
    <>
      <PageHeader
        title={<Trans>Members</Trans>}
        description={<Trans>Owners and admins see every inbox; agents see the inboxes they were given.</Trans>}
      />
      <MemberList />
      <Invites />
    </>
  )
}
