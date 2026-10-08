import { createYuvaClient } from "../src/index";

const requests: { method: string; path: string; headers: Record<string, string>; body: string }[] = [];
let sessions = 0;
let expireNext = false;

const json = (status: number, value: unknown) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
const inbox = { id: "i1", name: "Support" };

async function fetcher(url: string, init: RequestInit): Promise<Response> {
  const path = new URL(url).pathname;
  const headers = Object.fromEntries(new Headers(init.headers).entries());
  const body = init.body instanceof Blob ? await init.body.text() : String(init.body ?? "");
  const method = init.method ?? "GET";
  requests.push({ method, path, headers, body });
  if (path === "/client/v1/session" && method === "POST") {
    sessions++;
    return json(201, { token: `t${sessions}`, expires_at: new Date(Date.now() + 86_400_000).toISOString(), visitor_id: "v1", contact: { id: "c1", identified: false }, inbox });
  }
  if (expireNext) {
    expireNext = false;
    return json(401, { code: "unauthorized" });
  }
  if (path.endsWith("/messages") && method === "POST") {
    const sent = headers["content-type"]?.startsWith("application/json") ? (JSON.parse(body) as { body: string; client_id: string }) : { body: "", client_id: "" };
    return json(201, { id: "m1", conversation_id: "c9", body: sent.body, client_id: sent.client_id, author: { type: "contact" }, attachments: [], created_at: new Date().toISOString() });
  }
  return json(404, { code: "not_found" });
}

const client = createYuvaClient({ server: "https://yuva.example/", channel: "pk_test", fetch: fetcher });
const session = await client.start();
const first = await client.sendMessage("c9", { body: "Hello", client_id: "client-1" });
expireNext = true;
await client.sendMessage("c9", { body: "Again", client_id: "client-2" });
const file = new File(["png bytes"], "shot.png", { type: "image/png" });
await client.sendMessage("c9", { body: "With a file", files: [file], client_id: "client-3" }).catch(() => undefined);
await client.sendMessage("c9", { body: "With a file", files: [file], client_id: "client-3" }).catch(() => undefined);

console.log(
  JSON.stringify({
    dom: typeof document !== "undefined" || typeof window !== "undefined",
    visitor: session.visitor_id,
    stored: client.stored(),
    first,
    sessions,
    token: client.token,
    requests,
  }),
);
