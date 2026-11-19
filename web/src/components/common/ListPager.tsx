import { useLingui } from "@lingui/react/macro"
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

export function ListPager({
  at,
  total,
  more,
  place,
  onPrevious,
  onNext,
  previousLabel,
  nextLabel,
}: {
  at: number
  total: number
  more?: boolean
  place: string
  onPrevious: () => void
  onNext: () => void
  previousLabel: string
  nextLabel: string
}) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const n = fmt.format(at + 1)
  const of = `${fmt.format(total)}${more ? "+" : ""}`
  return (
    <div className="flex items-center gap-1" data-testid="list-pager">
      {at >= 0 && (
        <span className="me-1 text-small text-muted-foreground tabular-nums max-md:hidden" data-testid="list-pager-at">
          {t`${n} of ${of} in ${place}`}
        </span>
      )}
      <Button
        variant="ghost"
        size="icon-sm"
        className="size-[30px]"
        onClick={onPrevious}
        disabled={at <= 0}
        aria-label={previousLabel}
        title={previousLabel}
        data-testid="list-pager-previous"
      >
        <ChevronUpIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        className="size-[30px]"
        onClick={onNext}
        disabled={at < 0 || at >= total - 1}
        aria-label={nextLabel}
        title={nextLabel}
        data-testid="list-pager-next"
      >
        <ChevronDownIcon />
      </Button>
    </div>
  )
}
