import fs from "node:fs"
import type { APIRoute } from "astro"
import { specPath } from "@/lib/openapi"

export const GET: APIRoute = () => new Response(fs.readFileSync(specPath, "utf8"), { headers: { "Content-Type": "application/yaml; charset=utf-8" } })
