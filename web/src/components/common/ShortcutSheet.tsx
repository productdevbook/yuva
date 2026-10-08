import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { Kbd } from "@/components/common/Kbd"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useHotkeys } from "@/hooks/use-hotkeys"

export const SHORTCUTS = {
  next: "j",
  previous: "k",
  search: "/",
  back: "Escape",
  reply: "r",
  note: "n",
  attach: "f",
  assign: "a",
  assignMe: "i",
  status: "s",
  close: "e",
  reopen: "o",
  priority: "p",
  labels: "l",
  spam: "!",
  move: "m",
  quoted: "q",
  contact: "c",
  copyDetails: "y",
  help: "?",
} as const

const mod = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl"

export function useShortcutSheet() {
  const [open, setOpen] = useState(false)
  useHotkeys({ [SHORTCUTS.help]: () => setOpen(true) })
  return { open, setOpen }
}

export function ShortcutSheet({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const groups: { title: string; items: [string[], string][] }[] = [
    {
      title: t`Conversations`,
      items: [
        [[SHORTCUTS.next], t`Next conversation`],
        [[SHORTCUTS.previous], t`Previous conversation`],
        [[SHORTCUTS.search], t`Search`],
        [["Esc"], t`Back to the list`],
        [[SHORTCUTS.contact], t`Show or hide the contact`],
      ],
    },
    {
      title: t`Thread`,
      items: [
        [[SHORTCUTS.reply], t`Write a reply`],
        [[SHORTCUTS.note], t`Write a note`],
        [[SHORTCUTS.attach], t`Attach files`],
        [[mod, "Enter"], t`Send`],
        [["/"], t`Insert a canned reply (in the composer)`],
        [[SHORTCUTS.assign], t`Assign`],
        [[SHORTCUTS.assignMe], t`Assign to me`],
        [[SHORTCUTS.status], t`Change status`],
        [[SHORTCUTS.close], t`Close`],
        [[SHORTCUTS.reopen], t`Reopen`],
        [[SHORTCUTS.priority], t`Change priority`],
        [[SHORTCUTS.labels], t`Edit labels`],
        [[SHORTCUTS.spam], t`Mark or unmark as spam`],
        [[SHORTCUTS.move], t`Move to another inbox`],
        [[SHORTCUTS.quoted], t`Show or hide quoted text`],
        [[SHORTCUTS.copyDetails], t`Copy the feedback details`],
      ],
    },
    { title: t`General`, items: [[[SHORTCUTS.help], t`Keyboard shortcuts`]] },
  ]
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            <Trans>Keyboard shortcuts</Trans>
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-6 sm:grid-cols-2">
          {groups.map((g) => (
            <section key={g.title} className="flex flex-col gap-2">
              <h3 className="text-xs font-medium text-faint">{g.title}</h3>
              <ul className="flex flex-col gap-1.5">
                {g.items.map(([ks, label]) => (
                  <li key={label} className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
                    <span>{label}</span>
                    <span className="flex shrink-0 gap-1">
                      {ks.map((k) => (
                        <Kbd key={k}>{k}</Kbd>
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
