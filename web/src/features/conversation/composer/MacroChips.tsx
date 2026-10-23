import { useLingui } from "@lingui/react/macro"
import { XIcon, ZapIcon } from "lucide-react"

import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { macroSteps, type MacroPick } from "@/features/conversation/macros"
import { useLabels, useMemberMap } from "@/lib/workspace"

export function useDuration() {
  const { i18n } = useLingui()
  return (minutes: number) => {
    const [unit, n] =
      minutes % 1440 === 0
        ? (["day", minutes / 1440] as const)
        : minutes % 60 === 0
          ? (["hour", minutes / 60] as const)
          : (["minute", minutes] as const)
    return new Intl.NumberFormat(i18n.locale, { style: "unit", unit, unitDisplay: "long" }).format(n)
  }
}

export function useStepText() {
  const { t } = useLingui()
  const text = useEnumText()
  const labels = useLabels().data ?? []
  const members = useMemberMap()
  const duration = useDuration()
  return (pick: MacroPick, step: ReturnType<typeof macroSteps>[number]) => {
    const a = pick.macro.actions
    const labelName = labels.find((l) => l.id === step.id)?.name ?? "?"
    switch (step.kind) {
      case "add":
        return t`Add label ${labelName}`
      case "remove":
        return t`Remove label ${labelName}`
      case "assignee": {
        if (!step.id) return t`Unassign`
        const m = members.get(step.id)
        const name = m ? m.name || m.email : "?"
        return t`Assign to ${name}`
      }
      case "status": {
        if (a.snooze_minutes !== undefined) {
          const time = duration(a.snooze_minutes)
          return t`Snooze for ${time}`
        }
        const status = text.statusOf(a.status ?? "open")
        return t`Status: ${status}`
      }
      case "priority": {
        const priority = text.priority[a.priority ?? "normal"]
        return t`Priority: ${priority}`
      }
    }
  }
}

export function MacroChips({ pick, onChange }: { pick: MacroPick; onChange: (pick: MacroPick | undefined) => void }) {
  const { t } = useLingui()
  const stepText = useStepText()
  const steps = macroSteps(pick.macro)
    .filter((s) => !pick.dropped.includes(s.key))
    .map((s) => ({ ...s, label: stepText(pick, s) }))
  const chip = "flex max-w-full items-center gap-1.5 rounded-lg border bg-background py-0.5 ps-2 pe-0.5 text-caption"
  return (
    <ul className="flex flex-wrap gap-1.5 px-4 pb-2" data-testid="composer-macro">
      <li className={chip}>
        <ZapIcon className="size-3 shrink-0 text-brand" />
        <span className="max-w-48 truncate font-medium">{pick.macro.name}</span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="text-faint"
          aria-label={t`Remove ${pick.macro.name}`}
          onClick={() => onChange(undefined)}
        >
          <XIcon />
        </Button>
      </li>
      {steps.map((s) => {
        return (
          <li key={s.key} className={chip} data-testid="macro-chip">
            <span className="max-w-56 truncate">{s.label}</span>
            <Button
              variant="ghost"
              size="icon-xs"
              className="text-faint"
              aria-label={t`Remove ${s.label}`}
              onClick={() => {
                const dropped = [...pick.dropped, s.key]
                onChange(steps.length === 1 ? undefined : { ...pick, dropped })
              }}
            >
              <XIcon />
            </Button>
          </li>
        )
      })}
    </ul>
  )
}
