import { useThreadSort, useThreadView, viewFilters } from "@/features/docs/discussions/queries"
import { usePickedSite } from "@/features/docs/Toolbar"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import type { DiscussionFilters } from "@/lib/keys"

export function useThreadFilters(): DiscussionFilters {
  const [inboxId] = useInboxFilter()
  const site = usePickedSite(inboxId)
  const [view] = useThreadView()
  const [sort] = useThreadSort()
  return { inbox_id: inboxId || undefined, site: site || undefined, sort, ...viewFilters(view) }
}
