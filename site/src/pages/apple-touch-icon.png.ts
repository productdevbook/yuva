import type { APIRoute } from "astro"
import { markSvg, renderPng } from "@/lib/graphics"

export const GET: APIRoute = () => new Response(renderPng(markSvg(180), 180), { headers: { "Content-Type": "image/png" } })
