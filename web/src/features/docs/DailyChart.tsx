import { useLingui } from "@lingui/react/macro"
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts"

import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { cn } from "@/lib/utils"

export type DayRow = { day: string } & Record<string, number | string>

export function zeroFill<T extends { day: string }>(rows: T[], days: number, empty: Omit<T, "day">): T[] {
  const byDay = new Map(rows.map((r) => [r.day, r]))
  const out: T[] = []
  const today = new Date()
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() - i)).toISOString().slice(0, 10)
    out.push(byDay.get(d) ?? ({ ...empty, day: d } as T))
  }
  return out
}

export function DailyChart({ rows, config, stacked, className, testId }: { rows: DayRow[]; config: ChartConfig; stacked?: boolean; className?: string; testId?: string }) {
  const { i18n } = useLingui()
  const short = new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "short", timeZone: "UTC" })
  const long = new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium", timeZone: "UTC" })
  const date = (d: unknown) => new Date(`${String(d)}T00:00:00Z`)
  const keys = Object.keys(config)
  return (
    <ChartContainer config={config} className={cn("aspect-auto h-48 w-full", className)} data-testid={testId}>
      <BarChart data={rows} margin={{ top: 4, right: 4, bottom: 0, left: -20 }} barCategoryGap={rows.length > 40 ? 1 : "20%"}>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="day" tickLine={false} axisLine={false} tickMargin={8} minTickGap={24} tickFormatter={(d) => short.format(date(d))} />
        <YAxis allowDecimals={false} tickLine={false} axisLine={false} width={44} />
        <ChartTooltip cursor={false} content={<ChartTooltipContent labelFormatter={(_, p) => (p?.[0] ? long.format(date(p[0].payload.day)) : "")} />} />
        <ChartLegend content={<ChartLegendContent />} />
        {keys.map((k, i) => (
          <Bar
            key={k}
            dataKey={k}
            stackId={stacked ? "a" : undefined}
            fill={`var(--color-${k})`}
            stroke="var(--card)"
            strokeWidth={stacked ? 1 : 0}
            radius={stacked ? (i === keys.length - 1 ? [4, 4, 0, 0] : 0) : [4, 4, 0, 0]}
            maxBarSize={28}
          />
        ))}
      </BarChart>
    </ChartContainer>
  )
}
