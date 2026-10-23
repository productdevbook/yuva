import {
  API_KEY_PRESET_SCOPES,
  API_KEY_SCOPES,
  OWNER_ONLY_SCOPES,
  WORKSPACE_WIDE_SCOPES,
  type ApiKeyPreset,
} from "@/components/common/text"
import { useWizardDraft } from "@/hooks/use-wizard-draft"
import type { ApiKeyScope } from "@/lib/api"

export type ApiKeyDraft = {
  name: string
  botName: string
  avatar: string
  inboxIds: string[]
  preset: ApiKeyPreset
  custom: ApiKeyScope[]
}

export function useApiKeyDraft() {
  return useWizardDraft<ApiKeyDraft>("api-key", {
    name: "",
    botName: "",
    avatar: "",
    inboxIds: [],
    preset: "support",
    custom: API_KEY_PRESET_SCOPES.everything,
  })
}

export function chosenScopes(draft: ApiKeyDraft, isOwner: boolean): ApiKeyScope[] {
  const base = draft.preset === "custom" ? draft.custom : API_KEY_PRESET_SCOPES[draft.preset]
  const limited = draft.inboxIds.length > 0
  return API_KEY_SCOPES.filter(
    (s) =>
      base.includes(s) &&
      (!limited || !WORKSPACE_WIDE_SCOPES.includes(s)) &&
      (isOwner || !OWNER_ONLY_SCOPES.includes(s))
  )
}

export type ApiKeyDone = {
  name: string
  bot: string
  inboxNames: string[]
  scopes: ApiKeyScope[]
  expires: string | null
  secret?: string
}

export type ApiKeyWizardState = { back?: string; done?: ApiKeyDone }

export const API_KEY_PATHS = {
  name: "/setup/api-key",
  access: "/setup/api-key/access",
  review: "/setup/api-key/review",
  done: "/setup/api-key/done",
}
