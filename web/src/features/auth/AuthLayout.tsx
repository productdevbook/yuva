import { Trans } from "@lingui/react/macro"

import { YuvaMark } from "@/components/common"
import { LanguageMenu } from "@/components/common/LanguageMenu"
import { ThemeToggle } from "@/components/common/PageLayout"
import { useVersion } from "@/lib/api"
import { cn } from "@/lib/utils"

export const authField = "h-9 rounded-md px-2.5"
export const authAlert =
  "rounded-md border border-destructive/25 bg-destructive/5 px-3 py-2.5 text-body text-destructive"

const QUICK_START = "https://github.com/productdevbook/yuva/blob/main/docs/install.md"

export function AuthLayout({
  children,
  wide,
  below,
}: {
  children: React.ReactNode
  wide?: boolean
  below?: React.ReactNode
}) {
  const version = useVersion().data?.version
  return (
    <div className="flex min-h-svh flex-col border-t-2 border-brand bg-background" data-testid="auth-layout">
      <header className="mx-auto flex w-full max-w-[1440px] shrink-0 items-center gap-2 px-4 py-[18px] sm:px-6">
        <span className="flex min-w-0 flex-1 items-center gap-2.5 px-1 sm:px-2.5">
          <YuvaMark className="size-6 shrink-0" />
          <span className="text-[16px] font-semibold tracking-[-0.01em]">Yuva</span>
          {version && (
            <span className="rounded border border-input px-1.5 font-mono text-[11px] text-muted-foreground">
              {version}
            </span>
          )}
        </span>
        <LanguageMenu />
        <ThemeToggle />
      </header>
      <main className="flex flex-1 flex-col px-4 pt-6 pb-16 sm:pt-10">
        <div className={cn("mx-auto w-full", wide ? "max-w-[720px]" : "max-w-[440px]")}>{children}</div>
        {below && (
          <div
            className={cn(
              "mx-auto mt-8 flex w-full flex-col items-center gap-2 border-t border-line-soft pt-5 text-center text-small text-muted-foreground",
              wide ? "max-w-[720px]" : "max-w-[440px]"
            )}
          >
            {below}
          </div>
        )}
      </main>
      <footer className="flex flex-wrap justify-center gap-x-[18px] gap-y-1.5 px-4 pt-5 pb-7 text-small text-muted-foreground">
        <span>
          <Trans>
            Self-hosting?{" "}
            <a
              href={QUICK_START}
              target="_blank"
              rel="noreferrer noopener"
              className="font-medium text-foreground underline decoration-input underline-offset-4 hover:decoration-foreground"
            >
              Read the quick start
            </a>
          </Trans>
        </span>
      </footer>
    </div>
  )
}

export function AuthHeading({
  icon,
  tone = "brand",
  title,
  children,
  titleTestId,
  className,
}: {
  icon?: React.ReactNode
  tone?: "brand" | "muted" | "success" | "danger"
  title: React.ReactNode
  children?: React.ReactNode
  titleTestId?: string
  className?: string
}) {
  return (
    <div className={cn("mb-6 flex flex-col items-start", className)}>
      {icon && (
        <span
          className={cn(
            "mb-3 [&_svg]:size-5",
            tone === "brand" && "text-brand",
            tone === "muted" && "text-muted2",
            tone === "success" && "text-ok",
            tone === "danger" && "text-destructive"
          )}
        >
          {icon}
        </span>
      )}
      <h1 className="m-0 text-page text-balance break-words" data-testid={titleTestId}>
        {title}
      </h1>
      {children && <div className="mt-1.5 text-body text-pretty text-muted-foreground">{children}</div>}
    </div>
  )
}

export function AuthDivider({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 text-caption text-muted2">
      <span className="h-px flex-1 bg-border" />
      {children}
      <span className="h-px flex-1 bg-border" />
    </div>
  )
}
