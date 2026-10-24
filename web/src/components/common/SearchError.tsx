import { useLingui } from "@lingui/react/macro"

import { useErrorText } from "@/components/common/text"
import { ApiError } from "@/lib/api"
import { cn } from "@/lib/utils"

function searchField(error: unknown) {
  if (!(error instanceof ApiError) || error.status !== 400) return null
  if (error.field === "q" || error.field === "timezone") return error.field
  return /^(q|timezone) /.test(error.detail ?? "") ? (error.detail ?? "").split(" ")[0] : null
}

export function isSearchError(error: unknown) {
  return searchField(error) !== null
}

export function SearchError({ error, className }: { error: unknown; className?: string }) {
  const { t } = useLingui()
  const text = useErrorText()
  if (!isSearchError(error)) return null
  const e = error as ApiError
  const message =
    searchField(e) === "timezone"
      ? t`This browser's time zone is not recognised, so dates in the search cannot be read.`
      : e.detail
        ? t`This search cannot be used: ${e.detail}`
        : text(error)
  return (
    <p role="alert" className={cn("text-small text-destructive", className)} data-testid="search-error">
      {message}
    </p>
  )
}
