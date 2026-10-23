import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowDownIcon, LifeBuoyIcon, MailIcon, ShieldCheckIcon } from "lucide-react"
import { Link, Navigate, useParams, useSearchParams } from "react-router"

import { ErrorLine, SummaryList, WizardFrame, YuvaMark } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import { ChooseStep } from "@/features/migrations/ChooseStep"
import { ConnectStep } from "@/features/migrations/ConnectStep"
import { CopyStep } from "@/features/migrations/CopyStep"
import { DoneStep } from "@/features/migrations/DoneStep"
import { MapStep } from "@/features/migrations/MapStep"
import { useMigration, usePresets } from "@/features/migrations/queries"
import { ReviewStep } from "@/features/migrations/ReviewStep"
import { SignInConnectStep } from "@/features/migrations/SignInConnectStep"
import { TokenConnectStep } from "@/features/migrations/TokenConnectStep"
import { SourceStep } from "@/features/migrations/SourceStep"
import { SwitchStep } from "@/features/migrations/SwitchStep"
import { isHelpdesk, useFormatName, useMigrationText } from "@/features/migrations/text"
import { UploadStep } from "@/features/migrations/UploadStep"
import { PreviewCaption } from "@/features/setup/previews"
import type { Migration, MigrationPreset } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export const STEPS = ["source", "connect", "choose", "map", "review", "copy", "switch", "done"] as const
export type StepKey = (typeof STEPS)[number]

export function stepsFor(file: boolean): readonly StepKey[] {
  return file ? STEPS.filter((s) => s !== "switch") : STEPS
}

function isFile(m?: Migration, preset?: MigrationPreset) {
  return (m?.source ?? preset?.source) === "file"
}

const EARLY = ["draft", "connected", "scanning", "ready"]

export function currentStep(m: Migration): StepKey {
  switch (m.state) {
    case "draft":
      return "connect"
    case "connected":
      return "choose"
    case "scanning":
      return m.estimate ? "review" : "choose"
    case "ready":
      return m.channel_id ? "review" : "map"
    case "copying":
    case "paused":
      return "copy"
    case "needs_attention":
      return m.started_at ? "copy" : "connect"
    case "syncing":
      return "switch"
    default:
      return "done"
  }
}

function reachable(step: StepKey, m: Migration) {
  if (!stepsFor(isFile(m)).includes(step)) return false
  if (
    step === "connect" &&
    (m.actions.includes("set_credentials") || m.actions.includes("upload") || m.actions.includes("sign_in"))
  )
    return true
  if (!EARLY.includes(m.state)) return step === currentStep(m)
  if (step === "source") return m.state === "draft"
  if (step === "connect") return true
  if (step === "choose" || step === "map") return m.state !== "draft"
  return step === currentStep(m)
}

function useStepNames() {
  const { t } = useLingui()
  return {
    source: t`Source`,
    connect: t`Connect`,
    choose: t`Choose`,
    map: t`Map`,
    review: t`Review`,
    copy: t`Copy`,
    switch: t`Switch`,
    done: t`Done`,
  } satisfies Record<StepKey, string>
}

