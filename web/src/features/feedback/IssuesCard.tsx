import { Trans, useLingui } from "@lingui/react/macro"
import { CircleDotIcon, ExternalLinkIcon, LinkIcon, PlusIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { IssueState, useUnlinkIssue } from "@/features/issues/IssueChips"
import { providerName } from "@/features/issues/providers"
import { Overline, look } from "@/features/inbox/PageFrame"
import type { Conversation } from "@/lib/api"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"

export function IssuesCard({ c, onCreate, onLink }: { c: Conversation; onCreate: () => void; onLink: () => void }) {
  const { t } = useLingui()
  const { unlink, pending } = useUnlinkIssue({ kind: "conversation", id: c.id })
  const items = c.issues ?? []
  return (
    <section className="flex flex-col gap-2.5 border-b px-5 py-4" data-testid="feedback-issues">
      <Overline>
        <Trans>Issues</Trans>
      </Overline>
      {items.length > 0 && (
        <ul className="flex flex-col gap-2">
          {items.map((l) => {
            const key = l.key
            return (
              <li
                key={l.id}
                className="flex items-start gap-2.5 rounded-[10px] border bg-surface p-3"
                data-testid="issue-row"
              >
                <span className="grid size-7 shrink-0 place-items-center rounded-lg bg-brand-wash text-brand">
                  <CircleDotIcon className="size-[15px]" />
                </span>
                <a
                  href={l.url}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="flex min-w-0 flex-1 flex-col leading-[1.35] outline-none hover:underline focus-visible:underline"
                >
                  <span className="flex items-center gap-1 font-semibold" dir="auto">
                    <span className="min-w-0 truncate">{l.title}</span>
                    <ExternalLinkIcon className="size-3.5 shrink-0 opacity-60" />
                  </span>
                  <span className={look.faint}>
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
      {items.length > 0 && (
        <p className={look.faint}>
          <Trans>The issue's state sets the progress the person sees.</Trans>
        </p>
      )}
    </section>
  )
}
