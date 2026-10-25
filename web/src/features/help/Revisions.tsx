import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { useHelpRevisions } from "@/features/help/queries"
import { ErrorLine } from "@/features/help/text"
import type { HelpArticleRevision } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useMemberMap } from "@/lib/workspace"

function Author({ r }: { r: HelpArticleRevision }) {
  const members = useMemberMap()
  if (r.author_member_id) {
    const m = members.get(r.author_member_id)
    return <>{m ? m.name || m.email : <Trans>A former teammate</Trans>}</>
  }
  if (r.via) return <>{r.via}</>
  return <Trans>An API key</Trans>
}

export function Revisions({
  centerId,
  articleId,
  locale,
  restoring,
  error,
  onRestore,
  onClose,
}: {
  centerId: string
  articleId: string
  locale: string
  restoring: boolean
  error: unknown
  onRestore: (r: HelpArticleRevision) => void
  onClose: () => void
}) {
  const { i18n } = useLingui()
  const revisions = useHelpRevisions(centerId, articleId, locale, true)
  const [open, setOpen] = useState<string | null>(null)
  const kinds = { save: <Trans>Saved</Trans>, publish: <Trans>Published</Trans>, restore: <Trans>Restored</Trans> }
  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-[min(28rem,94vw)]" data-testid="help-revisions">
        <SheetHeader>
          <SheetTitle>
            <Trans>Revisions</Trans>
          </SheetTitle>
          <SheetDescription>
            <Trans>
              Earlier versions of this translation. Restoring one replaces the draft; the published text stays until you
              publish.
            </Trans>
          </SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-4 pb-4">
          <ErrorLine error={error ?? revisions.error} />
          {revisions.isPending ? (
            <Skeleton className="h-20 w-full rounded-xl" />
          ) : (revisions.data ?? []).length === 0 ? (
            <p className="py-6 text-center text-body text-faint">
              <Trans>No earlier versions yet.</Trans>
            </p>
          ) : (
            <ul className="flex flex-col gap-2">
              {(revisions.data ?? []).map((r) => (
                <li key={r.id} className="rounded-[14px] border bg-card" data-testid="help-revision">
                  <button
                    type="button"
                    className="flex w-full flex-col items-start gap-0.5 px-3 py-2.5 text-start outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    aria-expanded={open === r.id}
                    onClick={() => setOpen(open === r.id ? null : r.id)}
                  >
                    <span className="flex w-full items-center gap-2">
                      <span className="min-w-0 flex-1 truncate text-body font-medium">{r.title}</span>
                      <span className="shrink-0 text-caption text-faint">{kinds[r.kind]}</span>
                    </span>
                    <span className="text-caption text-faint">
                      <Author r={r} /> · {formatDateTime(r.created_at, i18n.locale)}
                    </span>
                  </button>
                  {open === r.id && (
                    <div className="flex flex-col gap-2 border-t px-3 py-2.5">
                      <div
                        className={cn(
                          "max-h-56 overflow-y-auto font-mono text-small break-words whitespace-pre-wrap text-muted-foreground"
                        )}
                        dir="auto"
                      >
                        {r.body || <Trans>Empty</Trans>}
                      </div>
                      <Button
                        size="sm"
                        variant="outline"
                        className="self-end"
                        disabled={restoring}
                        onClick={() => onRestore(r)}
                        data-testid="help-restore"
                      >
                        <Trans>Restore this version</Trans>
                      </Button>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
