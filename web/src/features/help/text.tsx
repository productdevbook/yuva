import { useLingui } from "@lingui/react/macro"
import { useMemo } from "react"

import { useErrorText } from "@/components/common/text"
import {
  ApiError,
  type HelpArticleSummary,
  type HelpCollection,
  type HelpIcon,
  type HelpTranslationState,
} from "@/lib/api"
import { cn } from "@/lib/utils"

export const HELP_ICONS: readonly HelpIcon[] = [
  "book",
  "rocket",
  "settings",
  "billing",
  "user",
  "team",
  "shield",
  "lock",
  "plug",
  "code",
  "chat",
  "mail",
  "bell",
  "chart",
  "globe",
  "mobile",
  "star",
  "help",
]

export function useLocaleName() {
  const { i18n } = useLingui()
  return useMemo(() => {
    let names: Intl.DisplayNames | undefined
    try {
      names = new Intl.DisplayNames([i18n.locale], { type: "language" })
    } catch {
      names = undefined
    }
    return (tag: string) => {
      let name: string | undefined
      try {
        name = names?.of(tag)
      } catch {
        name = undefined
      }
      return name && name !== tag ? name : tag
    }
  }, [i18n.locale])
}

export function useStateText(): Record<HelpTranslationState, string> {
  const { t } = useLingui()
  return { draft: t`Draft`, published: t`Published`, changed: t`Unpublished changes` }
}

export function useIconText(): Record<HelpIcon, string> {
  const { t } = useLingui()
  return {
    book: t`Book`,
    rocket: t`Rocket`,
    settings: t`Settings`,
    billing: t`Billing`,
    user: t`Person`,
    team: t`Team`,
    shield: t`Shield`,
    lock: t`Lock`,
    plug: t`Plug`,
    code: t`Code`,
    chat: t`Chat`,
    mail: t`Mail`,
    bell: t`Bell`,
    chart: t`Chart`,
    globe: t`Globe`,
    mobile: t`Phone`,
    star: t`Star`,
    help: t`Question mark`,
  }
}

export function useHelpErrorText() {
  const { t } = useLingui()
  const base = useErrorText()
  return (err: unknown): string => {
    if (err instanceof ApiError) {
      if (err.code === "slug_taken") return t`Another help center already uses this address, now or before.`
      if (err.code === "help_center_exists") return t`This inbox already has a help center.`
      if (err.code === "locale_not_offered")
        return t`The help center does not offer this language. Add it in the settings first.`
      if (err.code === "version_conflict") return t`Someone saved this article in the meantime.`
      if (err.code === "last_translation") return t`This is the article’s only translation. Delete the article instead.`
      if (err.code === "import_running") return t`An import is already running in this help center.`
      if (err.code === "source_not_connected")
        return t`This migration is no longer signed in to Zendesk. Start a new one in Moving to Yuva.`
      if (err.code === "source_mismatch") return t`This migration is not a Zendesk migration.`
      if (err.code === "query_empty") return t`Type a word to search for.`
      if (err.code === "rate_limited") return t`Too many requests. Wait a moment and try again.`
      if (err.code === "member_session_required") return t`Only owners and admins can do this.`
      if (err.code === "help_off") return t`Help centers are turned off on this server.`
      if (err.code === "hosted_limit")
        return t`This workspace has as many help articles as the server allows. Delete old ones first.`
    }
    return base(err)
  }
}

export function ErrorLine({ error, className }: { error: unknown; className?: string }) {
  const text = useHelpErrorText()
  if (!error) return null
  return (
    <p role="alert" className={cn("text-body text-destructive", className)}>
      {text(error)}
    </p>
  )
}

export function collectionName(c: HelpCollection, ...prefer: string[]) {
  for (const l of prefer) if (c.translations[l]?.name) return c.translations[l].name
  return Object.values(c.translations)[0]?.name ?? ""
}

export function articleTitle(a: HelpArticleSummary, ...prefer: string[]) {
  for (const l of prefer) {
    const tr = a.translations.find((x) => x.locale === l)
    if (tr) return tr.title
  }
  return a.translations[0]?.title ?? ""
}
