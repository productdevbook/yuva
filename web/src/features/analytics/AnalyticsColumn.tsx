import "@/features/analytics/i18n"

import { Trans, useLingui } from "@lingui/react/macro"
import { GaugeIcon } from "lucide-react"

import { Notice } from "@/components/common"
import type { AnalyticsUsage } from "@/lib/api"

export function UsageBanner({ usage, className }: { usage: AnalyticsUsage; className?: string }) {
  const { i18n } = useLingui()
  if (!usage.limit) return null
  const fmt = new Intl.NumberFormat(i18n.locale)
  const used = fmt.format(usage.events)
  const limit = fmt.format(usage.limit)
  if (usage.limit_reached_at) {
    return (
      <Notice tone="danger" icon={GaugeIcon} className={className} data-testid="analytics-cap-reached">
        <Trans>
          This workspace reached its {limit} analytics events for this month. New events are dropped until the month
          ends.
        </Trans>
      </Notice>
    )
  }
  if (usage.events < usage.limit * 0.8) return null
  return (
    <Notice tone="warning" icon={GaugeIcon} className={className} data-testid="analytics-cap-near">
      <Trans>
        {used} of {limit} analytics events used this month. Events over the limit are dropped until the month ends.
      </Trans>
    </Notice>
  )
}
