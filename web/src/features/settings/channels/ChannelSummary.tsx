import { Trans } from "@lingui/react/macro"

import { useEnumText } from "@/components/common/text"
import type { Channel } from "@/lib/api"

export function ChannelSummary({ ch }: { ch: Channel }) {
  const text = useEnumText()
  if (ch.email) {
    return (
      <>
        {ch.email.address}
        {" · "}
        {ch.email.smtp ? <Trans>sends through {ch.email.smtp.host}</Trans> : <Trans>receives only, no SMTP account</Trans>}
      </>
    )
  }
  if (ch.chat) {
    const first = ch.chat.allowed_origins[0] ?? ""
    const more = ch.chat.allowed_origins.length - 1
    return (
      <>
        {first}
        {more > 0 && <> +{more}</>}
        {" · "}
        {ch.chat.allow_anonymous ? <Trans>anonymous visitors allowed</Trans> : <Trans>signed-in users only</Trans>}
      </>
    )
  }
  if (ch.app) {
    const platforms = ch.app.platforms.map((p) => text.platform[p]).join(", ")
    return (
      <>
        {platforms}
        {" · "}
        {ch.app.allow_anonymous ? <Trans>anonymous users allowed</Trans> : <Trans>signed-in users only</Trans>}
      </>
    )
  }
  const count = Object.keys(ch.settings).length
  return count === 0 ? <Trans>No settings</Trans> : count === 1 ? <Trans>1 setting</Trans> : <Trans>{count} settings</Trans>
}
