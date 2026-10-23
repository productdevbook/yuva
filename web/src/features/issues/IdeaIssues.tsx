import { Trans, useLingui } from "@lingui/react/macro"
import { ExternalLinkIcon, LinkIcon, PlusIcon, XIcon } from "lucide-react"
import { useState } from "react"

import { ROADMAP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { ideaDraft } from "@/features/issues/draft"
import { IssueState, useUnlinkIssue } from "@/features/issues/IssueChips"
import { IssueDialogs, type IssueMode } from "@/features/issues/IssueMenu"
import { providerName } from "@/features/issues/providers"
import { Overline, look } from "@/features/inbox/PageFrame"
import { Card, EmptyRow } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Discussion } from "@/lib/api"
import { cn } from "@/lib/utils"

export function IdeaIssues({ idea, hotkeys }: { idea: Discussion; hotkeys: boolean }) {
  const { t } = useLingui()
  const subject = { kind: "idea", id: idea.id } as const
  const [mode, setMode] = useState<IssueMode | null>(null)
  const { unlink, pending } = useUnlinkIssue(subject)
  const items = idea.issues ?? []
  useHotkeys(
    {
      [ROADMAP_KEYS.createIssue]: () => setMode("create"),
      [ROADMAP_KEYS.linkIssue]: () => setMode("link"),
    },
    hotkeys && !mode
  )
  return (
    <section className="flex flex-col gap-2.5" data-testid="idea-issues-section">
      <div className="flex items-center gap-2">
        <Overline>
          <Trans>Issues</Trans>
        </Overline>
        <span className={cn(look.faint, "flex-1 tabular-nums")}>{items.length}</span>
        <Button
          variant="outline"
          className={look.btnSm}
          onClick={() => setMode("create")}
          title={`${t`Create issue`} (${keyLabel(ROADMAP_KEYS.createIssue).join("")})`}
          data-testid="idea-issue-create"
        >
          <PlusIcon />
          <Trans>Create issue</Trans>
        </Button>
        <Button
          variant="outline"
          className={look.btnSm}
          onClick={() => setMode("link")}
          title={`${t`Link issue`} (${keyLabel(ROADMAP_KEYS.linkIssue).join("")})`}
          data-testid="idea-issue-link"
        >
          <LinkIcon />
          <Trans>Link issue</Trans>
        </Button>
      </div>
      <Card flush data-testid="idea-issues">
        {items.length === 0 ? (
          <EmptyRow>
            <Trans>No issue is linked to this idea.</Trans>
          </EmptyRow>
        ) : (
          <ul className="divide-y">
            {items.map((l) => {
              const key = l.key
              return (
                <li key={l.id} className="flex items-center gap-3 px-4 py-2.5" data-testid="idea-issue">
                  <a
                    href={l.url}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="flex min-w-0 flex-1 flex-col outline-none hover:underline focus-visible:underline"
                  >
                    <span className="flex items-center gap-1.5 text-body font-medium">
                      <span className="truncate">{l.title}</span>
                      <ExternalLinkIcon className="size-3.5 shrink-0 opacity-60" />
                    </span>
                    <span className="truncate text-caption text-faint">
                      {providerName(l.provider)} · {l.key}
                    </span>
                  </a>
                  <IssueState link={l} />
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    disabled={pending}
                    onClick={() => unlink(l)}
                    aria-label={t`Unlink ${key}`}
                    title={t`Unlink ${key}`}
                    data-testid="idea-issue-unlink"
                  >
                    <XIcon />
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
      <p className={look.faint}>
        <Trans>An idea only shows its issues and their state. Its stage stays your decision.</Trans>
      </p>
      <IssueDialogs subject={subject} mode={mode} draft={ideaDraft(idea)} onClose={() => setMode(null)} />
    </section>
  )
}
