import { Trans, useLingui } from "@lingui/react/macro"
import { ShieldCheckIcon } from "lucide-react"
import { useEffect, useRef } from "react"
import { Link, Navigate, useParams, useSearchParams } from "react-router"

import { BackArrow, ErrorLine, StepBar } from "@/components/common"
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
import { SettingsLayout } from "@/features/settings/SettingsFrame"
import { Page, PageHeader } from "@/features/settings/ui"
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

const LINE = ["source", "connect", "choose", "start"] as const
type LineKey = (typeof LINE)[number]
const LINE_OF: Partial<Record<StepKey, LineKey>> = {
  source: "source",
  connect: "connect",
  choose: "choose",
  map: "choose",
  review: "start",
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
  const steps = stepsFor(isFile(m, preset))
  const index = steps.indexOf(step)
  const prev = index > 0 ? steps[index - 1] : undefined
  const back = m && prev && LINE_OF[step] && reachable(prev, m) ? `/migrations/${m.id}/${prev}` : undefined
  const journey = useJourney(m, preset, address ?? m?.source_address)
  const line = LINE_OF[step]
  const names: Record<LineKey, string> = {
    source: t`Source`,
    connect: t`Connect`,
    choose: t`Choose`,
    start: t`Review`,
  }
  const lineHref = (key: string) => {
    if (!m) return undefined
    const target = steps.find((s) => LINE_OF[s] === key && reachable(s, m))
    return target ? `/migrations/${m.id}/${target}` : undefined
  }
  const main = useRef<HTMLDivElement>(null)
  const first = useRef(true)
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    const active = document.activeElement
    if (active && active !== document.body && main.current?.contains(active)) return
    main.current?.querySelector<HTMLElement>("[data-wizard-heading]")?.focus({ preventScroll: true })
  }, [step])
  return (
    <SettingsLayout>
      <Page width="form" testId="setup">
        <PageHeader
          back={{ label: t`Imports`, to: "/settings/migrations" }}
          title={m ? journey.from : <Trans>New import</Trans>}
          description={
            m ? (
              <>
                {journey.provider} → {journey.to}
              </>
            ) : (
              <Trans>Copy the history of a mailbox or help desk into Yuva.</Trans>
            )
          }
        />
        <div ref={main} className="flex min-w-0 flex-col">
          {line ? (
            <>
              <p className="sr-only" data-testid="setup-step">
                <StepOf step={LINE.indexOf(line) + 1} count={LINE.length} />
                {" · "}
                {names[line]}
              </p>
              <StepBar
                steps={LINE.map((k) => ({ key: k, label: names[k] }))}
                current={line}
                stepHref={lineHref}
                label={t`Import steps`}
                className="mb-6"
              />
              <p
                className="mb-6 flex items-start gap-2 text-small text-muted-foreground"
                data-testid="migration-read-only"
              >
                <ShieldCheckIcon className="mt-0.5 size-3.5 shrink-0 text-success" aria-hidden />
                {journey.note}
              </p>
              {back && (
                <Link
                  to={back}
                  className="mb-5 inline-flex w-fit items-center gap-1.5 text-small text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                  data-testid="setup-back"
                >
                  <BackArrow className="size-3.5" />
                  <Trans>Back</Trans>
                </Link>
              )}
            </>
          ) : (
            m && <Facts items={journey.facts} />
          )}
          {children}
        </div>
      </Page>
    </SettingsLayout>
  )
}

function Facts({ items }: { items: [string, string][] }) {
  return (
    <dl
      className="mb-6 grid grid-cols-2 gap-x-4 gap-y-2.5 rounded-xl border bg-card px-4 py-3 sm:grid-cols-4"
      data-testid="migration-journey"
    >
      {items.map(([label, value]) => (
        <div key={label} className="flex min-w-0 flex-col gap-0.5">
          <dt className="text-caption text-muted-foreground">{label}</dt>
          <dd className="m-0 truncate text-body">{value}</dd>
        </div>
      ))}
    </dl>
  )
}

function StepOf({ step, count }: { step: number; count: number }) {
  return (
    <Trans>
      Step {step} of {count}
    </Trans>
  )
}

function useJourney(m?: Migration, preset?: MigrationPreset, address?: string) {
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
  const note = file
    ? t`Yuva reads only the file you upload and deletes it when the migration ends. History arrives quietly: no notifications, no auto-replies.`
    : desk
      ? t`Yuva only reads the help desk. History arrives quietly: no notifications, no auto-replies.`
      : t`Yuva only reads the mailbox. History arrives quietly: no notifications, no auto-replies.`
  return {
    file,
    desk,
    provider,
    from: address || (desk ? t`Your support address` : t`Your mailbox`),
    to: inbox?.name ?? t`Chosen in the Map step`,
    facts,
    note,
  }
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
  if (!m || !presets.data) return <div className="min-h-svh bg-background" data-testid="migration-loading" />
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
