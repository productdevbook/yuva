import { useLingui } from "@lingui/react/macro"
import { CodeXmlIcon, MessageCircleIcon, MousePointer2Icon, PlugIcon, SparklesIcon, SquareTerminalIcon, TerminalIcon } from "lucide-react"

import type { ApiKeyScope, OAuthGrant } from "@/lib/api"

export type AssistantId = "claude" | "claude-code" | "codex" | "chatgpt" | "cursor" | "vscode" | "other"

export type Assistant = {
  id: AssistantId
  name: string
  icon: React.ComponentType<{ className?: string }>
  cloud: boolean
  docs: string
  matches: (g: OAuthGrant) => boolean
}

const named = (re: RegExp) => (g: OAuthGrant) => re.test(g.client.name)

export const ASSISTANTS: Assistant[] = [
  {
    id: "claude",
    name: "Claude",
    icon: SparklesIcon,
    cloud: true,
    docs: "https://claude.com/docs/connectors/custom/add-unlisted",
    matches: (g) => (/^claude\b/i.test(g.client.name) && !/code/i.test(g.client.name)) || /^https:\/\/(claude\.ai|[^/]*anthropic\.com)\//.test(g.client.client_id),
  },
  {
    id: "claude-code",
    name: "Claude Code",
    icon: TerminalIcon,
    cloud: false,
    docs: "https://code.claude.com/docs/en/mcp",
    matches: named(/claude[\s-]?code/i),
  },
  {
    id: "codex",
    name: "Codex",
    icon: SquareTerminalIcon,
    cloud: false,
    docs: "https://developers.openai.com/codex/mcp",
    matches: named(/codex/i),
  },
  {
    id: "chatgpt",
    name: "ChatGPT",
    icon: MessageCircleIcon,
    cloud: true,
    docs: "https://developers.openai.com/plugins/deploy/connect-chatgpt",
    matches: named(/chatgpt|openai/i),
  },
  {
    id: "cursor",
    name: "Cursor",
    icon: MousePointer2Icon,
    cloud: false,
    docs: "https://cursor.com/docs/context/mcp",
    matches: named(/cursor/i),
  },
  {
    id: "vscode",
    name: "VS Code",
    icon: CodeXmlIcon,
    cloud: false,
    docs: "https://code.visualstudio.com/docs/copilot/customization/mcp-servers",
    matches: named(/visual studio code|vs ?code/i),
  },
  {
    id: "other",
    name: "",
    icon: PlugIcon,
    cloud: false,
    docs: "https://github.com/productdevbook/yuva/blob/main/docs/mcp.md",
    matches: () => false,
  },
]

export const SUGGESTED_SCOPES: ApiKeyScope[] = [
  "conversations:read",
  "inboxes:read",
  "contacts:read",
  "messages:write",
  "notes:write",
  "conversations:write",
]

export const SERVER_NAME = "yuva"

export function mcpUrl() {
  return `${window.location.origin}/mcp`
}

export function reachableFromCloud() {
  const { protocol, hostname } = window.location
  if (protocol !== "https:") return false
  return !/^(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|\[?::1\]?$|[^.]+$)|\.(local|internal|lan|home\.arpa)$/i.test(hostname)
}

const json = (v: unknown) => JSON.stringify(v, null, 2)

export function setup(url: string) {
  return {
    claudeCode: {
      add: `claude mcp add --transport http ${SERVER_NAME} ${url}`,
      login: `claude mcp login ${SERVER_NAME}`,
    },
    codex: {
      add: `codex mcp add ${SERVER_NAME} --url ${url}`,
      login: `codex mcp login ${SERVER_NAME}`,
    },
    cursor: {
      install: `cursor://anysphere.cursor-deeplink/mcp/install?name=${SERVER_NAME}&config=${encodeURIComponent(btoa(JSON.stringify({ url })))}`,
      config: json({ mcpServers: { [SERVER_NAME]: { url } } }),
    },
    vscode: {
      install: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: SERVER_NAME, type: "http", url }))}`,
      command: `code --add-mcp '${JSON.stringify({ name: SERVER_NAME, type: "http", url })}'`,
      config: json({ servers: { [SERVER_NAME]: { type: "http", url } } }),
    },
    other: {
      config: json({ mcpServers: { [SERVER_NAME]: { type: "http", url } } }),
    },
  }
}

export const LINKS = {
  claudeConnectors: "https://claude.ai/customize/connectors",
  claudeOrgConnectors: "https://claude.ai/admin-settings/connectors",
  chatgptPlugins: "https://chatgpt.com/plugins",
}

export function grantsFor(a: Assistant, grants: OAuthGrant[], memberId: string, fresh: Set<string>) {
  const other = (g: OAuthGrant) => ASSISTANTS.some((o) => o.id !== a.id && o.matches(g))
  return grants.filter((g) => g.member_id === memberId && (a.matches(g) || (fresh.has(g.id) && !other(g))))
}

export function useAssistantName() {
  const { t } = useLingui()
  return (a: Assistant) => a.name || t`Other MCP client`
}
