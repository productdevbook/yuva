import { createHmac } from "node:crypto";
import { join, normalize } from "node:path";

const root = join(import.meta.dir, "..");
const port = Number(process.env.PORT ?? 5180);
const secret = process.env.YUVA_IDENTITY_SECRET ?? "";

const base64url = (value: string | Buffer) => Buffer.from(value).toString("base64url");

function signIdentity(claims: { sub: string; email?: string; name?: string; locale?: string }): string {
  const now = Math.floor(Date.now() / 1000);
  const header = base64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const payload = base64url(JSON.stringify({ ...claims, iat: now, exp: now + 300 }));
  const signature = createHmac("sha256", secret).update(`${header}.${payload}`).digest("base64url");
  return `${header}.${payload}.${signature}`;
}

Bun.serve({
  port,
  hostname: process.env.HOST ?? "127.0.0.1",
  async fetch(request) {
    const url = new URL(request.url);
    if (url.pathname === "/example/identity-token") {
      if (!secret) return new Response("Set YUVA_IDENTITY_SECRET to sign identity tokens.", { status: 501 });
      const sub = url.searchParams.get("sub") || "example-user";
      const token = signIdentity({
        sub,
        email: url.searchParams.get("email") || undefined,
        name: url.searchParams.get("name") || undefined,
        locale: url.searchParams.get("locale") || undefined,
      });
      return Response.json({ token }, { headers: { "Cache-Control": "no-store" } });
    }
    const path = normalize(url.pathname === "/" ? "/example/index.html" : url.pathname);
    if (!path.startsWith("/example/") && !path.startsWith("/dist/")) return new Response("Not found", { status: 404 });
    const file = Bun.file(join(root, path));
    return (await file.exists()) ? new Response(file) : new Response("Not found", { status: 404 });
  },
});

console.log(`http://127.0.0.1:${port}/example/index.html`);
