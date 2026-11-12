import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowDownIcon, CheckIcon, LifeBuoyIcon, MailIcon, ShieldCheckIcon } from "lucide-react"
import { useEffect, useRef } from "react"
import { Link, Navigate, useParams, useSearchParams } from "react-router"

import { BackArrow, ErrorLine, InfoSection, KV, PageTop, SummaryList, WizardFrame, YuvaMark } from "@/components/common"
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
import { SettingsAside, SettingsLayout, SettingsTrail } from "@/features/settings/SettingsFrame"
import { PreviewCaption } from "@/features/setup/previews"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Migration, MigrationPreset } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
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

export function WizardLayout(props: {
  step: StepKey
  m?: Migration
  preset?: MigrationPreset
  address?: string
  children: React.ReactNode
}) {
  const phone = useIsPhone()
  return phone ? <StackedWizard {...props} /> : <FlatWizard {...props} />
}

function useStepLinks(step: StepKey, m?: Migration, preset?: MigrationPreset) {
  const names = useStepNames()
  const steps = stepsFor(isFile(m, preset))
  const index = steps.indexOf(step)
  const prev = index > 0 ? steps[index - 1] : undefined
  const back = m && prev && reachable(prev, m) ? `/migrations/${m.id}/${prev}` : undefined
  const href = (key: StepKey) => (m && reachable(key, m) ? `/migrations/${m.id}/${key}` : undefined)
  return { names, steps, index, back, href }
}

function StackedWizard({
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
  const { names, steps, back, href } = useStepLinks(step, m, preset)
  return (
    <WizardFrame
      current={step}
      steps={steps.map((s) => ({ key: s, label: names[s] }))}
      stepHref={(key) => href(key as StepKey)}
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

function FlatWizard({
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
  const { names, steps, index, back, href } = useStepLinks(step, m, preset)
  const journey = useJourney(m, preset, address ?? m?.source_address)
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
      <PageTop back={{ label: t`Move to Yuva`, to: "/settings/migrations", testId: "migration-close" }}>
        <SettingsTrail place="migrations" name={journey.from} />
      </PageTop>
      <div ref={main} className="mt-3 flex max-w-[720px] min-w-0 flex-col" data-testid="setup">
        <p className="sr-only" data-testid="setup-step">
          <StepOf step={index + 1} count={steps.length} />
          {" · "}
          {names[step]}
        </p>
        <ol
          aria-label={t`Migration steps`}
          className="m-0 mb-8 flex list-none flex-wrap gap-x-5 gap-y-1 border-b border-line p-0"
        >
          {steps.map((s, i) => {
            const link = i < index ? href(s) : undefined
            const cls = cn(
              "-mb-px inline-flex h-10 items-center gap-2 border-b-2 border-transparent text-body text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring",
              i === index && "border-brand font-medium text-foreground",
              i < index && "text-fg2",
              link && "hover:text-foreground"
            )
            const body = (
              <>
                <span
                  className={cn(
                    "font-mono text-[11px] text-muted2",
                    i === index && "text-brand",
                    i < index && "text-ok"
                  )}
                >
                  {i < index ? <CheckIcon aria-hidden className="size-3" /> : i + 1}
                </span>
                {names[s]}
              </>
            )
            return (
              <li key={s} aria-current={i === index ? "step" : undefined}>
                {link ? (
                  <Link to={link} className={cls}>
                    {body}
                  </Link>
                ) : (
                  <span className={cls}>{body}</span>
                )}
              </li>
            )
          })}
        </ol>
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
        <div className="max-w-[640px]">{children}</div>
      </div>
      <SettingsAside>
        <InfoSection title={journey.provider} testId="migration-journey">
          <KV label={t`Source`} value={journey.from} />
          <KV label={t`Yuva inbox`} value={journey.to} />
          {journey.facts.map(([label, value]) => (
            <KV key={label} label={label} value={value} />
          ))}
        </InfoSection>
        <InfoSection title={t`Read-only`}>
          <p className="m-0 text-small leading-5 text-muted-foreground">{journey.note}</p>
        </InfoSection>
      </SettingsAside>
    </SettingsLayout>
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

function Journey({ m, preset, address }: { m?: Migration; preset?: MigrationPreset; address?: string }) {
  const { file, desk, provider, from, to, facts, note } = useJourney(m, preset, address)
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
            <span className="block truncate font-medium">{from}</span>
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
            <span className="block truncate font-medium">{to}</span>
          </span>
        </div>
        {facts.length > 0 && <SummaryList items={facts} className="border-t pt-4" />}
        <p className="flex items-center gap-2 border-t pt-4 text-small text-muted-foreground">
          <ShieldCheckIcon className="size-4 shrink-0 text-success" />
          {file ? <Trans>Access to your mailbox: none</Trans> : <Trans>Changes made to {provider}: none</Trans>}
        </p>
      </div>
      <PreviewCaption>{note}</PreviewCaption>
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
