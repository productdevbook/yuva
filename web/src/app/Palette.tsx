import { Command as CommandPrimitive } from "cmdk"
import { Trans, useLingui } from "@lingui/react/macro"
import { useMemo, useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { inboxRoot, isChats, useShell } from "@/app/shell"
import { Kbd } from "@/components/common"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText } from "@/components/common/text"
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command"
import { useReplyChoices } from "@/features/conversation/composer/CannedMenu"
import { hasCommands, runCommand, runInsert, type CommandName } from "@/features/conversation/commands"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { focusListSearch, setListQuery } from "@/features/inbox/listSearch"
import { useConversation } from "@/features/conversation/queries"
import { useConversations } from "@/features/inbox/queries"
import { useAnalyticsVisible } from "@/features/analytics/properties"
import { useSession } from "@/lib/session"
import { isSupport, useChannelMap, useInboxes } from "@/lib/workspace"
import { setTheme } from "@/lib/theme"

type Item = { group: string; label: string; hint?: string; keys?: string[]; run: () => void }

function useItems(open: boolean): Item[] {
  const { t } = useLingui()
  const text = useEnumText()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { canManage, isOwner } = useSession()
  const analytics = useAnalyticsVisible()
  const { openShortcuts } = useShell()
  const inboxList = useInboxes().data
  const channels = useChannelMap()
  const [inboxId, setFilter] = useInboxFilter()
  const recent = useConversations({ status: ["open"], purpose: "support", inbox_id: inboxId || undefined }, open)
  const conversationId = /^\/(?:conversations|feedback)\/([^/]+)/.exec(pathname)?.[1]
  const current = useConversation(open ? conversationId : undefined).data
  const replies = useReplyChoices()
  const commands =
    open && hasCommands() && (pathname.startsWith("/conversations/") || pathname.startsWith("/feedback/"))
  return useMemo(() => {
    if (!open) return []
    const inboxes = inboxList ?? []
    const inboxName = (id: string) => inboxes.find((i) => i.id === id)?.name
    const where = (inboxId: string, channelId?: string) => {
      const ch = channelId ? channels.get(channelId) : undefined
      return [inboxName(inboxId), ch && text.channel[ch.kind]].filter(Boolean).join(" · ")
    }
    const list: Item[] = []
    for (const c of recent.data?.pages[0]?.items ?? []) {
      list.push({
        group: t`Open conversations`,
        label: c.contact.name || c.contact.email || t`Unnamed contact`,
        hint: where(c.inbox_id, c.channel_id),
        run: () => navigate(`/conversations/${c.id}`),
      })
    }
    if (commands) {
      const act = (name: CommandName, label: string, key?: string) =>
        list.push({
          group: t`This conversation`,
          label,
          keys: key ? keyLabel(key) : undefined,
          run: () => runCommand(name),
        })
      act("reply", t`Write a reply`, SHORTCUTS.reply)
      act("note", t`Write a note for the team`, SHORTCUTS.note)
      act("hand", t`Assign or hand to a teammate`, SHORTCUTS.hand)
      act("assignTeam", t`Assign to a team`, SHORTCUTS.assignTeam)
      act("snooze", t`Snooze`, SHORTCUTS.snooze)
      act("labels", t`Labels`, SHORTCUTS.labels)
      act("fields", t`Edit the custom fields`, SHORTCUTS.fields)
      if (pathname.startsWith("/feedback/")) act("progress", t`Set the feedback progress`, SHORTCUTS.progress)
      act("sla", t`Show the SLA targets`, SHORTCUTS.sla)
      act("close", t`Close or reopen`, SHORTCUTS.close)
      act("history", t`Other conversations`, SHORTCUTS.history)
      act("contact", t`Contact details`, SHORTCUTS.contact)
      act("more", t`Priority, inbox, spam and more`, SHORTCUTS.more)
      act("discussion", t`Turn into a discussion`, SHORTCUTS.discussion)
      act("incident", t`Link to a status incident`, SHORTCUTS.incident)
      act("forward", t`Forward by e-mail…`, SHORTCUTS.forward)
      act("article", t`Send a help article`, SHORTCUTS.article)
      act("recipients", t`Cc and Bcc the reply`, SHORTCUTS.recipients)
      act("signature", t`Leave your signature off this reply, or put it back`, SHORTCUTS.signature)
      act("idea", t`Link to a roadmap idea`, SHORTCUTS.idea)
      if (pathname.startsWith("/feedback/")) {
        act("createIssue", t`Create a GitHub or Linear issue`, SHORTCUTS.createIssue)
        act("linkIssue", t`Link a GitHub or Linear issue`, SHORTCUTS.linkIssue)
      }
      if (canManage && current) {
        if (current.trashed_at) act("restore", t`Restore from the Trash`, SHORTCUTS.restore)
        else act("trash", t`Move to the Trash`, SHORTCUTS.trash)
      }
      if (!current?.trashed_at) {
        for (const r of replies) {
          list.push({
            group: t`Replies`,
            label: r.title,
            hint: [r.kind === "macro" ? t`Macro` : "", `/${r.shortcut}`].filter(Boolean).join(" · "),
            run: () => runInsert(r),
          })
        }
      }
    }
    if (inboxes.filter(isSupport).length > 1) {
      list.push({ group: t`Inbox`, label: t`All inboxes`, run: () => setFilter("") })
      for (const i of inboxes.filter(isSupport))
        list.push({ group: t`Inbox`, label: t`Only ${i.name}`, run: () => setFilter(i.id) })
    }
    const go = (label: string, to: string, keys?: string[]) =>
      list.push({ group: t`Go to`, label, keys, run: () => navigate(to) })
    list.push({
      group: t`Go to`,
      label: t`Search conversations`,
      keys: [SHORTCUTS.search],
      run: () => {
        if (!isChats(pathname, canManage)) navigate(inboxRoot(canManage))
        focusListSearch()
      },
    })
    if (canManage) go(t`Home`, "/", keyLabel(SHORTCUTS.home))
    if (inboxList?.some((i) => i.purpose === "mail")) go(t`Mail`, "/mail", keyLabel(SHORTCUTS.mail))
    go(t`Feedback`, "/feedback", keyLabel(SHORTCUTS.feedback))
    go(t`Contacts`, "/contacts", keyLabel(SHORTCUTS.contacts))
    go(t`Docs`, "/docs", keyLabel(SHORTCUTS.docs))
    go(t`Discussions`, "/docs/discussions")
    go(t`Discussion posts waiting for approval`, "/docs/discussions/queue")
    go(t`Broadcasts`, "/broadcasts", keyLabel(SHORTCUTS.broadcasts))
    go(t`Status`, "/status", keyLabel(SHORTCUTS.status))
    go(t`Help`, "/knowledge", keyLabel(SHORTCUTS.helpCenter))
    go(t`Roadmap`, "/board", keyLabel(SHORTCUTS.roadmap))
    go(t`What’s new`, "/updates", keyLabel(SHORTCUTS.updates))
    if (analytics) go(t`Analytics`, "/analytics", keyLabel(SHORTCUTS.analytics))
    go(t`Mailing lists`, "/broadcasts/lists")
    go(t`E-mail designs`, "/broadcasts/designs")
    go(t`Settings`, "/settings", [mod, ","])
    go(t`Profile`, "/settings/profile")
    go(t`Signature`, "/settings/signature")
    go(t`Notifications`, "/settings/notifications")
    go(t`Appearance`, "/settings/appearance")
    go(t`Members`, "/settings/members")
    go(t`Canned replies`, "/settings/canned-replies")
    go(t`Labels`, "/settings/labels")
    go(t`Macros`, "/settings/macros")
    go(t`Connected apps`, "/settings/connected-apps")
    for (const i of inboxes) go(t`${i.name} settings`, `/settings/inboxes/${i.id}`)
    if (canManage) {
      go(t`Set up Yuva`, "/setup")
      go(t`Add an inbox`, "/setup/new")
      if (analytics) go(t`Add an analytics property`, "/setup/analytics")
      go(t`API keys`, "/settings/api-keys")
      go(t`Webhooks`, "/settings/webhooks")
      go(t`Issue trackers`, "/settings/issue-trackers")
      go(t`Workspace`, "/settings/workspace")
      go(t`Teams`, "/settings/teams")
    }
    if (isOwner) go(t`Audit log`, "/settings/audit-log")
    if (isOwner) go(t`Single sign-on`, "/settings/sso")
    list.push({ group: t`Go to`, label: t`Keyboard shortcuts`, keys: [SHORTCUTS.help], run: openShortcuts })
    list.push({ group: t`Appearance`, label: t`Light appearance`, run: () => setTheme("light") })
    list.push({ group: t`Appearance`, label: t`Dark appearance`, run: () => setTheme("dark") })
    list.push({ group: t`Appearance`, label: t`Follow the system appearance`, run: () => setTheme("system") })
    return list
  }, [
    open,
    commands,
    recent.data,
    inboxList,
    channels,
    text,
    t,
    navigate,
    openShortcuts,
    setFilter,
    canManage,
    isOwner,
    analytics,
    pathname,
    current,
    replies,
  ])
}

