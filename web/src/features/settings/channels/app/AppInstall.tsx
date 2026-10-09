import { Trans } from "@lingui/react/macro"
import { useState } from "react"

import { CodeLine, Notice } from "@/components/common"
import { FormBlock } from "@/features/settings/channels/parts"
import { PublicKeyField } from "@/features/settings/channels/PublicKeyField"
import { Field } from "@/features/settings/ui"
import type { Channel } from "@/lib/api"

export function AppInstall({ channel }: { channel: Channel }) {
  const [publicKey, setPublicKey] = useState(channel.app!.public_key)
  return (
    <FormBlock title={<Trans>Install</Trans>} data-testid="app-install">
      <Field label={<Trans>Server URL</Trans>}>
        <CodeLine value={window.location.origin} />
      </Field>
      <PublicKeyField
        channel={channel}
        value={publicKey}
        onRotated={setPublicKey}
        hint={<Trans>Not a secret: it ships inside your app. Native apps send no origin, so the key is not tied to one.</Trans>}
        warning={
          <Trans>
            App builds with the old key can no longer start sessions until you ship one with the new key. People already
            signed in keep their session.
          </Trans>
        }
      />
      <Notice className="text-caption" data-testid="app-hint">
        <ul className="flex flex-col gap-1.5">
          <li>
            <Trans>
              <span className="font-medium text-foreground">iOS:</span> add the Yuva repository as a Swift package and
              use the <code className="font-mono">YuvaKit</code> library; create the client with the server URL and
              this key.
            </Trans>
          </li>
          <li>
            <Trans>
              <span className="font-medium text-foreground">Android:</span> add the library from{" "}
              <code className="font-mono">sdk/kotlin</code> in the Yuva repository and configure it with the same two
              values.
            </Trans>
          </li>
          <li>
            <Trans>
              For signed-in users, your backend signs an identity token with the inbox's identity secret and the app
              passes it to the SDK.
            </Trans>
          </li>
        </ul>
      </Notice>
    </FormBlock>
  )
}
