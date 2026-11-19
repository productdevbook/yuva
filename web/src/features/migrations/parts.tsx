import { Trans, useLingui } from "@lingui/react/macro"
import { HistoryIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { Notice, useConfirm } from "@/components/common"
import { formatDate, formatDateTime } from "@/components/common/text"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import {
  percentOf,
  useFinishMigration,
  useMigrationStep,
  usePresets,
  useRemoveHistory,
} from "@/features/migrations/queries"
import { isHelpdesk, stateTones, useMigrationText } from "@/features/migrations/text"
import { Badge } from "@/components/ui/badge"
import { Progress } from "@/components/ui/progress"
import { cn } from "@/lib/utils"
import type { Migration, MigrationSkipped } from "@/lib/api"

export function isWaiting(m: Migration) {
  return !!m.waiting_until && (m.state === "copying" || m.state === "syncing" || m.state === "scanning")
}

export function MigrationStateTag({ m }: { m: Migration }) {
  const { t } = useLingui()
  const text = useMigrationText()
  const percent = percentOf(m)
  const label = isWaiting(m)
    ? t`Waiting`
    : m.state === "copying" && m.counts.total > 0
      ? t`Copying ${percent}%`
      : text.state[m.state]
  const tone = stateTones[m.state]
  return (
    <Badge
      variant={tone === "danger" ? "destructive" : tone === "muted" ? "secondary" : "outline"}
      className={cn(
        "rounded-md",
        tone === "success" && "border-success/35 text-success",
        tone === "warning" && "border-warning/40 text-warning"
      )}
      data-testid="migration-state"
      data-state={m.state}
    >
      {label}
    </Badge>
  )
}

export function MigrationBar({ m, thin }: { m: Migration; thin?: boolean }) {
  const { t } = useLingui()
  const num = useNumber()
  const c = m.counts
  const done = num(c.imported + c.duplicate + c.skipped + c.failed)
  const total = num(c.total)
  const percent = percentOf(m)
  const desk = isHelpdesk(m.source)
  const text =
    c.total <= 0
      ? t`Starting`
      : desk
        ? t`${percent}%, ${done} of ${total} conversations`
        : t`${percent}%, ${done} of ${total} messages`
  return (
    <Progress
      value={c.total > 0 ? percent : null}
      aria-label={desk ? t`Conversations copied` : t`Messages copied`}
      getAriaValueText={() => text}
      className={cn(thin && "[&_[data-slot=progress-track]]:h-1")}
      data-testid="migration-progress"
    />
  )
}

export function WaitingLine({ m }: { m: Migration }) {
  const { i18n } = useLingui()
  if (!isWaiting(m) || !m.waiting_until) return null
  const time = formatDateTime(m.waiting_until, i18n.locale)
  return (
    <p className="text-small text-warning" data-testid="migration-waiting">
      <Trans>Waiting for the provider's limit; continues at {time}.</Trans>
    </p>
  )
}

export function FollowsNotice({ m, className }: { m: Migration; className?: string }) {
  const { i18n } = useLingui()
  if (!m.follows_migration_id || !m.options.since) return null
  const date = formatDate(m.options.since, i18n.locale)
  return (
    <Notice icon={HistoryIcon} className={className} data-testid="migration-follows">
      <p className="font-medium text-foreground">
        <Trans>Only mail since {date}</Trans>
      </p>
      <p>
        <Trans>
          Older mail came from the export file you imported, so only newer mail is read here; mail that is in both is
          not imported twice. After the copy, the usual switch follows.
        </Trans>
      </p>
    </Notice>
  )
}

export function useStateAnnouncement(items: Migration[]) {
  const text = useMigrationText()
  const seen = useRef<Map<string, string> | null>(null)
  const [message, setMessage] = useState("")
  useEffect(() => {
    const now = new Map(items.map((m) => [m.id, m.state]))
    const before = seen.current
    seen.current = now
    if (!before) return
    const changed = items.find((m) => before.has(m.id) && before.get(m.id) !== m.state)
    if (changed) setMessage(`${changed.source_address}: ${text.state[changed.state]}`)
  }, [items, text.state])
  return (
    <p className="sr-only" role="status" aria-live="polite">
      {message}
    </p>
  )
}

export function useMigrationConfirms(m: Migration) {
  const { t } = useLingui()
  const num = useNumber()
  const [confirm, dialog] = useConfirm()
  const cancel = useMigrationStep(m.id, "cancel")
  const finish = useFinishMigration(m.id)
  const remove = useRemoveHistory(m.id)
  const removable = m.removable_conversations ?? 0
  const count = num(removable)
  const address = m.source_address
  const provider = usePresets().data?.find((p) => p.id === m.preset)?.name ?? ""
  const desk = isHelpdesk(m.source) && provider !== ""
  return {
    dialog,
    error: cancel.error ?? finish.error ?? remove.error,
    pending: cancel.isPending || finish.isPending || remove.isPending,
    finish: () =>
      confirm({
        title: t`Finish the migration?`,
        description: desk
          ? m.verified_at
            ? t`Sync stops and Yuva's token for ${provider} is deleted. What reaches only ${provider} from now on is not copied.`
            : t`No test message has reached Yuva by the new route yet. If you finish now, mail that still goes only to ${provider} is not copied. Yuva's token is deleted.`
          : m.verified_at
            ? t`Sync stops and the mailbox's credentials are deleted. Mail that reaches only the old mailbox from now on is not copied.`
            : t`No test message has reached Yuva by the new route yet. If you finish now, mail that still goes only to the old mailbox is not copied. The credentials are deleted.`,
        confirm: m.verified_at ? t`Finish` : t`Finish anyway`,
        run: () => finish.mutate(!m.verified_at),
      }),
    cancel: () =>
      confirm({
        title: desk ? t`Cancel moving from ${provider}?` : t`Cancel moving ${address}?`,
        description: desk
          ? t`It stops for good. What was imported stays, and Yuva's token is deleted. Nothing changes in ${provider}.`
          : t`It stops for good. What was imported stays, and the mailbox's credentials are deleted. Nothing changes in the mailbox.`,
        confirm: t`Cancel the migration`,
        run: () => cancel.mutate(),
      }),
    removeHistory: () =>
      confirm({
        title: t`Remove imported history?`,
        description:
          removable === 1
            ? t`1 imported conversation that nobody wrote in since is deleted with its messages and files. Conversations your team wrote in stay. Nothing changes in ${address}.`
            : t`${count} imported conversations that nobody wrote in since are deleted with their messages and files. Conversations your team wrote in stay. Nothing changes in ${address}.`,
        confirm: t`Remove history`,
        run: () => remove.mutate(removable),
      }),
  }
}

export function Figure({ value, label, detail }: { value: string; label: React.ReactNode; detail?: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-xl border px-3.5 py-3">
      <span className="text-page tabular-nums">{value}</span>
      <span className="text-small text-muted-foreground">{label}</span>
      {detail && <span className="text-caption text-faint">{detail}</span>}
    </div>
  )
}

export function SkippedList({ skipped }: { skipped: MigrationSkipped }) {
  const { t } = useLingui()
  const num = useNumber()
  const reasons: [keyof MigrationSkipped, string][] = [
    ["already_in_yuva", t`Already in Yuva`],
    ["duplicate_copies", t`Further copies of the same message`],
    ["outside_range", t`Older than the chosen range`],
    ["automatic", t`Threads of only automatic mail`],
    ["internal", t`Threads of only your team's mail`],
    ["spam", t`Spam`],
    ["trash", t`Trash`],
    ["drafts", t`Drafts`],
    ["other_folders", t`Folders not chosen or never read`],
    ["blocked", t`From blocked contacts`],
    ["too_large", t`Larger than this server's limit`],
  ]
  const rows = reasons.filter(([k]) => skipped[k] > 0)
  if (rows.length === 0)
    return (
      <p className="text-small text-faint">
        <Trans>Nothing is left out.</Trans>
      </p>
    )
  return (
    <ul className="flex flex-col divide-y rounded-xl border" data-testid="migration-skipped">
      {rows.map(([k, label]) => (
        <li key={k} className="flex items-baseline justify-between gap-3 px-3.5 py-2 text-body">
          <span className="min-w-0">{label}</span>
          <span className="shrink-0 text-faint tabular-nums">{num(skipped[k])}</span>
        </li>
      ))}
    </ul>
  )
}