type PaletteProps = { open: boolean; onOpenChange: (open: boolean) => void }

export function Palette({ open, onOpenChange }: PaletteProps) {
  const { t } = useLingui()
  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t`Everything`}
      description={t`Search people, actions or settings`}
      className="top-[14vh] w-[560px] max-w-[calc(100%-32px)] gap-0 rounded-[18px]! border bg-card p-0 shadow-[0_30px_80px_-20px_rgb(0_0_0/0.35)] sm:max-w-[560px]"
    >
      <PaletteBody open={open} onOpenChange={onOpenChange} />
    </CommandDialog>
  )
}

function PaletteBody({ open, onOpenChange }: PaletteProps) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { canManage } = useSession()
  const [q, setQ] = useState("")
  const all = useItems(open)
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
    <Command className="rounded-[18px]! bg-card p-0" data-testid="palette" loop>
      <CommandPrimitive.Input
        value={q}
        onValueChange={setQ}
        placeholder={t`Search people, actions or settings…`}
        aria-label={t`Search people, actions or settings`}
        className="w-full border-b bg-transparent px-[18px] py-4 text-base outline-none md:text-body placeholder:text-faint"
      />
      <CommandList className="max-h-[50vh] p-1.5">
        <CommandEmpty className="py-10 text-faint">
          <Trans>No results</Trans>
        </CommandEmpty>
        {groups.map((g) => (
          <CommandGroup
            key={`${g.name}:${g.items[0]?.label}`}
            heading={g.name}
            className="p-0 **:[[cmdk-group-heading]]:px-3 **:[[cmdk-group-heading]]:pt-2.5 **:[[cmdk-group-heading]]:pb-1 **:[[cmdk-group-heading]]:text-caption **:[[cmdk-group-heading]]:font-medium **:[[cmdk-group-heading]]:text-faint"
          >
            {g.items.map((x, i) => (
              <CommandItem
                key={`${x.label}:${i}`}
                value={`${x.label} ${x.hint ?? ""} ${g.name} ${i}`}
                onSelect={() => run(x)}
                className="gap-2.5 rounded-[10px]! px-3 py-[9px]"
              >
                <span className="truncate">{x.label}</span>
                {x.hint && <small className="min-w-0 truncate text-caption text-faint">{x.hint}</small>}
                {x.keys && (
                  <CommandShortcut className="flex gap-1">
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
          <CommandGroup
            heading={t`Search`}
            forceMount
            className="p-0 **:[[cmdk-group-heading]]:px-3 **:[[cmdk-group-heading]]:pt-2.5 **:[[cmdk-group-heading]]:pb-1 **:[[cmdk-group-heading]]:text-caption **:[[cmdk-group-heading]]:font-medium **:[[cmdk-group-heading]]:text-faint"
          >
            <CommandItem
              forceMount
              value={`search ${search}`}
              onSelect={() =>
                run({
                  group: "",
                  label: "",
                  run: () => {
                    setListQuery(search)
                    if (!isChats(pathname, canManage)) navigate(inboxRoot(canManage))
                  },
                })
              }
              className="gap-2.5 rounded-[10px]! px-3 py-[9px]"
            >
              <Trans>Search conversations for “{search}”</Trans>
            </CommandItem>
          </CommandGroup>
        )}
      </CommandList>
    </Command>
  )
}
