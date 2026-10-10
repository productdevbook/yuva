import { Trans, useLingui } from "@lingui/react/macro"
import { EllipsisIcon, FlagIcon, InboxIcon, MailIcon, PinIcon, PinOffIcon, RotateCcwIcon, ShieldAlertIcon, ShieldCheckIcon, TagIcon, UserIcon, UserRoundCheckIcon } from "lucide-react"

import { Dot, Kbd } from "@/components/common"
import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { PRIORITIES, useEnumText } from "@/components/common/text"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { priorityClass } from "@/features/conversation/controls/shared"
import type { Conversation, ConversationUpdate, Priority } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes, useLabels } from "@/lib/workspace"
import { Button } from "@/components/ui/button"

export function ConversationMenu({
  c,
  open,
  onOpenChange,
  update,
  move,
  onContact,
  pinned,
  onPin,
  onUnread,
}: {
  c: Conversation
  open: boolean
  onOpenChange: (open: boolean) => void
  update: (body: ConversationUpdate) => void
  move: (inboxId: string) => void
  onContact: () => void
  pinned: boolean
  onPin: () => void
  onUnread: () => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const { membership } = useSession()
  const labels = useLabels().data ?? []
  const others = (useInboxes().data ?? []).filter((i) => i.id !== c.inbox_id)
  const current = new Set(c.labels)
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon-sm" className="self-start text-faint" />}
        aria-label={t`More actions`}
        title={t`More actions`}
        data-testid="conversation-menu"
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-60">
        <DropdownMenuItem onClick={onContact}>
          <UserIcon />
          <span className="flex-1">
            <Trans>Contact details</Trans>
          </span>
          <Kbd>{keyLabel(SHORTCUTS.contact)[0]}</Kbd>
        </DropdownMenuItem>
        {c.assignee_id !== membership.member_id && (
          <DropdownMenuItem onClick={() => update({ assignee_id: membership.member_id })}>
            <UserRoundCheckIcon />
            <Trans>Assign to me</Trans>
          </DropdownMenuItem>
        )}
        {c.status !== "open" && (
          <DropdownMenuItem onClick={() => update({ status: "open" })}>
            <RotateCcwIcon />
            <Trans>Back to waiting</Trans>
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={onPin} data-testid="header-pin">
          {pinned ? <PinOffIcon /> : <PinIcon />}
          <span className="flex-1">{pinned ? <Trans>Unpin</Trans> : <Trans>Pin to the top</Trans>}</span>
          <Kbd>{SHORTCUTS.pin}</Kbd>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onUnread} data-testid="header-unread">
          <MailIcon />
          <span className="flex-1">
            <Trans>Mark as unread</Trans>
          </span>
          <Kbd>{SHORTCUTS.unread}</Kbd>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <TagIcon />
            <Trans>Labels</Trans>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="max-h-[60svh] min-w-52">
            {labels.length === 0 && (
              <p className="px-2.5 py-1.5 text-body text-muted-foreground">
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
                  update({ labels: [...next] })
                }}
              >
                <Dot color={l.color} />
                {l.name}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <FlagIcon className={priorityClass[c.priority]} />
            <Trans>Priority</Trans>
            <span className="ms-auto text-caption text-faint">{text.priority[c.priority]}</span>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="min-w-40">
            <DropdownMenuRadioGroup value={c.priority} onValueChange={(v) => update({ priority: v as Priority })}>
              {PRIORITIES.map((p) => (
                <DropdownMenuRadioItem key={p} value={p}>
                  <FlagIcon className={priorityClass[p]} />
                  {text.priority[p]}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger disabled={others.length === 0}>
            <InboxIcon />
            <Trans>Move to inbox</Trans>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="max-h-[60svh] min-w-52">
            {others.map((i) => (
              <DropdownMenuItem key={i.id} onClick={() => move(i.id)}>
                <Dot color={i.branding.color} />
                <span className="flex-1 truncate">{i.name}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => update({ spam: !c.spam })} data-testid="spam-toggle">
          {c.spam ? <ShieldCheckIcon /> : <ShieldAlertIcon />}
          <span className="flex-1">{c.spam ? <Trans>Not spam</Trans> : <Trans>Mark as spam</Trans>}</span>
          <Kbd>{SHORTCUTS.spam}</Kbd>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
