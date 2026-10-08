import type { APIRoute } from "astro"
import { logoSvg } from "@/lib/brand"

export const GET: APIRoute = () => new Response(logoSvg("#ffffff"), { headers: { "Content-Type": "image/svg+xml" } })
