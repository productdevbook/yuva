import type { APIRoute } from "astro"
import { iconSvg } from "@/lib/icon"

export const GET: APIRoute = () => new Response(iconSvg(1024, "square"), { headers: { "Content-Type": "image/svg+xml" } })
