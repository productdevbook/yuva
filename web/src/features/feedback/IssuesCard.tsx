import { Trans, useLingui } from "@lingui/react/macro"
import { CircleDotIcon, ExternalLinkIcon, LinkIcon, PlusIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { IssueState, useUnlinkIssue } from "@/features/issues/IssueChips"
import { providerName } from "@/features/issues/providers"
import { SectionHeading } from "@/components/common"
import { look } from "@/features/inbox/PageFrame"
import type { Conversation } from "@/lib/api"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"

export function IssuesCard({ c, onCreate, onLink }: { c: Conversation; onCreate: () => void; onLink: () => void }) {
  const { t } = useLingui()
  const { unlink, pending } = useUnlinkIssue({ kind: "conversation", id: c.id })
  const items = c.issues ?? []
  return (
    <section data-testid="feedback-issues">
      <SectionHeading
        title={<Trans>Issues</Trans>}
        lead={items.length > 0 && <Trans>The issue's state sets the progress the person sees.</Trans>}
      />
      {items.length > 0 && (
        <ul className="m-0 mb-3 flex list-none flex-col border-t border-line-soft p-0">
          {items.map((l) => {
            const key = l.key
            return (
              <li key={l.id} className="flex items-center gap-3 border-b border-line-soft py-3" data-testid="issue-row">
                <CircleDotIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                <a
                  href={l.url}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="flex min-w-0 flex-1 flex-col leading-[1.35] outline-none hover:underline focus-visible:underline"
                >
                  <span className="flex items-center gap-1 font-medium text-foreground" dir="auto">
                    <span className="min-w-0 truncate">{l.title}</span>
                    <ExternalLinkIcon className="size-3.5 shrink-0 opacity-60" />
                  </span>
                  <span className="text-small text-muted-foreground">
                    {providerName(l.provider)} · {l.key}
                  </span>
                </a>
                <IssueState link={l} />
                <Button
                  variant="ghost"
                  size="icon-xs"
                  className="text-faint"
                  disabled={pending}
                  onClick={() => unlink(l)}
                  aria-label={t`Unlink ${key}`}
                  title={t`Unlink ${key}`}
                  data-testid="issue-unlink"
                >
                  <XIcon />
                </Button>
              </li>
            )
          })}
        </ul>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          className={look.btnSm}
          onClick={onCreate}
          title={t`Create issue (${SHORTCUTS.createIssue})`}
          data-testid="issue-create"
        >
          <PlusIcon />
          <Trans>Create issue</Trans>
        </Button>
        <Button
          variant="outline"
          className={look.btnSm}
          onClick={onLink}
          title={t`Link issue (${SHORTCUTS.linkIssue})`}
          data-testid="issue-link"
        >
          <LinkIcon />
          <Trans>Link issue</Trans>
        </Button>
      </div>
    </section>
  )
}
