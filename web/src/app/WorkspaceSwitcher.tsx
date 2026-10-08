import { Trans } from "@lingui/react/macro"
import { ChevronsUpDownIcon } from "lucide-react"

import { YuvaMark } from "@/components/common"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useSession } from "@/lib/session"

export function WorkspaceSwitcher() {
  const { me, membership, switchWorkspace } = useSession()
  const name = membership.workspace.name
  const header = (
    <>
      <YuvaMark className="size-6" />
      <span className="min-w-0 flex-1 truncate text-start text-[0.95rem] font-semibold tracking-tight">{name}</span>
    </>
  )
  if (me.memberships.length < 2) return <div className="flex w-full items-center gap-2.5 px-2">{header}</div>
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="flex h-10 w-full items-center gap-2.5 rounded-lg px-2 transition-colors outline-none hover:bg-muted aria-expanded:bg-muted"
        aria-label={name}
        data-testid="workspace-switcher"
      >
        {header}
        <ChevronsUpDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>Workspaces</Trans>
          </DropdownMenuLabel>
          <DropdownMenuRadioGroup value={membership.workspace.id} onValueChange={(id) => switchWorkspace(String(id))}>
            {me.memberships.map((m) => (
              <DropdownMenuRadioItem key={m.workspace.id} value={m.workspace.id}>
                <span className="truncate">{m.workspace.name}</span>
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
