import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon } from "lucide-react"
import { Link, Navigate, useParams } from "react-router"

import { Button } from "@/components/ui/button"
import { PageTop } from "@/features/inbox/PageFrame"
import { mailPath, useFilterAccess, useMailInboxes } from "@/features/mail/folders"
import { RuleList } from "@/features/settings/automation/RuleList"
import { useInboxes } from "@/lib/workspace"

export function MailFiltersPage() {
  const { t } = useLingui()
  const { box = "" } = useParams()
  const loading = useInboxes().isPending
  const inboxes = useMailInboxes()
  const allowed = useFilterAccess(box)
  const inbox = inboxes.find((i) => i.id === box)
  if (loading) return null
  if (!inbox || !allowed) return <Navigate to="/mail" replace />
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="mail-filters">
      <PageTop
        title={<Trans>Filters</Trans>}
        sub={inbox.name}
        actions={
          <Button
            variant="outline"
            className="h-8 gap-1.5 rounded-[9px] border-input bg-card px-3 font-normal"
            render={<Link to={mailPath(box, "inbox")} />}
            aria-label={t`Back to the mail`}
          >
            <ArrowLeftIcon className="rtl:rotate-180" />
            <Trans>Mail</Trans>
          </Button>
        }
      />
      <div className="min-h-0 flex-1 overflow-y-auto bg-background px-6 py-5 phone:px-4">
        <div className="flex max-w-3xl flex-col gap-4">
          <RuleList
            inboxId={box}
            filter
            intro={
              <Trans>
                A filter looks at every mail that arrives here and stars, archives, trashes, labels or hands it to a
                member. Filters run in the order of this list.
              </Trans>
            }
          />
        </div>
      </div>
    </main>
  )
}
