import type { APIRoute } from "astro"
import { iconSvg } from "@/lib/icon"

export const GET: APIRoute = () => new Response(iconSvg(32), { headers: { "Content-Type": "image/svg+xml" } })
