import { useLingui } from "@lingui/react/macro"

import { hostPath } from "@/components/common"
import { categoryTone, useFeedbackCategories } from "@/components/common/feedbackCategories"
import { useEnumText } from "@/components/common/text"
import { look } from "@/features/inbox/PageFrame"
import { useCounts } from "@/features/inbox/queries"
import type { Feedback, FeedbackCategory, FeedbackProgress } from "@/lib/api"
import { cn } from "@/lib/utils"

type Category = FeedbackCategory | "all"

export const progressTone: Record<FeedbackProgress, string> = {
  planned: "border-blue-600/35 text-blue-600 dark:border-blue-400/35 dark:text-blue-400",
  in_progress: "border-warning/35 text-warning",
  done: "border-success/35 text-success",
  wont_do: "border-faint/35 text-faint",
}

export function CategoryPill({
  category,
  inboxId,
  className,
}: {
  category: FeedbackCategory
  inboxId?: string
  className?: string
}) {
  const { find } = useFeedbackCategories()
  return (
    <span
      className={cn(
        "inline-flex min-w-[52px] max-w-32 shrink-0 justify-center rounded-full border px-1 py-px text-caption font-medium",
        categoryTone(category).pill,
        className
      )}
      data-testid="category-chip"
      data-category={category}
    >
      <span className="truncate">{find(category, inboxId).name}</span>
    </span>
  )
}

export function ProgressPill({ progress }: { progress: FeedbackProgress }) {
  const text = useEnumText()
  return (
    <span className={cn(look.pill, progressTone[progress])} data-testid="progress-chip" data-progress={progress}>
      {text.progress[progress]}
    </span>
  )
}

export function feedbackPlace(f?: Feedback) {
  if (!f) return ""
  if (f.page_url) return [f.page_title || hostPath(f.page_url)].filter(Boolean).join(" · ")
  const app = [f.app_version && `${f.app_version}`, [f.os, f.os_version].filter(Boolean).join(" "), f.device_model]
  return app.filter(Boolean).join(" · ")
}

export type FeedbackCategoryFilter = Category

export function useFeedbackTypes(inboxId: string, category: Category) {
  const { t } = useLingui()
  const counts = useCounts().data
  const known = useFeedbackCategories(inboxId || undefined).list
  const open = (k: Category) =>
    k === "all" ? (counts?.feedback ?? 0) : (counts?.feedback_categories.find((x) => x.category === k)?.count ?? 0)
  const keys = [
    ...known.map((c) => c.key),
    ...(counts?.feedback_categories ?? []).map((x) => x.category).filter((k) => !known.some((c) => c.key === k)),
  ]
  const types: { key: Category; name: string; icon?: string }[] = [
    { key: "all", name: t`All` },
    ...keys
      .filter((k) => k !== "other" || category === "other" || open("other") > 0)
      .map((k) => known.find((c) => c.key === k) ?? { key: k, name: k, icon: "tag" }),
  ]
  return { types, open }
}
