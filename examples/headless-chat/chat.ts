import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { createInterface } from "node:readline/promises";
import { createYuvaClient, type ClientMessage, type YuvaStorage } from "useyuva";

const server = process.env.YUVA_SERVER ?? "http://localhost:8080";
const channel = process.env.YUVA_CHANNEL;
if (!channel) {
  console.error("Set YUVA_CHANNEL to the public key (yuva_pk_…) of an app channel.");
  process.exit(1);
}

const stateFile = process.env.YUVA_STATE_FILE ?? ".yuva-chat.json";
const storage: YuvaStorage = {
  getItem: (key) => {
    if (!existsSync(stateFile)) return null;
    return (JSON.parse(readFileSync(stateFile, "utf8")) as Record<string, string>)[key] ?? null;
  },
  setItem: (key, value) => {
    const all = existsSync(stateFile) ? (JSON.parse(readFileSync(stateFile, "utf8")) as Record<string, string>) : {};
    writeFileSync(stateFile, JSON.stringify({ ...all, [key]: value }));
  },
};

const yuva = createYuvaClient({
  server,
  channel,
  storage,
  identityToken: process.env.YUVA_IDENTITY_TOKEN ? () => process.env.YUVA_IDENTITY_TOKEN : null,
});

const seen = new Set<string>();
let conversationId: string | undefined;

function show(message: ClientMessage) {
  if (seen.has(message.id)) return;
  seen.add(message.id);
  const who = message.author.type === "contact" ? "you" : message.author.name || message.author.type;
  console.log(`[${who}] ${message.body}`);
}

yuva.on("event", (event) => {
  if (event.type === "message.created" && event.data.conversation_id === conversationId) show(event.data);
  if (event.type === "typing" && event.data.conversation_id === conversationId && event.data.typing) {
    console.log("… typing");
  }
});
yuva.on("connection", ({ state }) => {
  if (state === "reconnecting") console.log("(reconnecting)");
});

const session = await yuva.start();
console.log(`Connected to ${session.inbox.name}. Type a message and press Enter; Ctrl+D quits.`);

const { items } = await yuva.listConversations({ limit: 1 });
conversationId = items[0]?.id;
if (conversationId) {
  const history = await yuva.listMessages(conversationId, { order: "asc", limit: 50 });
  history.items.forEach(show);
}
await yuva.connect();

const rl = createInterface({ input: process.stdin });
for await (const line of rl) {
  const body = line.trim();
  if (!body) continue;
  if (conversationId) {
    show(await yuva.sendMessage(conversationId, { body }));
  } else {
    const created = await yuva.startConversation({ body });
    conversationId = created.conversation.id;
    show(created.message);
  }
}
yuva.disconnect();
