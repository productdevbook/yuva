import { Trans, useLingui } from "@lingui/react/macro"
import { CircleDotIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { Navigate } from "react-router"

import { CodeLine, ErrorLine, SecretDialog, toast, useConfirm } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { formatDateTime } from "@/components/common/text"
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
import { Skeleton } from "@/components/ui/skeleton"
import { PROVIDERS, providerName } from "@/features/issues/providers"
import { useIssueIntegrationMutations, useIssueIntegrations } from "@/features/issues/queries"
import { Card, ChoiceSelect, Field, PageHeader, Pill, Section } from "@/features/settings/ui"
import type { IssueIntegration, IssueProvider } from "@/lib/api"
import { useSession } from "@/lib/session"

type Reveal = { integration: IssueIntegration; secret: string }

function ConnectDialog({ onClose, onSecret }: { onClose: () => void; onSecret: (r: Reveal) => void }) {
  const { t } = useLingui()
  const { connect } = useIssueIntegrationMutations()
  const [provider, setProvider] = useState<IssueProvider>("github")
  const [name, setName] = useState("")
  const [token, setToken] = useState("")
  const [apiUrl, setApiUrl] = useState("")
  const [target, setTarget] = useState("")
  const github = provider === "github"
  const submit = () =>
    connect.mutate(
      {
        provider,
        token: token.trim(),
        name: name.trim() || undefined,
        api_url: github && apiUrl.trim() ? apiUrl.trim() : undefined,
        default_target: target.trim() || undefined,
      },
      {
        onSuccess: (r) => {
          toast(t`Connected ${providerName(provider)}`)
          onClose()
          if (r.webhook_secret) onSecret({ integration: r.integration, secret: r.webhook_secret })
        },
      }
    )
  return (
    <Dialog open onOpenChange={(o) => !o && !connect.isPending && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl" data-testid="issue-connect-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (token.trim() && !connect.isPending) submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Connect a tracker</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                Yuva checks the token with the tracker, stores it encrypted and never shows it again. It needs to read
                and write issues.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Tracker</Trans>} htmlFor="tracker-provider">
            <ChoiceSelect
              id="tracker-provider"
              value={provider}
              onChange={setProvider}
              options={PROVIDERS.map((p) => [p, providerName(p)] as const)}
              label={t`Tracker`}
            />
          </Field>
          <Field
            label={github ? <Trans>Fine-grained token</Trans> : <Trans>API key</Trans>}
            htmlFor="tracker-token"
            hint={
              github ? (
                <Trans>Limit it to the repositories issues go to, with read and write access to Issues.</Trans>
              ) : (
                <Trans>A personal API key from Linear, under Settings, Account, Security and access.</Trans>
              )
            }
          >
            <Input
              id="tracker-token"
              type="password"
              required
              autoFocus
              autoComplete="off"
              spellCheck={false}
              value={token}
              onChange={(e) => setToken(e.target.value)}
              data-testid="tracker-token"
            />
          </Field>
          {github && (
            <Field
              label={<Trans>GitHub Enterprise Server API</Trans>}
              htmlFor="tracker-api-url"
              hint={<Trans>Leave empty for github.com. Otherwise the server's API, https://host/api/v3.</Trans>}
            >
              <Input
                id="tracker-api-url"
                type="url"
                value={apiUrl}
                onChange={(e) => setApiUrl(e.target.value)}
                placeholder="https://github.example.com/api/v3"
                autoCapitalize="off"
                spellCheck={false}
                data-testid="tracker-api-url"
              />
            </Field>
          )}
          <Field
            label={<Trans>Default target</Trans>}
            htmlFor="tracker-target"
            hint={
              github ? (
                <Trans>Where new issues go when nobody picks a repository, as owner/repo.</Trans>
              ) : (
                <Trans>Where new issues go when nobody picks a team, as a team key such as ENG.</Trans>
              )
            }
          >
            <Input
              id="tracker-target"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder={github ? "owner/repo" : "ENG"}
              autoCapitalize="off"
              spellCheck={false}
              data-testid="tracker-target"
            />
          </Field>
          <Field label={<Trans>Name</Trans>} htmlFor="tracker-name" hint={<Trans>Shown to your team. Optional.</Trans>}>
            <Input id="tracker-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={100} />
          </Field>
          <ErrorLine error={connect.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!token.trim() || connect.isPending} data-testid="tracker-connect-submit">
              <Trans>Connect</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function SaveField({
  id,
  label,
  hint,
  value,
  onChange,
  onSave,
  saved,
  pending,
  error,
  password,
  placeholder,
  disabled,
  testId,
  action,
}: {
  id: string
  label: React.ReactNode
  hint?: React.ReactNode
  value: string
  onChange: (v: string) => void
  onSave: () => void
  saved?: boolean
  pending: boolean
  error: unknown
  password?: boolean
  placeholder?: string
  disabled: boolean
  testId: string
  action: React.ReactNode
}) {
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (!disabled && !pending) onSave()
      }}
    >
      <Field label={label} htmlFor={id} hint={hint}>
        <div className="flex gap-2">
          <Input
            id={id}
            type={password ? "password" : "text"}
            autoComplete="off"
            autoCapitalize="off"
            spellCheck={false}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder}
            data-testid={testId}
          />
          <Button type="submit" variant="outline" disabled={disabled || pending}>
            {action}
          </Button>
        </div>
      </Field>
      {saved && !error && (
        <span className="text-small text-muted-foreground" role="status">
          <Trans>Saved</Trans>
        </span>
      )}
      <ErrorLine error={error} className="text-small" />
    </form>
  )
}

