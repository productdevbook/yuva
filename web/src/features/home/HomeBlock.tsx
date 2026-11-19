import { useLingui } from "@lingui/react/macro"
import { ChevronRightIcon } from "lucide-react"
import { Link, type To } from "react-router"

import { Badge } from "@/components/ui/badge"
import { Card } from "@/components/ui/card"
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@/components/ui/item"
import { useNow } from "@/hooks/use-now"
import { cn } from "@/lib/utils"

export function Block({
  id,
  title,
  count,
  action,
  testId,
  children,
}: {
  id: string
  title: React.ReactNode
  count?: React.ReactNode
  action?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  return (
    <section aria-labelledby={id} data-testid={testId}>
      <Card size="sm" className="gap-0 py-0">
        <div className="flex min-h-11 items-center gap-2 border-b px-4 py-2">
          <h2 id={id} className="m-0 text-[15px] leading-5 font-semibold">
            {title}
          </h2>
          {count !== undefined && <span className="text-small text-muted-foreground tabular-nums">{count}</span>}
          <span className="flex-1" />
          {action}
        </div>
        <div role="list" className="flex flex-col">
          {children}
        </div>
      </Card>
    </section>
  )
}

export function BlockLink({
  to,
  state,
  onClick,
  children,
}: {
  to: To
  state?: unknown
  onClick?: () => void
  children: React.ReactNode
}) {
  return (
    <Link
      to={to}
      state={state}
      onClick={onClick}
      className="shrink-0 rounded-sm text-small font-medium text-foreground underline underline-offset-4 outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
    >
      {children}
    </Link>
  )
}

export function Row({
  to,
  onClick,
  media,
  title,
  description,
  end,
  endTone,
  conversation,
  late,
  wrap,
  testId,
}: {
  to: To
  onClick?: () => void
  media?: React.ReactNode
  title: React.ReactNode
  description?: React.ReactNode
  end?: React.ReactNode
  endTone?: "late" | "muted"
  conversation?: string
  late?: string
  wrap?: boolean
  testId?: string
}) {
  return (
    <Item
      role="listitem"
      size="sm"
      render={<Link to={to} onClick={onClick} data-home-row="" data-late={late} data-conversation={conversation} />}
      className="flex-nowrap rounded-none border-0 border-b border-line-soft px-4 last:border-b-0 focus-visible:bg-muted focus-visible:ring-0"
      data-testid={testId}
    >
      {media && <ItemMedia>{media}</ItemMedia>}
      <ItemContent className="min-w-0 gap-0.5">
        <ItemTitle className={cn("block w-full", wrap ? "line-clamp-2 font-normal" : "truncate")}>{title}</ItemTitle>
        {description && <ItemDescription className="truncate">{description}</ItemDescription>}
      </ItemContent>
      <ItemActions className="shrink-0">
        {end && (
          <span
            className={cn(
              "text-small whitespace-nowrap tabular-nums",
              endTone === "late" ? "font-medium text-destructive" : "text-muted-foreground"
            )}
          >
            {end}
          </span>
        )}
        <ChevronRightIcon aria-hidden className="size-4 text-muted2 phone:hidden rtl:rotate-180" />
      </ItemActions>
    </Item>
  )
}

export function Tag({ tone, children }: { tone: "late" | "plain" | "warn"; children: React.ReactNode }) {
  return (
    <Badge
      variant={tone === "late" ? "destructive" : tone === "warn" ? "outline" : "secondary"}
      className={cn("w-24 rounded-md phone:w-auto", tone === "warn" && "text-note-ink")}
    >
      {children}
    </Badge>
  )
}

export function useClock() {
  const { i18n } = useLingui()
  const now = useNow(30_000)
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "numeric", minute: "2-digit" })
  const dayTime = new Intl.DateTimeFormat(i18n.locale, { weekday: "short", hour: "numeric", minute: "2-digit" })
  const today = new Date(now).toDateString()
  return {
    now,
    endOfDay: new Date(now).setHours(23, 59, 59, 999),
    at: (iso: string | number) => {
      const d = new Date(iso)
      return d.toDateString() === today ? time.format(d) : dayTime.format(d)
    },
    ago: (iso: string) => {
      const s = Math.round((Date.parse(iso) - now) / 1000)
      const rel = new Intl.RelativeTimeFormat(i18n.locale, { numeric: "auto", style: "long" })
      const a = Math.abs(s)
      if (a < 60) return rel.format(0, "second")
      if (a < 3600) return rel.format(Math.round(s / 60), "minute")
      if (a < 86400) return rel.format(Math.round(s / 3600), "hour")
      return rel.format(Math.round(s / 86400), "day")
    },
  }
}
