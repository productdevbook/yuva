import { Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { CopyButton, ErrorLine, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Field } from "@/features/settings/ui"
import { api, unwrap, type Channel } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function PublicKeyField({
  channel,
  value,
  onRotated,
  hint,
  warning,
}: {
  channel: Channel
  value: string
  onRotated: (key: string) => void
  hint: React.ReactNode
  warning: React.ReactNode
}) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const rotate = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/channels/{channelId}/public-key", { params: { path: { channelId: channel.id } } })),
    onSuccess: (data) => {
      const key = data.chat?.public_key ?? data.app?.public_key
      if (key) onRotated(key)
      void qc.invalidateQueries({ queryKey: keys.channels(ws, channel.inbox_id) })
      qc.setQueryData(keys.channel(ws, channel.id), data)
    },
  })
  return (
    <Field label={<Trans>Public key</Trans>} hint={hint}>
      <div className="flex min-w-0 items-center gap-2">
        <code className="min-w-0 flex-1 truncate rounded-xl border bg-surface px-3 py-2 font-mono text-xs" data-testid="channel-public-key">
          {value}
        </code>
        <CopyButton value={value} />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={rotate.isPending}
          data-testid="rotate-public-key"
          onClick={() => confirm({ title: <Trans>Rotate the public key?</Trans>, description: warning, confirm: <Trans>Rotate</Trans>, run: () => rotate.mutate() })}
        >
          <Trans>Rotate</Trans>
        </Button>
      </div>
      <ErrorLine error={rotate.error} className="text-xs" />
      {confirmDialog}
    </Field>
  )
}
