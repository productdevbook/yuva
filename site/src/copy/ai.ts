import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"

export type AssistantStep = { text: string; code?: string; lang?: string }
export type Assistant = { id: string; name: string; steps: AssistantStep[]; say?: string; note?: string }

export const SERVER_TOKEN = "{server}"

export function aiCopy(i18n: I18n) {
  const signIn = i18n._(msg`The first time the assistant uses Yuva, your browser opens Yuva to sign in.`)
  return {
    meta: {
      title: i18n._(msg`Yuva in your AI assistant`),
      description: i18n._(
        msg`Connect Yuva to Claude, ChatGPT, Cursor, VS Code or Gemini CLI, then let the assistant look at your project and set Yuva up with you.`
      ),
    },
    kicker: i18n._(msg`Yuva and your AI assistant`),
    title: i18n._(msg`Tell your assistant to add Yuva.`),
    titleAccent: i18n._(msg`It does the rest with you.`),
    lead: i18n._(
      msg`Yuva has a built-in MCP server. Connect it once, and your assistant reads your project, sees what your workspace already has, and sets up live chat, analytics or newsletters, asking before each change.`
    ),
    server: {
      label: i18n._(msg`Your Yuva server`),
      hosted: i18n._(msg`Yuva cloud`),
      own: i18n._(msg`My own server`),
      placeholder: "https://support.example.com",
      hint: i18n._(msg`The commands below follow what you type.`),
    },
    pick: i18n._(msg`Pick your assistant`),
    connect: i18n._(msg`Connect Yuva`),
    say: i18n._(msg`Say this`),
    sayLead: i18n._(
      msg`Open the assistant in your project's folder. It reads your code on your machine and sends none of it to Yuva.`
    ),
    sentence: i18n._(
      msg`Look at this project and tell me how to add Yuva. Use Yuva's init prompt, recommend a plan, and change nothing until I pick the steps.`
    ),
    orSentence: i18n._(msg`Or type the sentence:`),
    copy: i18n._(msg`Copy`),
    copied: i18n._(msg`Copied`),
    moreTitle: i18n._(msg`Or ask for one thing`),
    more: [
      i18n._(msg`Add Yuva live chat to this site.`),
      i18n._(msg`Set up Yuva analytics for this site, with a goal for sign-ups.`),
      i18n._(msg`Move our support mailbox into Yuva.`),
    ],
    flowTitle: i18n._(msg`What happens when you sign in`),
    flow: [
      i18n._(msg`Your browser opens Yuva. Sign in with a one-time code; a new e-mail address gets a new account.`),
      i18n._(msg`New to Yuva? Name your workspace. It takes a few seconds.`),
      i18n._(msg`Choose the workspace and what the assistant may do, then approve.`),
      i18n._(msg`Back in the assistant, Yuva is connected. Settings → Connected apps shows it and revokes it.`),
    ],
    safetyTitle: i18n._(msg`You stay in charge`),
    safety: [
      i18n._(
        msg`The assistant acts as you, with no more access than you approve, and asks before it changes anything.`
      ),
      i18n._(msg`Your code stays on your machine. Yuva only sees what the assistant creates through it.`),
      i18n._(
        msg`If Yuva itself misbehaves, the assistant can prepare a GitHub issue with your names, addresses and keys removed. You read it before anything is opened.`
      ),
    ],
    guide: i18n._(msg`Read the MCP guide`),
    assistants: [
      {
        id: "claude-code",
        name: "Claude Code",
        steps: [
          {
            text: i18n._(msg`Add Yuva in your terminal:`),
            code: `claude mcp add --transport http yuva ${SERVER_TOKEN}/mcp`,
          },
          { text: i18n._(msg`Sign in. Your browser opens Yuva:`), code: "claude mcp login yuva" },
        ],
        say: "/mcp__yuva__init",
      },
      {
        id: "claude",
        name: "Claude",
        steps: [
          { text: i18n._(msg`In claude.ai or Claude Desktop, open Customize → Connectors → Add custom connector.`) },
          { text: i18n._(msg`Enter this address and sign in:`), code: `${SERVER_TOKEN}/mcp` },
        ],
        note: i18n._(msg`Needs a public https address.`),
      },
      {
        id: "chatgpt",
        name: "ChatGPT",
        steps: [
          { text: i18n._(msg`Open Plugins, select the plus button, then Add custom MCP server.`) },
          { text: i18n._(msg`Enter this address and sign in:`), code: `${SERVER_TOKEN}/mcp` },
        ],
        note: i18n._(msg`Needs a public https address.`),
      },
      {
        id: "cursor",
        name: "Cursor",
        steps: [
          {
            text: i18n._(msg`Add this to .cursor/mcp.json in your project:`),
            code: `{\n  "mcpServers": {\n    "yuva": { "url": "${SERVER_TOKEN}/mcp" }\n  }\n}`,
          },
          { text: signIn },
        ],
      },
      {
        id: "vscode",
        name: "VS Code",
        steps: [
          {
            text: i18n._(msg`Add this to .vscode/mcp.json in your project:`),
            code: `{\n  "servers": {\n    "yuva": { "type": "http", "url": "${SERVER_TOKEN}/mcp" }\n  }\n}`,
          },
          { text: signIn },
        ],
      },
      {
        id: "gemini",
        name: "Gemini CLI",
        steps: [
          {
            text: i18n._(msg`Add Yuva in your terminal:`),
            code: `gemini mcp add --transport http yuva ${SERVER_TOKEN}/mcp`,
          },
          { text: i18n._(msg`Sign in from Gemini CLI:`), code: "/mcp auth yuva" },
        ],
      },
    ] satisfies Assistant[],
  }
}
