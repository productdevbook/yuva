import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useQueryClient } from "@tanstack/react-query"
import { ChevronDownIcon, ClockIcon, PauseIcon, PlayIcon, TriangleAlertIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link } from "react-router"

import { ErrorLine, Notice, ProgressBar, StepHeader } from "@/components/common"
import { formatDate, formatDateTime, formatDuration } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import {
  isWaiting,
  MigrationBar,
  MigrationStateTag,
  useMigrationConfirms,
  useStateAnnouncement,
} from "@/features/migrations/parts"
import { useMigrationErrors, useMigrationFolders, useMigrationStep } from "@/features/migrations/queries"
import { isHelpdesk, useItemErrorText } from "@/features/migrations/text"
import { WizardLayout } from "@/features/migrations/Wizard"
import type { Migration, MigrationFolder, MigrationPreset } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function CopyStep({ m, preset }: { m: Migration; preset: MigrationPreset }) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const pause = useMigrationStep(m.id, "pause")
  const resume = useMigrationStep(m.id, "resume")
  const ask = useMigrationConfirms(m)
  const announcement = useStateAnnouncement([m])
  const can = (a: Migration["actions"][number]) => m.actions.includes(a)
  const address = m.source_address
  const provider = preset.name
  const attention = m.state === "needs_attention"
  const desk = isHelpdesk(m.source)
  const reconnect = attention && (can("set_credentials") || can("sign_in"))
  const rate = num(m.rate_per_minute ?? 0)
  const left = m.seconds_left ? formatDuration(m.seconds_left, i18n.locale) : ""
  const waiting = isWaiting(m) && m.waiting_until ? formatDateTime(m.waiting_until, i18n.locale) : ""
  const error = pause.error ?? resume.error ?? ask.error
  return (
    <WizardLayout step="copy" m={m} preset={preset}>
      <StepHeader title={desk ? <Trans>Copying {provider}</Trans> : <Trans>Copying {address}</Trans>}>
        <Trans>Newest conversations first. You can close this page; members can work meanwhile.</Trans>
      </StepHeader>
      {announcement}

      <div className="flex flex-col gap-8">
        <section className="flex flex-col gap-3 rounded-2xl border p-4" aria-label={t`Progress`}>
          <div className="flex items-center justify-between gap-3">
            <MigrationStateTag m={m} />
            {m.state === "copying" && !waiting && (m.rate_per_minute ?? 0) > 0 && (
              <span className="text-small text-faint tabular-nums" data-testid="migration-rate">
                {left ? (
                  <Trans>
                    {rate} a minute · about {left} left
                  </Trans>
                ) : (
                  <Trans>{rate} a minute</Trans>
                )}
              </span>
            )}
          </div>
          <MigrationBar m={m} />
          <Counts m={m} desk={desk} />
        </section>

        {waiting && (
          <Notice tone="warning" icon={ClockIcon} data-testid="migration-waiting">
            <p className="font-medium">
              <Trans>
                Waiting for {provider}'s limit; continues at {waiting}.
              </Trans>
            </p>
            {m.state_reason && <p className="text-small text-muted-foreground">{m.state_reason}</p>}
          </Notice>
        )}

        {attention && (
          <Notice tone="danger" icon={TriangleAlertIcon} data-testid="migration-attention">
            <p className="font-medium">
              <Trans>Needs attention</Trans>
            </p>
            {m.state_reason && <p>{m.state_reason}</p>}
            <p className="text-small text-muted-foreground">
              {m.source === "file" ? (
                <Trans>Nothing is lost: when it continues, copying picks up where it stopped.</Trans>
              ) : desk ? (
                <Trans>Nothing is lost: once {provider} is signed in again, copying continues where it stopped.</Trans>
              ) : m.preset === "microsoft" ? (
                <Trans>
                  Nothing is lost: once you sign in with Microsoft again, copying continues where it stopped.
                </Trans>
              ) : (
                <Trans>Nothing is lost: once the mailbox is connected again, copying continues where it stopped.</Trans>
              )}
            </p>
            {reconnect && (
              <Button
                size="sm"
                className="self-start"
                render={<Link to={`/migrations/${m.id}/connect`} />}
                data-testid="migration-reconnect"
              >
                {m.preset === "microsoft" ? <Trans>Sign in again</Trans> : <Trans>Reconnect</Trans>}
              </Button>
            )}
          </Notice>
        )}

        {m.state === "paused" && (
          <Notice icon={PauseIcon}>
            <p>
              {desk ? (
                <Trans>Paused. Nothing is read from {provider} until you resume.</Trans>
              ) : (
                <Trans>Paused. Nothing is read from {address} until you resume.</Trans>
              )}
            </p>
          </Notice>
        )}

        <ErrorLine error={error} />
        <div className="flex flex-wrap items-center gap-2">
          {can("pause") && (
            <Button
              variant="outline"
              onClick={() => pause.mutate()}
              disabled={pause.isPending}
              data-testid="migration-pause"
            >
              <PauseIcon />
              <Trans>Pause</Trans>
            </Button>
          )}
          {can("resume") && !reconnect && (
            <Button onClick={() => resume.mutate()} disabled={resume.isPending} data-testid="migration-resume">
              <PlayIcon />
              <Trans>Resume</Trans>
            </Button>
          )}
          {can("cancel") && (
            <Button
              variant="ghost"
              className="text-destructive"
              onClick={ask.cancel}
              disabled={ask.pending}
              data-testid="migration-cancel"
            >
              <Trans>Cancel migration</Trans>
            </Button>
          )}
        </div>
        {ask.dialog}

        <Folders m={m} />
        <Problems m={m} />
      </div>
    </WizardLayout>
  )
}

