import { ArrowLeftIcon } from "lucide-react"
import { Link, NavLink } from "react-router"

import { cn } from "@/lib/utils"

export function Column({ title, testId, actions, children, bodyClassName }: { title: React.ReactNode; testId: string; actions?: React.ReactNode; children: React.ReactNode; bodyClassName?: string }) {
  return (
    <section className="flex h-full min-h-0 flex-col bg-background" data-testid={testId}>
      <header className="flex h-15 shrink-0 items-center gap-2 ps-4 pe-2">
        <h2 className="min-w-0 flex-1 truncate text-title">{title}</h2>
        {actions}
      </header>
      <div className={cn("min-h-0 flex-1 overflow-y-auto px-3 pb-4", bodyClassName)}>{children}</div>
    </section>
  )
}

export function ColumnHeading({ children }: { children: React.ReactNode }) {
  return <h3 className="px-2.5 pt-4 pb-1.5 text-caption font-medium text-faint">{children}</h3>
}

export function ColumnLink({ to, end, state, children, testId }: { to: string; end?: boolean; state?: unknown; children: React.ReactNode; testId?: string }) {
  return (
    <NavLink
      to={to}
      end={end}
      state={state}
      className="flex min-h-12 items-center gap-3 rounded-xl px-2.5 py-2 outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-muted"
      data-testid={testId}
    >
      {children}
    </NavLink>
  )
}

export function Pane({ wide, testId, children, className }: { wide?: boolean; testId?: string; children: React.ReactNode; className?: string }) {
  return (
    <main className="min-h-0 flex-1 overflow-y-auto bg-background" data-testid={testId}>
      <div className={cn("mx-auto flex w-full flex-col gap-8 px-6 pt-6 pb-24 phone:px-4 phone:pt-3", wide ? "max-w-[1040px]" : "max-w-[640px]", className)}>{children}</div>
    </main>
  )
}

export function PaneEmpty({ icon: Icon, title, children, testId }: { icon: React.ComponentType<{ className?: string }>; title: React.ReactNode; children?: React.ReactNode; testId?: string }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center bg-surface px-8 text-center" data-testid={testId}>
      <div className="mb-5 grid size-16 place-items-center rounded-[20px] bg-brand-wash text-brand">
        <Icon className="size-7" />
      </div>
      <h1 className="text-title">{title}</h1>
      {children && <div className="mt-1.5 max-w-md text-body text-muted-foreground">{children}</div>}
    </div>
  )
}

export function PaneBack({ to, label, always }: { to: string; label: string; always?: boolean }) {
  return (
    <Link
      to={to}
      className={cn("mb-4.5 w-fit items-center gap-1.5 text-body text-faint transition-colors hover:text-foreground", always ? "inline-flex" : "hidden phone:inline-flex")}
      data-testid="pane-back"
    >
      <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
      {label}
    </Link>
  )
}
