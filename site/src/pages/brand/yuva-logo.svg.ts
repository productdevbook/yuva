import type { APIRoute } from "astro"
import { logoSvg } from "@/lib/brand"

export const GET: APIRoute = () => new Response(logoSvg("#0b0b0f"), { headers: { "Content-Type": "image/svg+xml" } })
