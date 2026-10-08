import { useLingui } from "@lingui/react/macro"
import { MenuIcon } from "lucide-react"
import { createContext, useContext } from "react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export const ShellContext = createContext<{ openNav: () => void }>({ openNav: () => {} })

function NavButton({ className }: { className?: string }) {
  const { t } = useLingui()
  const { openNav } = useContext(ShellContext)
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      className={cn("-ms-1.5 md:hidden", className)}
      onClick={openNav}
      aria-label={t`Open the menu`}
      data-testid="open-nav"
    >
      <MenuIcon />
    </Button>
  )
}

export function PaneHeader({
  title,
  children,
  leading,
  className,
}: {
  title: React.ReactNode
  children?: React.ReactNode
  leading?: React.ReactNode
  className?: string
}) {
  return (
    <header className={cn("flex h-14 shrink-0 items-center gap-2 px-4", className)}>
      {leading ?? <NavButton />}
      <h1 className="min-w-0 flex-1 truncate text-[0.95rem] font-semibold tracking-tight">{title}</h1>
      {children}
    </header>
  )
}
