import { Trans } from "@lingui/react/macro"
import { useMutation, useQuery } from "@tanstack/react-query"
import { ArrowUpRightIcon, BuildingIcon, ClockIcon, PlugIcon, RefreshCwIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Navigate, useLocation, useSearchParams } from "react-router"

import { CheckItem, CheckList, ErrorLine, Notice } from "@/components/common"
import { API_KEY_SCOPES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { AuthLayout } from "@/features/auth/AuthLayout"
import { activate, i18n } from "@/i18n"
import { api, ApiError, unwrap, type ApiKeyScope, type OAuthRequest } from "@/lib/api"
import { signInPath } from "@/lib/next"
import { useMe, useSignOut } from "@/lib/session"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"

const heading = "text-page"

function defaultScopes(req: OAuthRequest, offered: ApiKeyScope[]) {
  const asked = req.requested_scopes
  return new Set(asked ? offered.filter((s) => asked.includes(s)) : offered)
}

function Ended({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <AuthLayout>
      <span className="mb-5 grid size-11 place-items-center rounded-full border text-faint">
        <ClockIcon className="size-5" />
      </span>
      <h1 className={heading}>{title}</h1>
      <p className="mt-3 text-pretty text-muted-foreground" data-testid="consent-ended">
        {children}
      </p>
    </AuthLayout>
  )
}

function Expired() {
  return (
    <Ended title={<Trans>This request has ended</Trans>}>
      <Trans>This request expired or was already used. Start connecting again from the app.</Trans>
    </Ended>
  )
}

function Fact({ label, children, testId }: { label: React.ReactNode; children: React.ReactNode; testId?: string }) {
  return (
    <div className="flex flex-col gap-0.5 px-4 py-3" data-testid={testId}>
      <dt className="text-caption text-faint">{label}</dt>
      <dd className="text-body break-words">{children}</dd>
    </div>
  )
}

function Consent({ req, email }: { req: OAuthRequest; email: string }) {
  const text = useEnumText()
  const signOut = useSignOut()
  const [workspaceId, setWorkspaceId] = useState(req.workspaces[0]?.workspace_id ?? "")
  const workspace = req.workspaces.find((w) => w.workspace_id === workspaceId)
  const offered = API_KEY_SCOPES.filter((s) => workspace?.scopes.includes(s))
  const [scopes, setScopes] = useState(() => defaultScopes(req, offered))
  const chosen = offered.filter((s) => scopes.has(s))
  const unavailable = (req.requested_scopes ?? []).filter((s) => !offered.includes(s as ApiKeyScope))
  const clientName = req.client.name
  const host = req.redirect_host

  const go = (r: { redirect_url: string }) => window.location.assign(r.redirect_url)
  const approve = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/v1/oauth/requests/{oauthRequestId}/approve", {
          params: { path: { oauthRequestId: req.id } },
          body: { workspace_id: workspaceId, scopes: chosen },
        }),
      ),
    onSuccess: go,
  })
  const deny = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/oauth/requests/{oauthRequestId}/deny", { params: { path: { oauthRequestId: req.id } } })),
    onSuccess: go,
  })
  const error = approve.error ?? deny.error
  if (error instanceof ApiError && error.status === 404) return <Expired />
  const leaving = approve.isSuccess || deny.isSuccess
  const busy = approve.isPending || deny.isPending || leaving

  return (
    <AuthLayout wide>
      <span className="mb-5 grid size-11 place-items-center rounded-full bg-brand-wash text-brand">
        <PlugIcon className="size-5" />
      </span>
      <h1 className={heading} data-testid="consent-title">
        <Trans>Connect {clientName} to Yuva?</Trans>
      </h1>
      <p className="mt-3 text-pretty text-muted-foreground">
        <Trans>
          It will act as you, <span className="font-medium break-words text-foreground">{email}</span>, with only the access
          you allow below.
        </Trans>
      </p>

      <dl className="mt-6 divide-y rounded-2xl border">
        <Fact label={<Trans>App</Trans>} testId="consent-client">
          <span className="font-medium">{clientName}</span>
          <span className="block text-caption text-muted-foreground">
            <Trans>This is the name the app gives itself. Yuva has not verified it.</Trans>
          </span>
          {req.client.client_uri && (
            <a
              href={req.client.client_uri}
              target="_blank"
              rel="noreferrer noopener"
              className="mt-0.5 inline-flex items-center gap-1 text-caption text-brand underline-offset-4 hover:underline"
            >
              {req.client.client_uri}
              <ArrowUpRightIcon className="size-3" />
            </a>
          )}
        </Fact>
        <Fact label={<Trans>Returns you to</Trans>} testId="consent-redirect">
          <code className="font-mono text-small">{host}</code>
        </Fact>
        <Fact label={<Trans>Access</Trans>} testId="consent-resource">
          {req.resource === "mcp" ? (
            <Trans>MCP only: the assistant tools of this workspace.</Trans>
          ) : (
            <Trans>The full API: MCP, the REST API and live updates, like a member app.</Trans>
          )}
        </Fact>
      </dl>

      {req.workspaces.length === 0 ? (
        <Notice icon={BuildingIcon} className="mt-6">
          <Trans>You are not a member of any workspace, so there is nothing to connect.</Trans>
        </Notice>
      ) : (
        <div className="mt-6 flex flex-col gap-5">
          <div className="flex min-w-0 flex-col gap-2" role="radiogroup" aria-labelledby="consent-workspace">
            <span id="consent-workspace" className="text-body font-medium">
              <Trans>Workspace</Trans>
            </span>
            <RadioGroup
              aria-labelledby="consent-workspace"
              value={workspaceId}
              onValueChange={(v) => {
                const w = req.workspaces.find((x) => x.workspace_id === v)
                if (!w) return
                setWorkspaceId(w.workspace_id)
                setScopes(defaultScopes(req, API_KEY_SCOPES.filter((s) => w.scopes.includes(s))))
              }}
              className="gap-0 divide-y rounded-xl border"
            >
              {req.workspaces.map((w) => (
                <Label key={w.workspace_id} className="gap-3 px-3.5 py-2.5 font-normal" data-testid="consent-workspace">
                  <RadioGroupItem value={w.workspace_id} />
                  <span className="min-w-0 flex-1 truncate text-body">{w.name}</span>
                  <span className="shrink-0 text-caption text-faint">{text.role[w.role]}</span>
                </Label>
              ))}
            </RadioGroup>
          </div>

          {offered.length === 0 ? (
            <Notice tone="warning" data-testid="consent-no-scopes">
              <Trans>Your role in this workspace cannot give any of the access this app asks for.</Trans>
            </Notice>
          ) : (
            <CheckList
              labelId="consent-scopes"
              label={<Trans>Allow it to</Trans>}
              hint={
                <>
                  {chosen.length === 0 && (
                    <p role="alert" className="text-caption text-destructive">
                      <Trans>Pick at least one.</Trans>
                    </p>
                  )}
                  {unavailable.length > 0 && (
                    <p className="text-caption text-muted-foreground" data-testid="consent-unavailable">
                      <Trans>It also asked for access your role here cannot give:</Trans>{" "}
                      <span className="font-mono">{unavailable.join(", ")}</span>
                    </p>
                  )}
                </>
              }
            >
              {offered.map((s) => (
                <CheckItem
                  key={s}
                  checked={scopes.has(s)}
                  onChange={(on) =>
                    setScopes((x) => {
                      const next = new Set(x)
                      if (on) next.add(s)
                      else next.delete(s)
                      return next
                    })
                  }
                  testId="consent-scope"
                >
                  <span className="text-body">{text.scope[s]}</span>
                  <code className="font-mono text-caption text-faint">{s}</code>
                </CheckItem>
              ))}
            </CheckList>
          )}
        </div>
      )}

      <ErrorLine error={error} className="mt-5" />
      <div className="mt-6 flex flex-col gap-2.5 sm:flex-row-reverse">
        <Button
          size="lg" className="sm:flex-1"
          disabled={busy || !workspace || chosen.length === 0}
          onClick={() => approve.mutate()}
          data-testid="consent-approve"
        >
          <Trans>Allow</Trans>
        </Button>
        <Button variant="outline" size="lg" className="sm:flex-1" disabled={busy} onClick={() => deny.mutate()} data-testid="consent-deny">
          <Trans>Deny</Trans>
        </Button>
      </div>
      {leaving && (
        <p role="status" className="mt-4 text-body text-muted-foreground">
          <Trans>Returning you to {host}…</Trans>
        </p>
      )}
      <p className="mt-6 text-caption text-faint">
        <Trans>You can disconnect it at any time in Settings, under Connected apps.</Trans>{" "}
        <Button variant="link" className="text-muted-foreground underline hover:text-foreground" onClick={() => void signOut()}>
          <Trans>Use another account</Trans>
        </Button>
      </p>
    </AuthLayout>
  )
}

