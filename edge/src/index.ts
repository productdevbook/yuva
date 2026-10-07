export interface Env {
  YUVA_URL: string;
  INGRESS_SECRET: string;
}

const encoder = new TextEncoder();

export async function sign(secret: string, timestamp: string, envelopeTo: string, body: Uint8Array): Promise<string> {
  const key = await crypto.subtle.importKey("raw", encoder.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const prefix = encoder.encode(`${timestamp}.${envelopeTo}.`);
  const data = new Uint8Array(prefix.length + body.length);
  data.set(prefix, 0);
  data.set(body, prefix.length);
  const mac = new Uint8Array(await crypto.subtle.sign("HMAC", key, data));
  return Array.from(mac, (b) => b.toString(16).padStart(2, "0")).join("");
}

async function rejectReason(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { reason?: unknown };
    if (typeof body.reason === "string" && body.reason !== "") return body.reason;
  } catch {}
  return `Rejected by recipient server (${res.status})`;
}

export async function handleEmail(message: ForwardableEmailMessage, env: Env, now: () => number = Date.now): Promise<void> {
  if (!env.YUVA_URL || !env.INGRESS_SECRET) throw new Error("YUVA_URL and INGRESS_SECRET must be set");

  const body = new Uint8Array(await new Response(message.raw).arrayBuffer());
  const timestamp = Math.floor(now() / 1000).toString();
  const signature = await sign(env.INGRESS_SECRET, timestamp, message.to, body);

  const res = await fetch(`${env.YUVA_URL.replace(/\/+$/, "")}/ingress/email`, {
    method: "POST",
    headers: {
      "Content-Type": "message/rfc822",
      "X-Yuva-Envelope-To": message.to,
      "X-Yuva-Envelope-From": message.from,
      "X-Yuva-Timestamp": timestamp,
      "X-Yuva-Signature": `v1=${signature}`,
    },
    body,
  });

  if (res.ok) return;
  if (res.status >= 400 && res.status < 500) {
    message.setReject(await rejectReason(res));
    return;
  }
  throw new Error(`ingress returned ${res.status}`);
}

export default {
  email: (message, env) => handleEmail(message, env),
} satisfies ExportedHandler<Env>;
