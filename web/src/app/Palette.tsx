import { Dialog as DialogPrimitive } from "@base-ui/react/dialog"
import { Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useRef, useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { useShell } from "@/app/shell"
import { Kbd } from "@/components/common"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText } from "@/components/common/text"
import { overlayClass } from "@/components/ui/dialog"
import { hasCommands, runCommand, type CommandName } from "@/features/conversation/commands"
import { useInboxFilter, useQueue } from "@/features/inbox/queue"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

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
    return list
  }, [open, commands, queue, inboxes, channels, text, t, navigate, openDrawer, openShortcuts, setFilter, canManage])
}

export function Palette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const { openDrawer } = useShell()
  const [q, setQ] = useState("")
  const [index, setIndex] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)
  const all = useItems(open)
  const query = q.trim().toLowerCase()
  const items = useMemo(() => {
    const found = all.filter((x) => `${x.label} ${x.hint ?? ""} ${x.group}`.toLowerCase().includes(query))
    if (query.length >= 2) {
      found.push({ group: t`Search`, label: t`Search conversations for “${q.trim()}”`, run: () => openDrawer(q.trim()) })
    }
    return found
  }, [all, query, q, t, openDrawer])
  const active = Math.max(0, Math.min(index, items.length - 1))

  useEffect(() => {
    if (open) {
      setQ("")
      setIndex(0)
    }
  }, [open])
  useEffect(() => {
    listRef.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: "nearest" })
  }, [active, open])

  const run = (x: Item | undefined) => {
    if (!x) return
    onOpenChange(false)
    x.run()
  }

  let last = ""
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className={overlayClass} />
        <DialogPrimitive.Popup
          className="fixed top-[14vh] left-1/2 z-50 w-[560px] max-w-[calc(100%-32px)] -translate-x-1/2 overflow-hidden rounded-[18px] border bg-card text-foreground shadow-[0_30px_80px_-20px_rgb(0_0_0/0.35)] outline-none"
          data-testid="palette"
        >
          <DialogPrimitive.Title className="sr-only">
            <Trans>Everything</Trans>
          </DialogPrimitive.Title>
          <input
            autoFocus
            value={q}
            onChange={(e) => {
              setQ(e.target.value)
              setIndex(0)
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault()
                setIndex(active + (e.key === "ArrowDown" ? 1 : -1))
              } else if (e.key === "Enter") {
                e.preventDefault()
                run(items[active])
              }
            }}
            placeholder={t`Search people, actions or settings…`}
            aria-label={t`Search people, actions or settings`}
            className="w-full border-b bg-transparent px-[18px] py-4 text-base outline-none placeholder:text-faint"
          />
          <div ref={listRef} className="max-h-[50vh] overflow-y-auto p-1.5" role="listbox">
            {items.length === 0 && (
              <p className="px-4 py-10 text-center text-sm text-faint">
                <Trans>No results</Trans>
              </p>
            )}
            {items.map((x, i) => {
              const head = x.group !== last
              last = x.group
              return (
                <div key={`${x.group}:${x.label}:${i}`}>
                  {head && <div className="px-3 pt-2.5 pb-1 text-[11px] tracking-[0.04em] text-faint uppercase">{x.group}</div>}
                  <button
                    type="button"
                    role="option"
                    aria-selected={i === active}
                    data-active={i === active}
                    onMouseMove={() => i !== active && setIndex(i)}
                    onClick={() => run(x)}
                    className={cn("flex w-full items-center gap-2.5 rounded-[10px] px-3 py-[9px] text-start text-sm", i === active && "bg-brand-wash")}
                  >
                    <span className="truncate">{x.label}</span>
                    {x.hint && <small className="min-w-0 truncate text-xs text-faint">{x.hint}</small>}
                    {x.keys && (
                      <span className="ms-auto flex shrink-0 gap-1">
                        {x.keys.map((k) => (
                          <Kbd key={k}>{k}</Kbd>
                        ))}
                      </span>
                    )}
                  </button>
                </div>
              )
            })}
          </div>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
