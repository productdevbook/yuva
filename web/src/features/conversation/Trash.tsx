import { Trans, useLingui } from "@lingui/react/macro"
import { Trash2Icon } from "lucide-react"

import { Kbd } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { Alert } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { look } from "@/features/inbox/PageFrame"
import type { Conversation } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useMemberMap } from "@/lib/workspace"

export function useTrashFacts(c: Conversation) {
  const { t, i18n } = useLingui()
  const members = useMemberMap()
  const day = (iso?: string) =>
    iso ? new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium" }).format(new Date(iso)) : ""
  const by = c.trashed_by
  const member = by?.member_id ? members.get(by.member_id) : undefined
  const who = member ? member.name || member.email : by?.api_key_id ? t`an API key` : ""
  const date = day(c.trashed_at)
  const until = day(c.deletes_at)
  return {
    deleted: who ? t`Deleted by ${who} on ${date}` : t`Deleted on ${date}`,
    until: until ? t`Deleted for good on ${until}` : "",
  }
}

export function TrashLine({ c }: { c: Conversation }) {
  const facts = useTrashFacts(c)
  return (
    <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint" data-testid="row-trash">
      <Trash2Icon className="size-3.5 shrink-0" />
      <span className="min-w-0 truncate">{[facts.deleted, facts.until].filter(Boolean).join(" · ")}</span>
    </span>
  )
}

export function TrashBanner({ c, onRestore, busy }: { c: Conversation; onRestore: () => void; busy: boolean }) {
  const { canManage } = useSession()
  const facts = useTrashFacts(c)
  return (
    <Alert
      role="status"
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 rounded-none border-x-0 border-t-0 border-b bg-surface px-4 py-2.5 text-small"
      data-testid="trash-banner"
    >
      <Trash2Icon className="size-4 shrink-0 text-faint" />
      <span className="min-w-0 flex-1">
        <b className="font-medium">
          <Trans>This conversation is in the Trash and read-only.</Trans>
        </b>{" "}
        <span className="text-muted-foreground">{[facts.deleted, facts.until].filter(Boolean).join(". ")}</span>
      </span>
      {canManage && (
        <Button className={look.btnInkSm} onClick={onRestore} disabled={busy} data-testid="restore-conversation">
          <Trans>Restore</Trans>
          <Kbd>{SHORTCUTS.restore}</Kbd>
        </Button>
      )}
    </Alert>
  )
}
