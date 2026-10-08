import { Trans, useLingui } from "@lingui/react/macro"
import { TagIcon } from "lucide-react"

import { Dot, Kbd } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { menuProps, pillClass, type ControlProps } from "@/features/conversation/controls/shared"
import { useLabels } from "@/lib/workspace"

export function LabelsMenu(p: ControlProps) {
  const { t } = useLingui()
  const labels = useLabels().data ?? []
  const current = new Set(p.conversation.labels)
  const chosen = labels.filter((l) => current.has(l.id))
  return (
    <DropdownMenu {...menuProps("labels", p)}>
      <DropdownMenuTrigger className={pillClass} aria-label={t`Edit labels`} data-testid="labels-menu">
        {chosen.length === 0 ? (
          <>
            <TagIcon />
            <Trans>Labels</Trans>
          </>
        ) : (
          chosen.map((l) => (
            <span key={l.id} className="flex min-w-0 items-center gap-1.5">
              <Dot color={l.color} className="size-1.5" />
              <span className="truncate">{l.name}</span>
            </span>
          ))
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="max-h-[60svh] min-w-52">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <Trans>Labels</Trans>
            <Kbd>{SHORTCUTS.labels}</Kbd>
          </DropdownMenuLabel>
          {labels.length === 0 && (
            <p className="px-2.5 py-1.5 text-sm text-muted-foreground">
              <Trans>No labels yet. Add them in settings.</Trans>
            </p>
          )}
          {labels.map((l) => (
            <DropdownMenuCheckboxItem
              key={l.id}
              checked={current.has(l.id)}
              onCheckedChange={(on) => {
                const next = new Set(current)
                if (on) next.add(l.id)
                else next.delete(l.id)
                p.update({ labels: [...next] })
              }}
            >
              <Dot color={l.color} />
              {l.name}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
