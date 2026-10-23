import { Trans, useLingui } from "@lingui/react/macro"
import {
  CheckIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  EllipsisIcon,
  FlagIcon,
  FlameIcon,
  InboxIcon,
  LightbulbIcon,
  MailIcon,
  MessagesSquareIcon,
  PinIcon,
  PinOffIcon,
  RotateCcwIcon,
  SearchIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  TagIcon,
  Trash2Icon,
} from "lucide-react"

import { Dot, Kbd } from "@/components/common"
import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { PRIORITIES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { priorityClass } from "@/features/conversation/controls/shared"
import { look } from "@/features/inbox/PageFrame"
import type { Conversation, ConversationUpdate, Priority } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes, useLabels } from "@/lib/workspace"

export type MenuView = "root" | "priority" | "labels" | "move"

export function ConversationMenu({
  c,
  open,
  onOpenChange,
  view,
  onView,
  update,
  move,
  pinned,
  onPin,
  onUnread,
  onDiscussion,
  onIncident,
  onIdea,
  onFind,
  onTrash,
}: {
  c: Conversation
  open: boolean
  onOpenChange: (open: boolean) => void
  view: MenuView
  onView: (view: MenuView) => void
  update: (body: ConversationUpdate) => void
  move: (inboxId: string) => void
  pinned: boolean
  onPin: () => void
  onUnread: () => void
  onDiscussion: () => void
  onIncident: () => void
  onIdea: () => void
  onFind: () => void
  onTrash?: () => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const labels = useLabels().data ?? []
  const others = (useInboxes().data ?? []).filter((i) => i.id !== c.inbox_id)
  const current = new Set(c.labels)
  const sub = (v: MenuView) => (
    <DropdownMenuItem closeOnClick={false} onClick={() => onView(v)} data-testid={`menu-${v}`}>
      <ChevronLeftIcon className="rtl:rotate-180" />
      {v === "priority" ? (
        <Trans>Priority</Trans>
      ) : v === "labels" ? (
        <Trans>Labels</Trans>
      ) : (
        <Trans>Move to inbox</Trans>
      )}
    </DropdownMenuItem>
  )
  const enter = (
    v: MenuView,
    icon: React.ReactNode,
    label: React.ReactNode,
    hint?: React.ReactNode,
    disabled?: boolean
  ) => (
    <DropdownMenuItem closeOnClick={false} disabled={disabled} onClick={() => onView(v)} data-testid={`menu-open-${v}`}>
      {icon}
      <span className="flex-1">{label}</span>
      {hint && <span className="text-caption text-faint">{hint}</span>}
      <ChevronRightIcon className="rtl:rotate-180" />
    </DropdownMenuItem>
  )
  return (
    <DropdownMenu
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o)
        if (!o) onView("root")
      }}
    >
      <DropdownMenuTrigger
        render={<Button variant="outline" className={cn(look.btnSm, "w-[30px] px-0")} />}
        aria-label={t`More actions`}
        title={t`More actions`}
        data-testid="conversation-menu"
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[70svh] min-w-60">
        {view === "root" && (
          <>
            {enter(
              "priority",
              <FlagIcon className={priorityClass[c.priority]} />,
              <Trans>Priority</Trans>,
              text.priority[c.priority]
            )}
            {enter("labels", <TagIcon />, <Trans>Labels</Trans>, c.labels.length > 0 ? c.labels.length : undefined)}
            {enter("move", <InboxIcon />, <Trans>Move to inbox</Trans>, undefined, others.length === 0)}
            <DropdownMenuSeparator />
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
            {c.status !== "open" && (
              <DropdownMenuItem onClick={() => update({ status: "open" })} data-testid="back-to-waiting">
                <RotateCcwIcon />
                <Trans>Back to waiting</Trans>
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            {c.kind === "feedback" && (
              <DropdownMenuItem onClick={onIdea} data-testid="header-idea">
                <LightbulbIcon />
                <span className="flex-1">
                  <Trans>Link to an idea</Trans>
                </span>
                <Kbd>{SHORTCUTS.idea}</Kbd>
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onClick={onIncident} data-testid="header-incident">
              <FlameIcon />
              <span className="flex-1">
                <Trans>Link to an incident</Trans>
              </span>
              <Kbd>{SHORTCUTS.incident}</Kbd>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onDiscussion} data-testid="header-discussion">
              <MessagesSquareIcon />
              <span className="flex-1">
                <Trans>Turn into a discussion</Trans>
              </span>
              <span className="flex gap-0.5">
                {keyLabel(SHORTCUTS.discussion).map((k) => (
                  <Kbd key={k}>{k}</Kbd>
                ))}
              </span>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onFind} data-testid="find-toggle">
              <SearchIcon />
              <span className="flex-1">
                <Trans>Search in this conversation</Trans>
              </span>
              <span className="flex gap-0.5">
                {keyLabel(SHORTCUTS.find).map((k) => (
                  <Kbd key={k}>{k}</Kbd>
                ))}
              </span>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant={c.spam ? "default" : "destructive"}
              onClick={() => update({ spam: !c.spam })}
              data-testid="spam-toggle"
            >
              {c.spam ? <ShieldCheckIcon /> : <ShieldAlertIcon />}
              <span className="flex-1">{c.spam ? <Trans>Not spam</Trans> : <Trans>Mark as spam</Trans>}</span>
              <Kbd>{SHORTCUTS.spam}</Kbd>
            </DropdownMenuItem>
            {onTrash && (
              <DropdownMenuItem variant="destructive" onClick={onTrash} data-testid="trash-conversation">
                <Trash2Icon />
                <span className="flex-1">
                  <Trans>Move to the Trash</Trans>
                </span>
                <Kbd>{SHORTCUTS.trash}</Kbd>
              </DropdownMenuItem>
            )}
          </>
        )}
        {view === "priority" && (
          <>
            {sub("priority")}
            <DropdownMenuSeparator />
            {PRIORITIES.map((p) => (
              <DropdownMenuItem
                key={p}
                onClick={() => update({ priority: p as Priority })}
                data-testid={`priority-${p}`}
              >
                <FlagIcon className={priorityClass[p]} />
                <span className="flex-1">{text.priority[p]}</span>
                {c.priority === p && <CheckIcon className="text-brand" />}
              </DropdownMenuItem>
            ))}
          </>
        )}
        {view === "labels" && (
          <>
            {sub("labels")}
            <DropdownMenuSeparator />
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
          </>
        )}
        {view === "move" && (
          <>
            {sub("move")}
            <DropdownMenuSeparator />
            {others.map((i) => (
              <DropdownMenuItem key={i.id} onClick={() => move(i.id)}>
                <Dot color={i.branding.color} />
                <span className="flex-1 truncate">{i.name}</span>
              </DropdownMenuItem>
            ))}
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
