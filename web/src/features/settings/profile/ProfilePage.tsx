import { Trans } from "@lingui/react/macro"

import { DeleteAccountButton } from "@/features/auth/DeleteAccountButton"
import { Passkeys } from "@/features/settings/profile/Passkeys"
import { ProfileForm } from "@/features/settings/profile/ProfileForm"
import { Card, PageHeader, Section } from "@/features/settings/ui"
import { useSession } from "@/lib/session"

export function ProfilePage() {
  const { me } = useSession()
  return (
    <>
      <PageHeader title={<Trans>My profile</Trans>} description={me.person.email} />
      <ProfileForm key={me.person.locale} />
      <Passkeys />
      <Section
        title={<Trans>Delete my account</Trans>}
        description={
          <Trans>
            Removes you from every workspace and deletes your account. Messages you wrote stay, shown as from a
            deleted member.
          </Trans>
        }
      >
        <Card tone="danger">
          <div>
            <DeleteAccountButton email={me.person.email} size="sm" />
          </div>
        </Card>
      </Section>
    </>
  )
}
