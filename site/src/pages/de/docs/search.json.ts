import type { APIRoute } from "astro"
import { searchIndex } from "@/lib/search"

export const GET: APIRoute = () => searchIndex("de")
