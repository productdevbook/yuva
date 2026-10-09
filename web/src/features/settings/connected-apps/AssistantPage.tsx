import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { ArrowUpRightIcon, CircleCheckIcon, GlobeIcon, MessageSquareDashedIcon, SendIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, Navigate, useParams } from "react-router"

import { CodeBlock, CodeLine, ErrorLine, Notice } from "@/components/common"
import { formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  ASSISTANTS,
  grantsFor,
  LINKS,
  mcpUrl,
  reachableFromCloud,
  setup,
  SUGGESTED_SCOPES,
  type Assistant,
  type AssistantId,
  useAssistantName,
} from "@/features/settings/connected-apps/assistants"
import { useGrants } from "@/features/settings/connected-apps/queries"
import { Card, PageHeader, Row, Rows, Section } from "@/features/settings/ui"
import { api, unwrap, type OAuthGrant } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

const code = "rounded-md bg-surface px-1 py-px font-mono text-[0.92em] whitespace-nowrap"

function ExternalButton({ href, children, testId }: { href: string; children: React.ReactNode; testId?: string }) {
  const external = href.startsWith("http")
  return (
    <Button
      variant="outline"
      size="sm"
      className="self-start"
      render={<a href={href} {...(external ? { target: "_blank", rel: "noreferrer" } : {})} />}
      data-testid={testId}
    >
      {children}
      <ArrowUpRightIcon className="rtl:-scale-x-100" />
    </Button>
  )
}

function Steps({ children }: { children: React.ReactNode[] }) {
  return (
    <ol className="flex flex-col" data-testid="assistant-steps">
      {children.map((step, i) => (
        <li key={i} className="relative flex gap-3.5 pb-6 last:pb-0">
          {i < children.length - 1 && <span aria-hidden className="absolute start-[13px] top-8 bottom-1.5 w-px bg-border" />}
          <span className="grid size-[27px] shrink-0 place-items-center rounded-full border bg-card text-caption font-medium text-muted-foreground tabular-nums">
            {i + 1}
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-3 pt-[3px] text-body text-foreground">{step}</div>
        </li>
      ))}
    </ol>
  )
}

function Hint({ children }: { children: React.ReactNode }) {
  return <p className="text-small text-faint">{children}</p>
}

