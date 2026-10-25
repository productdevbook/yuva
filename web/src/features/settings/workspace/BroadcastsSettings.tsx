import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useRequestApproval } from "@/features/broadcasts/broadcastQueries"
import { StatusNotice, useNumber } from "@/features/broadcasts/BroadcastParts"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { look } from "@/features/inbox/PageFrame"
import { LoadProblem } from "@/features/reports/parts"
import { Field, Meter, Pill } from "@/features/settings/ui"
import { Sec, SecBody, secCard, StepMark } from "@/features/settings/workspace/parts"
import { api, unwrap, type BroadcastsStatus, type WorkspaceBroadcasts } from "@/lib/api"
import { keys } from "@/lib/keys"
import { meKey, useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

function useStatusText(): Record<BroadcastsStatus, string> {
  const { t } = useLingui()
  return {
    allowed: t`Allowed`,
    approval_required: t`Needs the operator’s approval`,
    approval_requested: t`Waiting for the operator`,
    suspended: t`Suspended`,
  }
}

function Counter({ value }: { value: string }) {
  const { i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <span className="self-end text-caption text-faint tabular-nums">{`${fmt.format(value.length)} / ${fmt.format(2000)}`}</span>
  )
}

function ApprovalForm() {
  const { t } = useLingui()
  const [website, setWebsite] = useState("")
  const [sender, setSender] = useState("")
  const [consent, setConsent] = useState("")
  const request = useRequestApproval()
  return (
    <form
      className={secCard}
      onSubmit={(e) => {
        e.preventDefault()
        request.mutate({
          website: website.trim(),
          sender_description: sender.trim(),
          consent_description: consent.trim(),
        })
      }}
    >
      <SecBody>
        <div className="flex flex-col gap-0.5">
          <b className="font-semibold">
            <Trans>Ask for approval</Trans>
          </b>
          <p className="text-body text-muted-foreground">
            <Trans>
              The server’s operator reads this before letting the workspace send. New workspaces start with a small
              daily limit that grows with every clean day.
            </Trans>
          </p>
        </div>
        <Field label={<Trans>Your website</Trans>} htmlFor="approval-website">
          <Input
            id="approval-website"
            type="url"
            required
            maxLength={2000}
            placeholder="https://example.com"
            value={website}
            onChange={(e) => setWebsite(e.target.value)}
            className="h-[38px] rounded-[9px] border-input"
            data-testid="approval-website"
          />
        </Field>
        <Field
          label={<Trans>Who sends, and what about?</Trans>}
          htmlFor="approval-sender"
          hint={<Trans>For example: a monthly product newsletter from our company to our customers.</Trans>}
        >
          <Textarea
            id="approval-sender"
            required
            rows={3}
            maxLength={2000}
            value={sender}
            onChange={(e) => setSender(e.target.value)}
            className="rounded-[9px] border-input"
            data-testid="approval-sender"
          />
          <Counter value={sender} />
        </Field>
        <Field
          label={<Trans>How did your subscribers agree?</Trans>}
          htmlFor="approval-consent"
          hint={<Trans>For example: a subscribe form with confirmation on our site, or a checkbox at sign-up.</Trans>}
        >
          <Textarea
            id="approval-consent"
            required
            rows={3}
            maxLength={2000}
            placeholder={t`They signed up on our website and confirmed their address.`}
            value={consent}
            onChange={(e) => setConsent(e.target.value)}
            className="rounded-[9px] border-input"
            data-testid="approval-consent"
          />
          <Counter value={consent} />
        </Field>
      </SecBody>
      <div className="flex min-h-14 flex-wrap items-center gap-3 border-t bg-surface px-5 py-2.5 @max-[760px]:px-4">
        <ErrorLine error={request.error} className="min-w-0 flex-1" />
        <span className="flex-1" />
        <Button
          type="submit"
          className={look.btnInkSm}
          disabled={request.isPending || !website.trim() || !sender.trim() || !consent.trim()}
        >
          <Trans>Ask for approval</Trans>
        </Button>
      </div>
    </form>
  )
}

function KeysSwitch({ b }: { b: WorkspaceBroadcasts }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const save = useMutation({
    mutationFn: (on: boolean) => unwrap(api.PATCH("/v1/workspace", { body: { broadcasts_by_keys: on } })),
    onSuccess: (w) => {
      qc.setQueryData(keys.workspace(ws), w)
      void qc.invalidateQueries({ queryKey: meKey })
    },
  })
  const on = save.isPending ? save.variables : b.by_keys
  return (
    <div className={secCard}>
      <SecBody className="gap-3">
        <div className="flex items-start gap-3">
          <Switch
            checked={on}
            disabled={save.isPending}
            onCheckedChange={(v) => save.mutate(v)}
            aria-label={t`Keys and assistants may send broadcasts`}
            data-testid="broadcasts-by-keys"
          />
          <span className="flex min-w-0 flex-col gap-1">
            <span className="font-semibold">
              <Trans>Keys and assistants may send broadcasts</Trans>
            </span>
            <span className="text-body text-muted-foreground">
              <Trans>
                When off, API keys and connected assistants can write drafts and send tests, and a member schedules
                them. When on, a key or assistant with the “Schedule and send broadcasts” permission can schedule and
                resume them.
              </Trans>
            </span>
          </span>
        </div>
        <ErrorLine error={save.error} />
      </SecBody>
    </div>
  )
}

function Steps({ status }: { status: BroadcastsStatus }) {
  const text = useStatusText()
  const tones: Record<BroadcastsStatus, ("ok" | "warn" | "bad" | "busy" | "none")[]> = {
    allowed: ["ok", "ok", "ok"],
    approval_required: ["warn", "none", "none"],
    approval_requested: ["ok", "busy", "none"],
    suspended: ["ok", "ok", "bad"],
  }
  const labels = [
    text.approval_required,
    text.approval_requested,
    status === "suspended" ? text.suspended : text.allowed,
  ]
  return (
    <ol className="flex flex-wrap items-center gap-x-2 gap-y-1.5">
      {labels.map((label, i) => (
        <li key={i} className="flex items-center gap-2 text-small text-muted-foreground">
          {i > 0 && <span className="hidden h-px w-4 bg-border @min-[760px]:block" aria-hidden />}
          <StepMark tone={tones[status][i]} />
          <span className={cn(tones[status][i] !== "none" && "font-semibold text-foreground")}>{label}</span>
        </li>
      ))}
    </ol>
  )
}

export function BroadcastsSettings() {
  const { i18n } = useLingui()
  const num = useNumber()
  const statusText = useStatusText()
  const { workspaceId: ws, membership, canManage } = useSession()
  const workspace = useQuery({ queryKey: keys.workspace(ws), queryFn: () => unwrap(api.GET("/v1/workspace")) })
  const b = workspace.data?.broadcasts ?? membership.workspace.broadcasts
  const title = <Trans>Broadcast sending</Trans>
  if (!b)
    return (
      <Sec id="w-bc" title={title}>
        {workspace.isPending ? (
          <Skeleton className="h-52 rounded-[14px]" aria-busy="true" data-testid="broadcasts-loading" />
        ) : (
          <LoadProblem error={workspace.error} onRetry={() => void workspace.refetch()} testId="broadcasts-error" />
        )}
      </Sec>
    )
  const owner = membership.role === "owner"
  const limit = b.daily_limit !== undefined ? num(b.daily_limit) : null
  const sent = num(b.sent_today)
  const since = b.status_at ? formatDateTime(b.status_at, i18n.locale) : null
  const tone = b.status === "allowed" ? "success" : b.status === "suspended" ? "danger" : "warning"
  const near = b.daily_limit ? b.sent_today / b.daily_limit >= 0.9 : false
  const showReason = b.reason && b.status !== "suspended"
  const showSince = since && b.status !== "allowed"
  return (
    <Sec
      id="w-bc"
      title={title}
      pill={
        b.mode === "off" ? undefined : (
          <Pill tone={tone} data-testid="broadcasts-status">
            {statusText[b.status]}
          </Pill>
        )
      }
    >
      {b.mode === "off" ? (
        <StatusNotice settingsLink={false} />
      ) : (
        <>
          <div className={secCard} data-testid="broadcasts-settings">
            <SecBody>
              <Steps status={b.status} />
              {(showReason || showSince) && (
                <div
                  className={cn(
                    "flex flex-col gap-0.5 rounded-[10px] px-3 py-2.5 text-body",
                    b.status === "approval_required" || b.status === "approval_requested"
                      ? "bg-warning/10 text-warning"
                      : "bg-muted text-muted-foreground"
                  )}
                >
                  {showReason && <span>{b.reason}</span>}
                  {showSince && (
                    <span className="text-small opacity-80">
                      <Trans>Since {since}</Trans>
                    </span>
                  )}
                </div>
              )}
              <div className="flex flex-col gap-2">
                <b className="text-[28px] leading-8 font-semibold tracking-[-0.02em] tabular-nums">{sent}</b>
                {b.daily_limit ? (
                  <Meter
                    value={b.sent_today}
                    max={b.daily_limit}
                    color={near ? "var(--warning)" : "var(--success)"}
                    className="h-2"
                  />
                ) : null}
                <p className="text-small text-faint tabular-nums" data-testid="broadcasts-today">
                  {limit ? (
                    <Trans>
                      Sent today: {sent} of {limit} allowed. The day starts at 00:00 UTC.
                    </Trans>
                  ) : (
                    <Trans>Sent today: {sent}. There is no daily limit.</Trans>
                  )}
                </p>
              </div>
            </SecBody>
          </div>
          {b.status !== "allowed" && <StatusNotice settingsLink={false} />}
          {b.status === "approval_required" && owner && <ApprovalForm />}
          {canManage && <KeysSwitch b={b} />}
        </>
      )}
    </Sec>
  )
}
