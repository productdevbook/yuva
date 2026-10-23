import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { ArrowRightIcon, CheckIcon, ScanSearchIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { CheckItem, CheckList, ErrorLine, Notice, ProgressBar, Segmented, StepHeader } from "@/components/common"
import { formatBytes } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import { FollowsNotice, WaitingLine } from "@/features/migrations/parts"
import { useMigrationFolders, useMigrationStep, useUpdateMigration } from "@/features/migrations/queries"
import { isHelpdesk, NEVER_READ, useMigrationText } from "@/features/migrations/text"
import { WizardLayout } from "@/features/migrations/Wizard"
import type { Migration, MigrationOptionsInput, MigrationPreset } from "@/lib/api"

type Range = "all" | "3" | "12" | "24" | "custom"

function monthsAgo(n: number, now: string) {
  const d = new Date(now)
  return new Date(Date.UTC(d.getFullYear(), d.getMonth() - n, d.getDate())).toISOString()
}

function sameTime(a: string | null | undefined, b: string | null | undefined) {
  return (a ? new Date(a).getTime() : null) === (b ? new Date(b).getTime() : null)
}

function dayOf(iso: string) {
  return iso.slice(0, 10)
}

export function ChooseStep({ m, preset }: { m: Migration; preset: MigrationPreset }) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const text = useMigrationText()
  const folders = useMigrationFolders(m.id)
  const update = useUpdateMigration(m.id)
  const scanStep = useMigrationStep(m.id, "scan")
  const [picked, setPicked] = useState<Set<string> | null>(null)
  const [range, setRange] = useState<Range>(m.options.since ? "custom" : "all")
  const [day, setDay] = useState(m.options.since ? dayOf(m.options.since) : "")
  const [skipAuto, setSkipAuto] = useState(m.options.skip_automatic)
  const [today] = useState(() => new Date().toISOString())
  const scanning = m.state === "scanning"
  const ready = m.state === "ready"
  const canEdit = m.actions.includes("update")
  const desk = isHelpdesk(m.source)
  const list = folders.data ?? []
  const selected = picked ?? new Set(list.filter((f) => f.selected).map((f) => f.name))
  const since =
    range === "all" ? null : range === "custom" ? (day ? new Date(day).toISOString() : null) : monthsAgo(+range, today)
  const changed = picked !== null || !sameTime(since, m.options.since) || skipAuto !== m.options.skip_automatic

  const scan = useMutation({
    mutationFn: async () => {
      let current = m
      if (changed && canEdit) {
        const options: MigrationOptionsInput = { since, skip_automatic: skipAuto }
        if (picked) options.folders = [...picked]
        current = await update.mutateAsync({ options })
      }
      if (current.actions.includes("scan")) {
        await scanStep.mutateAsync()
      }
      setPicked(null)
    },
  })

  const reset = () => update.mutate({ options: { folders: null } }, { onSuccess: () => setPicked(null) })

  const toggle = (name: string, on: boolean) => {
    const next = new Set(selected)
    if (on) next.add(name)
    else next.delete(name)
    setPicked(next)
  }

  const est = m.estimate
  const conversations = num(est?.conversations ?? 0)
  const messages = num(est?.messages ?? 0)
  const rangeItems = [
    { value: "all" as const, label: t`Everything` },
    { value: "3" as const, label: t`3 months` },
    { value: "12" as const, label: t`12 months` },
    { value: "24" as const, label: t`24 months` },
    { value: "custom" as const, label: t`Since…` },
  ]
  return (
    <WizardLayout step="choose" m={m} preset={preset}>
      <StepHeader title={<Trans>What should Yuva bring?</Trans>}>
        {desk ? (
          <Trans>
            Pick what to bring and how far back to go. The scan lists the tickets to show exactly what will be imported;
            nothing is imported yet.
          </Trans>
        ) : (
          <Trans>
            Pick the folders and how far back to go. The scan reads only the headers to show exactly what will be
            imported; nothing is imported yet.
          </Trans>
        )}
      </StepHeader>

      <div className="flex flex-col gap-7">
        {folders.isPending ? (
          <Skeleton className="h-48 rounded-xl" />
        ) : (
          <CheckList
            labelId="migration-folders"
            label={
              m.source === "zendesk" ? (
                <Trans>Brands</Trans>
              ) : m.source === "helpscout" ? (
                <Trans>Mailboxes</Trans>
              ) : m.source === "front" ? (
                <Trans>Inboxes</Trans>
              ) : (
                <Trans>Folders</Trans>
              )
            }
            hint={
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small text-faint">
                <span>
                  {desk ? (
                    <Trans>Spam and deleted tickets are never imported.</Trans>
                  ) : (
                    <Trans>Spam, trash and drafts are never read.</Trans>
                  )}
                </span>
                {canEdit && (picked || m.options.folders) && (
                  <Button
                    type="button"
                    variant="link"
                    size="sm"
                    className="text-small"
                    onClick={reset}
                    disabled={update.isPending}
                  >
                    <Trans>Use the usual choice</Trans>
                  </Button>
                )}
              </div>
            }
          >
            {list.map((f) => {
              const never = NEVER_READ.includes(f.kind)
              const count = num(f.messages)
              const size = f.size_bytes > 0 ? formatBytes(f.size_bytes, i18n.locale) : ""
              return (
                <CheckItem
                  key={f.id}
                  checked={!never && selected.has(f.name)}
                  disabled={never || !canEdit}
                  onChange={(on) => toggle(f.name, on)}
                  testId="migration-folder"
                >
                  <span className="flex items-baseline gap-2">
                    <span className="min-w-0 flex-1 truncate">{f.name}</span>
                    <span className="shrink-0 text-small text-faint tabular-nums">
                      {desk ? (
                        <Plural value={f.messages} one="# conversation" other="# conversations" />
                      ) : size ? (
                        <Trans>
                          {count} messages · {size}
                        </Trans>
                      ) : (
                        <Trans>{count} messages</Trans>
                      )}
                    </span>
                  </span>
                  <span className="truncate text-small text-faint">
                    {desk ? (
                      (f.address ?? <Trans>No support address</Trans>)
                    ) : f.use === "labels" && f.kind === "sent" ? (
                      <Trans>Read to suggest your team's addresses: its mail comes from All mail.</Trans>
                    ) : f.use === "labels" ? (
                      <Trans>Label only: its mail comes from All mail, and its conversations get the label.</Trans>
                    ) : (
                      text.folderKind[f.kind]
                    )}
                  </span>
                </CheckItem>
              )
            })}
          </CheckList>
        )}
        <ErrorLine error={folders.error} />

        <FollowsNotice m={m} />

        <fieldset className="flex min-w-0 flex-col gap-2.5" disabled={!canEdit}>
          <legend className="mb-2.5 text-body font-medium">
            <Trans>How far back</Trans>
          </legend>
          <Segmented
            value={range}
            onChange={setRange}
            items={rangeItems}
            label={t`How far back`}
            className="w-full overflow-x-auto"
            itemClassName="flex-1"
          />
          {range === "custom" && (
            <Label className="flex-col items-start gap-2 font-normal">
              <span className="text-small text-faint">
                <Trans>Threads whose last message is on or after</Trans>
              </span>
              <Input
                type="date"
                value={day}
                max={dayOf(today)}
                onChange={(e) => setDay(e.target.value)}
                className="h-10 w-48 rounded-xl"
                data-testid="migration-since"
              />
            </Label>
          )}
          <p className="text-small text-faint">
            {desk ? (
              <Trans>A conversation is taken whole when it was last updated in the range.</Trans>
            ) : (
              <Trans>A thread is taken whole when its last message is in the range.</Trans>
            )}
          </p>
        </fieldset>

        {!desk && (
          <Label className="items-start gap-3 font-normal">
            <Checkbox checked={skipAuto} onCheckedChange={setSkipAuto} disabled={!canEdit} className="mt-0.5" />
            <span className="flex flex-col gap-0.5">
              <span className="font-medium">
                <Trans>Skip threads that contain only automatic mail</Trans>
              </span>
              <span className="text-small text-faint">
                <Trans>Auto-replies, newsletters and delivery reports nobody answered.</Trans>
              </span>
            </span>
          </Label>
        )}

        {scanning && (
          <div className="flex flex-col gap-3 rounded-2xl border p-4" data-testid="migration-scanning">
            <p role="status" aria-live="polite" className="flex items-center gap-2 text-body font-medium">
              <ScanSearchIcon className="size-4 text-faint" />
              {desk ? <Trans>Listing the tickets…</Trans> : <Trans>Scanning the headers…</Trans>}
            </p>
            <ProgressBar label={t`Scan`} text={t`Scanning`} />
            <WaitingLine m={m} />
            <p className="text-small text-faint">
              {desk ? (
                <Trans>
                  Only read requests are sent. You can close this page; the scan goes on and the list in Settings shows
                  when it is done.
                </Trans>
              ) : (
                <Trans>
                  Only headers are read: no message is opened or marked. You can close this page; the scan goes on and
                  the list in Settings shows when it is done.
                </Trans>
              )}
            </p>
          </div>
        )}

        {ready && est && !changed && (
          <Notice icon={CheckIcon} data-testid="migration-scanned">
            <p className="font-medium text-foreground">
              <Trans>
                Scan finished: {conversations} conversations, {messages} messages
              </Trans>
            </p>
            <p>
              <Trans>Next, choose where they go.</Trans>
            </p>
          </Notice>
        )}

        <ErrorLine error={scan.error} />

        <div className="flex flex-wrap items-center gap-3">
          {(!ready || changed) && (
            <Button
              size="lg"
              onClick={() => scan.mutate()}
              disabled={scanning || scan.isPending || (!canEdit && !m.actions.includes("scan")) || selected.size === 0}
              data-testid="migration-scan"
            >
              {ready || est ? <Trans>Scan again</Trans> : <Trans>Scan</Trans>}
            </Button>
          )}
          {(ready || scanning) && !changed && (
            <Button
              size="lg"
              variant={ready ? "default" : "outline"}
              render={<Link to={`/migrations/${m.id}/map`} />}
              data-testid="migration-choose-continue"
            >
              <Trans>Continue</Trans>
              <ArrowRightIcon className="rtl:rotate-180" />
            </Button>
          )}
        </div>
      </div>
    </WizardLayout>
  )
}
