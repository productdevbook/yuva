import { Trans, useLingui } from "@lingui/react/macro"
import { MessageSquareIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Navigate, useSearchParams } from "react-router"

import { ErrorLine, Notice, toast, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import {
  useInboxSlack,
  useSetInboxSlack,
  useSlack,
  useSlackChannels,
  useSlackMutations,
} from "@/features/settings/slack/queries"
import { Card, ChoiceSelect, Field, PageHeader, Pill, Section } from "@/features/settings/ui"
import type { InboxSlackUpdate, SlackChannel, SlackConnectResult } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

const GUIDE = "https://github.com/productdevbook/yuva/blob/main/docs/slack.md"
const RESULTS: readonly SlackConnectResult[] = [
  "connected",
  "denied",
  "expired",
  "session_mismatch",
  "team_taken",
  "scope_missing",
  "failed",
]

function ResultBanner({ result }: { result: SlackConnectResult }) {
  if (result === "connected")
    return (
      <Notice data-testid="slack-result" data-result={result}>
        <Trans>Slack is connected.</Trans>
      </Notice>
    )
  return (
    <Notice tone="danger" data-testid="slack-result" data-result={result}>
      {result === "denied" && <Trans>The install was cancelled at Slack.</Trans>}
      {result === "expired" && <Trans>The install took longer than 10 minutes. Start again.</Trans>}
      {result === "session_mismatch" && (
        <Trans>The install was finished in another browser session. Start again in this one.</Trans>
      )}
      {result === "team_taken" && (
        <Trans>That Slack workspace is connected to another Yuva workspace. Disconnect it there first.</Trans>
      )}
      {result === "scope_missing" && (
        <Trans>
          Slack granted fewer permissions than Yuva needs. Check the app's scopes in the guide and try again.
        </Trans>
      )}
      {result === "failed" && <Trans>Slack refused the install or did not answer. Try again.</Trans>}
    </Notice>
  )
}

function InboxRow({ id, name, channels }: { id: string; name: string; channels: SlackChannel[] }) {
  const { t } = useLingui()
  const q = useInboxSlack(id, true)
  const set = useSetInboxSlack(id)
  const options = [
    ["", t`None`] as const,
    ...channels.map((c) => [c.id, c.private ? t`${c.name} (private)` : `#${c.name}`] as const),
  ]
  const s = q.data
  const save = (body: InboxSlackUpdate) =>
    s &&
    set.mutate({
      new_conversations_channel_id: s.new_conversations_channel_id ?? null,
      mentions_channel_id: s.mentions_channel_id ?? null,
      include_content: s.include_content,
      sla_channel_id: s.sla_channel_id ?? null,
      sla_warnings: s.sla_warnings,
      ...body,
    })
  return (
    <li className="flex flex-col gap-3 px-4 py-3.5" data-testid="slack-inbox" data-inbox-id={id}>
      <span className="truncate text-reading font-semibold">{name}</span>
      {q.isPending ? (
        <Skeleton className="h-9 w-full" />
      ) : s ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={<Trans>New conversations</Trans>} htmlFor={`slack-new-${id}`}>
            <ChoiceSelect
              id={`slack-new-${id}`}
              value={s.new_conversations_channel_id ?? ""}
              onChange={(v) => save({ new_conversations_channel_id: v || null })}
              options={options}
              className="w-full"
              label={t`Channel for new conversations in ${name}`}
              disabled={set.isPending}
            />
          </Field>
          <Field label={<Trans>Mentions</Trans>} htmlFor={`slack-mentions-${id}`}>
            <ChoiceSelect
              id={`slack-mentions-${id}`}
              value={s.mentions_channel_id ?? ""}
              onChange={(v) => save({ mentions_channel_id: v || null })}
              options={options}
              className="w-full"
              label={t`Channel for mentions in ${name}`}
              disabled={set.isPending}
            />
          </Field>
          <Field label={<Trans>SLA breaches</Trans>} htmlFor={`slack-sla-${id}`}>
            <ChoiceSelect
              id={`slack-sla-${id}`}
              value={s.sla_channel_id ?? ""}
              onChange={(v) => save({ sla_channel_id: v || null })}
              options={options}
              className="w-full"
              label={t`Channel for SLA breaches in ${name}`}
              disabled={set.isPending}
            />
          </Field>
          <label className="flex items-center gap-3 self-end pb-2">
            <Switch
              checked={s.sla_warnings}
              onCheckedChange={(on) => save({ sla_warnings: on })}
              disabled={set.isPending || !s.sla_channel_id}
              aria-label={t`Also post SLA warnings for ${name}`}
              data-testid="slack-sla-warnings"
            />
            <span className="text-body font-medium">
              <Trans>Also post warnings</Trans>
            </span>
          </label>
          <label className="flex items-start gap-3 sm:col-span-2">
            <Switch
              checked={s.include_content}
              onCheckedChange={(on) => save({ include_content: on })}
              disabled={set.isPending}
              aria-label={t`Include message text for ${name}`}
              data-testid="slack-include-content"
            />
            <span className="flex min-w-0 flex-col">
              <span className="text-body font-medium">
                <Trans>Include the start of the message</Trans>
              </span>
              <span className="text-small text-muted-foreground">
                <Trans>
                  Off, a post carries only the subject, the contact's name and a link. On, it also carries the first 300
                  characters of the message or note, which then leave Yuva for Slack.
                </Trans>
              </span>
            </span>
          </label>
        </div>
      ) : null}
      <ErrorLine error={q.error ?? set.error} className="text-small" />
    </li>
  )
}