function IntegrationCard({ i, onSecret }: { i: IssueIntegration; onSecret: (r: Reveal) => void }) {
  const { t, i18n } = useLingui()
  const { update, rotate, disconnect } = useIssueIntegrationMutations()
  const [confirm, confirmDialog] = useConfirm()
  const [target, setTarget] = useState(i.default_target ?? "")
  const [token, setToken] = useState("")
  const [secret, setSecret] = useState("")
  const github = i.provider === "github"
  const name = i.name
  const failedAt = i.last_error_at ? formatDateTime(i.last_error_at, i18n.locale) : ""
  return (
    <Card data-testid="tracker-card" data-provider={i.provider}>
      <div className="flex flex-wrap items-start gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-lg border bg-surface text-muted-foreground">
          <CircleDotIcon className="size-4" />
        </span>
        <div className="flex min-w-0 flex-1 basis-48 flex-col">
          <span className="truncate text-reading font-semibold">{i.name}</span>
          <span className="truncate text-body text-muted-foreground">
            {providerName(i.provider)} · {i.account}
            {i.api_url ? ` · ${i.api_url}` : ""}
          </span>
        </div>
        {i.last_error ? (
          <Pill tone="warning" title={i.last_error} data-testid="tracker-error">
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
          data-testid="tracker-disconnect"
          onClick={() =>
            confirm({
              title: <Trans>Disconnect {name}?</Trans>,
              description: (
                <Trans>
                  The token is removed and every link to its issues is deleted. Progress already set on feedback stays.
                </Trans>
              ),
              confirm: <Trans>Disconnect</Trans>,
              run: () => disconnect.mutate(i, { onSuccess: () => toast(t`Disconnected ${name}`) }),
            })
          }
        >
          <Trans>Disconnect</Trans>
        </Button>
      </div>
      {i.last_error && (
        <p className="text-small text-destructive" role="alert">
          {i.last_error}
          {failedAt && ` · ${failedAt}`}
        </p>
      )}
      <ErrorLine error={disconnect.error} className="text-small" />
      <SaveField
        id={`target-${i.id}`}
        label={<Trans>Default target</Trans>}
        hint={
          github ? (
            <Trans>Where new issues go when nobody picks a repository, as owner/repo.</Trans>
          ) : (
            <Trans>Where new issues go when nobody picks a team, as a team key such as ENG.</Trans>
          )
        }
        value={target}
        onChange={setTarget}
        placeholder={github ? "owner/repo" : "ENG"}
        pending={update.isPending}
        disabled={target.trim() === (i.default_target ?? "")}
        error={update.variables?.body.default_target !== undefined ? update.error : null}
        onSave={() => update.mutate({ id: i.id, body: { default_target: target.trim() || null } })}
        testId="tracker-default-target"
        action={<Trans>Save</Trans>}
      />
      <SaveField
        id={`token-${i.id}`}
        label={github ? <Trans>Replace the token</Trans> : <Trans>Replace the API key</Trans>}
        hint={<Trans>Checked with the tracker before it replaces the old one. The current one is never shown.</Trans>}
        value={token}
        onChange={setToken}
        password
        pending={update.isPending}
        disabled={!token.trim()}
        error={update.variables?.body.token !== undefined ? update.error : null}
        onSave={() => update.mutate({ id: i.id, body: { token: token.trim() } }, { onSuccess: () => setToken("") })}
        testId="tracker-new-token"
        action={<Trans>Replace</Trans>}
      />
      <div className="flex flex-col gap-2 rounded-xl border bg-surface p-3.5">
        <h3 className="text-body font-medium">
          <Trans>Webhook</Trans>
        </h3>
        <p className="text-small text-muted-foreground">
          {github ? (
            <Trans>
              So Yuva hears of a change at once. In the repository or organization, open Settings, Webhooks, Add
              webhook. Paste the address, choose application/json, paste the secret, and select the Issues event.
            </Trans>
          ) : (
            <Trans>
              So Yuva hears of a change at once. In Linear, open Settings, API, Webhooks, New webhook. Paste the
              address, select Issues, then paste the signing secret Linear shows below.
            </Trans>
          )}
        </p>
        <Field label={<Trans>Address</Trans>}>
          <CodeLine value={i.webhook_url} testId="tracker-webhook-url" />
        </Field>
        {github ? (
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-0 flex-1 text-small text-muted-foreground">
              {i.webhook_secret_set ? (
                <Trans>The secret is shown once, when it is made. Rotate it to get a new one.</Trans>
              ) : (
                <Trans>No secret yet. Make one, then paste it into GitHub.</Trans>
              )}
            </p>
            <Button
              variant="outline"
              size="sm"
              disabled={rotate.isPending}
              data-testid="tracker-rotate"
              onClick={() =>
                confirm({
                  title: <Trans>Make a new webhook secret?</Trans>,
                  description: (
                    <Trans>
                      You get a new secret once. Webhooks from GitHub are refused until you paste it into the webhook's
                      settings there.
                    </Trans>
                  ),
                  confirm: <Trans>Make a new secret</Trans>,
                  run: () =>
                    rotate.mutate(i, {
                      onSuccess: (r) =>
                        r.webhook_secret && onSecret({ integration: r.integration, secret: r.webhook_secret }),
                    }),
                })
              }
            >
              {i.webhook_secret_set ? <Trans>Rotate secret</Trans> : <Trans>Make a secret</Trans>}
            </Button>
          </div>
        ) : (
          <SaveField
            id={`secret-${i.id}`}
            label={<Trans>Linear signing secret</Trans>}
            hint={
              i.webhook_secret_set ? (
                <Trans>A secret is set. Paste a new one to replace it.</Trans>
              ) : (
                <Trans>Not set yet, so Linear webhooks are refused.</Trans>
              )
            }
            value={secret}
            onChange={setSecret}
            password
            pending={update.isPending}
            disabled={!secret.trim()}
            error={update.variables?.body.webhook_secret !== undefined ? update.error : null}
            onSave={() =>
              update.mutate({ id: i.id, body: { webhook_secret: secret.trim() } }, { onSuccess: () => setSecret("") })
            }
            testId="tracker-linear-secret"
            action={<Trans>Save</Trans>}
          />
        )}
        <ErrorLine error={rotate.error} className="text-small" />
      </div>
      {confirmDialog}
    </Card>
  )
}

export function IssueTrackersPage() {
  const { canManage } = useSession()
  const list = useIssueIntegrations(canManage)
  const [connecting, setConnecting] = useState(false)
  const [reveal, setReveal] = useState<Reveal | null>(null)
  if (!canManage) return <Navigate to="/settings/profile" replace />
  const items = list.data ?? []
  const revealName = reveal?.integration.name
  return (
    <>
      <PageHeader
        title={<Trans>Issue trackers</Trans>}
        description={
          <Trans>
            Turn feedback and roadmap ideas into GitHub or Linear issues. The issue's state then tells the customer
            where their feedback stands.
          </Trans>
        }
        action={
          <Button size="sm" onClick={() => setConnecting(true)} data-testid="tracker-connect">
            <PlusIcon />
            <Trans>Connect a tracker</Trans>
          </Button>
        }
      />
      <Section>
        {list.isPending ? (
          <Skeleton className="h-40 w-full rounded-xl" />
        ) : items.length === 0 ? (
          <EmptyBox icon={CircleDotIcon} title={<Trans>No tracker connected</Trans>} testId="tracker-empty">
            <Trans>Connect GitHub or Linear to create and link issues from feedback and ideas.</Trans>
          </EmptyBox>
        ) : (
          <div className="flex flex-col gap-4">
            {items.map((i) => (
              <IntegrationCard key={i.id} i={i} onSecret={setReveal} />
            ))}
          </div>
        )}
        <ErrorLine error={list.error} />
      </Section>
      {connecting && <ConnectDialog onClose={() => setConnecting(false)} onSecret={setReveal} />}
      <SecretDialog
        secret={reveal?.secret ?? null}
        title={<Trans>Webhook secret for {revealName}</Trans>}
        description={<Trans>Paste it into the webhook's settings at the tracker now. It is not shown again.</Trans>}
        onClose={() => setReveal(null)}
      />
    </>
  )
}
