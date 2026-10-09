import { useLingui } from "@lingui/react/macro"

import { Dot } from "@/components/common"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useInboxFilter, useQueue } from "@/features/inbox/queue"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const ALL = "all"

export function InboxPicker({ className }: { className?: string }) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const [value, set] = useInboxFilter()
  const { setCurrent } = useQueue()
  if (inboxes.length < 2) return null
  const items: Record<string, string> = { [ALL]: t`All inboxes`, ...Object.fromEntries(inboxes.map((i) => [i.id, i.name])) }
  return (
    <Select
      value={value || ALL}
      items={items}
      onValueChange={(v) => {
        set(!v || v === ALL ? "" : String(v))
        setCurrent(null)
      }}
    >
      <SelectTrigger
        aria-label={t`Inbox`}
        size="sm"
        className={cn("max-w-48 border-transparent bg-transparent ps-2 pe-2 text-muted-foreground shadow-none hover:border-transparent hover:bg-muted hover:text-foreground", className)}
        data-testid="inbox-picker"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent className="min-w-52">
        <SelectItem value={ALL}>{t`All inboxes`}</SelectItem>
        {inboxes.map((i) => (
          <SelectItem key={i.id} value={i.id}>
            <Dot color={i.branding.color} className="mt-1.5 size-2 rounded-[3px]" />
            {i.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