function Inboxes() {
  const inboxes = useInboxes()
  const channels = useSlackChannels(true)
  return (
    <Section
      title={<Trans>Inboxes</Trans>}
      description={
        <Trans>
          Only channels the Yuva bot is a member of are listed. In Slack, invite it with /invite and the bot's name.
        </Trans>
      }
    >
      <Card flush>
        {inboxes.isPending || channels.isPending ? (
          <Skeleton className="h-32 w-full" />
        ) : (
          <ul className="divide-y">
            {(inboxes.data ?? []).map((i) => (
              <InboxRow key={i.id} id={i.id} name={i.name} channels={channels.data ?? []} />
            ))}
          </ul>
        )}
      </Card>
      <ErrorLine error={channels.error ?? inboxes.error} />
    </Section>
  )
}

export function SlackPage() {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const q = useSlack(canManage)
  const { connect, update, disconnect } = useSlackMutations()
  const [confirm, confirmDialog] = useConfirm()
  const [params, setParams] = useSearchParams()
  const [command, setCommand] = useState<string | null>(null)
  const result = RESULTS.find((r) => r === params.get("result"))
  const data = q.data
  const shown = command ?? data?.reply_command ?? ""
  useEffect(() => {
    if (result === "connected") toast(t`Slack connected`)
  }, [result, t])
  if (!canManage) return <Navigate to="/settings/profile" replace />
  const start = () => connect.mutate(undefined, { onSuccess: (r) => window.location.assign(r.authorize_url) })
  const failedAt = data?.last_error_at ? formatDateTime(data.last_error_at, i18n.locale) : ""
  const teamName = data?.team_name ?? ""
  return (
    <>
      <PageHeader
        title={<Trans>Slack</Trans>}
        description={
          <Trans>
            Post new conversations and mentions to a Slack channel, and answer from the thread. The{" "}
            <a href={GUIDE} target="_blank" rel="noreferrer" className="underline">
              guide
            </a>{" "}
            shows how to set up the Slack app.
          </Trans>
        }
      />
      {result && (
        <div className="flex flex-col gap-2">
          <ResultBanner result={result} />
          <Button variant="ghost" size="sm" className="self-start" onClick={() => setParams({}, { replace: true })}>
            <Trans>Dismiss</Trans>
          </Button>
        </div>
      )}
      <Section>
        {q.isPending ? (
          <Skeleton className="h-32 w-full rounded-xl" />
        ) : data && !data.available ? (
          <Notice icon={MessageSquareIcon} data-testid="slack-unavailable">
            <p>
              <Trans>This server has no Slack app set up, so Slack cannot be connected.</Trans>
            </p>
            <p>
              <Trans>
                The server's operator registers the Slack app and sets its credentials. The guide explains how.
              </Trans>{" "}
              <a href={GUIDE} target="_blank" rel="noreferrer" className="underline">
                <Trans>Open the guide</Trans>
              </a>
            </p>
          </Notice>
        ) : data && !data.connected ? (
          <Card data-testid="slack-disconnected">
            <p className="text-body text-muted-foreground">
              <Trans>
                Connect your Slack workspace. Yuva asks Slack for the permissions it needs, and the bot token is stored
                encrypted.
              </Trans>
            </p>
            <div>
              <Button onClick={start} disabled={connect.isPending} data-testid="slack-connect">
                <Trans>Connect Slack</Trans>
              </Button>
            </div>
            <ErrorLine error={connect.error} className="text-small" />
          </Card>
        ) : data ? (
          <Card data-testid="slack-connected">
            <div className="flex flex-wrap items-center gap-3">
              <span className="grid size-9 shrink-0 place-items-center rounded-lg border bg-surface text-muted-foreground">
                <MessageSquareIcon className="size-4" />
              </span>
              <div className="flex min-w-0 flex-1 basis-48 flex-col">
                <span className="truncate text-reading font-semibold">{teamName}</span>
                {data.connected_at && (
                  <span className="text-body text-muted-foreground">
                    <Trans>Connected {formatDateTime(data.connected_at, i18n.locale)}</Trans>
                  </span>
                )}
              </div>
              {data.last_error ? (
                <Pill tone="warning" title={data.last_error} data-testid="slack-error">
                  <Trans>Needs attention</Trans>
                </Pill>
              ) : (
                <Pill tone="success">
                  <Trans>Connected</Trans>
                </Pill>
              )}
              <Button
                variant="outline"
                size="sm"
                className="text-destructive"
                disabled={disconnect.isPending}
                data-testid="slack-disconnect"
                onClick={() =>
                  confirm({
                    title: <Trans>Disconnect {teamName}?</Trans>,
                    description: (
                      <Trans>
                        The bot token is revoked, and every Slack thread and every member's Slack link is deleted.
                        Replies in old threads no longer reach Yuva.
                      </Trans>
                    ),
                    confirm: <Trans>Disconnect</Trans>,
                    run: () => disconnect.mutate(undefined, { onSuccess: () => toast(t`Slack disconnected`) }),
                  })
                }
              >
                <Trans>Disconnect</Trans>
              </Button>
            </div>
            {data.last_error && (
              <p className="text-small text-destructive" role="alert">
                {data.last_error}
                {failedAt && ` · ${failedAt}`}
              </p>
            )}
            <ErrorLine error={disconnect.error} className="text-small" />
            <form
              className="flex flex-col gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                if (shown.trim() !== data.reply_command && !update.isPending) update.mutate(shown.trim())
              }}
            >
              <Field
                label={<Trans>Reply command</Trans>}
                htmlFor="slack-reply-command"
                hint={
                  <Trans>
                    A thread reply that starts with this goes to the customer. Any other reply becomes a note. An
                    exclamation mark and 1 to 20 lowercase letters.
                  </Trans>
                }
              >
                <div className="flex gap-2">
                  <Input
                    id="slack-reply-command"
                    value={shown}
                    onChange={(e) => setCommand(e.target.value)}
                    maxLength={21}
                    autoCapitalize="off"
                    spellCheck={false}
                    data-testid="slack-reply-command"
                  />
                  <Button
                    type="submit"
                    variant="outline"
                    disabled={!shown.trim() || shown.trim() === data.reply_command || update.isPending}
                  >
                    <Trans>Save</Trans>
                  </Button>
                </div>
              </Field>
              <ErrorLine error={update.error} className="text-small" />
            </form>
          </Card>
        ) : null}
        <ErrorLine error={q.error} />
      </Section>
      {data?.connected && <Inboxes />}
      {confirmDialog}
    </>
  )
}
