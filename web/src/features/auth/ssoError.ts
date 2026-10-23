import { useLingui } from "@lingui/react/macro"

export function useSsoErrorText() {
  const { t } = useLingui()
  return (code: string | null): string | null => {
    switch (code) {
      case null:
        return null
      case "expired":
        return t`The sign-in took too long or was already used. Start again.`
      case "denied":
        return t`Your organisation's sign-in was cancelled or refused. Try again, or ask your administrator whether you may use Yuva.`
      case "invalid_token":
        return t`The identity provider's answer could not be verified. Try again, and tell your administrator if it keeps happening.`
      case "email_unverified":
        return t`Your identity provider has not verified your e-mail address, so Yuva cannot trust it. Ask your administrator to verify it there.`
      case "domain_not_claimed":
        return t`Your e-mail address is not in a domain this organisation has claimed in Yuva. Sign in with the address of your organisation.`
      case "group_not_allowed":
        return t`Your account is not in a group that may use Yuva. Ask your administrator to add you.`
      case "not_member":
        return t`You are not a member of this workspace yet. Ask an owner to invite you.`
      default:
        return t`Signing in through your organisation did not work. Try again, or use an e-mailed code.`
    }
  }
}
