import { Trans, useLingui } from "@lingui/react/macro"
import { BugIcon, CheckIcon, CopyIcon, HeartIcon, LightbulbIcon, MessageSquareMoreIcon } from "lucide-react"
import { useState } from "react"

import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Feedback, FeedbackCategory } from "@/lib/api"
import { cn } from "@/lib/utils"

export const categoryIcons: Record<FeedbackCategory, React.ComponentType<{ className?: string }>> = {
  bug: BugIcon,
  idea: LightbulbIcon,
  praise: HeartIcon,
  other: MessageSquareMoreIcon,
}

const categoryClass: Record<FeedbackCategory, string> = {
  bug: "border-destructive/30 text-destructive",
  idea: "border-amber-500/40 text-amber-700 dark:text-amber-400",
  praise: "border-pink-500/30 text-pink-700 dark:text-pink-400",
  other: "text-muted-foreground",
}

export function CategoryChip({ category, className }: { category: FeedbackCategory; className?: string }) {
  const text = useEnumText()
  const Icon = categoryIcons[category]
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full border px-1.5 text-xs leading-4",
        categoryClass[category],
        className,
      )}
      data-testid="category-chip"
      data-category={category}
    >
      <Icon className="size-3" />
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
  rows.push([t`Replies`, f.allow_email ? t`In the app and by e-mail` : t`Only in the app`])
  return rows
}

export function FeedbackDetails({ feedback }: { feedback: Feedback }) {
  const { t } = useLingui()
  const text = useEnumText()
  const rows = useRows(feedback)
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    const lines = [`${t`Category`}: ${text.category[feedback.category]}`, ...rows.map(([k, v]) => `${k}: ${v}`)]
    await navigator.clipboard.writeText(lines.join("\n"))
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  useHotkeys({ [SHORTCUTS.copyDetails]: () => void copy() })
  return (
    <div className="flex items-start gap-2 rounded-lg border bg-muted/40 px-2.5 py-2" data-testid="feedback-details">
      <dl className="grid min-w-0 flex-1 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-0.5 text-xs sm:grid-cols-[auto_minmax(0,1fr)_auto_minmax(0,1fr)]">
        {rows.map(([k, v, mono]) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{k}</dt>
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
        onClick={() => void copy()}
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
