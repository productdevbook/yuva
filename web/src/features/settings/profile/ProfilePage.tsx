import { Trans } from "@lingui/react/macro"
import { useNavigate } from "react-router"

import { DeleteAccountButton } from "@/features/auth/DeleteAccountButton"
import { Passkeys } from "@/features/settings/profile/Passkeys"
import { ProfileForm } from "@/features/settings/profile/ProfileForm"
import { Card, PageHeader, Rows, Section, SettingRow } from "@/features/settings/ui"
import { useSession, useSignOut } from "@/lib/session"
import { Item } from "@/components/ui/item"

export function ProfilePage() {
  const { me } = useSession()
  const navigate = useNavigate()
  const signOut = useSignOut()
  return (
    <>
      <PageHeader title={<Trans>Profile</Trans>} />
      <ProfileForm key={me.person.id} />
      <Passkeys />
      <Section>
        <Card flush>
          <Rows>
            <li>
              <Item render={<button type="button" />} className="min-h-[52px] rounded-none text-destructive [button]:hover:bg-background"
                onClick={async () => {
                  await signOut()
                  navigate("/sign-in", { replace: true })
                }}
              >
                <Trans>Sign out</Trans>
              </Item>
            </li>
            <SettingRow
              title={<Trans>Delete my account</Trans>}
              hint={<Trans>Removes you from every workspace. Messages you wrote stay, shown as from a deleted member.</Trans>}
            >
              <DeleteAccountButton email={me.person.email} size="sm" />
            </SettingRow>
          </Rows>
        </Card>
      </Section>
    </>
  )
}