function useClientSteps(a: Assistant, url: string): React.ReactNode[] {
  const { canManage } = useSession()
  const s = setup(url)
  switch (a.id) {
    case "claude":
      return [
        <>
          <p>
            <Trans>
              In Claude, open <b className="font-medium">Customize › Connectors</b> and choose{" "}
              <b className="font-medium">Add custom connector</b>. It works for claude.ai and Claude Desktop alike.
            </Trans>
          </p>
          <ExternalButton href={LINKS.claudeConnectors} testId="assistant-install">
            <Trans>Open Claude connectors</Trans>
          </ExternalButton>
          <Hint>
            <Trans>
              On Team and Enterprise plans an owner adds it once in{" "}
              <a href={LINKS.claudeOrgConnectors} target="_blank" rel="noreferrer" className="underline underline-offset-2 hover:text-foreground">
                Organization settings › Connectors
              </a>
              ; members then find it under Connectors and click Connect.
            </Trans>
          </Hint>
        </>,
        <p>
          <Trans>
            Name it Yuva, paste the server URL and leave authentication on signing in. If Claude asks how to identify
            itself, keep its published identity. Click <b className="font-medium">Add</b>, then{" "}
            <b className="font-medium">Connect</b>.
          </Trans>
        </p>,
      ]
    case "claude-code":
      return [
        <>
          <p>
            <Trans>Add the server in a terminal:</Trans>
          </p>
          <CodeBlock value={s.claudeCode.add} testId="assistant-command" />
          <Hint>
            <Trans>
              It is added for you in the current project. Add <code className={code}>--scope user</code> for every
              project, or <code className={code}>--scope project</code> to share it in <code className={code}>.mcp.json</code>.
            </Trans>
          </Hint>
        </>,
        <>
          <p>
            <Trans>Sign in:</Trans>
          </p>
          <CodeBlock value={s.claudeCode.login} testId="assistant-login" />
          <Hint>
            <Trans>
              Or run <code className={code}>/mcp</code> inside a session. On a machine without a browser, add{" "}
              <code className={code}>--no-browser</code> and open the printed link yourself.
            </Trans>
          </Hint>
        </>,
      ]
    case "codex":
      return [
        <>
          <p>
            <Trans>Add the server in a terminal:</Trans>
          </p>
          <CodeBlock value={s.codex.add} testId="assistant-command" />
          <Hint>
            <Trans>
              It goes into <code className={code}>~/.codex/config.toml</code>, which the Codex CLI and IDE extension share.
            </Trans>
          </Hint>
        </>,
        <>
          <p>
            <Trans>Sign in:</Trans>
          </p>
          <CodeBlock value={s.codex.login} testId="assistant-login" />
        </>,
      ]
    case "chatgpt":
      return [
        <>
          <p>
            <Trans>
              In ChatGPT on the web, open <b className="font-medium">Plugins</b>, select the plus button, then{" "}
              <b className="font-medium">Add custom MCP server</b>.
            </Trans>
          </p>
          <ExternalButton href={LINKS.chatgptPlugins} testId="assistant-install">
            <Trans>Open ChatGPT plugins</Trans>
          </ExternalButton>
          <Hint>
            <Trans>Your plan and workspace policies decide whether custom MCP servers are available.</Trans>
          </Hint>
        </>,
        <p>
          <Trans>
            Name it Yuva. Under <b className="font-medium">Connection</b> choose{" "}
            <b className="font-medium">Public endpoint</b> and paste the server URL; choose OAuth for authentication.
          </Trans>
        </p>,
        <p>
          <Trans>
            Confirm the warning and select <b className="font-medium">Create as a plugin</b>. ChatGPT registers itself
            with this server.
          </Trans>
        </p>,
      ]
    case "cursor":
      return [
        <>
          <p>
            <Trans>Install the server in Cursor; it opens and asks you to confirm.</Trans>
          </p>
          <ExternalButton href={s.cursor.install} testId="assistant-install">
            <Trans>Add to Cursor</Trans>
          </ExternalButton>
          <Hint>
            <Trans>
              Or put this in <code className={code}>~/.cursor/mcp.json</code> for every project, or in{" "}
              <code className={code}>.cursor/mcp.json</code> for one:
            </Trans>
          </Hint>
          <CodeBlock value={s.cursor.config} testId="assistant-config" />
        </>,
        <p>
          <Trans>
            In Cursor's MCP settings, sign in to <b className="font-medium">yuva</b> when it asks.
          </Trans>
        </p>,
      ]
    case "vscode":
      return [
        <>
          <p>
            <Trans>Install the server in VS Code; it opens the server's page, where you select Install.</Trans>
          </p>
          <ExternalButton href={s.vscode.install} testId="assistant-install">
            <Trans>Install in VS Code</Trans>
          </ExternalButton>
          <Hint>
            <Trans>
              Or run the command below, or put the configuration in <code className={code}>.vscode/mcp.json</code> (or
              run <b className="font-medium">MCP: Add Server</b>):
            </Trans>
          </Hint>
          <CodeBlock value={s.vscode.command} testId="assistant-command" />
          <CodeBlock value={s.vscode.config} testId="assistant-config" />
        </>,
        <p>
          <Trans>Start the server. VS Code asks to sign in to Yuva and opens your browser.</Trans>
        </p>,
      ]
    case "other":
      return [
        <>
          <p>
            <Trans>
              Add a remote server with the Streamable HTTP transport and the server URL. Many clients take a
              configuration like this:
            </Trans>
          </p>
          <CodeBlock value={s.other.config} testId="assistant-config" />
          <Hint>
            <Trans>
              For a client that only starts local programs, use the <code className={code}>yuva mcp stdio</code> bridge
              from the setup guide.
            </Trans>
          </Hint>
        </>,
        <>
          <p>
            <Trans>
              Sign in from the client. It finds everything from the URL: OAuth 2.1 with PKCE, registering itself
              automatically or with a Client ID Metadata Document.
            </Trans>
          </p>
          <Hint>
            {canManage ? (
              <Trans>
                A client that cannot sign in can send an API key as{" "}
                <code className={code}>Authorization: Bearer</code>. Make one in{" "}
                <Link to="/settings/api-keys" className="underline underline-offset-2 hover:text-foreground">
                  API keys
                </Link>
                .
              </Trans>
            ) : (
              <Trans>
                A client that cannot sign in can send an API key as{" "}
                <code className={code}>Authorization: Bearer</code>; an owner or admin makes one.
              </Trans>
            )}
          </Hint>
        </>,
      ]
  }
}

