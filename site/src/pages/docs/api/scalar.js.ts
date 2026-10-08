import fs from "node:fs"
import path from "node:path"
import type { APIRoute } from "astro"

const file = path.resolve("node_modules/@scalar/api-reference/dist/browser/standalone.js")

export const GET: APIRoute = () => new Response(fs.readFileSync(file, "utf8"), { headers: { "Content-Type": "text/javascript; charset=utf-8" } })
