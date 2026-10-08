import type { APIRoute } from "astro"
import { iconSvg } from "@/lib/icon"
import { renderPng } from "@/lib/graphics"

export const GET: APIRoute = () => new Response(renderPng(iconSvg(1024, "square"), 1024), { headers: { "Content-Type": "image/png" } })
