import { join, normalize } from "node:path";

const root = join(import.meta.dir, "..");
const port = Number(process.env.PORT ?? 5180);

Bun.serve({
  port,
  hostname: process.env.HOST ?? "127.0.0.1",
  async fetch(request) {
    const { pathname } = new URL(request.url);
    const path = normalize(pathname === "/" ? "/example/index.html" : pathname);
    if (!path.startsWith("/example/") && !path.startsWith("/dist/")) return new Response("Not found", { status: 404 });
    const file = Bun.file(join(root, path));
    return (await file.exists()) ? new Response(file) : new Response("Not found", { status: 404 });
  },
});

console.log(`http://127.0.0.1:${port}/example/index.html`);
