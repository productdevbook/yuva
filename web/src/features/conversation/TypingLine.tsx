import { Trans, useLingui } from "@lingui/react/macro"

import { TypingDots } from "@/components/common"
import { useTyping } from "@/lib/typing"
import { useMemberMap } from "@/lib/workspace"

export function TypingLine({ conversationId, contactName }: { conversationId: string; contactName: string }) {
  const { t, i18n } = useLingui()
  const members = useMemberMap()
  const typists = useTyping(conversationId)
  if (typists.length === 0) return null
  const names = typists.map((a) => {
    if (a.type === "contact") return contactName
    const m = a.member_id ? members.get(a.member_id) : undefined
    return a.name || m?.name || m?.email || t`A teammate`
  })
  const list = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(names)
  return (
    <p className="mx-auto flex w-full max-w-3xl shrink-0 items-center gap-2 px-5 pb-1.5 text-xs text-faint sm:px-7" role="status" data-testid="typing">
      <TypingDots />
      {names.length === 1 ? <Trans>{list} is typing…</Trans> : <Trans>{list} are typing…</Trans>}
    </p>
  )
}
