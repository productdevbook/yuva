import { Command as CommandPrimitive } from "cmdk"
import { Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { useShell } from "@/app/shell"
import { Kbd } from "@/components/common"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText } from "@/components/common/text"
import { Command, CommandDialog, CommandEmpty, CommandGroup, CommandItem, CommandList, CommandShortcut } from "@/components/ui/command"
import { hasCommands, runCommand, type CommandName } from "@/features/conversation/commands"
import { useInboxFilter, useQueue } from "@/features/inbox/queue"
import { useSession } from "@/lib/session"
import { useView } from "@/lib/view"
import { useChannelMap, useInboxes } from "@/lib/workspace"
import { setTheme } from "@/lib/theme"

type Item = { group: string; label: string; hint?: string; keys?: string[]; run: () => void }

function useItems(open: boolean): Item[] {
  const { t } = useLingui()
  const text = useEnumText()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { canManage } = useSession()
  const { openDrawer, openShortcuts } = useShell()
  const queue = useQueue()
  const inboxes = useInboxes().data ?? []
  const channels = useChannelMap()
  const [, setFilter] = useInboxFilter()
  const [view, setView] = useView()
  const commands = open && hasCommands() && (pathname === "/" || pathname.startsWith("/conversations/"))
  return useMemo(() => {
    if (!open) return []
    const inboxName = (id: string) => inboxes.find((i) => i.id === id)?.name
    const where = (inboxId: string, channelId?: string) => {
      const ch = channelId ? channels.get(channelId) : undefined
      return [inboxName(inboxId), ch && text.channel[ch.kind]].filter(Boolean).join(" · ")
    }
    const list: Item[] = []
    for (const c of queue.waiting) {
      list.push({ group: t`Waiting`, label: c.contact.name || c.contact.email || t`Unnamed contact`, hint: where(c.inbox_id, c.channel_id), run: () => queue.show(c.id) })
    }
    for (const c of queue.team) {
      list.push({
        group: t`With team`,
        label: c.contact.name || c.contact.email || t`Unnamed contact`,
        hint: where(c.inbox_id, c.channel_id),
        run: () => navigate(`/conversations/${c.id}`),
      })
    }
    if (commands) {
      const act = (name: CommandName, label: string, key?: string) =>
        list.push({ group: t`This conversation`, label, keys: key ? keyLabel(key) : undefined, run: () => runCommand(name) })
      act("reply", t`Write a reply`, SHORTCUTS.reply)
      act("note", t`Write a note for the team`, SHORTCUTS.note)
      act("snooze", t`Later`, SHORTCUTS.snooze)
      act("hand", t`Hand to a teammate`, SHORTCUTS.hand)
      act("close", t`Close without a reply`, SHORTCUTS.close)
      act("history", t`Other conversations`, SHORTCUTS.history)
      act("contact", t`Contact details`, SHORTCUTS.contact)
      act("more", t`Labels, priority, inbox`, SHORTCUTS.more)
      act("next", t`Next in the queue`, SHORTCUTS.next)
    }
    if (inboxes.length > 1) {
      list.push({ group: t`Inbox`, label: t`All inboxes`, run: () => (setFilter(""), queue.setCurrent(null)) })
      for (const i of inboxes) list.push({ group: t`Inbox`, label: t`Only ${i.name}`, run: () => (setFilter(i.id), queue.setCurrent(null)) })
    }
    const go = (label: string, to: string, keys?: string[]) => list.push({ group: t`Go to`, label, keys, run: () => navigate(to) })
    list.push({
      group: t`Go to`,
      label: view === "list" ? t`Switch to the queue` : t`Switch to the list`,
      keys: keyLabel(SHORTCUTS.view),
      run: () => {
        setView(view === "list" ? "queue" : "list")
        if (pathname !== "/") navigate("/")
      },
    })
    list.push({ group: t`Go to`, label: t`All conversations`, keys: [SHORTCUTS.search], run: () => openDrawer() })
    go(t`Settings`, "/settings", [mod, ","])
    go(t`Profile`, "/settings/profile")
    go(t`Notifications`, "/settings/notifications")
    go(t`Members`, "/settings/members")
    go(t`Canned replies`, "/settings/canned-replies")
    go(t`Labels`, "/settings/labels")
    go(t`Connected apps`, "/settings/connected-apps")
    for (const i of inboxes) go(t`${i.name} settings`, `/settings/inboxes/${i.id}`)
    if (canManage) {
      go(t`Add an inbox`, "/settings/new")
      go(t`API keys`, "/settings/api-keys")
      go(t`Webhooks`, "/settings/webhooks")
      go(t`Workspace`, "/settings/workspace")
    }
    list.push({ group: t`Go to`, label: t`Keyboard shortcuts`, keys: [SHORTCUTS.help], run: openShortcuts })
    list.push({ group: t`Appearance`, label: t`Light appearance`, run: () => setTheme("light") })
    list.push({ group: t`Appearance`, label: t`Dark appearance`, run: () => setTheme("dark") })
    list.push({ group: t`Appearance`, label: t`Follow the system appearance`, run: () => setTheme("system") })
    return list
  }, [open, commands, queue, inboxes, channels, text, t, navigate, openDrawer, openShortcuts, setFilter, canManage, view, setView, pathname])
}