export function ConsentPage() {
  const location = useLocation()
  const [params] = useSearchParams()
  const id = params.get("request") ?? ""
  const me = useMe()
  const signedIn = !!me.data
  const request = useQuery({
    queryKey: ["oauth-request", id],
    queryFn: () => unwrap(api.GET("/v1/oauth/requests/{oauthRequestId}", { params: { path: { oauthRequestId: id } } })),
    enabled: signedIn && !!id,
    retry: false,
    staleTime: Infinity,
  })
  const personLocale = me.data?.person.locale
  useEffect(() => {
    if (personLocale && personLocale !== i18n.locale) activate(personLocale)
  }, [personLocale])

  if (me.isPending) return <div className="min-h-svh bg-background" />
  if (!me.data && !me.error) return <Navigate to={signInPath(location.pathname, location.search)} replace />
  if (!id) return <Expired />
  if (request.error instanceof ApiError && (request.error.status === 404 || request.error.status === 400)) return <Expired />
  if (me.error || request.error) {
    return (
      <Ended title={<Trans>Could not reach the server</Trans>}>
        <Button variant="outline" className="mt-2" onClick={() => (me.error ? me.refetch() : request.refetch())}>
          <RefreshCwIcon />
          <Trans>Try again</Trans>
        </Button>
      </Ended>
    )
  }
  if (!request.data || !me.data) return <div className="min-h-svh bg-background" />
  return <Consent key={request.data.id} req={request.data} email={me.data.person.email} />
}
