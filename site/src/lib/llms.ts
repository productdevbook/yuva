import path from "node:path"
import { description, docRel, ordered, title, type Doc } from "./docs"
import { spec, tags, tagTitle } from "./openapi"
import { repoLink } from "./repo-links"
import { absolute, docsRoot } from "./routes"

const docsDir = path.resolve("../docs")
const summary =
  "Yuva is open-source customer messaging: support e-mail, live chat and in-app conversations from many products in one shared inbox. Self-hosted, one Go binary with Postgres. The server is AGPL-3.0; the SDKs and the API contract are MIT."

const docUrl = (slug: string) => absolute(`${docsRoot("en")}${docRel(slug)}`)

function absoluteLinks(markdown: string, from: string) {
  return markdown.replace(/(!?\[[^\]]*\]\()([^)\s]+)(\))/g, (_, open: string, href: string, close: string) => {
    const next = repoLink(href, from)
    return `${open}${next.startsWith("/") ? absolute(next) : next}${close}`
  })
}

function body(doc: Doc) {
  return absoluteLinks((doc.body ?? "").trim(), path.join(docsDir, `${doc.id}.md`))
}

function oneLine(text: string) {
  return text.replace(/\s+/g, " ").trim()
}

export async function llmsTxt() {
  const docs = await ordered()
  const s = spec()
  const lines = [
    "# Yuva",
    "",
    `> ${summary}`,
    "",
    `The documentation describes the version on the main branch. Every endpoint is in the OpenAPI ${s.openapi} contract (API version ${s.info?.version}); ${absolute("/llms-full.txt")} has all guides and the endpoint list in one file.`,
    "",
    "## Docs",
    "",
    ...docs.filter((d) => !["architecture", "roadmap"].includes(d.id)).map((d) => `- [${title(d)}](${docUrl(d.id)}): ${description(d)}`),
    "",
    "## API",
    "",
    `- [OpenAPI contract](${absolute("/openapi.yaml")}): the /v1 member and API key API, the /client/v1 contact API, realtime and webhook payloads, as OpenAPI ${s.openapi}.`,
    `- [API reference](${docUrl("api")}): every endpoint and webhook event, generated from the contract.`,
    "",
    "## Optional",
    "",
    ...docs.filter((d) => ["architecture", "roadmap"].includes(d.id)).map((d) => `- [${title(d)}](${docUrl(d.id)}): ${description(d)}`),
    `- [Source code](https://github.com/productdevbook/yuva): the repository, with the server, panel, SDKs and examples.`,
    "",
  ]
  return lines.join("\n")
}

export async function llmsFullTxt() {
  const docs = await ordered()
  const s = spec()
  const out = [`# Yuva`, "", `> ${summary}`, ""]
  for (const doc of docs) {
    out.push("---", "", `Source: ${docUrl(doc.id)}`, "", body(doc), "")
  }
  out.push("---", "", `Source: ${absolute("/openapi.yaml")}`, "", `# API endpoints`, "")
  out.push(
    `OpenAPI ${s.openapi}, API version ${s.info?.version}. Base URL: your Yuva server. Schemas and every field are in the contract at ${absolute("/openapi.yaml")}.`,
    "",
  )
  for (const tag of tags()) {
    out.push(`## ${tagTitle(tag.name)}`, "")
    for (const op of tag.operations) {
      out.push(`### ${op.method} ${op.path}`, "", `${op.summary} (\`${op.id}\`)`, "")
      if (op.description) out.push(op.description.trim(), "")
    }
  }
  const hooks = Object.entries((s.webhooks ?? {}) as Record<string, { post?: { summary?: string; description?: string } }>)
  if (hooks.length) {
    out.push("## Webhook events", "")
    for (const [name, item] of hooks) {
      out.push(`### ${name}`, "", oneLine(`${item.post?.summary ?? ""}. ${item.post?.description ?? ""}`), "")
    }
  }
  return out.join("\n")
}
