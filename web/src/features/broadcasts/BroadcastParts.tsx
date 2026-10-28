import { Trans, useLingui } from "@lingui/react/macro"
import { BanIcon, ClockIcon, ShieldAlertIcon } from "lucide-react"
import { Link } from "react-router"

import { Notice } from "@/components/common"
import { formatDateTime, formatRelative, useEnumText } from "@/components/common/text"
import { keyLabel } from "@/components/common/ShortcutSheet"
import type { Broadcast, BroadcastKind, BroadcastState, Channel, WorkspaceBroadcasts } from "@/lib/api"
import { useSession } from "@/lib/session"
import { clock, utcLine, zoneCity } from "@/lib/zones"
import { cn } from "@/lib/utils"

export function keyHint(label: string, key: string) {
  return `${label} (${keyLabel(key).join("")})`
}

export type SenderState = "ready" | "no_smtp" | "no_from"

export function senderState(ch: Channel): SenderState {
  const e = ch.email
  if (!e || (!e.smtp && !e.broadcast_smtp)) return "no_smtp"
  if (e.address.startsWith("*@") && !e.from_address) return "no_from"
  return "ready"
}

const tones: Record<BroadcastState, string> = {
  draft: "border-faint/40 text-faint",
  scheduled: "border-blue-600/35 text-blue-600 dark:border-blue-400/35 dark:text-blue-400",
  preparing: "border-brand/35 text-brand",
  sending: "border-brand/35 text-brand",
  active: "border-brand/35 text-brand",
  paused: "border-warning/40 text-warning",
  sent: "border-success/35 text-success",
  cancelled: "border-faint/40 text-faint",
  failed: "border-destructive/35 text-destructive",
}

export function StateBadge({
  state,
  kind,
  className,
}: {
  state: BroadcastState
  kind?: BroadcastKind
  className?: string
}) {
  const { t } = useLingui()
  const text = useEnumText()
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-px text-caption font-medium",
        tones[state],
        className
      )}
      data-testid="broadcast-state"
      data-state={state}
    >
      {(state === "sending" || state === "preparing" || state === "active") && (
        <span className="size-1.5 animate-pulse rounded-full bg-current" aria-hidden />
      )}
      {kind === "triggered" && state === "cancelled" ? t`Stopped` : text.broadcastState[state]}
    </span>
  )
}

export function progressOf(b: Broadcast) {
  const c = b.counts
  if (b.state === "preparing") return { done: c.prepared ?? 0, total: 0 }
  const done = c.sent + c.failed + c.bounced + c.complained + c.skipped.unsubscribed + c.skipped.cancelled
  return { done: Math.min(done, c.recipients), total: c.recipients }
}

export function ProgressBar({ b, className }: { b: Broadcast; className?: string }) {
  const { done, total } = progressOf(b)
  const share = total > 0 ? Math.min(1, done / total) : 0
  return (
    <span
      className={cn("relative block h-1.5 w-full overflow-hidden rounded-full bg-muted", className)}
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={total || undefined}
      aria-valuenow={done}
      data-testid="broadcast-progress"
    >
      <span
        className={cn(
          "absolute inset-y-0 start-0 rounded-full transition-[width] duration-700",
          b.state === "paused" ? "bg-warning" : "bg-brand",
          b.state === "preparing" && "w-1/4 animate-pulse"
        )}
        style={b.state === "preparing" ? undefined : { width: `${share * 100}%` }}
      />
    </span>
  )
}

export function useNumber() {
  const { i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (n: number) => fmt.format(n)
}

export function usePercent() {
  const { i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 1 })
  return (n: number) => fmt.format(n)
}

export function WaitingLine({ until, zone }: { until: string; zone: string }) {
  const { i18n } = useLingui()
  const time = clock(new Date(until), zone, i18n.locale)
  const city = zoneCity(zone)
  return (
    <p className="flex items-start gap-1.5 text-body text-muted-foreground" data-testid="broadcast-waiting">
      <ClockIcon className="mt-0.5 size-4 shrink-0 text-warning" />
      <span>
        <Trans>
          Daily limit reached · continues at {time} {city} time
        </Trans>
      </span>
    </p>
  )
}

export function WindowLine({ opens, zone }: { opens: string; zone: string }) {
  const { i18n } = useLingui()
  const time = clock(new Date(opens), zone, i18n.locale)
  const city = zoneCity(zone)
  const relative = formatRelative(opens, i18n.locale)
  return (
    <p className="flex items-start gap-1.5 text-body text-muted-foreground" data-testid="broadcast-window-closed">
      <ClockIcon className="mt-0.5 size-4 shrink-0 text-warning" />
      <span>
        <Trans>
          Waiting for sending hours · opens at {time} {city} time ({relative})
        </Trans>
      </span>
    </p>
  )
}

export function PaceLine({ at }: { at: string }) {
  const { i18n } = useLingui()
  const local = formatDateTime(at, i18n.locale)
  const utc = utcLine(at, i18n.locale)
  return (
    <p className="flex items-start gap-1.5 text-body text-muted-foreground" data-testid="broadcast-pace-next">
      <ClockIcon className="mt-0.5 size-4 shrink-0 text-faint" />
      <span>
        <Trans>
          Next mail on {local} ({utc} UTC), at this broadcast’s pace.
        </Trans>
      </span>
    </p>
  )
}

export function useBroadcastsStatus(): WorkspaceBroadcasts | undefined {
  return useSession().membership.workspace.broadcasts
}

export function StatusNotice({ settingsLink = true, className }: { settingsLink?: boolean; className?: string }) {
  const s = useBroadcastsStatus()
  const { membership } = useSession()
  const owner = membership.role === "owner"
  if (!s) return null
  const reason = s.reason
  const link = settingsLink && (
    <Link
      to="/settings/workspace"
      className="w-fit text-brand underline-offset-4 hover:underline"
      data-testid="broadcasts-status-link"
    >
      <Trans>Open the workspace settings</Trans>
    </Link>
  )
  if (s.mode === "off")
    return (
      <Notice icon={BanIcon} className={className} data-testid="broadcasts-off">
        <p>
          <Trans>
            Broadcasts are turned off on this server. Lists and their forms still work, so you can collect subscribers.
          </Trans>
        </p>
      </Notice>
    )
  if (s.status === "approval_required")
    return (
      <Notice tone="warning" icon={ShieldAlertIcon} className={className} data-testid="broadcasts-approval-required">
        <p>
          {owner ? (
            <Trans>
              This server sends broadcasts for a workspace once its operator approves it. Tell them about your mail to
              ask. Until then you can build lists, designs and drafts and send tests.
            </Trans>
          ) : (
            <Trans>
              This server sends broadcasts for a workspace once its operator approves it. An owner of the workspace can
              ask. Until then you can build lists, designs and drafts and send tests.
            </Trans>
          )}
        </p>
        {owner && link}
      </Notice>
    )
  if (s.status === "approval_requested")
    return (
      <Notice icon={ClockIcon} className={className} data-testid="broadcasts-approval-requested">
        <p>
          <Trans>
            The server’s operator has your request to send broadcasts. You can schedule them once it is approved.
          </Trans>
        </p>
      </Notice>
    )
  if (s.status === "suspended")
    return (
      <Notice tone="danger" icon={BanIcon} className={className} data-testid="broadcasts-suspended">
        <p>
          <Trans>This workspace may not send broadcasts for now. Running broadcasts are paused.</Trans>
        </p>
        {reason && <p className="text-muted-foreground">{reason}</p>}
        {link}
      </Notice>
    )
  return null
}
