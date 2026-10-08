import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import { useConversation } from "@/features/conversation/queries"

export function RelatedLine({ id, hrefFor }: { id: string; hrefFor: (id: string) => string }) {
  const { t } = useLingui()
  const related = useConversation(id)
  if (related.isPending) return null
  const title = related.data?.subject || t`No subject`
  return (
    <p className="min-w-0 truncate text-xs text-faint" data-testid="related-conversation">
      {related.data ? (
        <Trans>
          Replied in the thread of{" "}
          <Link to={hrefFor(id)} className="font-medium text-muted-foreground underline-offset-2 hover:text-foreground hover:underline">
            {title}
          </Link>
        </Trans>
      ) : (
        <Trans>Replied in the thread of another conversation</Trans>
      )}
    </p>
  )
}
