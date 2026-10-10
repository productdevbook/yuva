import { Trans, useLingui } from "@lingui/react/macro"
import { BugIcon, CheckIcon, CopyIcon, HeartIcon, LightbulbIcon, MessageSquareMoreIcon } from "lucide-react"

import { useCopy } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Feedback, FeedbackCategory } from "@/lib/api"
import { cn } from "@/lib/utils"

const categoryIcons: Record<FeedbackCategory, React.ComponentType<{ className?: string }>> = {
  bug: BugIcon,
  idea: LightbulbIcon,
  praise: HeartIcon,
  other: MessageSquareMoreIcon,
}

const categoryClass: Record<FeedbackCategory, string> = {
  bug: "text-destructive",
  idea: "text-warning",
  praise: "text-brand",
  other: "text-faint",
}

export function CategoryChip({ category, className }: { category: FeedbackCategory; className?: string }) {
  const text = useEnumText()
  const Icon = categoryIcons[category]
  return (
    <span
      className={cn("inline-flex shrink-0 items-center gap-1 text-caption text-muted-foreground", className)}
      data-testid="category-chip"
      data-category={category}
    >
      <Icon className={cn("size-3.5", categoryClass[category])} />
      {text.category[category]}
    </span>
  )
}

function useRows(f: Feedback): [string, string, boolean?][] {
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

export function FeedbackDetails({ feedback }: { feedback: Feedback }) {
  const { t } = useLingui()
  const text = useEnumText()
  const rows = useRows(feedback)
  const [copied, copy] = useCopy()
  const copyAll = () =>
    copy([`${t`Category`}: ${text.category[feedback.category]}`, ...rows.map(([k, v]) => `${k}: ${v}`)].join("\n"))
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
