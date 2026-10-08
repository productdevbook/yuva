import { PLATFORMS } from "@/components/common/text"
import type { AppChannel, AppChannelInput, AppPlatform } from "@/lib/api"

export type AppForm = { allowAnonymous: boolean; platforms: AppPlatform[]; touched: boolean }

export function appForm(c?: AppChannel): AppForm {
  return { allowAnonymous: c?.allow_anonymous ?? false, platforms: c?.platforms ?? ["ios", "android"], touched: false }
}

export function appChannelInput(f: AppForm): AppChannelInput | null {
  if (f.platforms.length === 0) return null
  return { allow_anonymous: f.allowAnonymous, platforms: PLATFORMS.filter((p) => f.platforms.includes(p)) }
}
