import { Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, toast, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Sec } from "@/features/settings/workspace/parts"
import { Card, Field, FormActions, FormCard } from "@/features/settings/ui"
import {
  api,
  unwrap,
  type AppChannelInput,
  type Channel,
  type ChannelAttachmentsInput,
  type ChatChannelInput,
  type EmailChannelInput,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export type ChannelPatch = {
  name?: string
  attachments?: ChannelAttachmentsInput
  settings?: Record<string, unknown>
  email?: EmailChannelInput
  chat?: ChatChannelInput
  app?: AppChannelInput
}

export function useLatestChannel(channel: Channel) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => qc.getQueryData<Channel>(keys.channel(ws, channel.id)) ?? channel
}

export function usePatchChannel(channel: Channel) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: ChannelPatch) =>
      unwrap(api.PATCH("/v1/channels/{channelId}", { params: { path: { channelId: channel.id } }, body })),
    onSuccess: (saved) => {
      qc.setQueryData(keys.channel(ws, saved.id), saved)
      void qc.invalidateQueries({ queryKey: keys.channels(ws, channel.inbox_id) })
    },
  })
}

export function PageSection({
  id,
  title,
  description,
  action,
  tone,
  children,
}: {
  id: string
  title: React.ReactNode
  description?: React.ReactNode
  action?: React.ReactNode
  tone?: "danger"
  children: React.ReactNode
}) {
  return (
    <div id={id} className="scroll-mt-4" data-testid={`section-${id}`}>
      <Sec id={`${id}-sec`} title={title} description={description} pill={action} tone={tone}>
        {children}
      </Sec>
    </div>
  )
}

export function NameSection({ channel }: { channel: Channel }) {
  const patch = usePatchChannel(channel)
  const [name, setName] = useState(channel.name)
  const dirty = name.trim() !== channel.name
  return (
    <PageSection id="general" title={<Trans>General</Trans>}>
      <FormCard
        onSubmit={() => patch.mutate({ name: name.trim() }, { onSuccess: (saved) => setName(saved.name) })}
        footer={
          <FormActions
            pending={patch.isPending}
            saved={patch.isSuccess && !dirty}
            error={patch.error}
            disabled={!dirty || !name.trim()}
          />
        }
      >
        <Field label={<Trans>Name</Trans>} htmlFor="channel-name">
          <Input
            id="channel-name"
            required
            maxLength={200}
            value={name}
            onChange={(e) => {
              patch.reset()
              setName(e.target.value)
            }}
            data-testid="channel-name"
          />
        </Field>
      </FormCard>
    </PageSection>
  )
}

function parseSettings(raw: string): Record<string, unknown> | null {
  if (raw.trim() === "") return {}
  try {
    const v = JSON.parse(raw)
    return v && typeof v === "object" && !Array.isArray(v) ? v : null
  } catch {
    return null
  }
}

export function ApiSettingsSection({ channel }: { channel: Channel }) {
  const patch = usePatchChannel(channel)
  const stored = JSON.stringify(channel.settings, null, 2)
  const [raw, setRaw] = useState(stored)
  const settings = parseSettings(raw)
  const dirty = raw !== stored
  return (
    <PageSection id="settings" title={<Trans>Settings</Trans>}>
      <FormCard
        onSubmit={() =>
          settings &&
          patch.mutate({ settings }, { onSuccess: (saved) => setRaw(JSON.stringify(saved.settings, null, 2)) })
        }
        footer={
          <FormActions
            pending={patch.isPending}
            saved={patch.isSuccess && !dirty}
            error={patch.error}
            disabled={!dirty || !settings}
          />
        }
      >
        <Field
          label={<Trans>Settings (JSON)</Trans>}
          htmlFor="channel-settings"
          error={settings ? undefined : <Trans>This is not a JSON object.</Trans>}
        >
          <Textarea
            id="channel-settings"
            rows={6}
            spellCheck={false}
            className="font-mono text-caption"
            value={raw}
            aria-invalid={!settings}
            onChange={(e) => {
              patch.reset()
              setRaw(e.target.value)
            }}
          />
        </Field>
      </FormCard>
    </PageSection>
  )
}

export function RemoveSection({ channel }: { channel: Channel }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/channels/{channelId}", { params: { path: { channelId: channel.id } } })),
    onSuccess: () => {
      toast(<Trans>Channel removed</Trans>)
      qc.removeQueries({ queryKey: keys.channel(ws, channel.id) })
      void qc.invalidateQueries({ queryKey: keys.channels(ws, channel.inbox_id) })
      navigate(`/settings/inboxes/${channel.inbox_id}`, { replace: true })
    },
  })
  return (
    <PageSection id="remove" title={<Trans>Remove</Trans>} tone="danger">
      <Card tone="danger">
        <div className="flex flex-wrap items-center gap-3">
          <p className="min-w-0 flex-1 text-body text-muted-foreground">
            <Trans>Conversations that started on this channel keep their messages.</Trans>
          </p>
          <Button
            variant="destructive"
            size="sm"
            disabled={remove.isPending}
            data-testid="channel-remove"
            onClick={() =>
              confirm({
                title: <Trans>Remove this channel?</Trans>,
                description: <Trans>Conversations that started on it keep their messages.</Trans>,
                confirm: <Trans>Remove</Trans>,
                run: () => remove.mutate(),
              })
            }
          >
            <Trans>Remove channel</Trans>
          </Button>
        </div>
        <ErrorLine error={remove.error} className="text-caption" />
      </Card>
      {confirmDialog}
    </PageSection>
  )
}
