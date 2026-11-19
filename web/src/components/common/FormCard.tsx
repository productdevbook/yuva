import { Trans } from "@lingui/react/macro"

import { ErrorLine } from "@/components/common/ErrorLine"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { cn } from "@/lib/utils"

export function FormCard({
  title,
  description,
  submit,
  onSubmit,
  onCancel,
  cancel,
  busy,
  ready = true,
  error,
  className,
  testId,
  children,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  submit: React.ReactNode
  onSubmit: () => void
  onCancel?: () => void
  cancel?: React.ReactNode
  busy?: boolean
  ready?: boolean
  error?: unknown
  className?: string
  testId?: string
  children: React.ReactNode
}) {
  return (
    <Card className={cn("mt-4 gap-0 py-0 shadow-none", className)} data-testid={testId}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          if (ready && !busy) onSubmit()
        }}
      >
        <CardHeader className="gap-0.5 border-b px-4 py-3 [.border-b]:pb-3">
          <CardTitle className="text-[15px] font-semibold">{title}</CardTitle>
          {description && <CardDescription className="text-small">{description}</CardDescription>}
        </CardHeader>
        <CardContent className="flex flex-col gap-4 px-4 pt-4">{children}</CardContent>
        <CardFooter className="flex-wrap gap-2 px-4 pt-4 pb-4">
          <ErrorLine error={error} className="basis-full" />
          <Button type="submit" disabled={!ready || busy} data-testid={testId ? `${testId}-submit` : undefined}>
            {submit}
          </Button>
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel}>
              {cancel ?? <Trans>Cancel</Trans>}
            </Button>
          )}
        </CardFooter>
      </form>
    </Card>
  )
}

export function FormGrid({ children }: { children: React.ReactNode }) {
  return <div className="grid gap-4 sm:grid-cols-2">{children}</div>
}
