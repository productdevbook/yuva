import { Trans, useLingui } from "@lingui/react/macro"

import { Kbd } from "@/components/common/Kbd"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"

export const SHORTCUTS = {
  next: "j",
  reply: "r",
  note: "n",
  attach: "f",
  snooze: "s",
  hand: "a",
  close: "e",
  leave: "L",
  history: "h",
  contact: "c",
  more: ".",
  spam: "!",
  quoted: "q",
  discardSuggestion: "X",
  copyDetails: "y",
  search: "/",
  help: "?",
} as const

export function keyLabel(k: string) {
  return /^[A-Z]$/.test(k) ? ["⇧", k] : [k.toUpperCase()]
}

export const mod = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl"

export function ShortcutSheet({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const k = keyLabel
  const groups: { title: string; items: [string[], string][] }[] = [
    {
      title: t`Answering`,
      items: [
        [k(SHORTCUTS.reply), t`Write a reply`],
        [k(SHORTCUTS.note), t`Write a note for the team`],
        [[mod, "↵"], t`Send and close`],
        [["⇧", mod, "↵"], t`Send only`],
        [["Tab"], t`Use the suggested reply`],
        [k(SHORTCUTS.discardSuggestion), t`Discard the suggested reply`],
        [k(SHORTCUTS.attach), t`Attach files`],
        [["/"], t`Canned replies, in the reply box`],
        [k(SHORTCUTS.leave), t`Leave it to the teammate who is typing`],
      ],
    },
    {
      title: t`This conversation`,
      items: [
        [k(SHORTCUTS.next), t`Next in the queue`],
        [k(SHORTCUTS.snooze), t`Later`],
        [k(SHORTCUTS.hand), t`Hand to a teammate`],
        [k(SHORTCUTS.close), t`Close without a reply`],
        [k(SHORTCUTS.history), t`Other conversations`],
        [k(SHORTCUTS.contact), t`Contact details`],
        [k(SHORTCUTS.more), t`Labels, priority, inbox`],
        [k(SHORTCUTS.spam), t`Mark or unmark as spam`],
        [k(SHORTCUTS.quoted), t`Show or hide quoted text`],
        [k(SHORTCUTS.copyDetails), t`Copy the feedback details`],
      ],
    },
    {
      title: t`Everywhere`,
      items: [
        [[SHORTCUTS.search], t`Search conversations`],
        [[mod, "K"], t`Everything`],
        [[mod, ","], t`Settings`],
        [[SHORTCUTS.help], t`This list`],
        [["Esc"], t`Close a panel or leave a field`],
      ],
    },
  ]
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-2xl" data-testid="shortcut-sheet">
        <DialogHeader>
          <DialogTitle>
            <Trans>Keyboard shortcuts</Trans>
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-x-8 gap-y-6 sm:grid-cols-2">
          {groups.map((g) => (
            <section key={g.title} className="flex flex-col">
              <h3 className="mb-1 text-xs font-medium text-faint">{g.title}</h3>
              <ul className="flex flex-col">
                {g.items.map(([ks, label]) => (
                  <li key={label} className="flex items-center justify-between gap-3 border-t py-1.5 text-sm">
                    <span>{label}</span>
                    <span className="flex shrink-0 gap-1">
                      {ks.map((key) => (
                        <Kbd key={key}>{key}</Kbd>
                      ))}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
