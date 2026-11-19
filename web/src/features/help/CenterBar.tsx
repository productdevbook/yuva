import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import { useConfirm } from "@/components/common"
import { useUpdateHelpCenter } from "@/features/help/queries"

export function DocsNote() {
  return (
    <p className="m-0 text-small text-muted-foreground" data-testid="help-docs-note">
      <Trans>
        Written here, shown on pages Yuva hosts, searched from the chat widget and the reply box. Documentation pages on
        your own site live under{" "}
        <Link to="/docs" className="text-brand hover:underline">
          Docs
        </Link>
        .
      </Trans>
    </p>
  )
}

export function usePublishToggle(centerId: string, published: boolean | undefined) {
  const { t } = useLingui()
  const patch = useUpdateHelpCenter(centerId)
  const [confirm, dialog] = useConfirm()
  const toggle = () => {
    if (!published) return patch.mutate({ published: true })
    confirm({
      title: t`Unpublish this help center?`,
      description: t`Visitors can no longer open it and the chat widget stops suggesting its articles. Your articles stay as they are.`,
      confirm: t`Unpublish`,
      run: () => patch.mutate({ published: false }),
    })
  }
  return { toggle, dialog, pending: patch.isPending, error: patch.error }
}
