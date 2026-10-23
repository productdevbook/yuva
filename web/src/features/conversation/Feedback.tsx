import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, CircleCheckIcon, CircleDashedIcon, CircleDotIcon, CircleSlashIcon, CopyIcon } from "lucide-react"

import { Segmented, useCopy } from "@/components/common"
import { CategoryIcon, categoryTone, useFeedbackCategories } from "@/components/common/feedbackCategories"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { ConversationUpdate, Feedback, FeedbackCategory, FeedbackProgress } from "@/lib/api"
import { cn } from "@/lib/utils"

export function CategoryChip({
  category,
  inboxId,
  className,
}: {
  category: FeedbackCategory
  inboxId?: string
  className?: string
}) {
  const { find } = useFeedbackCategories()
  const option = find(category, inboxId)
  return (
    <span
      className={cn("inline-flex shrink-0 items-center gap-1 text-caption text-muted-foreground", className)}
      data-testid="category-chip"
      data-category={category}
    >
      <CategoryIcon name={option.icon} className={categoryTone(category).icon} />
      {option.name}
    </span>
  )
}

export function ScoreMark({ score, className }: { score?: number; className?: string }) {
  const { t } = useLingui()
  if (!score) return null
  const label = t`Score ${score} out of 5`
  return (
    <span
      className={cn("inline-flex shrink-0 items-center gap-1.5 text-caption text-muted-foreground", className)}
      role="img"
      aria-label={label}
      title={label}
      data-testid="score-mark"
      data-score={score}
    >
      <span className="inline-flex gap-px" aria-hidden>
        {[1, 2, 3, 4, 5].map((i) => (
          <span key={i} className={cn("h-2.5 w-1.5 rounded-[2px]", i <= score ? "bg-brand" : "bg-border")} />
        ))}
      </span>
      <span className="tabular-nums" aria-hidden>
        {score}/5
      </span>
    </span>
  )
}

export const progressIcons: Record<FeedbackProgress, React.ComponentType<{ className?: string }>> = {
  planned: CircleDashedIcon,
  in_progress: CircleDotIcon,
  done: CircleCheckIcon,
  wont_do: CircleSlashIcon,
}

export const progressClass: Record<FeedbackProgress, string> = {
  planned: "bg-muted text-muted-foreground",
  in_progress: "bg-brand-wash text-brand",
  done: "bg-success/10 text-success",
  wont_do: "bg-muted text-faint",
}

export function ProgressChip({ progress, className }: { progress: FeedbackProgress; className?: string }) {
  const text = useEnumText()
  const Icon = progressIcons[progress]
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-px text-caption font-medium",
        progressClass[progress],
        className
      )}
      data-testid="progress-chip"
      data-progress={progress}
    >
      <Icon className="size-3" />
      {text.progress[progress]}
    </span>
  )
}

export function useFeedbackRows(f: Feedback): [string, string, boolean?][] {
  const { t } = useLingui()
  const rows: [string, string, boolean?][] = []
  const app = [f.app_version, f.build && `(${f.build})`].filter(Boolean).join(" ")
  const os = [f.os, f.os_version].filter(Boolean).join(" ")
  if (app) rows.push([t`App`, app])
  if (os) rows.push([t`System`, os])
  if (f.device_model) rows.push([t`Device`, f.device_model])
  if (f.locale) rows.push([t`Locale`, f.locale])
  if (f.screen) rows.push([t`Screen`, f.screen, true])
  if (f.installation_id) rows.push([t`Installation`, f.installation_id, true])
  if (f.page_url) {
    if (f.page_title) rows.push([t`Page`, f.page_title])
    rows.push([t`Address`, f.page_url, true])
    if (f.rating) rows.push([t`Rating`, f.rating === "up" ? t`Helpful` : t`Not helpful`])
    rows.push([t`Replies`, f.allow_email ? t`By e-mail` : t`No e-mail address given`])
  } else {
    rows.push([t`Replies`, f.allow_email ? t`In the app and by e-mail` : t`Only in the app`])
  }
  return rows
}

export function FeedbackDetails({ feedback, inboxId }: { feedback: Feedback; inboxId?: string }) {
  const { t } = useLingui()
  const { find } = useFeedbackCategories()
  const rows = useFeedbackRows(feedback)
  const [copied, copy] = useCopy()
  const copyAll = () =>
    copy([`${t`Category`}: ${find(feedback.category, inboxId).name}`, ...rows.map(([k, v]) => `${k}: ${v}`)].join("\n"))
  useHotkeys({ [SHORTCUTS.copyDetails]: () => void copyAll() })
  return (
    <div className="flex items-start gap-2 rounded-xl px-2.5 py-1.5" data-testid="feedback-details">
      <dl className="grid min-w-0 flex-1 grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-caption sm:grid-cols-[auto_minmax(0,1fr)_auto_minmax(0,1fr)]">
        {rows.map(([k, v, mono]) => (
          <div key={k} className="contents">
            <dt className="text-faint">{k}</dt>
            <dd className={cn("truncate", mono && "font-mono")} title={v}>
              {v}
            </dd>
          </div>
        ))}
      </dl>
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        onClick={() => void copyAll()}
        aria-label={t`Copy the feedback details`}
        title={t`Copy the feedback details`}
        data-testid="copy-feedback"
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </Button>
      <span className="sr-only" role="status">
        {copied && <Trans>Copied</Trans>}
      </span>
    </div>
  )
}

const STEPS = ["none", "planned", "in_progress", "done", "wont_do"] as const

export function ProgressStrip({
  progress,
  update,
}: {
  progress?: FeedbackProgress
  update: (body: ConversationUpdate) => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  return (
    <div
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 border-b bg-background px-4 py-2"
      data-testid="progress-strip"
    >
      <span className="text-caption text-faint">
        <Trans>Progress · the person sees this</Trans>
      </span>
      <div className="max-w-full overflow-x-auto">
        <Segmented
          value={progress ?? "none"}
          onChange={(v) => update({ feedback_progress: v === "none" ? null : v })}
          label={t`Progress`}
          itemClassName="px-2.5"
          items={STEPS.map((p) => ({
            value: p,
            label: p === "none" ? t`None` : text.progress[p],
            testId: `progress-step-${p}`,
          }))}
        />
      </div>
    </div>
  )
}
