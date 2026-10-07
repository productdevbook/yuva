import { afterEach, describe, expect, mock, test } from "bun:test";
import { handleEmail, type Env } from "../src/index";

const raw = new Uint8Array(await Bun.file(new URL("./fixtures/plain.eml", import.meta.url)).arrayBuffer());
const env: Env = { YUVA_URL: "https://yuva.example.net/", INGRESS_SECRET: "test-secret" };
const now = () => 1791364364_123;
const realFetch = globalThis.fetch;

type Captured = { url: string; headers: Headers; body: Uint8Array };

function message() {
  const rejects: string[] = [];
  const forwards: string[] = [];
  const msg = {
    from: "customer@example.org",
    to: "support@example.com",
    raw: new Blob([raw]).stream(),
    rawSize: raw.length,
    headers: new Headers(),
    setReject: (reason: string) => rejects.push(reason),
    forward: async (to: string) => {
      forwards.push(to);
    },
  } as unknown as ForwardableEmailMessage;
  return { msg, rejects, forwards };
}

function serve(respond: () => Response | Promise<Response>): Captured[] {
  const calls: Captured[] = [];
  globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input, init);
    calls.push({ url: req.url, headers: req.headers, body: new Uint8Array(await req.arrayBuffer()) });
    return respond();
  }) as unknown as typeof fetch;
  return calls;
}

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("email handler", () => {
  test("forwards the raw message with envelope and signature headers", async () => {
    const calls = serve(() => new Response(null, { status: 202 }));
    const { msg, rejects } = message();

    await handleEmail(msg, env, now);

    expect(rejects).toEqual([]);
    expect(calls).toHaveLength(1);
    const [call] = calls;
    expect(call.url).toBe("https://yuva.example.net/ingress/email");
    expect(call.body).toEqual(raw);
    expect(call.headers.get("X-Yuva-Envelope-To")).toBe("support@example.com");
    expect(call.headers.get("X-Yuva-Envelope-From")).toBe("customer@example.org");
    expect(call.headers.get("X-Yuva-Timestamp")).toBe("1791364364");
    expect(call.headers.get("X-Yuva-Signature")).toBe(
      "v1=2a6b3a29c5330e504f6ab4b1fb4a2c2c43900fa6a54f29f12425a8ccf0c6046a",
    );
  });

  test("signature verifies the way the server checks it", async () => {
    const calls = serve(() => new Response(null, { status: 202 }));
    await handleEmail(message().msg, env, now);
    const [call] = calls;

    const { createHmac } = await import("node:crypto");
    const expected = createHmac("sha256", env.INGRESS_SECRET)
      .update(`${call.headers.get("X-Yuva-Timestamp")}.${call.headers.get("X-Yuva-Envelope-To")}.`)
      .update(call.body)
      .digest("hex");
    expect(call.headers.get("X-Yuva-Signature")).toBe(`v1=${expected}`);
  });

  test("rejects with the server's reason on 4xx", async () => {
    serve(() => Response.json({ reason: "No inbox for support@example.com" }, { status: 404 }));
    const { msg, rejects } = message();

    await handleEmail(msg, env, now);

    expect(rejects).toEqual(["No inbox for support@example.com"]);
  });

  test("rejects with a generic reason on 4xx without a JSON reason", async () => {
    serve(() => new Response("nope", { status: 403 }));
    const { msg, rejects } = message();

    await handleEmail(msg, env, now);

    expect(rejects).toEqual(["Rejected by recipient server (403)"]);
  });

  test("throws on 5xx so the sender retries", async () => {
    serve(() => new Response(null, { status: 503 }));
    const { msg, rejects } = message();

    await expect(handleEmail(msg, env, now)).rejects.toThrow("ingress returned 503");
    expect(rejects).toEqual([]);
  });

  test("throws on a network error", async () => {
    serve(() => {
      throw new TypeError("connection refused");
    });
    const { msg, rejects } = message();

    await expect(handleEmail(msg, env, now)).rejects.toThrow("connection refused");
    expect(rejects).toEqual([]);
  });
});

describe("fallback forwarding", () => {
  const withFallback: Env = { ...env, FALLBACK_FORWARD: "owner@example.org" };

  test("forwards to the fallback address on 5xx", async () => {
    serve(() => new Response(null, { status: 502 }));
    const { msg, rejects, forwards } = message();

    await handleEmail(msg, withFallback, now);

    expect(forwards).toEqual(["owner@example.org"]);
    expect(rejects).toEqual([]);
  });

  test("forwards to the fallback address on a network error", async () => {
    serve(() => {
      throw new TypeError("connection refused");
    });
    const { msg, rejects, forwards } = message();

    await handleEmail(msg, withFallback, now);

    expect(forwards).toEqual(["owner@example.org"]);
    expect(rejects).toEqual([]);
  });

  test("forwards to the fallback address on a timeout", async () => {
    const calls = serve(() => {
      throw new DOMException("The operation timed out.", "TimeoutError");
    });
    const { msg, forwards } = message();

    await handleEmail(msg, withFallback, now);

    expect(calls).toHaveLength(1);
    expect(forwards).toEqual(["owner@example.org"]);
  });

  test("sends the ingress request with a timeout", async () => {
    let signal: AbortSignal | null | undefined;
    globalThis.fetch = mock(async (_input: RequestInfo | URL, init?: RequestInit) => {
      signal = init?.signal;
      return new Response(null, { status: 202 });
    }) as unknown as typeof fetch;

    await handleEmail(message().msg, withFallback, now);

    expect(signal).toBeInstanceOf(AbortSignal);
  });

  test("forwards when the Worker is not configured", async () => {
    const calls = serve(() => new Response(null, { status: 202 }));
    const { msg, forwards } = message();

    await handleEmail(msg, { YUVA_URL: "", INGRESS_SECRET: "", FALLBACK_FORWARD: "owner@example.org" }, now);

    expect(calls).toHaveLength(0);
    expect(forwards).toEqual(["owner@example.org"]);
  });

  test("still rejects on 4xx", async () => {
    serve(() => Response.json({ reason: "No such recipient" }, { status: 404 }));
    const { msg, rejects, forwards } = message();

    await handleEmail(msg, withFallback, now);

    expect(rejects).toEqual(["No such recipient"]);
    expect(forwards).toEqual([]);
  });

  test("does not forward accepted mail", async () => {
    serve(() => new Response(null, { status: 202 }));
    const { msg, forwards } = message();

    await handleEmail(msg, withFallback, now);

    expect(forwards).toEqual([]);
  });

  test("a blank fallback address keeps the retry behaviour", async () => {
    serve(() => new Response(null, { status: 503 }));
    const { msg, forwards } = message();

    await expect(handleEmail(msg, { ...env, FALLBACK_FORWARD: " " }, now)).rejects.toThrow("ingress returned 503");
    expect(forwards).toEqual([]);
  });
});
