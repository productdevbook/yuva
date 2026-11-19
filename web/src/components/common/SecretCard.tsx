import { CodeLine } from "@/components/common/CopyButton"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { cn } from "@/lib/utils"

export function SecretCard({
  title,
  description,
  label,
  secret,
  hint,
  actions,
  className,
  testId,
  children,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  label: React.ReactNode
  secret: string
  hint: React.ReactNode
  actions: React.ReactNode
  className?: string
  testId?: string
  children?: React.ReactNode
}) {
  return (
    <Card className={cn("mt-4 gap-0 py-0 shadow-none", className)} data-testid={testId}>
      <CardHeader className="gap-0.5 border-b px-4 py-3 [.border-b]:pb-3">
        <CardTitle className="text-[15px] font-semibold">{title}</CardTitle>
        {description && <CardDescription className="text-small">{description}</CardDescription>}
      </CardHeader>
      <CardContent className="flex flex-col gap-2 px-4 pt-4">
        <p className="text-body font-medium">{label}</p>
        <CodeLine value={secret} testId={testId ? `${testId}-secret` : undefined} />
        <p className="text-small text-faint">{hint}</p>
        {children}
      </CardContent>
      <CardFooter className="flex-wrap gap-2 px-4 pt-4 pb-4">{actions}</CardFooter>
    </Card>
  )
}
