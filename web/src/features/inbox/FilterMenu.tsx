import { Trans, useLingui } from "@lingui/react/macro"
import { ListFilterIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { ListBase, ListFilters } from "@/features/inbox/views"
import { cn } from "@/lib/utils"
import { useInboxes, useLabels, useMembers } from "@/lib/workspace"

function Radio({ label, value, onChange, items }: { label: React.ReactNode; value: string; onChange: (v: string) => void; items: [string, React.ReactNode][] }) {
  return (
    <DropdownMenuGroup>
      <DropdownMenuLabel>{label}</DropdownMenuLabel>
      <DropdownMenuRadioGroup value={value} onValueChange={(v) => onChange(String(v))}>
        {items.map(([v, text]) => (
          <DropdownMenuRadioItem key={v} value={v}>
            <span className="truncate">{text}</span>
          </DropdownMenuRadioItem>
        ))}
      </DropdownMenuRadioGroup>
    </DropdownMenuGroup>
  )
}

export function FilterMenu({
  base,
  filters,
  setFilters,
}: {
  base: ListBase
  filters: ListFilters
  setFilters: (patch: Partial<ListFilters>) => void
}) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const members = useMembers().data ?? []
  const active =
    (base.kind !== "inbox" && filters.inbox !== "") ||
    (base.kind !== "label" && filters.label !== "") ||
    (base.kind !== "view" && filters.assignee !== "")
  const groups: React.ReactNode[] = []
  if (base.kind !== "view") {
    groups.push(
      <Radio
        key="assignee"
        label={<Trans>Assignee</Trans>}
        value={filters.assignee}
        onChange={(assignee) => setFilters({ assignee })}
        items={[
          ["", <Trans>Anyone</Trans>],
          ["me", <Trans>Me</Trans>],
          ["unassigned", <Trans>Unassigned</Trans>],
          ...members.map((m): [string, React.ReactNode] => [m.id, m.name || m.email]),
        ]}
      />,
    )
  }
  if (base.kind !== "inbox") {
    groups.push(
      <Radio
        key="inbox"
        label={<Trans>Inbox</Trans>}
        value={filters.inbox}
        onChange={(inbox) => setFilters({ inbox })}
        items={[["", <Trans>All inboxes</Trans>], ...inboxes.map((i): [string, React.ReactNode] => [i.id, i.name])]}
      />,
    )
  }
  if (base.kind !== "label" && labels.length > 0) {
    groups.push(
      <Radio
        key="label"
        label={<Trans>Label</Trans>}
        value={filters.label}
        onChange={(label) => setFilters({ label })}
        items={[["", <Trans>Any label</Trans>], ...labels.map((l): [string, React.ReactNode] => [l.id, l.name])]}
      />,
    )
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon-sm" className={cn(active && "bg-brand-wash text-brand hover:bg-brand-wash hover:text-brand")} />}
        aria-label={t`Filters`}
      >
        <ListFilterIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[70svh] min-w-56">
        {groups.flatMap((g, i) => (i === 0 ? [g] : [<DropdownMenuSeparator key={`s${i}`} />, g]))}
        {active && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => setFilters({ inbox: "", label: "", assignee: "" })}>
              <Trans>Clear filters</Trans>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