function Counts({ m, desk }: { m: Migration; desk: boolean }) {
  const num = useNumber()
  const c = m.counts
  const imported = num(c.imported)
  const already = num(c.duplicate)
  const skipped = num(c.skipped)
  const failed = num(c.failed)
  return (
    <p className="text-small text-faint" data-testid="migration-counts">
      {desk ? (
        <Trans>
          {imported} conversations imported · {already} already in Yuva · {skipped} skipped · {failed} failed
        </Trans>
      ) : (
        <Trans>
          {imported} imported · {already} already in Yuva · {skipped} skipped · {failed} failed
        </Trans>
      )}
    </p>
  )
}

function Folders({ m }: { m: Migration }) {
  const folders = useMigrationFolders(m.id)
  const rows = (folders.data ?? []).filter((f) => f.selected && f.use === "import")
  if (folders.isPending) return <Skeleton className="h-24 rounded-xl" />
  if (rows.length === 0) return null
  return (
    <section className="flex flex-col gap-3" aria-labelledby="migration-folders-progress">
      <h2 id="migration-folders-progress" className="text-title">
        {m.source === "zendesk" ? (
          <Trans>Brands</Trans>
        ) : m.source === "helpscout" ? (
          <Trans>Mailboxes</Trans>
        ) : m.source === "front" ? (
          <Trans>Inboxes</Trans>
        ) : m.source === "freshdesk" ? (
          <Trans>Products</Trans>
        ) : m.source === "crisp" ? (
          <Trans>Website</Trans>
        ) : (
          <Trans>Folders</Trans>
        )}
      </h2>
      <ul className="flex flex-col divide-y rounded-xl border">
        {rows.map((f) => (
          <FolderRow key={f.id} f={f} />
        ))}
      </ul>
    </section>
  )
}

function FolderRow({ f }: { f: MigrationFolder }) {
  const { t } = useLingui()
  const num = useNumber()
  const done = f.imported + f.duplicate + f.skipped + f.failed
  const percent = f.messages > 0 ? Math.min(100, Math.floor((done / f.messages) * 100)) : 0
  const doneText = num(done)
  const total = num(f.messages)
  const name = f.name
  return (
    <li className="flex flex-col gap-1.5 px-3.5 py-2.5" data-testid="migration-folder-progress">
      <div className="flex items-baseline justify-between gap-3 text-body">
        <span className="min-w-0 truncate">{name}</span>
        <span className="shrink-0 text-small text-faint tabular-nums">
          <Trans>
            {doneText} of {total}
          </Trans>
        </span>
      </div>
      <ProgressBar thin value={percent} label={t`${name} copied`} text={t`${percent}%, ${doneText} of ${total}`} />
      {f.failed > 0 && (
        <span className="text-caption text-destructive">
          <Plural value={f.failed} one="# failed" other="# failed" />
        </span>
      )}
    </li>
  )
}

function Problems({ m }: { m: Migration }) {
  const { i18n } = useLingui()
  const num = useNumber()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [open, setOpen] = useState(false)
  const errors = useMigrationErrors(m.id, open)
  const retry = useMigrationStep(m.id, "retry")
  const codeText = useItemErrorText()
  const total = m.counts.failed + m.counts.skipped
  const count = num(total)
  useEffect(() => {
    if (open) void qc.invalidateQueries({ queryKey: keys.migrationErrors(ws, m.id) })
  }, [total, open, qc, ws, m.id])
  if (total === 0) return null
  const items = errors.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <section className="flex flex-col gap-3" aria-labelledby="migration-problems">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="migration-problems" className="text-title">
          <Button
            variant="ghost"
            className="-ms-3 text-title"
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
            aria-controls="migration-problem-list"
            data-testid="migration-problems-toggle"
          >
            <ChevronDownIcon className={cn("transition-transform", open && "rotate-180")} />
            <Trans>Skipped and failed ({count})</Trans>
          </Button>
        </h2>
        {m.counts.failed > 0 && m.actions.includes("retry") && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => retry.mutate()}
            disabled={retry.isPending}
            data-testid="migration-retry"
          >
            <Trans>Retry failed items</Trans>
          </Button>
        )}
      </div>
      <ErrorLine error={retry.error} />
      {open && (
        <div id="migration-problem-list" className="flex flex-col gap-2">
          {errors.isPending ? (
            <Skeleton className="h-24 rounded-xl" />
          ) : (
            <ul className="flex flex-col divide-y rounded-xl border" data-testid="migration-errors">
              {items.map((e) => (
                <li key={e.id} className="flex flex-col gap-0.5 px-3.5 py-2.5">
                  <span className="truncate text-body">{e.subject || <Trans>(no subject)</Trans>}</span>
                  <span className="text-small text-muted-foreground">{codeText(e.code) ?? e.detail}</span>
                  <span className="truncate text-caption text-faint">
                    {e.folder}
                    {e.item_date && <> · {formatDate(e.item_date, i18n.locale)}</>}
                  </span>
                </li>
              ))}
            </ul>
          )}
          <ErrorLine error={errors.error} />
          {errors.hasNextPage && (
            <Button
              variant="ghost"
              size="sm"
              className="self-start"
              onClick={() => errors.fetchNextPage()}
              disabled={errors.isFetchingNextPage}
            >
              <Trans>Show more</Trans>
            </Button>
          )}
        </div>
      )}
    </section>
  )
}