export function WizardLayout({
  step,
  m,
  preset,
  address,
  children,
}: {
  step: StepKey
  m?: Migration
  preset?: MigrationPreset
  address?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  const names = useStepNames()
  const steps = stepsFor(isFile(m, preset))
  const index = steps.indexOf(step)
  const prev = index > 0 ? steps[index - 1] : undefined
  const back = m && prev && reachable(prev, m) ? `/migrations/${m.id}/${prev}` : undefined
  return (
    <WizardFrame
      current={step}
      steps={steps.map((s) => ({ key: s, label: names[s] }))}
      stepHref={(key) => (m && reachable(key as StepKey, m) ? `/migrations/${m.id}/${key}` : undefined)}
      stepsLabel={t`Migration steps`}
      back={back ?? (step === "source" ? "/settings/migrations" : undefined)}
      leave={
        <Button variant="ghost" size="sm" render={<Link to="/settings/migrations" />} data-testid="migration-close">
          <Trans>Close</Trans>
        </Button>
      }
      preview={<Journey m={m} preset={preset} address={address ?? m?.source_address} />}
    >
      {children}
    </WizardFrame>
  )
}

function Journey({ m, preset, address }: { m?: Migration; preset?: MigrationPreset; address?: string }) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const text = useMigrationText()
  const formatName = useFormatName()
  const file = isFile(m, preset)
  const desk = isHelpdesk(m?.source ?? preset?.source)
  const inboxes = useInboxes().data ?? []
  const inbox = m?.inbox_id ? inboxes.find((i) => i.id === m.inbox_id) : undefined
  const provider = preset?.name ?? t`Your mail provider`
  const est = m?.estimate
  const since = m?.options.since
  const facts: [string, string][] = []
  if (m && file) {
    const format = m.upload?.format
    facts.push([t`File`, m.upload?.completed_at ? (m.upload.filename ?? t`Uploaded`) : t`Not uploaded yet`])
    if (format) facts.push([t`Format`, formatName[format]])
  } else if (m) {
    const host = m.credentials?.server.host ?? ""
    facts.push([t`Connection`, host ? t`Connected to ${host}` : t`Not connected yet`])
  }
  if (m) {
    facts.push([t`State`, text.state[m.state]])
    facts.push([t`Since`, since ? formatDate(since, i18n.locale) : t`Everything`])
    if (m.started_at) {
      const imported = num(m.counts.imported)
      facts.push([t`Imported`, desk ? t`${imported} conversations` : t`${imported} messages`])
    } else if (est) {
      const conversations = num(est.conversations)
      const messages = num(est.messages)
      facts.push([t`Dry run`, t`${conversations} conversations, ${messages} messages`])
    }
  }
  return (
    <>
      <div
        className="flex w-full max-w-sm flex-col gap-4 rounded-2xl border bg-card p-5 shadow-[0_24px_60px_-28px_rgb(15_23_42/0.35)]"
        data-testid="migration-journey"
      >
        <div className="flex items-center gap-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-surface text-muted-foreground [&_svg]:size-[18px]">
            {desk ? <LifeBuoyIcon /> : <MailIcon />}
          </span>
          <span className="min-w-0">
            <span className="block text-caption text-faint">{provider}</span>
            <span className="block truncate font-medium">
              {address || (desk ? t`Your support address` : t`Your mailbox`)}
            </span>
          </span>
        </div>
        <ArrowDownIcon className="ms-3 size-4 text-faint" aria-hidden="true" />
        <div className="flex items-center gap-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-brand-wash">
            <YuvaMark className="size-5" />
          </span>
          <span className="min-w-0">
            <span className="block text-caption text-faint">
              <Trans>Yuva inbox</Trans>
            </span>
            <span className="block truncate font-medium">{inbox?.name ?? t`Chosen in the Map step`}</span>
          </span>
        </div>
        {facts.length > 0 && <SummaryList items={facts} className="border-t pt-4" />}
        <p className="flex items-center gap-2 border-t pt-4 text-small text-muted-foreground">
          <ShieldCheckIcon className="size-4 shrink-0 text-success" />
          {file ? <Trans>Access to your mailbox: none</Trans> : <Trans>Changes made to {provider}: none</Trans>}
        </p>
      </div>
      <PreviewCaption>
        {file ? (
          <Trans>
            Yuva reads only the file you upload and deletes it when the migration ends. History arrives quietly: no
            notifications, no auto-replies.
          </Trans>
        ) : desk ? (
          <Trans>Yuva only reads the help desk. History arrives quietly: no notifications, no auto-replies.</Trans>
        ) : (
          <Trans>Yuva only reads the mailbox. History arrives quietly: no notifications, no auto-replies.</Trans>
        )}
      </PreviewCaption>
    </>
  )
}

export function NewMigrationPage() {
  const { canManage } = useSession()
  const [params] = useSearchParams()
  if (!canManage) return <Navigate to="/" replace />
  return <SourceStep inboxId={params.get("inbox") ?? undefined} />
}

export function MigrationPage() {
  const { canManage } = useSession()
  const { migrationId, step } = useParams()
  const query = useMigration(migrationId)
  const presets = usePresets()
  if (!canManage) return <Navigate to="/" replace />
  const m = query.data
  const error = query.error ?? presets.error
  if (error)
    return (
      <WizardLayout step="source">
        <ErrorLine error={error} />
        <Button variant="outline" className="mt-6" render={<Link to="/settings/migrations" />}>
          <Trans>Back to the migrations</Trans>
        </Button>
      </WizardLayout>
    )
  if (!m || !presets.data) return <div className="min-h-svh bg-background" />
  const wanted = stepsFor(isFile(m)).includes(step as StepKey) ? (step as StepKey) : currentStep(m)
  if (!reachable(wanted, m)) return <Navigate to={`/migrations/${m.id}/${currentStep(m)}`} replace />
  if (!step) return <Navigate to={`/migrations/${m.id}/${wanted}`} replace />
  const preset = presets.data.find((p) => p.id === m.preset)!
  switch (wanted) {
    case "source":
      return <SourceStep migration={m} />
    case "connect":
      return isFile(m) ? (
        <UploadStep key={m.id} m={m} preset={preset} />
      ) : preset.auth.includes("token") ? (
        <TokenConnectStep key={m.id} m={m} preset={preset} />
      ) : isHelpdesk(m.source) || m.preset === "microsoft" ? (
        <SignInConnectStep key={m.id} m={m} preset={preset} />
      ) : (
        <ConnectStep key={m.id} m={m} preset={preset} />
      )
    case "choose":
      return <ChooseStep key={m.id} m={m} preset={preset} />
    case "map":
      return <MapStep key={m.id} m={m} preset={preset} />
    case "review":
      return <ReviewStep key={m.id} m={m} preset={preset} />
    case "copy":
      return <CopyStep key={m.id} m={m} preset={preset} />
    case "switch":
      return <SwitchStep key={m.id} m={m} preset={preset} />
    default:
      return <DoneStep key={m.id} m={m} preset={preset} />
  }
}
