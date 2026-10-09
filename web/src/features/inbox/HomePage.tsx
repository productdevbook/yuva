import { ListPage } from "@/features/inbox/ListPage"
import { QueuePage } from "@/features/inbox/QueuePage"
import { useView } from "@/lib/view"

export function HomePage() {
  const [view] = useView()
  return view === "list" ? <ListPage /> : <QueuePage />
}
