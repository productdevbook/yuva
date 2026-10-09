import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { setTimeout as sleep } from "node:timers/promises";
import { createYuvaApi, type components } from "useyuva/api";

type StoredEvent = components["schemas"]["StoredEvent"];

const server = process.env.YUVA_SERVER ?? "http://localhost:8080";
const apiKey = process.env.YUVA_API_KEY;
if (!apiKey) {
  console.error("Set YUVA_API_KEY to a key with conversations:read.");
  process.exit(1);
}
const cursorFile = process.env.YUVA_CURSOR_FILE ?? ".yuva-cursor";
const once = process.argv.includes("--once");

const api = createYuvaApi({ server, apiKey });

async function loadOpenConversations() {
  const { data, error } = await api.GET("/v1/conversations", { params: { query: { status: "open", limit: 50 } } });
  if (error) throw new Error(`${error.code}: ${error.detail ?? error.title}`);
  for (const c of data.items) console.log(`open  ${c.id}  ${c.subject ?? ""}`);
}

async function latest() {
  const { data, error } = await api.GET("/v1/events/latest");
  if (error) throw new Error(`${error.code}: ${error.detail ?? error.title}`);
  return data.id;
}

function show(event: StoredEvent) {
  switch (event.type) {
    case "message.created": {
      const m = event.data;
      const author = m.author.type === "contact" ? "contact" : (m.author.name ?? m.author.type);
      console.log(`#${event.id} message  ${m.conversation_id}  ${m.direction} ${m.kind} by ${author}: ${m.body.slice(0, 80)}`);
      break;
    }
    case "draft.created":
    case "draft.updated":
    case "draft.deleted":
      console.log(`#${event.id} ${event.type}  ${event.conversation_id}`);
      break;
    default:
      console.log(`#${event.id} ${event.type}`);
  }
}

let after: number;
if (existsSync(cursorFile)) {
  after = Number(readFileSync(cursorFile, "utf8"));
} else {
  after = await latest();
  await loadOpenConversations();
}

for (;;) {
  const { data, error, response } = await api.GET("/v1/events", { params: { query: { after, limit: 100 } } });
  if (response.status === 410) {
    console.log("cursor expired: reloading");
    after = await latest();
    await loadOpenConversations();
    continue;
  }
  if (error) throw new Error(`${error.code}: ${error.detail ?? error.title}`);
  data.events.forEach(show);
  after = data.next;
  writeFileSync(cursorFile, String(after));
  if (data.has_more) continue;
  if (once) break;
  await sleep(5000);
}
