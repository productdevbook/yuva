import { useLingui } from "@lingui/react/macro"

import { useInboxFilter, useQueue } from "@/features/inbox/queue"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const chevron =
  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 10 10'%3E%3Cpath d='M2 4l3 3 3-3' fill='none' stroke='%238a8a93' stroke-width='1.5'/%3E%3C/svg%3E\")"

export function InboxPicker({ className }: { className?: string }) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const [value, set] = useInboxFilter()
  const { setCurrent } = useQueue()
  if (inboxes.length < 2) return null
  return (
    <select
      value={value}
      onChange={(e) => {
        set(e.target.value)
        setCurrent(null)
      }}
      aria-label={t`Inbox`}
      className={cn(
        "max-w-48 cursor-pointer appearance-none truncate rounded-lg bg-transparent bg-[right_9px_center] bg-no-repeat py-1.5 ps-2 pe-7 text-sm text-muted-foreground transition-colors outline-none hover:bg-muted hover:text-foreground",
        className,
      )}
      style={{ backgroundImage: chevron }}
      data-testid="inbox-picker"
    >
      <option value="">{t`All inboxes`}</option>
      {inboxes.map((i) => (
        <option key={i.id} value={i.id}>
          {i.name}
        </option>
      ))}
    </select>
  )
}
