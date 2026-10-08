import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ChannelIcon, ErrorLine, useConfirm } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { ChannelDialog } from "@/features/settings/channels/ChannelDialog"
import { ChannelSummary } from "@/features/settings/channels/ChannelSummary"
import { useInboxOutlet } from "@/features/settings/inboxes/InboxLayout"
import { Card, EmptyRow, Row, Rows, RowText, Section } from "@/features/settings/ui"
import { api, unwrap, type Channel } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function ChannelsPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { inbox } = useInboxOutlet()
  const { workspaceId: ws, canManage } = useSession()
  const text = useEnumText()
  const [editing, setEditing] = useState<Channel | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const channels = useQuery({
    queryKey: keys.channels(ws, inbox.id),
    queryFn: () => unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: inbox.id } } })).then((r) => r.items),
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/channels/{channelId}", { params: { path: { channelId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.channels(ws, inbox.id) }),
  })
  return (
    <Section
      title={<Trans>Channels</Trans>}
      description={
        <Trans>
          How messages reach this inbox. E-mail channels receive and send mail, web chat channels power the chat widget,
          mobile app channels the iOS and Android SDKs, and API channels take feedback from your backend.
        </Trans>
      }
      action={
        canManage && (
          <Button variant="outline" size="sm" onClick={() => setEditing("new")}>
            <Trans>Add channel</Trans>
          </Button>
        )
      }
    >
      <Card flush>
        {channels.data && channels.data.length === 0 ? (
          <EmptyRow>
            <Trans>No channels yet.</Trans>
          </EmptyRow>
        ) : (
          <Rows>
            {(channels.data ?? []).map((ch) => (
              <Row key={ch.id} data-testid="channel-row">
                <span className="grid size-8 shrink-0 place-items-center rounded-full border text-muted-foreground" title={text.channel[ch.kind]}>
                  <ChannelIcon kind={ch.kind} className="size-4" />
                </span>
                <RowText title={ch.name} detail={<ChannelSummary ch={ch} />} />
                {canManage && (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={() => setEditing(ch)}>
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t`Remove`}
                      onClick={() =>
                        confirm({
                          title: <Trans>Remove this channel?</Trans>,
                          description: <Trans>Conversations that started on it keep their messages.</Trans>,
                          confirm: <Trans>Remove</Trans>,
                          run: () => remove.mutate(ch.id),
                        })
                      }
                    >
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </Row>
            ))}
          </Rows>
        )}
        <ErrorLine error={channels.error ?? remove.error} className="px-5 pb-4" />
      </Card>
      {editing && (
        <ChannelDialog
          key={editing === "new" ? "new" : editing.id}
          inboxId={inbox.id}
          channel={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onCreated={(ch) => setEditing(ch.kind === "chat" || ch.kind === "app" ? ch : null)}
        />
      )}
      {confirmDialog}
    </Section>
  )
}
