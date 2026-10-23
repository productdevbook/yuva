import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import { ErrorLine } from "@/components/common"
import { Segmented } from "@/components/common/Segmented"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useRuleRuns } from "@/features/settings/automation/queries"
import { OUTCOMES, useAutomationText } from "@/features/settings/automation/text"
import { Pill } from "@/features/settings/ui"
import type { AutomationRule, AutomationRun, AutomationRunOutcome } from "@/lib/api"
import { useState } from "react"

function OutcomePill({ outcome }: { outcome: AutomationRunOutcome }) {
  const text = useAutomationText()
  return (
    <Pill tone={outcome === "applied" ? "success" : outcome === "failed" ? "danger" : "muted"}>
      {text.outcome[outcome]}
    </Pill>
  )
}

function RunRow({ run }: { run: AutomationRun }) {
  const { i18n } = useLingui()
  const text = useAutomationText()
  const reason = run.error ? (text.reason[run.error] ?? run.error) : undefined
  return (
    <li className="flex flex-col gap-1 border-b px-1 py-3 last:border-b-0" data-testid="automation-run">
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
        <OutcomePill outcome={run.outcome} />
        <time dateTime={run.created_at} className="text-caption text-muted-foreground">
          {formatDateTime(run.created_at, i18n.locale)}
        </time>
        <span className="text-caption text-faint">{text.trigger[run.trigger]}</span>
        <Link
          to={`/conversations/${run.conversation_id}`}
          className="ms-auto text-caption font-medium text-brand hover:underline"
        >
          <Trans>Open conversation</Trans>
        </Link>
      </div>
      {reason && (
        <p className="text-body">
          <span className="font-medium">{reason}</span>
          {run.detail && <span className="text-muted-foreground"> {run.detail}</span>}
        </p>
      )}
      {run.actions.length > 0 && (
        <p className="text-caption text-faint">
          {run.actions.map((a) => `${text.action[a.type]}: ${text.result[a.result]}`).join(" · ")}
        </p>
      )}
    </li>
  )
}

function Log({ rule }: { rule: AutomationRule }) {
  const { t } = useLingui()
  const text = useAutomationText()
  const [outcome, setOutcome] = useState<AutomationRunOutcome | "all">("all")
  const runs = useRuleRuns(rule.id, outcome)
  const items = runs.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <div className="flex flex-col gap-3">
      <Segmented<AutomationRunOutcome | "all">
        value={outcome}
        onChange={setOutcome}
        label={t`Outcome`}
        className="self-start"
        items={[
          { value: "all", label: t`All`, testId: "runs-all" },
          ...OUTCOMES.map((o) => ({ value: o, label: text.outcome[o], testId: `runs-${o}` })),
        ]}
      />
      <ErrorLine error={runs.error} />
      {runs.isPending ? null : items.length === 0 ? (
        <p className="py-6 text-center text-faint" data-testid="runs-empty">
          <Trans>No runs in the last 30 days.</Trans>
        </p>
      ) : (
        <ul>
          {items.map((r) => (
            <RunRow key={r.id} run={r} />
          ))}
        </ul>
      )}
      {runs.hasNextPage && (
        <Button
          variant="outline"
          className="self-center"
          disabled={runs.isFetchingNextPage}
          onClick={() => void runs.fetchNextPage()}
        >
          <Trans>Show more</Trans>
        </Button>
      )}
    </div>
  )
}

export function RunLog({ rule, onClose }: { rule: AutomationRule; onClose: () => void }) {
  const name = rule.name
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <Trans>Log of {name}</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Every run of the last 30 days, newest first.</Trans>
          </DialogDescription>
        </DialogHeader>
        <Log rule={rule} />
      </DialogContent>
    </Dialog>
  )
}
