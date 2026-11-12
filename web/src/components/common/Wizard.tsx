import { Radio as RadioPrimitive } from "@base-ui/react/radio"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, ArrowRightIcon, CheckIcon, ChevronDownIcon, ChevronRightIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useLocation, useNavigate } from "react-router"

import { YuvaMark } from "@/components/common/Logo"
import { LanguageMenu } from "@/components/common/LanguageMenu"
import { ThemeToggle } from "@/components/common/PageLayout"
import { Button } from "@/components/ui/button"
import { RadioGroup } from "@/components/ui/radio-group"
import { Spinner } from "@/components/ui/spinner"
import { useMediaQuery } from "@/hooks/use-media-query"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export type WizardStep = { key: string; label: string }

export function StepBar({
  steps,
  current,
  stepHref,
  label,
  className,
}: {
  steps: WizardStep[]
  current: string
  stepHref?: (key: string) => string | undefined
  label: string
  className?: string
}) {
  const index = Math.max(
    0,
    steps.findIndex((s) => s.key === current)
  )
  return (
    <ol
      aria-label={label}
      className={cn("m-0 flex list-none flex-wrap gap-x-5 gap-y-1 border-b border-line p-0", className)}
    >
      {steps.map((s, i) => {
        const link = i < index ? stepHref?.(s.key) : undefined
        const cls = cn(
          "-mb-px inline-flex h-10 items-center gap-2 border-b-2 border-transparent text-body text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring",
          i === index && "border-brand font-medium text-foreground",
          i < index && "text-fg2",
          link && "hover:text-foreground"
        )
        const body = (
          <>
            <span
              className={cn("font-mono text-[11px] text-muted2", i === index && "text-brand", i < index && "text-ok")}
            >
              {i < index ? <CheckIcon aria-hidden className="size-3" /> : i + 1}
            </span>
            <span className={cn(i !== index && "max-sm:sr-only")}>{s.label}</span>
          </>
        )
        return (
          <li key={s.key} aria-current={i === index ? "step" : undefined}>
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
  )
}

export function Stepper({
  steps,
  current,
  stepHref,
  label,
  testId,
  className,
}: {
  steps: WizardStep[]
  current: string
  stepHref?: (key: string) => string | undefined
  label: string
  testId?: string
  className?: string
}) {
  const count = steps.length
  const step =
    Math.max(
      0,
      steps.findIndex((s) => s.key === current)
    ) + 1
  return (
    <div className={className}>
      <StepBar steps={steps} current={current} stepHref={stepHref} label={label} className="mb-6" />
      <p className="eyebrow font-mono text-muted2" data-testid={testId}>
        <Trans>
          Step {step} of {count}
        </Trans>
      </p>
    </div>
  )
}

export function StepHeader({
  title,
  children,
  icon,
}: {
  title: React.ReactNode
  children?: React.ReactNode
  icon?: React.ReactNode
}) {
  return (
    <div className="mb-7">
      {icon && (
        <span className="mb-4 grid size-10 place-items-center rounded-full bg-brand-wash text-brand [&_svg]:size-[18px]">
          {icon}
        </span>
      )}
      <h1 className="text-page text-balance outline-none" tabIndex={-1} data-wizard-heading="">
        {title}
      </h1>
      {children && <p className="mt-2 text-reading text-pretty text-muted-foreground">{children}</p>}
    </div>
  )
}

let framesMounted = 0

export function WizardFrame({
  steps,
  current,
  stepHref,
  stepsLabel,
  back,
  leave,
  onLeave,
  preview,
  previewOpen,
  children,
}: {
  steps?: WizardStep[]
  current?: string
  stepHref?: (key: string) => string | undefined
  stepsLabel: string
  back?: string
  leave?: React.ReactNode
  onLeave?: () => void
  preview: React.ReactNode
  previewOpen?: boolean
  children: React.ReactNode
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const location = useLocation()
  const state = (location.state ?? {}) as { back?: string }
  const { membership } = useSession()
  const wide = useMediaQuery("(min-width: 1024px)")
  const [shown, setShown] = useState(false)
  const main = useRef<HTMLElement>(null)
  const later = () => {
    onLeave?.()
    navigate(state.back ?? "/", { replace: true })
  }
  const index = steps && current ? steps.findIndex((s) => s.key === current) : -1

  useEffect(() => {
    const first = framesMounted++ === 0
    if (first) return
    const active = document.activeElement
    if (active && active !== document.body && main.current?.contains(active)) return
    main.current?.querySelector<HTMLElement>("[data-wizard-heading]")?.focus({ preventScroll: true })
  }, [current])

  return (
    <div className="flex min-h-svh flex-col bg-background" data-testid="setup">
      <header className="sticky top-0 z-10 flex h-13 shrink-0 items-center justify-between gap-3 border-t-2 border-b border-t-brand border-b-line-soft bg-background px-4 sm:px-6">
        <span className="flex min-w-0 items-center gap-2.5 px-1 sm:px-2.5">
          <YuvaMark className="size-6 shrink-0" />
          <span className="truncate text-[16px] font-semibold tracking-[-0.01em]">{membership.workspace.name}</span>
        </span>
        <span className="flex shrink-0 items-center gap-1">
          <LanguageMenu />
          <ThemeToggle />
          {leave !== undefined
            ? leave
            : steps &&
              index >= 0 &&
              index < steps.length - 1 && (
                <Button variant="ghost" size="sm" onClick={later} data-testid="setup-later">
                  {state.back ? <Trans>Cancel</Trans> : <Trans>Do this later</Trans>}
                </Button>
              )}
        </span>
      </header>
      <div className="grid flex-1 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <main ref={main} className="flex min-w-0 justify-center px-4 pt-8 pb-12 sm:px-8 lg:pt-[9vh] lg:pb-16">
          <div className="w-full max-w-[720px] lg:max-w-[36rem]">
            {back && (
              <Link
                to={back}
                state={state}
                className="mb-5 inline-flex w-fit items-center gap-1.5 text-small text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                data-testid="setup-back"
              >
                <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
                <Trans>Back</Trans>
              </Link>
            )}
            {steps && current && (
              <Stepper
                steps={steps}
                current={current}
                stepHref={stepHref}
                label={stepsLabel}
                testId="setup-step"
                className="mb-2"
              />
            )}
            {children}
            {!wide && (
              <section className="mt-10 border-t pt-4" aria-label={t`Preview`}>
                {!previewOpen && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="-ms-3"
                    onClick={() => setShown((s) => !s)}
                    aria-expanded={shown}
                    data-testid="setup-preview-toggle"
                  >
                    <ChevronDownIcon className={cn("transition-transform", shown && "rotate-180")} />
                    {shown ? <Trans>Hide the preview</Trans> : <Trans>Show the preview</Trans>}
                  </Button>
                )}
                {(previewOpen || shown) && (
                  <div className="mt-3 overflow-hidden rounded-2xl border bg-surface p-4 sm:p-6">{preview}</div>
                )}
              </section>
            )}
          </div>
        </main>
        {wide && (
          <aside className="flex border-s bg-surface" aria-label={t`Preview`}>
            <div className="sticky top-13 flex h-[calc(100svh-3.25rem)] flex-1 flex-col items-center justify-center overflow-hidden p-10 xl:p-14">
              {preview}
            </div>
          </aside>
        )}
      </div>
    </div>
  )
}

export function StepActions({
  children,
  secondary,
  pending,
  disabled,
  type = "submit",
  onClick,
  testId,
}: {
  children: React.ReactNode
  secondary?: React.ReactNode
  pending?: boolean
  disabled?: boolean
  type?: "submit" | "button"
  onClick?: () => void
  testId?: string
}) {
  return (
    <div className="sticky bottom-0 -mx-5 flex flex-wrap items-center gap-3 border-t bg-background px-5 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:static sm:mx-0 sm:border-t-0 sm:bg-transparent sm:px-0 sm:pt-0 sm:pb-0">
      <Button type={type} size="lg" disabled={disabled || pending} onClick={onClick} data-testid={testId}>
        {pending && <Spinner />}
        {children}
        <ArrowRightIcon className="rtl:rotate-180" />
      </Button>
      {secondary}
    </div>
  )
}

const rowClass =
  "group flex w-full items-center gap-3.5 rounded-2xl border bg-card px-4 py-3.5 text-start transition-colors outline-none hover:border-faint focus-visible:ring-3 focus-visible:ring-ring/30"

export type Choice = {
  value: string
  icon: React.ReactNode
  title: React.ReactNode
  hint?: React.ReactNode
  disabled?: React.ReactNode
  to?: string
  state?: unknown
  onSelect?: () => void
  onActive?: () => void
  active?: boolean
  testId?: string
}

function ChoiceBody({ choice, chevron }: { choice: Choice; chevron: boolean }) {
  return (
    <>
      <span className="grid size-9 shrink-0 place-items-center rounded-[10px] border bg-surface text-muted-foreground [&_svg]:size-4">
        {choice.icon}
      </span>
      <span className="min-w-0 flex-1">
        <span className={cn("block font-medium", !choice.hint && "truncate")}>{choice.title}</span>
        {choice.hint && <span className="mt-0.5 block text-small text-faint">{choice.hint}</span>}
        {choice.disabled && <span className="mt-0.5 block text-small text-faint">{choice.disabled}</span>}
      </span>
      {chevron && (
        <ChevronRightIcon className="size-4 shrink-0 text-faint transition-transform group-hover:translate-x-0.5 rtl:rotate-180" />
      )}
    </>
  )
}

export function ChoiceCards({
  items,
  value,
  onValueChange,
  label,
  className,
  testId,
}: {
  items: Choice[]
  value?: string
  onValueChange?: (value: string) => void
  label?: string
  className?: string
  testId?: string
}) {
  if (onValueChange) {
    return (
      <RadioGroup
        value={value}
        onValueChange={(v) => onValueChange(v as string)}
        aria-label={label}
        className={cn("flex flex-col gap-2.5", className)}
        data-testid={testId}
      >
        {items.map((c) => (
          <RadioPrimitive.Root
            key={c.value}
            value={c.value}
            disabled={!!c.disabled}
            className={cn(rowClass, "data-checked:border-brand disabled:cursor-not-allowed disabled:opacity-60")}
            onMouseEnter={c.onActive}
            onFocus={c.onActive}
            data-testid={c.testId}
          >
            <ChoiceBody choice={c} chevron={false} />
          </RadioPrimitive.Root>
        ))}
      </RadioGroup>
    )
  }
  return (
    <ul className={cn("flex flex-col gap-2.5", className)} aria-label={label} data-testid={testId}>
      {items.map((c) => {
        const cls = cn(rowClass, c.active && "border-faint")
        return (
          <li key={c.value}>
            {c.to ? (
              <Link to={c.to} state={c.state} className={cls} data-testid={c.testId}>
                <ChoiceBody choice={c} chevron />
              </Link>
            ) : (
              <button
                type="button"
                className={cls}
                onMouseEnter={c.onActive}
                onFocus={c.onActive}
                onClick={c.onSelect}
                data-testid={c.testId}
              >
                <ChoiceBody choice={c} chevron />
              </button>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function SummaryList({ items, className }: { items: [React.ReactNode, React.ReactNode][]; className?: string }) {
  return (
    <dl className={cn("grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-small", className)}>
      {items.map(([k, v], i) => (
        <div key={i} className="contents">
          <dt className="text-faint">{k}</dt>
          <dd className="truncate text-end">{v}</dd>
        </div>
      ))}
    </dl>
  )
}
