import { Trans } from "@lingui/react/macro"
import { CheckIcon } from "lucide-react"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { look } from "@/features/inbox/PageFrame"
import { cn } from "@/lib/utils"

export function Sec({
  id,
  title,
  description,
  pill,
  tone,
  children,
}: {
  id: string
  title: React.ReactNode
  description?: React.ReactNode
  pill?: React.ReactNode
  tone?: "danger"
  children: React.ReactNode
}) {
  return (
    <section id={id} aria-labelledby={`${id}-h`} className="flex scroll-mt-4 flex-col gap-3">
      <div className="flex items-start gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <h2
            id={`${id}-h`}
            className={cn("text-[15px] leading-[22px] font-semibold", tone === "danger" && "text-destructive")}
          >
            {title}
          </h2>
          {description && <p className="text-body text-muted-foreground">{description}</p>}
        </div>
        {pill}
      </div>
      {children}
    </section>
  )
}

export const secCard = "overflow-hidden rounded-[14px] border bg-card"

export function SecBody({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("flex flex-col gap-4 p-5 @max-[760px]:p-4", className)}>{children}</div>
}

export function SecFoot({
  dirty,
  pending,
  saved,
  error,
  readOnly,
  invalid,
  onDiscard,
  label,
}: {
  dirty: boolean
  pending: boolean
  saved: boolean
  error: unknown
  readOnly?: React.ReactNode
  invalid?: boolean
  onDiscard: () => void
  label?: React.ReactNode
}) {
  return (
    <div
      className={cn(
        "flex min-h-14 flex-wrap items-center gap-x-3 gap-y-2 border-t bg-surface px-5 py-2.5 @max-[760px]:px-4",
        dirty && "bg-brand-wash"
      )}
      data-dirty={dirty || undefined}
    >
      {readOnly ? (
        <p className="min-w-0 flex-1 text-body text-muted-foreground">{readOnly}</p>
      ) : (
        <>
          <div className="flex min-w-0 flex-1 items-center gap-2 text-body text-muted-foreground">
            {error ? (
              <ErrorLine error={error} className="min-w-0 flex-1" />
            ) : dirty ? (
              <>
                <span className="size-2 shrink-0 rounded-full bg-brand" aria-hidden />
                <Trans>Unsaved changes</Trans>
              </>
            ) : saved ? (
              <span className="flex items-center gap-1.5 text-success" role="status">
                <CheckIcon className="size-3.5" aria-hidden />
                <Trans>Saved</Trans>
              </span>
            ) : null}
          </div>
          {dirty && (
            <Button type="button" variant="outline" className={look.btnSm} onClick={onDiscard} disabled={pending}>
              <Trans>Discard</Trans>
            </Button>
          )}
          <Button type="submit" className={look.btnInkSm} disabled={pending || !dirty || invalid}>
            {label ?? <Trans>Save</Trans>}
          </Button>
        </>
      )}
    </div>
  )
}

export function RadioRow({
  on,
  disabled,
  onPick,
  title,
  hint,
  testId,
}: {
  on: boolean
  disabled?: boolean
  onPick: () => void
  title: React.ReactNode
  hint: React.ReactNode
  testId?: string
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={on}
      disabled={disabled}
      onClick={onPick}
      data-testid={testId}
      className={cn(
        "flex w-full items-start gap-3 px-4 py-3.5 text-start outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset disabled:cursor-not-allowed disabled:opacity-60",
        on ? "bg-brand-wash" : "hover:bg-surface"
      )}
    >
      <Dot on={on} />
      <span className="flex min-w-0 flex-col gap-0.5">
        <b className="font-semibold">{title}</b>
        <span className="text-body text-muted-foreground">{hint}</span>
      </span>
    </button>
  )
}

export function Dot({ on, className }: { on: boolean; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "mt-px size-4 shrink-0 rounded-full border-[1.5px] bg-card",
        on ? "border-[5px] border-brand" : "border-input",
        className
      )}
    />
  )
}

export function StepMark({ tone }: { tone: "ok" | "warn" | "bad" | "busy" | "none" }) {
  if (tone === "none" || tone === "busy")
    return (
      <span
        aria-hidden
        className={cn(
          "size-4 shrink-0 rounded-full border-[1.5px] border-input",
          tone === "busy" && "border-faint border-t-transparent motion-safe:animate-spin"
        )}
      />
    )
  return (
    <span
      aria-hidden
      className={cn(
        "grid size-4 shrink-0 place-items-center rounded-full text-[10px] leading-none font-bold text-card",
        tone === "ok" && "bg-success",
        tone === "warn" && "bg-warning",
        tone === "bad" && "bg-destructive"
      )}
    >
      {tone === "ok" ? "✓" : tone === "warn" ? "!" : "✕"}
    </span>
  )
}
