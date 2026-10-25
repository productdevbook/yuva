import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import type { HomeProperty } from "@/lib/api"

function Spark({ values }: { values: number[] }) {
  const max = Math.max(1, ...values)
  return (
    <span aria-hidden className="flex h-7 shrink-0 items-end gap-0.5">
      {values.map((v, i) => (
        <span
          key={i}
          className="w-1 rounded-[1px] bg-success"
          style={{ height: Math.max(3, Math.round((28 * v) / max)), opacity: i === values.length - 1 ? 1 : 0.35 }}
        />
      ))}
    </span>
  )
}

export function SiteCard({ property: p }: { property: HomeProperty }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const short = new Intl.DateTimeFormat(i18n.locale, { weekday: "short", timeZone: "UTC" })
  const name = p.name
  const app = p.domains.length === 0 && p.platforms.length > 0
  const live = fmt.format(p.realtime_visitors)
  const series = p.days
    .map((d) => `${short.format(new Date(`${d.day}T00:00:00Z`))} ${fmt.format(d.visitors)}`)
    .join(", ")
  return (
    <Link
      to={`/analytics/${p.id}`}
      className="flex min-w-0 flex-col gap-2.5 rounded-2xl border bg-card p-4 outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring"
      aria-label={t`${name}, visitors in the last seven days: ${series}`}
      data-testid={`home-property-${p.id}`}
    >
      <span className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 flex-1 truncate font-semibold">{p.name}</span>
        {p.realtime_visitors > 0 && (
          <span className="inline-flex shrink-0 items-center gap-1.5 text-caption text-success">
            <span aria-hidden className="size-1.5 rounded-full bg-success" />
            <Trans>{live} now</Trans>
          </span>
        )}
      </span>
      <span className="flex items-end justify-between gap-2">
        <span className="flex min-w-0 flex-col">
          <b className="text-xl leading-6 font-semibold tabular-nums">{fmt.format(p.visitors_today)}</b>
          <span className="truncate text-caption text-faint">
            {app ? <Trans>users today</Trans> : <Trans>visitors today</Trans>}
          </span>
        </span>
        <Spark values={p.days.map((d) => d.visitors)} />
      </span>
    </Link>
  )
}
