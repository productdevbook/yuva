import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ClockIcon, RotateCcwIcon, TagIcon, UserRoundCheckIcon, UserRoundPlusIcon } from "lucide-react"
import { useState } from "react"

import { Dot, Kbd, MemberAvatar } from "@/components/common"
import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
  popupClass,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Item } from "@/components/ui/item"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Separator } from "@/components/ui/separator"
import { firstName, type ConversationActions } from "@/features/conversation/actions"
import { useCounts } from "@/features/inbox/queries"
import type { Conversation, ConversationUpdate } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useAssignableMembers, useLabels } from "@/lib/workspace"

export type HeaderMenu = "snooze" | "hand" | "labels" | null

const iconButton = <Button variant="ghost" size="icon-sm" />

function useSnoozeTimes() {
  const { t, i18n } = useLingui()
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const now = new Date()
  const hour = new Date(now.getTime() + 60 * 60 * 1000)
  const evening = new Date(now)
  evening.setHours(17, 0, 0, 0)
  const morning = new Date(now)
  morning.setDate(now.getDate() + 1)
  morning.setHours(9, 0, 0, 0)
  const list = [{ at: hour, label: t`In 1 hour`, when: t`in an hour`, detail: time.format(hour) }]
  if (evening.getTime() - now.getTime() > 30 * 60 * 1000) {
    const at = time.format(evening)
    list.push({ at: evening, label: t`Today at ${at}`, when: t`today at ${at}`, detail: "" })
  }
  const at = time.format(morning)
  list.push({ at: morning, label: t`Tomorrow at ${at}`, when: t`tomorrow at ${at}`, detail: "" })
  return list
}

export function SnoozeMenu({ name, actions, open, onOpenChange }: { name: string; actions: ConversationActions; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const times = useSnoozeTimes()
  const first = firstName(name)
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger render={iconButton} aria-label={t`Snooze (S)`} title={t`Snooze (S)`} data-testid="snooze-menu">
        <ClockIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[280px] max-w-[calc(100vw-32px)]">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>When should it come back?</Trans>
          </DropdownMenuLabel>
          {times.map((o) => (
            <DropdownMenuItem key={o.label} onClick={() => actions.snooze(o.at, o.when)}>
              <span className="flex-1">{o.label}</span>
              {o.detail && <span className="text-caption text-faint">{o.detail}</span>}
            </DropdownMenuItem>
          ))}
          <DropdownMenuItem onClick={actions.untilReply}>
            <Trans>When {first} writes again</Trans>
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function HandMenu({ c, actions, open, onOpenChange }: { c: Conversation; actions: ConversationActions; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const { membership } = useSession()
  const here = (m: { online: boolean; availability: string }) => (m.online && m.availability === "auto" ? 0 : m.availability === "away" ? 2 : 1)
  const people = useAssignableMembers(c.inbox_id)
    .filter((m) => m.id !== membership.member_id)
    .sort((a, b) => here(a) - here(b))
  const [note, setNote] = useState("")
  const load = new Map((useCounts().data?.assignees ?? []).map((a) => [a.id, a.count]))
  const mine = c.assignee_id === membership.member_id
  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o)
        if (!o) setNote("")
      }}
    >
      <PopoverTrigger render={iconButton} aria-label={t`Assign (A)`} title={t`Assign (A)`} data-testid="hand-menu">
        <UserRoundPlusIcon />
      </PopoverTrigger>
      <PopoverContent align="end" sideOffset={6} className={cn(popupClass, "w-[300px] max-w-[calc(100vw-32px)] gap-0 rounded-xl p-1.5 ring-0")} data-testid="hand-popup">
        {!mine && (
          <>
            <Item
              render={<button type="button" />}
              size="xs"
              className="flex-nowrap"
              onClick={() => {
                onOpenChange(false)
                actions.claim()
              }}
              data-testid="assign-me"
            >
              <UserRoundCheckIcon className="size-4 text-muted-foreground" />
              <span className="flex-1 text-start">
                <Trans>Assign to me</Trans>
              </span>
            </Item>
            <Separator className="mx-1.5 my-1 w-auto" />
          </>
        )}
        <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
          <Trans>Who should take it?</Trans>
        </p>
        {people.length === 0 && (
          <p className="px-2.5 py-2 text-body text-muted-foreground">
            <Trans>Nobody else can see this inbox yet.</Trans>
          </p>
        )}
        {people.map((m) => {
          const who = m.name || m.email
          return (
            <Item
              key={m.id}
              render={<button type="button" />}
              size="xs"
              onClick={() => {
                onOpenChange(false)
                actions.hand(m, note)
                setNote("")
              }}
              className={cn("flex-nowrap", c.assignee_id === m.id && "bg-muted")}
            >
              <MemberAvatar name={who} online={m.online} away={m.availability === "away"} ring="ring-card" />
              <span className="min-w-0 flex-1 text-start">
                <span className="block truncate">{who}</span>
                <small className="block truncate text-caption text-faint">
                  {m.availability === "away" ? <Trans>Away</Trans> : m.online ? <Trans>Online</Trans> : <Trans>Offline</Trans>}
                  {" · "}
                  <Plural value={load.get(m.id) ?? 0} one="# conversation" other="# conversations" />
                </small>
              </span>
            </Item>
          )
        })}
        {people.length > 0 && (
          <Input
            name="hand-note"
            autoFocus
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder={t`Add a note (optional)`}
            aria-label={t`Note for the teammate`}
            className="mt-1.5 rounded-[10px]"
            data-testid="hand-note"
          />
        )}
      </PopoverContent>
    </Popover>
  )
}

export function LabelsMenu({ c, update, open, onOpenChange }: { c: Conversation; update: (body: ConversationUpdate) => void; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const labels = useLabels().data ?? []
  const current = new Set(c.labels)
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger render={iconButton} aria-label={t`Labels (L)`} title={t`Labels (L)`} data-testid="labels-menu">
        <TagIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[60svh] min-w-52">
        {labels.length === 0 && (
          <p className="px-2.5 py-1.5 text-body text-muted-foreground">
            <Trans>No labels yet. Add them in settings.</Trans>
          </p>
        )}
        {labels.map((l) => (
          <DropdownMenuCheckboxItem
            key={l.id}
            checked={current.has(l.id)}
            closeOnClick={false}
            onCheckedChange={(on) => {
              const next = new Set(current)
              if (on) next.add(l.id)
              else next.delete(l.id)
              update({ labels: [...next] })
            }}
          >
            <Dot color={l.color} />
            {l.name}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function CloseButton({ c, actions }: { c: Conversation; actions: ConversationActions }) {
  const { t } = useLingui()
  if (c.status === "closed") {
    return (
      <Button variant="outline" size="sm" onClick={actions.reopen} data-testid="reopen-conversation">
        <RotateCcwIcon />
        <span className="phone:hidden @max-xl/person:hidden">
          <Trans>Reopen</Trans>
        </span>
      </Button>
    )
  }
  return (
    <Button variant="outline" size="sm" onClick={actions.close} title={t`Close (E)`} data-testid="close-conversation">
      <CheckIcon />
      <span className="phone:hidden @max-xl/person:hidden">
        <Trans context="conversation">Close</Trans>
      </span>
      <Kbd className="phone:hidden @max-xl/person:hidden">{keyLabel(SHORTCUTS.close)[0]}</Kbd>
    </Button>
  )
}
