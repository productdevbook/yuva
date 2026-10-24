import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { toast } from "@/components/common/Toast"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { BulkFailure, BulkResult } from "@/lib/api"

function usedBy(f: BulkFailure) {
  const u = f.used_by
  if (!u) return ""
  return [
    ...u.designs.map((d) => d.name),
    ...(u.help_articles ?? []).map((a) => a.title),
    ...(u.inbox_signatures ?? []).map((i) => i.name),
    ...(u.member_signatures ?? []).map((m) => m.name),
  ].join(", ")
}

function Reason({ failure }: { failure: BulkFailure }) {
  const names = usedBy(failure)
  if (failure.code === "image_in_use") return <Trans>In use by {names}</Trans>
  if (failure.code === "not_found") return <Trans>It is gone, or you cannot see it</Trans>
  if (failure.code === "forbidden") return <Trans>You may not do this</Trans>
  return <>{failure.detail ?? failure.code}</>
}

type Report = { failures: BulkFailure[]; name: (id: string) => string }

export function useBulkReport() {
  const { t } = useLingui()
  const [report, setReport] = useState<Report | null>(null)
  const count = report?.failures.length ?? 0
  const show = (result: BulkResult, name: (id: string) => string, undo?: () => void) => {
    const changed = result.succeeded.length
    if (changed > 0) toast(t`${changed} changed`, undo)
    if (result.failed.length > 0) setReport({ failures: result.failed, name })
  }
  const dialog = (
    <Dialog open={report !== null} onOpenChange={(o) => !o && setReport(null)}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-md" data-testid="bulk-refusals">
        <DialogHeader>
          <DialogTitle>
            <Trans>{count} could not be changed</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>The others were changed.</Trans>
          </DialogDescription>
        </DialogHeader>
        <ul className="flex flex-col gap-2.5">
          {report?.failures.map((f) => (
            <li key={f.id} className="flex min-w-0 flex-col" data-testid="bulk-refusal">
              <span className="truncate font-medium">{report.name(f.id)}</span>
              <span className="text-small text-destructive">
                <Reason failure={f} />
              </span>
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button onClick={() => setReport(null)}>
            <Trans>Done</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
  return [show, dialog] as const
}