export function Palette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const { openDrawer } = useShell()
  const [q, setQ] = useState("")
  const all = useItems(open)
  useEffect(() => {
    if (open) setQ("")
  }, [open])
  const groups = useMemo(() => {
    const out: { name: string; items: Item[] }[] = []
    for (const x of all) {
      const g = out.at(-1)
      if (g && g.name === x.group) g.items.push(x)
      else out.push({ name: x.group, items: [x] })
    }
    return out
  }, [all])
  const run = (x: Item) => {
    onOpenChange(false)
    x.run()
  }
  const search = q.trim()
  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t`Everything`}
      description={t`Search people, actions or settings`}
      className="top-[14vh] w-[560px] max-w-[calc(100%-32px)] gap-0 rounded-[18px]! border bg-card p-0 shadow-[0_30px_80px_-20px_rgb(0_0_0/0.35)] sm:max-w-[560px]"
    >
      <Command className="rounded-[18px]! bg-card p-0" data-testid="palette" loop>
        <CommandPrimitive.Input
          value={q}
          onValueChange={setQ}
          placeholder={t`Search people, actions or settings…`}
          aria-label={t`Search people, actions or settings`}
          className="w-full border-b bg-transparent px-[18px] py-4 text-base outline-none placeholder:text-faint"
        />
        <CommandList className="max-h-[50vh] p-1.5">
          <CommandEmpty className="py-10 text-faint">
            <Trans>No results</Trans>
          </CommandEmpty>
          {groups.map((g) => (
            <CommandGroup
              key={`${g.name}:${g.items[0]?.label}`}
              heading={g.name}
              className="p-0 **:[[cmdk-group-heading]]:px-3 **:[[cmdk-group-heading]]:pt-2.5 **:[[cmdk-group-heading]]:pb-1 **:[[cmdk-group-heading]]:text-[11px] **:[[cmdk-group-heading]]:font-normal **:[[cmdk-group-heading]]:tracking-[0.04em] **:[[cmdk-group-heading]]:text-faint **:[[cmdk-group-heading]]:uppercase"
            >
              {g.items.map((x, i) => (
                <CommandItem
                  key={`${x.label}:${i}`}
                  value={`${x.label} ${x.hint ?? ""} ${g.name} ${i}`}
                  onSelect={() => run(x)}
                  className="gap-2.5 rounded-[10px]! px-3 py-[9px] data-selected:bg-brand-wash"
                >
                  <span className="truncate">{x.label}</span>
                  {x.hint && <small className="min-w-0 truncate text-xs text-faint">{x.hint}</small>}
                  {x.keys && (
                    <CommandShortcut className="flex gap-1 tracking-normal">
                      {x.keys.map((k) => (
                        <Kbd key={k}>{k}</Kbd>
                      ))}
                    </CommandShortcut>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          ))}
          {search.length >= 2 && (
            <CommandGroup heading={t`Search`} forceMount className="p-0 **:[[cmdk-group-heading]]:px-3 **:[[cmdk-group-heading]]:pt-2.5 **:[[cmdk-group-heading]]:pb-1 **:[[cmdk-group-heading]]:text-[11px] **:[[cmdk-group-heading]]:font-normal **:[[cmdk-group-heading]]:text-faint **:[[cmdk-group-heading]]:uppercase">
              <CommandItem
                forceMount
                value={`search ${search}`}
                onSelect={() => run({ group: "", label: "", run: () => openDrawer(search) })}
                className="gap-2.5 rounded-[10px]! px-3 py-[9px] data-selected:bg-brand-wash"
              >
                <Trans>Search conversations for “{search}”</Trans>
              </CommandItem>
            </CommandGroup>
          )}
        </CommandList>
      </Command>
    </CommandDialog>
  )
}
