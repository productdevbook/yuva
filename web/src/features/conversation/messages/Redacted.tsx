import { Trans, useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { TrashIcon } from "lucide-react"

import { formatDateTime } from "@/components/common/text"
import { Marker, MarkerContent } from "@/components/ui/marker"
import { timeOf, type ThreadContext } from "@/features/conversation/messages/context"
import { api, unwrap, type Message } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function useRemover(m: Message, ctx: ThreadContext) {
  const { t } = useLingui()
  const { workspaceId: ws, canManage } = useSession()
  const by = m.redacted_by
  const keyId = by?.member_id ? undefined : by?.api_key_id
  const apiKeys = useQuery({
    queryKey: keys.apiKeys(ws),
    queryFn: () => unwrap(api.GET("/v1/api-keys")).then((r) => r.items),
    enabled: canManage && !!keyId,
  })
  if (by?.member_id) {
    const member = ctx.members.get(by.member_id)
    return member ? member.name || member.email : t`a former member`
  }
  if (keyId) {
    const key = apiKeys.data?.find((k) => k.id === keyId)
    return key ? key.bot_name || key.name : t`an API key`
  }
  return t`a former member`
}

export function RedactedLine({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { i18n } = useLingui()
  const who = useRemover(m, ctx)
  const at = m.redacted_at ?? m.created_at
  return (
    <Marker className="justify-center py-2" data-testid="redacted-line" data-message-id={m.id}>
      <MarkerContent className="max-w-full rounded-full border border-dashed bg-card px-2.5 py-[3px] text-center text-caption text-faint">
        <TrashIcon aria-hidden className="me-1 inline size-3 align-[-1px]" />
        <Trans>Removed by {who}</Trans>
        <span aria-hidden> · </span>
        <time dateTime={at} title={formatDateTime(at, i18n.locale)}>
          {timeOf(at, i18n.locale)}
        </time>
      </MarkerContent>
    </Marker>
  )
}
