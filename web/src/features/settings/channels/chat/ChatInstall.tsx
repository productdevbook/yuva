import { Trans } from "@lingui/react/macro"
import { useState } from "react"

import { CopyButton } from "@/components/common"
import { FormBlock } from "@/features/settings/channels/parts"
import { PublicKeyField } from "@/features/settings/channels/PublicKeyField"
import { Field } from "@/features/settings/ui"
import type { Channel } from "@/lib/api"

function embedSnippet(publicKey: string) {
  const origin = window.location.origin
  return `<script src="${origin}/yuva.js"></script>\n<yuva-chat channel="${publicKey}" server="${origin}"></yuva-chat>`
}

export function ChatInstall({ channel }: { channel: Channel }) {
  const [publicKey, setPublicKey] = useState(channel.chat!.public_key)
  const snippet = embedSnippet(publicKey)
  return (
    <FormBlock title={<Trans>Install</Trans>} data-testid="chat-embed">
      <PublicKeyField
        channel={channel}
        value={publicKey}
        onRotated={setPublicKey}
        hint={<Trans>Not a secret: it is part of your web pages. Rotate it to stop old embeds from starting new chats.</Trans>}
        warning={
          <Trans>
            Pages that embed the old key can no longer start chats until you update the snippet. Visitors already
            chatting keep their session.
          </Trans>
        }
      />
      <Field label={<Trans>Embed snippet</Trans>} hint={<Trans>Paste it before the closing body tag of every page that should show the chat.</Trans>}>
        <div className="flex flex-col gap-2">
          <pre className="max-w-full rounded-xl border bg-surface px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap" data-testid="chat-snippet">
            {snippet}
          </pre>
          <CopyButton value={snippet} className="self-start" />
        </div>
      </Field>
    </FormBlock>
  )
}
