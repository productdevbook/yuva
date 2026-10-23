import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronDownIcon, CircleDotIcon, LinkIcon, PlusIcon } from "lucide-react"
import { lazy, Suspense, useState } from "react"

import { Kbd } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import type { IssueDraft } from "@/features/issues/IssueDialog"
import type { IssueSubject } from "@/features/issues/queries"
import { look } from "@/features/inbox/PageFrame"

const IssueDialog = lazy(() => import("@/features/issues/IssueDialog"))

export type IssueMode = "create" | "link"

export function IssueDialogs({
  subject,
  mode,
  draft,
  onClose,
}: {
  subject: IssueSubject
  mode: IssueMode | null
  draft: IssueDraft
  onClose: () => void
}) {
  if (!mode) return null
  return (
    <Suspense fallback={null}>
      <IssueDialog subject={subject} mode={mode} draft={draft} onClose={onClose} />
    </Suspense>
  )
}

export function IssueMenu({
  keys = [SHORTCUTS.createIssue, SHORTCUTS.linkIssue],
  onPick,
  count,
}: {
  keys?: [string, string]
  onPick: (mode: IssueMode) => void
  count: number
}) {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger
        render={<Button variant="outline" className={look.btnSm} />}
        title={`${t`Issue`} (${keys[0]})`}
        data-testid="issue-menu"
      >
        <CircleDotIcon />
        {count > 0 ? <Trans>Issues</Trans> : <Trans>Issue</Trans>}
        <ChevronDownIcon className="text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-48">
        <DropdownMenuItem onClick={() => onPick("create")} data-testid="issue-create">
          <PlusIcon />
          <span className="flex-1">
            <Trans>Create issue</Trans>
          </span>
          <Kbd>{keys[0]}</Kbd>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => onPick("link")} data-testid="issue-link">
          <LinkIcon />
          <span className="flex-1">
            <Trans>Link issue</Trans>
          </span>
          <Kbd>{keys[1]}</Kbd>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
