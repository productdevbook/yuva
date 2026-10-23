import { useLingui } from "@lingui/react/macro"
import {
  BugIcon,
  CircleHelpIcon,
  FlameIcon,
  GaugeIcon,
  HeartIcon,
  LightbulbIcon,
  LockIcon,
  MessageCircleIcon,
  MessageSquareMoreIcon,
  PaletteIcon,
  RocketIcon,
  ShieldAlertIcon,
  SparklesIcon,
  StarIcon,
  TagIcon,
  ThumbsDownIcon,
  ThumbsUpIcon,
  WalletIcon,
  WrenchIcon,
  ZapIcon,
  type LucideIcon,
} from "lucide-react"
import { useCallback, useMemo } from "react"

import type { Inbox, InboxFeedbackCategory } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export const CATEGORY_ICONS: Record<string, LucideIcon> = {
  bug: BugIcon,
  lightbulb: LightbulbIcon,
  heart: HeartIcon,
  "message-circle": MessageCircleIcon,
  "message-square-more": MessageSquareMoreIcon,
  star: StarIcon,
  sparkles: SparklesIcon,
  zap: ZapIcon,
  flame: FlameIcon,
  rocket: RocketIcon,
  wrench: WrenchIcon,
  gauge: GaugeIcon,
  palette: PaletteIcon,
  wallet: WalletIcon,
  lock: LockIcon,
  "shield-alert": ShieldAlertIcon,
  "circle-help": CircleHelpIcon,
  "thumbs-up": ThumbsUpIcon,
  "thumbs-down": ThumbsDownIcon,
  tag: TagIcon,
}

const TONES: Record<string, { icon: string; pill: string; tag: string }> = {
  bug: {
    icon: "text-destructive",
    pill: "border-destructive/35 text-destructive",
    tag: "bg-destructive/10 text-destructive",
  },
  idea: {
    icon: "text-warning",
    pill: "border-blue-600/35 text-blue-600 dark:border-blue-400/35 dark:text-blue-400",
    tag: "bg-warning/12 text-warning",
  },
  praise: { icon: "text-brand", pill: "border-success/35 text-success", tag: "bg-success/12 text-success" },
}
const NEUTRAL = { icon: "text-faint", pill: "border-faint/35 text-faint", tag: "bg-muted text-muted-foreground" }

export function categoryTone(key: string) {
  return TONES[key] ?? NEUTRAL
}

export function CategoryIcon({ name, className }: { name: string; className?: string }) {
  const Icon = CATEGORY_ICONS[name] ?? TagIcon
  return <Icon className={cn("size-3.5", className)} />
}

export function categoryName(names: Record<string, string>, locales: string[]) {
  for (const l of locales) {
    const hit = names[l] ?? names[l.split("-")[0]!]
    if (hit) return hit
  }
  return names.en ?? Object.values(names)[0]
}

export type CategoryOption = { key: string; name: string; icon: string }

function option(c: InboxFeedbackCategory, inbox: Inbox, locale: string): CategoryOption {
  return { key: c.key, icon: c.icon, name: categoryName(c.names, [locale, inbox.default_locale]) ?? c.key }
}

export function useFeedbackCategories(inboxId?: string) {
  const { i18n } = useLingui()
  const inboxes = useInboxes().data
  const locale = i18n.locale
  const list = useMemo(() => {
    const seen = new Map<string, CategoryOption>()
    for (const inbox of inboxes ?? []) {
      if (inboxId && inbox.id !== inboxId) continue
      for (const c of inbox.feedback_categories) if (!seen.has(c.key)) seen.set(c.key, option(c, inbox, locale))
    }
    return [...seen.values()]
  }, [inboxes, inboxId, locale])
  const find = useCallback(
    (key: string, owner?: string): CategoryOption => {
      const inbox = owner ? inboxes?.find((i) => i.id === owner) : undefined
      const own = inbox?.feedback_categories.find((c) => c.key === key)
      if (inbox && own) return option(own, inbox, locale)
      return list.find((c) => c.key === key) ?? { key, name: key, icon: "tag" }
    },
    [inboxes, list, locale]
  )
  return { list, find }
}
