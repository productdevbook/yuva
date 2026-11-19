import type { RoadmapBoard } from "@/lib/api"

export const openIdeas = (b: RoadmapBoard) =>
  b.counts.ideas + b.counts.planned + b.counts.in_progress + b.counts.shipped
