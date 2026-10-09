import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { Link, Navigate, useLocation, useNavigate, useParams } from "react-router"

import { ChannelIcon, CodeLine, ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { AppInstall } from "@/features/settings/channels/app/AppInstall"
import { ChannelDialog } from "@/features/settings/channels/ChannelDialog"
import { ChatInstall } from "@/features/settings/channels/chat/ChatInstall"
import { useInbox } from "@/features/settings/inboxes/queries"
import { slugify } from "@/features/settings/inboxes/slugify"
import { PageHeader, RowIcon, Section } from "@/features/settings/ui"
import { api, ApiError, unwrap, type ChannelKind } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { Input } from "@/components/ui/input"
import { Item } from "@/components/ui/item"

type FlowState = { secret?: string }

function Steps({ n }: { n: number }) {
  return <Trans>{n} of 3</Trans>
}

function NameStep() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { workspaceId: ws } = useSession()
  const [name, setName] = useState("")
  const create = useMutation({
    mutationFn: async () => {
      const base = slugify(name) || "inbox"
      const body = (slug: string) => ({ name: name.trim(), slug, mode: "async" as const, ask_for_rating: false, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone })
      try {
        return await unwrap(api.POST("/v1/inboxes", { body: body(base) }))
      } catch (err) {
        if (!(err instanceof ApiError) || err.status !== 409) throw err
        return unwrap(api.POST("/v1/inboxes", { body: body(`${base}-${Math.random().toString(36).slice(2, 6)}`) }))
      }
    },
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
      navigate(`/settings/new/${r.inbox.id}`, { state: { secret: r.identity_secret } satisfies FlowState })
    },
  })
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        if (name.trim()) create.mutate()
      }}
    >
      <PageHeader eyebrow={<Steps n={1} />} title={<Trans>What is the product called?</Trans>} description={<Trans>Your customers will see this name.</Trans>} />
      <Input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        maxLength={200}
        placeholder={t`For example Fieldnote`}
        aria-label={t`Product name`}
        className="mt-6 h-auto rounded-[14px] bg-card px-4 py-3.5 text-title md:text-title"
        data-testid="new-inbox-name"
      />
      <ErrorLine error={create.error} className="mt-3" />
      <div className="mt-5 flex justify-end">
        <Button type="submit" disabled={!name.trim() || create.isPending}>
          <Trans>Continue</Trans>
        </Button>
      </div>
    </form>
  )
}

function ChannelStep({ inboxId }: { inboxId: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as FlowState
  const inbox = useInbox(inboxId).data
  const [kind, setKind] = useState<ChannelKind | null>(null)
  const name = inbox?.name ?? "…"
  const picks: [ChannelKind, string, string][] = [
    ["email", t`E-mail`, t`Mail sent to your support address`],
    ["chat", t`Website chat`, t`One line on your site, a chat bubble in the corner`],
    ["app", t`Mobile app`, t`The iOS and Android SDKs`],
  ]
  return (
    <>
      <PageHeader
        title={<Trans>Where will {name} customers write from?</Trans>}
        description={<Trans>Pick one; you can add the others later.</Trans>}
        eyebrow={<Steps n={2} />}
      />
      <div className="grid gap-2.5">
        {picks.map(([k, title, hint]) => (
          <Item render={<button type="button" />} variant="outline" className="bg-card hover:border-brand"
            key={k}
            onClick={() => setKind(k)}
            data-testid={`pick-${k}`}
          >
            <RowIcon>
              <ChannelIcon kind={k} />
            </RowIcon>
            <span>
              <b className="block font-medium">{title}</b>
              <small className="text-small text-faint">{hint}</small>
            </span>
          </Item>
        ))}
      </div>
      <p className="text-center text-body">
        <Link to={`/settings/inboxes/${inboxId}`} className="text-faint hover:text-foreground">
          <Trans>Skip for now</Trans>
        </Link>
      </p>
      {kind && (
        <ChannelDialog
          inboxId={inboxId}
          channel={null}
          kind={kind}
          onClose={() => setKind(null)}
          onCreated={(ch) => navigate(`/settings/new/${inboxId}/${ch.id}`, { state })}
        />
      )}
    </>
  )
}

function DoneStep({ inboxId, channelId }: { inboxId: string; channelId: string }) {
  const { t } = useLingui()
  const state = (useLocation().state ?? {}) as FlowState
  const inbox = useInbox(inboxId).data
  const channel = useQuery({
    queryKey: keys.channel(useSession().workspaceId, channelId),
    queryFn: () => unwrap(api.GET("/v1/channels/{channelId}", { params: { path: { channelId } } })),
  }).data
  const name = inbox?.name ?? "…"
  return (
    <>
      <PageHeader eyebrow={<Steps n={3} />} title={<Trans>Ready.</Trans>} description={<Trans>{name} can now receive messages. One last step:</Trans>} back={false} />
      {channel?.chat && <ChatInstall channel={channel} />}
      {channel?.app && <AppInstall channel={channel} />}
      {channel?.email && (
        <Section title={<Trans>Forward your mail</Trans>}>
          <CodeLine value={channel.email.address} />
          <p className="mx-1 text-small text-faint">
            <Trans>
              Route mail for this address to Yuva's e-mail ingress, for example with the Cloudflare Email Worker in edge/.
              Replies go out through the SMTP account you entered.
            </Trans>
          </p>
        </Section>
      )}
      {state.secret && (
        <Section
          title={<Trans>Identity secret, shown only now</Trans>}
          description={<Trans>Your backend signs identity tokens with it so Yuva knows who your signed-in users are. You can rotate it in the inbox settings.</Trans>}
        >
          <CodeLine value={state.secret} />
        </Section>
      )}
      <div className="flex justify-end">
        <Button render={<Link to={`/settings/inboxes/${inboxId}`} />} aria-label={t`Go to the inbox`}>
          <Trans>Go to the inbox</Trans>
        </Button>
      </div>
    </>
  )
}

export function NewInboxPage() {
  const { canManage } = useSession()
  const { inboxId, channelId } = useParams()
  if (!canManage) return <Navigate to="/settings" replace />
  if (inboxId && channelId) return <DoneStep inboxId={inboxId} channelId={channelId} />
  if (inboxId) return <ChannelStep inboxId={inboxId} />
  return <NameStep />
}
