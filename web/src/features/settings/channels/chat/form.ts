import { ApiError, type ChatChannel, type ChatChannelInput, type ChatLauncherPosition } from "@/lib/api"

const MAX_ORIGINS = 20
const DEFAULT_COLOR = "#d4431c"
export const POSITIONS: ChatLauncherPosition[] = ["right", "left"]

export type ChatForm = {
  origins: string
  allowAnonymous: boolean
  askEmailOffline: boolean
  greeting: string
  position: ChatLauncherPosition
  customColor: boolean
  color: string
  touched: boolean
}

export function chatForm(c?: ChatChannel): ChatForm {
  return {
    origins: c?.allowed_origins.join("\n") ?? "",
    allowAnonymous: c?.allow_anonymous ?? false,
    askEmailOffline: c?.ask_email_offline ?? true,
    greeting: c?.greeting ?? "",
    position: c?.launcher.position ?? "right",
    customColor: !!c?.launcher.color,
    color: c?.launcher.color ?? DEFAULT_COLOR,
    touched: false,
  }
}

function normalizeOrigin(raw: string): string | null {
  const v = raw.trim().replace(/\/$/, "")
  if (!/^https?:\/\/[^/?#\s]+$/i.test(v)) return null
  try {
    const u = new URL(v)
    if (u.username || u.password) return null
    return u.origin.length <= 300 ? u.origin : null
  } catch {
    return null
  }
}

export type ChatInputResult = { ok: true; value: ChatChannelInput } | { ok: false; empty: boolean; tooMany: boolean; bad: string[] }

export function chatChannelInput(f: ChatForm): ChatInputResult {
  const lines = f.origins
    .split(/[\n,]+/)
    .map((x) => x.trim())
    .filter(Boolean)
  const bad = lines.filter((l) => !normalizeOrigin(l))
  const origins = [...new Set(lines.map(normalizeOrigin).filter((x): x is string => !!x))]
  if (bad.length > 0 || origins.length === 0 || origins.length > MAX_ORIGINS) {
    return { ok: false, empty: lines.length === 0, tooMany: origins.length > MAX_ORIGINS, bad }
  }
  return {
    ok: true,
    value: {
      allowed_origins: origins,
      allow_anonymous: f.allowAnonymous,
      ask_email_offline: f.askEmailOffline,
      greeting: f.greeting.trim(),
      launcher: { position: f.position, ...(f.customColor ? { color: f.color.toLowerCase() } : {}) },
    },
  }
}

export type ChatField = "allowed_origins" | "greeting" | "launcher.color" | "launcher.position"

export function chatErrorField(err: unknown): ChatField | null {
  if (!(err instanceof ApiError) || err.status !== 400 || !err.detail) return null
  const m = /\bchat\.([a-z_]+(?:\.[a-z_]+)?)/.exec(err.detail)
  return (m?.[1] as ChatField | undefined) ?? null
}