function useConsentSteps(clientName: string): React.ReactNode[] {
  const { membership } = useSession()
  const host = window.location.host
  const workspace = membership.workspace.name
  return [
    <p>
      <Trans>
        Your browser opens the consent page on <b className="font-medium">{host}</b>. Sign in with a code or a passkey
        if you are not signed in already.
      </Trans>
    </p>,
    <p>
      <Trans>
        Pick the workspace <b className="font-medium">{workspace}</b>, check the scopes and allow. You go back to{" "}
        {clientName}, and the connection shows up here.
      </Trans>
    </p>,
  ]
}

function Status({ grants, clientName }: { grants: OAuthGrant[]; clientName: string }) {
  const { i18n } = useLingui()
  const latest = [...grants].sort((x, y) => y.created_at.localeCompare(x.created_at))[0]
  if (!latest) {
    return (
      <div className="flex items-center gap-3 rounded-2xl border border-dashed px-4 py-3.5" data-testid="assistant-status" data-connected="false">
        <span className="relative grid size-2 place-items-center">
          <span className="absolute size-2 animate-ping rounded-full bg-faint/40 motion-reduce:hidden" />
          <span className="size-1.5 rounded-full bg-faint" />
        </span>
        <p className="min-w-0 flex-1 text-body text-muted-foreground">
          <Trans>Not connected yet. This page updates when you finish signing in.</Trans>
        </p>
      </div>
    )
  }
  const when = formatDateTime(latest.created_at, i18n.locale)
  const name = latest.client.name
  return (
    <div className="flex items-center gap-3 rounded-2xl border border-success/30 bg-success/6 px-4 py-3.5" data-testid="assistant-status" data-connected="true">
      <CircleCheckIcon className="size-5 shrink-0 text-success" />
      <div className="min-w-0 flex-1">
        <p className="text-body font-medium">
          <Trans>{clientName} is connected</Trans>
        </p>
        <p className="truncate text-small text-muted-foreground">
          <Trans>
            As {name}, since {when}
          </Trans>
        </p>
      </div>
      <Button variant="ghost" size="sm" render={<Link to="/settings/connected-apps" />}>
        <Trans>Manage</Trans>
      </Button>
    </div>
  )
}

function SendingNote() {
  const { workspaceId: ws, canManage } = useSession()
  const workspace = useQuery({ queryKey: keys.workspace(ws), queryFn: () => unwrap(api.GET("/v1/workspace")) })
  if (!workspace.data) return null
  if (workspace.data.bots_may_send) {
    return (
      <Notice icon={SendIcon} data-testid="assistant-sending" data-bots-may-send="true">
        <p>
          <Trans>
            <b className="font-medium text-foreground">Bots may send</b> is on in this workspace, so an assistant with
            the scope to write messages can send replies itself. Without it, replies stay drafts.
          </Trans>
        </p>
      </Notice>
    )
  }
  return (
    <Notice icon={MessageSquareDashedIcon} data-testid="assistant-sending" data-bots-may-send="false">
      <p>
        <Trans>
          Replies the assistant writes are drafts: you or a teammate review them in the conversation and send them. It
          shows as you, via the assistant.
        </Trans>
      </p>
      <p className="text-small text-faint">
        {canManage ? (
          <Trans>
            To let assistants send on their own, turn on{" "}
            <Link to="/settings/api-keys" className="underline underline-offset-2 hover:text-foreground">
              Bots may send
            </Link>
            .
          </Trans>
        ) : (
          <Trans>An owner or admin can let assistants send on their own with Bots may send.</Trans>
        )}
      </p>
    </Notice>
  )
}

function useFreshGrants(grants: OAuthGrant[] | undefined) {
  const seen = useRef<Set<string> | null>(null)
  const [fresh, setFresh] = useState(() => new Set<string>())
  useEffect(() => {
    if (!grants) return
    if (!seen.current) {
      seen.current = new Set(grants.map((g) => g.id))
      return
    }
    const added = grants.filter((g) => !seen.current!.has(g.id)).map((g) => g.id)
    if (added.length) setFresh((f) => new Set([...f, ...added]))
  }, [grants])
  return fresh
}

