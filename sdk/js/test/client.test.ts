import { expect, test } from "bun:test";

interface Run {
  dom: boolean;
  visitor: string;
  stored: { visitor_id?: string; session?: { token: string } };
  first: { body: string; client_id: string };
  sessions: number;
  token: string;
  requests: { method: string; path: string; headers: Record<string, string>; body: string }[];
}

test("createYuvaClient opens a session and sends messages without a DOM", () => {
  const run = Bun.spawnSync([process.execPath, `${import.meta.dir}/headless.ts`], { env: { ...process.env, NODE_ENV: "test" } });
  expect(run.stderr.toString()).toBe("");
  const result = JSON.parse(run.stdout.toString()) as Run;
  expect(result.dom).toBe(false);

  const [start, send] = result.requests;
  expect(start?.path).toBe("/client/v1/session");
  expect(JSON.parse(start?.body ?? "{}")).toEqual({ channel_key: "pk_test" });
  expect(result.visitor).toBe("v1");
  expect(result.stored.visitor_id).toBe("v1");

  expect(send?.path).toBe("/client/v1/conversations/c9/messages");
  expect(send?.headers.authorization).toBe("Bearer t1");
  expect(send?.headers["idempotency-key"]).toBe("client-1");
  expect(JSON.parse(send?.body ?? "{}")).toEqual({ body: "Hello", client_id: "client-1" });
  expect(result.first).toMatchObject({ body: "Hello", client_id: "client-1" });

  const renewed = result.requests.slice(2, 5).map((r) => `${r.method} ${r.path} ${r.headers.authorization ?? ""}`);
  expect(renewed).toEqual([
    "POST /client/v1/conversations/c9/messages Bearer t1",
    "POST /client/v1/session ",
    "POST /client/v1/conversations/c9/messages Bearer t2",
  ]);
  expect(JSON.parse(result.requests[3]?.body ?? "{}")).toEqual({ channel_key: "pk_test", visitor_id: "v1" });
  expect(result.sessions).toBe(2);
  expect(result.token).toBe("t2");
  expect(result.stored.session?.token).toBe("t2");

  const uploads = result.requests.slice(5);
  expect(uploads).toHaveLength(2);
  expect(uploads[0]?.headers["content-type"]).toStartWith("multipart/form-data; boundary=");
  expect(uploads[0]?.headers["idempotency-key"]).toBe("client-3");
  expect(uploads[0]?.body).toContain('filename="shot.png"');
  expect(uploads[1]?.body).toBe(uploads[0]?.body ?? "");
  expect(uploads[1]?.headers["content-type"]).toBe(uploads[0]?.headers["content-type"] ?? "");
});
