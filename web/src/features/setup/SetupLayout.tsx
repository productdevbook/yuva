import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, ChevronDownIcon } from "lucide-react"
import { useState } from "react"
import { Link, useLocation, useNavigate } from "react-router"

import { YuvaMark } from "@/components/common"
import { LanguageMenu } from "@/components/common/LanguageMenu"
import { Button } from "@/components/ui/button"
import { putOff, type SetupState } from "@/features/setup/setup"
import { useMediaQuery } from "@/hooks/use-media-query"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export const STEP_COUNT = 5

function Progress({ step }: { step: number }) {
  const { t } = useLingui()
  const names = [t`Name`, t`Channel`, t`Install`, t`First message`, t`Done`]
  return (
    <div className="mb-8">
      <ol className="flex gap-1.5" aria-label={t`Setup steps`}>
        {names.map((name, i) => (
          <li
            key={name}
            className={cn("h-1 flex-1 rounded-full bg-border transition-colors", i < step && "bg-brand")}
            aria-current={i + 1 === step ? "step" : undefined}
          >
            <span className="sr-only">{name}</span>
          </li>
        ))}
      </ol>
      <p className="mt-3 text-small text-faint" data-testid="setup-step">
        <Trans>
          Step {step} of {STEP_COUNT}
        </Trans>
        <span aria-hidden="true"> · </span>
        {names[step - 1]}
      </p>
    </div>
  )
}

export function SetupLayout({
  step,
  preview,
  previewOpen,
  back,
  children,
}: {
  step: number
  preview: React.ReactNode
  previewOpen?: boolean
  back?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const location = useLocation()
  const state = (location.state ?? {}) as SetupState
  const { workspaceId: ws, membership } = useSession()
  const wide = useMediaQuery("(min-width: 1024px)")
  const [shown, setShown] = useState(false)
  const later = () => {
    if (!state.back) putOff(ws)
    navigate(state.back ?? "/", { replace: true })
  }
  return (
    <div className="grid min-h-svh bg-background lg:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)]" data-testid="setup">
      <div className="flex min-w-0 flex-col px-5 sm:px-8">
        <header className="flex h-16 shrink-0 items-center justify-between gap-3">
          <span className="flex min-w-0 items-center gap-2.5">
            <YuvaMark className="size-[26px] shrink-0" />
            <span className="hidden truncate text-title sm:inline">{membership.workspace.name}</span>
          </span>
          <span className="flex shrink-0 items-center gap-1">
            <LanguageMenu />
            {step < STEP_COUNT && (
              <Button variant="ghost" size="sm" onClick={later} data-testid="setup-later">
                {state.back ? <Trans>Cancel</Trans> : <Trans>Do this later</Trans>}
              </Button>
            )}
          </span>
        </header>
        <main className="flex flex-1 justify-center pt-[5vh] pb-10 lg:pt-[12vh] lg:pb-16">
          <div className="w-full max-w-[30rem]">
            {back && (
              <Link
                to={back}
                state={state}
                className="mb-6 inline-flex w-fit items-center gap-1.5 text-body text-faint transition-colors hover:text-foreground"
                data-testid="setup-back"
              >
                <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
                <Trans>Back</Trans>
              </Link>
            )}
            <Progress step={step} />
            {children}
            {!wide && (
              <section className="mt-10 border-t pt-4" aria-label={t`Preview`}>
                {!previewOpen && (
                  <Button variant="ghost" size="sm" className="-ms-3" onClick={() => setShown((s) => !s)} aria-expanded={shown} data-testid="setup-preview-toggle">
                    <ChevronDownIcon className={cn("transition-transform", shown && "rotate-180")} />
                    {shown ? <Trans>Hide the preview</Trans> : <Trans>Show the preview</Trans>}
                  </Button>
                )}
                {(previewOpen || shown) && <div className="mt-3 overflow-hidden rounded-[1.5rem] bg-surface p-4 sm:p-6">{preview}</div>}
              </section>
            )}
          </div>
        </main>
      </div>
      {wide && (
        <aside className="flex p-3" aria-label={t`Preview`}>
          <div className="sticky top-3 flex max-h-[calc(100svh-1.5rem)] min-h-[calc(100svh-1.5rem)] flex-1 flex-col items-center justify-center overflow-hidden rounded-[1.75rem] bg-surface p-12">
            {preview}
          </div>
        </aside>
      )}
    </div>
  )
}

export function StepHeader({ title, children, icon }: { title: React.ReactNode; children?: React.ReactNode; icon?: React.ReactNode }) {
  return (
    <div className="mb-8">
      {icon && <span className="mb-5 grid size-11 place-items-center rounded-full bg-brand-wash text-brand [&_svg]:size-5">{icon}</span>}
      <h1 className="text-page text-balance">{title}</h1>
      {children && <p className="mt-3 text-pretty text-muted-foreground">{children}</p>}
    </div>
  )
}
