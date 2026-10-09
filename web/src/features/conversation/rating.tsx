import { useLingui } from "@lingui/react/macro"

import type { Conversation, Rating } from "@/lib/api"

export function currentRating(c: Pick<Conversation, "rating" | "closed_at" | "status">) {
  if (c.status !== "closed" || !c.rating) return undefined
  if (c.closed_at && c.rating.rated_at < c.closed_at) return undefined
  return c.rating
}

export function RatingMark({ rating, comment }: { rating: Rating; comment?: string }) {
  const { t } = useLingui()
  const label = rating === "good" ? t`Rated good` : t`Rated bad`
  return (
    <span role="img" aria-label={label} title={comment ? `${label} · “${comment}”` : label} data-testid="rating" data-rating={rating}>
      {rating === "good" ? "👍" : "👎"}
    </span>
  )
}
