import { useParams } from "react-router"

import { BroadcastPage } from "@/features/broadcasts/BroadcastPage"
import { useBroadcast } from "@/features/broadcasts/broadcastQueries"
import { TriggeredPage } from "@/features/broadcasts/TriggeredPage"

export function BroadcastRoute() {
  const { broadcastId = "" } = useParams()
  const b = useBroadcast(broadcastId).data
  return b?.kind === "triggered" ? <TriggeredPage key={b.id} b={b} /> : <BroadcastPage />
}