export function AssistantPage() {
  const { t } = useLingui()
  const text = useEnumText()
  const nameOf = useAssistantName()
  const { assistant: id } = useParams<{ assistant: AssistantId }>()
  const a = ASSISTANTS.find((x) => x.id === id)
  const { membership } = useSession()
  const grants = useGrants()
  const fresh = useFreshGrants(grants.data)
  const url = mcpUrl()
  const steps = useClientSteps(a ?? ASSISTANTS[0], url)
  const name = a ? nameOf(a) : ""
  const consent = useConsentSteps(name)
  if (!a) return <Navigate to="/settings/connected-apps" replace />
  const mine = grantsFor(a, grants.data ?? [], membership.member_id, fresh)
  const Icon = a.icon
  const descriptions: Record<AssistantId, React.ReactNode> = {
    claude: <Trans>A custom connector, for claude.ai and Claude Desktop. It acts as you, with the access you allow.</Trans>,
    "claude-code": <Trans>Search, triage and draft replies from your terminal. It acts as you, with the access you allow.</Trans>,
    codex: <Trans>Search, triage and draft replies from Codex. It acts as you, with the access you allow.</Trans>,
    chatgpt: <Trans>A custom MCP server in ChatGPT on the web. It acts as you, with the access you allow.</Trans>,
    cursor: <Trans>Work with conversations from Cursor's agent. It acts as you, with the access you allow.</Trans>,
    vscode: <Trans>Work with conversations from agent mode in VS Code. It acts as you, with the access you allow.</Trans>,
    other: <Trans>Any client that speaks MCP over HTTP. It acts as you, with the access you allow.</Trans>,
  }
  return (
    <div className="flex flex-col gap-8" data-testid="assistant-page" data-assistant={a.id}>
      <PageHeader
        back={{ to: "/settings/connected-apps", label: t`Connected apps` }}
        eyebrow={<Trans>Connect an assistant</Trans>}
        title={
          <span className="flex items-center gap-3">
            <span className="grid size-10 shrink-0 place-items-center rounded-xl border bg-surface text-muted-foreground">
              <Icon className="size-5" />
            </span>
            {name}
          </span>
        }
        description={descriptions[a.id]}
      />
      <Status grants={mine} clientName={name} />
      <ErrorLine error={grants.error} />
      {a.cloud && !reachableFromCloud() && (
        <Notice tone="warning" icon={GlobeIcon} data-testid="assistant-unreachable">
          <p>
            <Trans>
              {name} connects from its vendor's cloud, so it needs this server at a public https address. The panel is
              open at {url}, which it probably cannot reach.
            </Trans>
          </p>
        </Notice>
      )}
      <Section title={<Trans>Server URL</Trans>}>
        <Card>
          <CodeLine value={url} testId="assistant-url" />
        </Card>
      </Section>
      <Section
        title={<Trans>Steps</Trans>}
        action={
          <a
            href={a.docs}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-small text-faint transition-colors hover:text-foreground"
            data-testid="assistant-docs"
          >
            {a.id === "other" ? <Trans>Setup guide</Trans> : <Trans>{name} docs</Trans>}
            <ArrowUpRightIcon className="size-3.5 rtl:-scale-x-100" />
          </a>
        }
      >
        <Card>
          <Steps>{[...steps, ...consent]}</Steps>
        </Card>
      </Section>
      <Section
        title={<Trans>Suggested scopes</Trans>}
        description={<Trans>The consent page ticks what the client asks for. Leave out what you do not want it to do; it never gets more than your role allows.</Trans>}
      >
        <Card flush>
          <Rows>
            {SUGGESTED_SCOPES.map((s) => (
              <Row key={s} className="min-h-0 py-2.5" data-testid="assistant-scope">
                <span className="min-w-0 flex-1 text-body">{text.scope[s]}</span>
                <code className="shrink-0 font-mono text-caption text-faint phone:hidden">{s}</code>
              </Row>
            ))}
          </Rows>
        </Card>
      </Section>
      <SendingNote />
      {mine.length === 0 && grants.data && grants.data.length > 0 && (
        <p className="mx-1 text-small text-faint">
          <Trans>
            Connected under another name? All connections are in{" "}
            <Link to="/settings/connected-apps" className="underline underline-offset-2 hover:text-foreground">
              Connected apps
            </Link>
            .
          </Trans>
        </p>
      )}
    </div>
  )
}
