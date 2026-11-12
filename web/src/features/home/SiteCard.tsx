import { useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import type { HomeProperty } from "@/lib/api"

export function SiteRow({ property: p }: { property: HomeProperty }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const short = new Intl.DateTimeFormat(i18n.locale, { weekday: "short", timeZone: "UTC" })
  const name = p.name
  const values = p.days.map((d) => d.visitors)
  const max = Math.max(1, ...values)
  const total = fmt.format(values.reduce((a, b) => a + b, 0))
  const series = p.days
    .map((d) => `${short.format(new Date(`${d.day}T00:00:00Z`))} ${fmt.format(d.visitors)}`)
    .join(", ")
  return (
    <Link
      to={`/analytics/${p.id}`}
      className="group flex flex-col gap-1 rounded-sm py-1.5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
      aria-label={t`${name}, visitors in the last seven days: ${series}`}
      data-testid={`home-property-${p.id}`}
    >
      <span className="flex items-baseline gap-2 text-small">
        <span className="min-w-0 flex-1 truncate text-fg2 underline-offset-4 group-hover:underline">{p.name}</span>
        <span className="shrink-0 font-mono text-caption">{total}</span>
      </span>
      <span aria-hidden className="flex h-[22px] items-end gap-[3px]">
        {values.map((v, i) => (
          <span
            key={i}
            className="flex-1 rounded-[1px] bg-brand opacity-75"
            style={{ height: `${Math.max(4, Math.round((100 * v) / max))}%` }}
          />
        ))}
      </span>
    </Link>
  )
}
