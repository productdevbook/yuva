import { useLingui } from "@lingui/react/macro"
import { CircleDotIcon, ExternalLinkIcon, XIcon } from "lucide-react"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { progressClass } from "@/features/conversation/Feedback"
import { useIssueLinkMutations, type IssueSubject } from "@/features/issues/queries"
import type { IssueLink } from "@/lib/api"
import { cn } from "@/lib/utils"

export function IssueState({ link, className }: { link: IssueLink; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex max-w-32 shrink-0 items-center rounded-full px-2 py-px text-caption font-medium",
        progressClass[link.progress],
        className
      )}
      data-testid="issue-state"
      data-progress={link.progress}
      title={link.state}
    >
      <span className="truncate">{link.state}</span>
    </span>
  )
}

export function useUnlinkIssue(subject: IssueSubject) {
  const errorText = useErrorText()
  const { remove } = useIssueLinkMutations(subject)
  return {
    pending: remove.isPending,
    unlink: (link: IssueLink) => remove.mutate(link.id, { onError: (e) => toast(errorText(e)) }),
  }
}

export function IssueChips({ subject, items }: { subject: IssueSubject; items: IssueLink[] }) {
  const { t } = useLingui()
  const { unlink, pending } = useUnlinkIssue(subject)
  return (
    <>
      {items.map((l) => {
        const key = l.key
        return (
          <span
            key={l.id}
            className="inline-flex max-w-full items-center rounded-full border bg-card text-caption text-muted-foreground"
            data-testid="issue-chip"
          >
            <a
              href={l.url}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex min-w-0 items-center gap-1 rounded-s-full py-px ps-2 pe-1 outline-none hover:text-foreground focus-visible:text-foreground"
              title={l.title}
            >
              <CircleDotIcon className="size-3 shrink-0" />
              <span className="truncate">{l.key}</span>
              <IssueState link={l} className="py-0" />
              <ExternalLinkIcon className="size-3 shrink-0 opacity-60" />
            </a>
            <button
              type="button"
              onClick={() => unlink(l)}
              disabled={pending}
              className="grid size-5 shrink-0 place-items-center rounded-e-full outline-none hover:text-foreground focus-visible:text-foreground"
              aria-label={t`Unlink ${key}`}
              title={t`Unlink ${key}`}
              data-testid="issue-chip-unlink"
            >
              <XIcon className="size-3" />
            </button>
          </span>
        )
      })}
    </>
  )
}
