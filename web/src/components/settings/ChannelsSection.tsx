import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { CHANNEL_KINDS, useEnumText } from "@/components/common/text"
import { Field, Section } from "@/components/settings/SettingsLayout"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { api, unwrap, type Channel, type ChannelKind, type Inbox } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function parseSettings(raw: string): Record<string, unknown> | null {
  if (raw.trim() === "") return {}
  try {
    const v = JSON.parse(raw)
    return v && typeof v === "object" && !Array.isArray(v) ? v : null
  } catch {
    return null
  }
}

export function ChannelsSection({ inbox }: { inbox: Inbox }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const text = useEnumText()
  const [editing, setEditing] = useState<Channel | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const channels = useQuery({
    queryKey: keys.channels(ws, inbox.id),
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId: inbox.id } } })).then(
        (r) => r.items,
      ),
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/channels/{channelId}", { params: { path: { channelId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.channels(ws, inbox.id) }),
  })
  return (
    <Section
      title={<Trans>Channels</Trans>}
      description={<Trans>How messages reach this inbox. Settings are stored as given for now.</Trans>}
      action={
        canManage && (
          <Button variant="outline" size="sm" onClick={() => setEditing("new")}>
            <PlusIcon />
            <Trans>Add channel</Trans>
          </Button>
        )
      }
    >
      {channels.data && channels.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Trans>No channels yet.</Trans>
        </p>
      ) : (
        <ul className="flex flex-col divide-y rounded-lg border">
          {(channels.data ?? []).map((ch) => {
            const keyCount = Object.keys(ch.settings).length
            return (
              <li key={ch.id} className="flex items-center gap-3 px-3 py-2.5" data-testid="channel-row">
                <Badge variant="secondary">{text.channel[ch.kind]}</Badge>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{ch.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {keyCount === 0 ? (
                      <Trans>No settings</Trans>
                    ) : keyCount === 1 ? (
                      <Trans>1 setting</Trans>
                    ) : (
                      <Trans>{keyCount} settings</Trans>
                    )}
                  </p>
                </div>
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
              </li>
            )
          })}
        </ul>
      )}
      <ErrorLine error={channels.error ?? remove.error} />
      {editing && (
        <ChannelDialog
          inboxId={inbox.id}
          channel={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      )}
      {confirmDialog}
    </Section>
  )
}

function ChannelDialog({
  inboxId,
  channel,
  onClose,
}: {
  inboxId: string
  channel: Channel | null
  onClose: () => void
}) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const text = useEnumText()
  const [kind, setKind] = useState<ChannelKind>(channel?.kind ?? "email")
  const [name, setName] = useState(channel?.name ?? "")
  const [raw, setRaw] = useState(channel ? JSON.stringify(channel.settings, null, 2) : "{}")
  const settings = parseSettings(raw)
  const save = useMutation({
    mutationFn: () =>
      channel
        ? unwrap(
            api.PATCH("/v1/channels/{channelId}", {
              params: { path: { channelId: channel.id } },
              body: { name, settings: settings ?? {} },
            }),
          )
        : unwrap(
            api.POST("/v1/inboxes/{inboxId}/channels", {
              params: { path: { inboxId } },
              body: { kind, name, settings: settings ?? {} },
            }),
          ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inboxId) })
      onClose()
    },
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (settings) save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>{channel ? <Trans>Edit channel</Trans> : <Trans>Add channel</Trans>}</DialogTitle>
            <DialogDescription>
              <Trans>The kind cannot be changed later.</Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Kind</Trans>}>
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as ChannelKind)}
              items={text.channel}
              disabled={!!channel}
            >
              <SelectTrigger className="w-full" aria-label={t`Kind`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CHANNEL_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {text.channel[k]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label={<Trans>Name</Trans>} htmlFor="channel-name">
            <Input
              id="channel-name"
              required
              maxLength={200}
              value={name}
              placeholder={t`Support e-mail`}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field
            label={<Trans>Settings (JSON)</Trans>}
            htmlFor="channel-settings"
            hint={settings ? undefined : <span className="text-destructive"><Trans>This is not a JSON object.</Trans></span>}
          >
            <Textarea
              id="channel-settings"
              rows={6}
              spellCheck={false}
              className="font-mono text-xs"
              value={raw}
              aria-invalid={!settings}
              onChange={(e) => setRaw(e.target.value)}
            />
          </Field>
          <ErrorLine error={save.error} />
          <DialogFooter>
            <Button type="submit" disabled={save.isPending || !settings}>
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
