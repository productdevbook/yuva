import { Trans, useLingui } from "@lingui/react/macro"
import {
  BookUserIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  MegaphoneIcon,
  PinIcon,
  CommandIcon,
  EllipsisVerticalIcon,
  InboxIcon,
  KeyboardIcon,
  MessageSquarePlusIcon,
  ReplyIcon,
  SearchIcon,
  SettingsIcon,
  SquareCheckBigIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { useShell } from "@/app/shell"
import { TeamMenu } from "@/app/TeamMenu"
import { UserMenu } from "@/app/UserMenu"
import {
  ContactAvatar,
  Dot,
  ErrorLine,
  ImportedChip,
  isVisitor,
  Kbd,
  MemberAvatar,
  placeOf,
  SlaBadge,
  sourceOf,
  toast,
  useContactName,
  useSourceText,
  VisitorAvatar,
} from "@/components/common"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDuration, formatShort, useErrorText, usePreviewText } from "@/components/common/text"
import { useLatest } from "@/hooks/use-latest"
import { useNow } from "@/hooks/use-now"
import { useConfirm } from "@/components/common/ConfirmDialog"
import { Alert } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { NewConversationDialog } from "@/features/contact/ContactDetails"
import { ContactSearch } from "@/features/contact/ContactSearch"
import { useDraftText } from "@/features/conversation/actions"
import { TrashLine } from "@/features/conversation/Trash"
import { useLiveIds } from "@/features/conversation/liveness"
import { Ticks } from "@/features/conversation/messages/MessageActions"
import { currentRating, RatingMark } from "@/features/conversation/rating"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useListCursor } from "@/features/inbox/listCursor"
import { LinkIncidentDialog } from "@/features/status/LinkIncidentDialog"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { ImportBanner } from "@/features/migrations/ImportBanner"
import { InboxChip } from "@/features/inbox/InboxPicker"
import { UnderlineTabs } from "@/features/inbox/ListHead"
import { useActiveView } from "@/features/inbox/views/useActiveView"
import { ViewsMenu } from "@/features/inbox/views/ViewsMenu"
import { ProductTag } from "@/features/inbox/ProductTag"
import { LabelTag, look } from "@/features/inbox/PageFrame"
import { useListActions } from "@/features/inbox/memberState"
import { setListQuery, useListSearch } from "@/features/inbox/listSearch"
import { BulkFailuresDialog } from "@/features/fields/BulkFailures"
import { SourceFilter, sourceFilters, useSourceFilter } from "@/features/inbox/SourceFilter"
import {
  useBulkTeam,
  useBulkUpdateConversations,
  useConversations,
  useCounts,
  useLoadedCount,
} from "@/features/inbox/queries"
import type { ConversationBulkFailure, Contact, ConversationListItem, ConversationStatus } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"
import { useSetAvailability } from "@/lib/availability"
import { useSession } from "@/lib/session"
import { useTyping } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useChannelMap, useLabels, useMemberMap, useMyTeams, useTeams } from "@/lib/workspace"

export type Chip = "waiting" | "all" | "mine" | "teams" | "unassigned" | "snoozed" | "done" | "trash"
export const PAGE_CHIPS: Chip[] = ["waiting", "all", "mine", "teams", "unassigned", "snoozed", "done"]

export function useChips(): Chip[] {
  const { canManage } = useSession()
  const inTeam = useMyTeams().length > 0
  const chips = inTeam ? PAGE_CHIPS : PAGE_CHIPS.filter((c) => c !== "teams")
  return canManage ? [...chips, "trash"] : chips
}

const chipFilters: Record<Chip, ConversationFilters> = {
  waiting: { status: ["open"] },
  all: {},
  mine: { status: ["open"], assignee: "me" },
  teams: { status: ["open"], team: "mine" },
  unassigned: { status: ["open"], assignee: "unassigned" },
  snoozed: { status: ["snoozed"] },
  done: { status: ["closed"] },
  trash: { trashed: true, kind: undefined },
}

export function isWaiting(c: ConversationListItem) {
  return c.status === "open" && c.last_message?.author_type === "contact"
}

let remembered: Chip = "waiting"
let focused = 0

export function useRememberedChip(): Chip {
  const offered = useChips()
  return offered.includes(remembered) ? remembered : "waiting"
}

export function rememberChip(c: Chip) {
  remembered = c
}

export function useChipLabels(): Record<Chip, string> {
  const { t } = useLingui()
  return {
    waiting: t`Waiting`,
    all: t`All`,
    mine: t`Mine`,
    teams: t`My teams`,
    unassigned: t`Unassigned`,
    snoozed: t`Snoozed`,
    done: t`Closed`,
    trash: t`Trash`,
  }
}

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

function Preview({
  c,
  current,
  receipts,
  subject,
}: {
  c: ConversationListItem
  current: boolean
  receipts: boolean
  subject?: string
}) {
  const { t } = useLingui()
  const previewText = usePreviewText()
  const draft = useDraftText(c.id)
  const typing = useTyping(c.id).some((a) => a.type === "contact")
  if (typing) {
    return (
      <span className="truncate text-brand" data-testid="row-typing">
        <Trans>typing…</Trans>
      </span>
    )
  }
  if (draft && !current) {
    return (
      <span className="truncate" data-testid="row-draft">
        <span className="font-medium text-brand">
          <Trans>Draft:</Trans>
        </span>{" "}
        {draft}
      </span>
    )
  }
  if (subject) return <span className="truncate">{subject}</span>
  const m = c.last_message
  if (!m) return <span className="truncate italic">{c.subject || t`No messages yet`}</span>
  return (
    <span className="flex min-w-0 items-center gap-1">
      {m.author_type === "bot" && <BotIcon className="size-3.5 shrink-0" aria-label={t`Bot`} data-testid="row-bot" />}
      {m.author_type === "member" && !receipts && (
        <ReplyIcon className="size-3.5 shrink-0" aria-label={t`Team reply`} />
      )}
      {(m.author_type === "member" || m.author_type === "bot") && receipts && (
        <Ticks at={m.created_at} readAt={c.last_read_by_contact_at} />
      )}
      <span className={cn("truncate", m.redacted && "italic")}>{previewText(m) || c.subject}</span>
    </span>
  )
}

function Row({
  c,
  current,
  cursor,
  live,
  selecting,
  checked,
  onOpen,
}: {
  c: ConversationListItem
  current: boolean
  cursor: boolean
  live: boolean
  selecting: boolean
  checked: boolean
  onOpen: () => void
}) {
  const { t, i18n } = useLingui()
  const channel = useChannelMap().get(c.channel_id ?? "")
  const members = useMemberMap()
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const contactName = useContactName()
  const visitor = isVisitor(c.contact.name, c.contact.email)
  const name = contactName(c, channel, c.contact.name, c.contact.email)
  const source = sourceOf(c, channel)
  const sourceText = useSourceText()
  const place = placeOf(c, channel)
  const rating = currentRating(c)
  const labels = useLabels().data ?? []
  const chips = labels.filter((l) => c.labels.includes(l.id))
  const waiting = isWaiting(c)
  const now = useNow(60_000)
  const subject = c.subject.trim()
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "group flex w-full items-start gap-2.5 border-b px-3.5 py-3 text-start outline-none transition-colors",
        current ? "bg-surface" : "hover:bg-surface",
        cursor && !current && "bg-surface",
        (current || cursor) && "shadow-[inset_2px_0_0_var(--brand)] rtl:shadow-[inset_-2px_0_0_var(--brand)]"
      )}
      aria-current={current || undefined}
      aria-pressed={selecting ? checked : undefined}
      data-checked={checked || undefined}
      data-testid="list-row"
      data-conversation-id={c.id}
      data-unread={c.unread || undefined}
      data-pinned={c.pinned_at ? true : undefined}
    >
      {selecting && (
        <span
          aria-hidden
          className={cn(
            "grid size-5 shrink-0 place-items-center rounded-[6px] border",
            checked ? "border-brand bg-brand text-white" : "border-input bg-background"
          )}
          data-testid="row-check"
        >
          {checked && <CheckIcon className="size-3.5" strokeWidth={3} />}
        </span>
      )}
      <span className="relative shrink-0">
        {visitor ? (
          <VisitorAvatar className="size-[34px]" />
        ) : (
          <ContactAvatar id={c.contact.id} name={name} className="size-[34px] font-semibold" />
        )}
        {live && (
          <span
            className="absolute -end-0.5 bottom-0 size-3 rounded-full border-2 border-background bg-success"
            title={t`Online now`}
            data-testid="row-live"
          />
        )}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className={cn("min-w-0 flex-1 truncate text-body", c.unread ? "font-semibold" : "font-medium")}>
            {name}
          </span>
          {c.pinned_at && (
            <PinIcon
              className="size-3.5 shrink-0 rotate-45 self-center text-faint"
              aria-label={t`Pinned`}
              data-testid="row-pinned"
            />
          )}
          {c.unread && (
            <span
              className="size-2 shrink-0 self-center rounded-full bg-brand"
              aria-label={t`Unread`}
              data-testid="row-unread"
            />
          )}
          <time
            className={cn("shrink-0 text-caption", waiting ? "font-medium text-brand" : "text-faint")}
            dateTime={c.last_activity_at}
          >
            {waiting
              ? formatDuration(Math.max(60, (now - Date.parse(c.last_activity_at)) / 1000), i18n.locale)
              : formatShort(c.last_activity_at, i18n.locale)}
          </time>
        </span>
        <span className="flex min-w-0 items-center gap-1.5 text-body text-muted-foreground">
          {c.trashed_at && <Trash2Icon className="size-3.5 shrink-0 text-faint" aria-label={t`Trash`} />}
          {c.status === "snoozed" && <ClockIcon className="size-3.5 shrink-0 text-faint" aria-label={t`Snoozed`} />}
          <span className="flex min-w-0 flex-1">
            <Preview
              c={c}
              current={current}
              subject={subject}
              receipts={channel?.kind === "chat" || channel?.kind === "app"}
            />
          </span>
        </span>
        {c.trashed_at && <TrashLine c={c} />}
        <span className="flex min-w-0 items-center gap-1.5 text-caption" data-testid="row-source">
          <ProductTag id={c.inbox_id} />
          {c.imported && <ImportedChip at={c.imported.at} />}
          {source && <span className="shrink-0 text-faint">{sourceText[source]}</span>}
          {c.broadcast_id && (
            <MegaphoneIcon
              className="size-3.5 shrink-0 text-brand"
              aria-label={t`Reply to a broadcast`}
              data-testid="row-broadcast"
            />
          )}
          {place && <span className="min-w-0 truncate text-faint">{place}</span>}
          {chips.map((l) => (
            <LabelTag key={l.id} name={l.name} />
          ))}
          <span className="flex-1" />
          {c.sla && <SlaBadge sla={c.sla} />}
          {rating && <RatingMark rating={rating.rating} comment={rating.comment} />}
          {assignee && (
            <span title={t`Assigned to ${assignee.name || assignee.email}`} data-testid="row-assignee">
              <MemberAvatar name={assignee.name || assignee.email} className="size-[18px] text-[8px]" />
            </span>
          )}
        </span>
      </span>
    </button>
  )
}

function AwayBanner() {
  const { me } = useSession()
  const set = useSetAvailability()
  if (me.person.availability !== "away") return null
  return (
    <Alert
      role="status"
      className="mx-3 mb-2 flex items-center gap-2 rounded-xl border-border bg-card py-2 ps-3 pe-2 text-small text-muted-foreground"
      data-testid="away-banner"
    >
      <ClockIcon className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1">
        <Trans>You are away. Live chat shows nobody available and notifications pause.</Trans>
      </span>
      <Button
        variant="outline"
        size="sm"
        className="shrink-0"
        onClick={() => set.mutate("auto")}
        disabled={set.isPending}
      >
        <Trans>I'm back</Trans>
      </Button>
    </Alert>
  )
}

function CleanupBanner({ inboxId }: { inboxId: string }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const counts = useCounts(["conversation"]).data
  const spam = useConversations({ status: ["open"], spam: true, kind: ["conversation"] }, !!counts?.spam)
  const bulk = useBulkUpdateConversations()
  const items = (spam.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
  if (!counts?.spam || items.length === 0) return null
  const ids = items.slice(0, 100).map((c) => c.id)
  const closeAll = () =>
    bulk.mutate(
      { conversation_ids: ids, status: "closed" },
      {
        onSuccess: (r) => {
          const done = r.updated.map((c) => c.id)
          toast(t`${done.length} closed`, () =>
            bulk.mutate({ conversation_ids: done, status: "open" }, { onError: (e) => toast(errorText(e)) })
          )
        },
        onError: (e) => toast(errorText(e)),
      }
    )
  return (
    <Alert
      role="status"
      className="mx-3 mb-2 flex flex-wrap items-center gap-2 rounded-xl border-dashed border-border bg-card py-2 ps-3 pe-2 text-small"
      data-testid="cleanup-banner"
    >
      <span className="min-w-0 flex-1">
        <b className="font-medium">
          <Trans>Quick cleanup</Trans>
        </b>{" "}
        <span className="text-faint">
          <Trans>{items.length} marked as spam · no reply needed</Trans>
        </span>
      </span>
      <Button
        variant="outline"
        size="sm"
        className={look.btnSm}
        onClick={closeAll}
        disabled={bulk.isPending}
        data-testid="cleanup-close-all"
      >
        <Trans>Close all</Trans>
      </Button>
    </Alert>
  )
}

export function StartConversation({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [contact, setContact] = useState<Contact | null>(null)
  return (
    <>
      <Dialog
        open={open && !contact}
        onOpenChange={(o) => {
          if (!o) onOpenChange(false)
        }}
      >
        <DialogContent data-testid="start-conversation">
          <DialogHeader>
            <DialogTitle>
              <Trans>Who is it for?</Trans>
            </DialogTitle>
          </DialogHeader>
          {open && <ContactSearch exclude="" onPick={setContact} />}
        </DialogContent>
      </Dialog>
      {contact && (
        <NewConversationDialog
          contact={contact}
          open={open}
          onOpenChange={(o) => {
            if (!o) {
              setContact(null)
              onOpenChange(false)
            }
          }}
        />
      )}
    </>
  )
}

export function ListMenu({ className }: { className?: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const { openPalette, openShortcuts } = useShell()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon-sm" className={className} />}
        aria-label={t`Menu`}
        title={t`Menu`}
        data-testid="list-menu"
      >
        <EllipsisVerticalIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <DropdownMenuItem onClick={() => navigate("/contacts")} data-testid="open-contacts">
          <BookUserIcon />
          <Trans>Contacts</Trans>
          <DropdownMenuShortcut>{keyLabel(SHORTCUTS.contacts).join(" ")}</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => navigate("/settings")}>
          <SettingsIcon />
          <Trans>Settings</Trans>
          <DropdownMenuShortcut>{mod} ,</DropdownMenuShortcut>
        </DropdownMenuItem>
        {canManage && (
          <DropdownMenuItem onClick={() => navigate("/setup/new")}>
            <InboxIcon />
            <Trans>Add an inbox</Trans>
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={openPalette}>
          <CommandIcon />
          <Trans>Everything</Trans>
          <DropdownMenuShortcut>{mod} K</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={openShortcuts}>
          <KeyboardIcon />
          <Trans>Keyboard shortcuts</Trans>
          <DropdownMenuShortcut>?</DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function PhoneHeader({ children }: { children: React.ReactNode }) {
  const { t } = useLingui()
  const { membership } = useSession()
  const [starting, setStarting] = useState(false)
  return (
    <div className="flex shrink-0 flex-col border-b">
      <div className="flex h-15 shrink-0 items-center gap-1 ps-2 pe-1.5">
        <UserMenu align="start" />
        <span className="min-w-0 flex-1 truncate ps-1 text-title" data-testid="workspace-name">
          {membership.workspace.name}
        </span>
        <TeamMenu />
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => setStarting(true)}
          aria-label={t`New conversation`}
          title={t`New conversation`}
          data-testid="new-conversation"
        >
          <MessageSquarePlusIcon />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          render={<Link to="/settings" />}
          aria-label={t`Settings`}
          title={t`Settings (${mod},)`}
          data-testid="open-settings"
        >
          <SettingsIcon />
        </Button>
        <ListMenu />
      </div>
      <div className="px-3 [&>[role=toolbar]]:mt-0">{children}</div>
      <StartConversation open={starting} onOpenChange={setStarting} />
    </div>
  )
}

type Order = "newest" | "oldest" | "sla_due"

function OrderChip({
  order,
  unreadOnly,
  onOrder,
  onUnread,
}: {
  order: Order
  unreadOnly: boolean
  onOrder: (o: Order) => void
  onUnread: (on: boolean) => void
}) {
  const { t } = useLingui()
  const names: Record<Order, string> = { newest: t`Newest first`, oldest: t`Waiting longest`, sla_due: t`Due first` }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={cn(unreadOnly ? look.chipOn : look.chip, "ms-auto")} />}
        aria-label={t`Order and view`}
        data-testid="list-order"
      >
        <span className="truncate">{unreadOnly ? t`Unread · ${names[order]}` : names[order]}</span>
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-52">
        <DropdownMenuRadioGroup value={order} onValueChange={(v) => onOrder(v as Order)}>
          <DropdownMenuRadioItem value="newest" data-testid="list-order-newest">
            {names.newest}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="oldest" data-testid="list-order-oldest">
            {names.oldest}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="sla_due" data-testid="list-order-sla-due">
            {names.sla_due}
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        <DropdownMenuCheckboxItem checked={unreadOnly} onCheckedChange={onUnread} data-testid="chip-unread">
          <Trans>Unread only</Trans>
        </DropdownMenuCheckboxItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function LabelChip({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { t } = useLingui()
  const labels = useLabels().data ?? []
  const current = labels.find((l) => l.id === value)
  if (labels.length === 0) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={current ? look.chipOn : look.chip} />}
        aria-label={t`Filter by label`}
        title={t`Filter by label`}
        data-testid="label-filter"
      >
        {current ? (
          <>
            <Dot color={current.color} className="size-2" />
            <span className="max-w-24 truncate">{current.name}</span>
          </>
        ) : (
          <Trans>Label</Trans>
        )}
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
        <DropdownMenuRadioGroup value={value} onValueChange={(v) => onChange(String(v))}>
          <DropdownMenuRadioItem value="">
            <Trans>Any label</Trans>
          </DropdownMenuRadioItem>
          {labels.map((l) => (
            <DropdownMenuRadioItem key={l.id} value={l.id}>
              <Dot color={l.color} />
              <span className="truncate">{l.name}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function TeamChip({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { t } = useLingui()
  const teams = useTeams().data ?? []
  const current = teams.find((x) => x.id === value)
  if (teams.length === 0) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={current ? look.chipOn : look.chip} />}
        aria-label={t`Filter by team`}
        title={t`Filter by team`}
        data-testid="team-filter"
      >
        {current ? <span className="max-w-24 truncate">{current.name}</span> : <Trans>Team</Trans>}
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
        <DropdownMenuRadioGroup value={value} onValueChange={(v) => onChange(String(v))}>
          <DropdownMenuRadioItem value="">
            <Trans>Any team</Trans>
          </DropdownMenuRadioItem>
          {teams.map((team) => (
            <DropdownMenuRadioItem key={team.id} value={team.id}>
              <span className="truncate">{team.name}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function statusGroups(prev: ConversationListItem[]) {
  const groups = new Map<string, { status: ConversationStatus; until?: string; ids: string[] }>()
  for (const c of prev) {
    const until =
      c.status === "snoozed" && c.snooze_until && Date.parse(c.snooze_until) > Date.now() ? c.snooze_until : undefined
    const status: ConversationStatus = c.status === "snoozed" && !until ? "open" : c.status
    const key = `${status}|${until ?? ""}`
    const g = groups.get(key) ?? { status, until, ids: [] }
    g.ids.push(c.id)
    groups.set(key, g)
  }
  return groups.values()
}

export function ChatList({
  currentId,
  onOpen,
  active = true,
  chip: controlledChip,
  onChip,
  inbox: controlledInbox,
  onInbox,
  embedded = false,
}: {
  currentId: string | null
  onOpen: (id: string) => void
  active?: boolean
  chip?: Chip
  onChip?: (c: Chip) => void
  inbox?: string
  onInbox?: (id: string) => void
  embedded?: boolean
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { canManage } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const tabs = useChips()
  const initialChip = useRememberedChip()
  const { query, focus, select } = useListSearch()
  const q = useDebounced(query.trim(), 250)
  const [sidebarInbox] = useInboxFilter()
  const [ownInbox, setOwnInbox] = useState("")
  const chipInbox = controlledInbox ?? ownInbox
  const locked = !!sidebarInbox
  const inboxId = sidebarInbox || chipInbox
  const [ownChip, setOwnChip] = useState<Chip>(initialChip)
  const chip = controlledChip ?? ownChip
  const [order, setOrder] = useState<Order | null>(null)
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [labelId, setLabelId] = useState("")
  const [teamId, setTeamId] = useState("")
  const search = useRef<HTMLInputElement>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const [nearEnd, setNearEnd] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [linking, setLinking] = useState(false)
  const view = useActiveView()
  const sort: Order = order ?? (view.id ? (view.sla ? "sla_due" : "newest") : chip === "waiting" ? "oldest" : "newest")

  const [source, setSource] = useSourceFilter()
  const narrowed =
    !!source.source ||
    !!source.site ||
    !!source.broadcast ||
    !!source.field ||
    !!source.sla ||
    !!labelId ||
    !!teamId ||
    !!chipInbox
  const own = sourceFilters(source)
  const filters: ConversationFilters = view.base
    ? {
        kind: ["conversation"],
        ...view.base,
        ...Object.fromEntries(
          Object.entries({
            ...own,
            inbox_id: inboxId || undefined,
            label_id: labelId || undefined,
            team_id: teamId || undefined,
          }).filter(([, v]) => v !== undefined)
        ),
        fields: view.base.fields || own.fields ? { ...view.base.fields, ...own.fields } : undefined,
        sort: sort === "sla_due" ? "sla_due" : undefined,
        q: q || undefined,
      }
    : {
        kind: ["conversation"],
        ...chipFilters[chip],
        ...own,
        inbox_id: inboxId || undefined,
        label_id: labelId || undefined,
        team_id: teamId || undefined,
        sort: sort === "sla_due" ? "sla_due" : undefined,
        q: q || undefined,
      }
  const list = useConversations(filters)
  const pinnedList = useConversations({ ...filters, pinned: true })
  const loaded = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const pinnedLoaded = useMemo(() => pinnedList.data?.pages.flatMap((p) => p.items) ?? [], [pinnedList.data])
  const keep = (c: ConversationListItem) =>
    (!unreadOnly || c.unread) && (view.id !== null || chip !== "waiting" || isWaiting(c))
  const byTime = (a: ConversationListItem, b: ConversationListItem) =>
    (a.last_activity_at < b.last_activity_at ? 1 : -1) * (sort === "oldest" ? -1 : 1)
  const pinnedRows = [...pinnedLoaded, ...loaded.filter((c) => c.pinned_at && !pinnedLoaded.some((x) => x.id === c.id))]
    .filter((c) => c.pinned_at && keep(c))
    .sort(sort === "sla_due" ? () => 0 : byTime)
  const mainRows = loaded.filter((c) => !c.pinned_at && keep(c))
  if (sort === "oldest") mainRows.sort(byTime)
  const rows = [...pinnedRows, ...mainRows]
  const live = useLiveIds(rows.filter((c) => c.status !== "closed"))
  const { cursorId, setCursorId, moveCursor, openCursor } = useListCursor(rows, currentId, onOpen, active, scroller)

  const scope = [
    chip,
    view.id,
    q,
    inboxId,
    labelId,
    teamId,
    unreadOnly,
    source.source,
    source.site,
    source.broadcast?.id,
    source.field?.key,
    source.field?.value,
    source.sla,
    sort,
  ].join("|")
  const shown = useRef({ scope, ids: [] as string[] })
  useEffect(() => {
    const before = shown.current
    const ids = rows.map((c) => c.id)
    shown.current = { scope, ids }
    if (!active || !currentId || selecting || list.isPending || before.scope !== scope) return
    const at = before.ids.indexOf(currentId)
    if (at < 0 || ids.includes(currentId)) return
    const next = ids[Math.min(at, ids.length - 1)]
    if (next) onOpen(next)
  })

  const toggle = (id: string) => {
    setSelecting(true)
    setSelected((s) => {
      const next = new Set(s)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }
  const stopSelecting = () => {
    setSelecting(false)
    setSelected(new Set())
  }
  const linkKey = keyLabel(SHORTCUTS.linkSelected).join("")
  const chosenRows = rows.filter((c) => selected.has(c.id))
  const chosen = chosenRows.map((c) => c.id)
  const picked = chosen.length
  useHotkeys(
    {
      [SHORTCUTS.select]: () => {
        const id = cursorId ?? currentId
        if (id) toggle(id)
        else setSelecting(true)
      },
      ...(selecting
        ? {
            Escape: stopSelecting,
            ...(chosen.length > 0 ? { [SHORTCUTS.linkSelected]: () => setLinking(true) } : {}),
          }
        : {}),
    },
    active
  )

  const [selectSeen, setSelectSeen] = useState(select)
  if (select !== selectSeen) {
    setSelectSeen(select)
    setSelecting(true)
  }

  useEffect(() => {
    if (focus <= focused) return
    focused = focus
    requestAnimationFrame(() => search.current?.focus())
  }, [focus])

  useEffect(() => {
    const el = sentinel.current
    const root = scroller.current
    if (!el || !root || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => setNearEnd(entries.some((e) => e.isIntersecting)), {
      root,
      rootMargin: "400px",
    })
    io.observe(el)
    return () => io.disconnect()
  }, [])
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = list
  useEffect(() => {
    if (nearEnd && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [nearEnd, hasNextPage, isFetchingNextPage, fetchNextPage, loaded.length])

  const reset = () => {
    setCursorId(null)
    scroller.current?.scrollTo({ top: 0 })
  }
  const seenView = useRef(view.id)
  const changeView = useLatest(() => {
    setSource({})
    if (view.id) setListQuery(view.q)
    setCursorId(null)
  })
  useEffect(() => {
    const was = seenView.current
    seenView.current = view.id
    if (!view.id && !was) return
    setOrder(null)
    setLabelId("")
    setTeamId("")
    setUnreadOnly(false)
    changeView.current()
    scroller.current?.scrollTo({ top: 0 })
  }, [view.id, changeView])
  const setChip = (c: Chip) => {
    view.clear()
    rememberChip(c)
    setOwnChip(c)
    onChip?.(c)
    reset()
  }
  const setInbox = (id: string) => {
    setOwnInbox(id)
    onInbox?.(id)
    reset()
  }
  const [shownChip, setShownChip] = useState(chip)
  if (shownChip !== chip) {
    setShownChip(chip)
    setCursorId(null)
  }
  useEffect(() => {
    scroller.current?.scrollTo({ top: 0 })
  }, [chip])

  const labels = useChipLabels()
  const empty: Record<Chip, React.ReactNode> = {
    waiting: <Trans>Nobody is waiting for a reply.</Trans>,
    all: <Trans>No conversations yet.</Trans>,
    mine: <Trans>Nothing is assigned to you.</Trans>,
    teams: <Trans>Nothing is assigned to your teams.</Trans>,
    unassigned: <Trans>Every open conversation has someone.</Trans>,
    snoozed: <Trans>Nothing is snoozed.</Trans>,
    done: <Trans>Nothing is closed yet.</Trans>,
    trash: <Trans>The Trash is empty.</Trans>,
  }
  const filtered = !!q || narrowed || unreadOnly
  const clearFilters = () => {
    setListQuery("")
    setLabelId("")
    setTeamId("")
    setSource({})
    setUnreadOnly(false)
    setInbox("")
    view.clear()
  }

  const bulk = useBulkUpdateConversations()
  const memberActions = useListActions()
  const failed = (e: unknown) => toast(errorText(e))
  const restoreStatus = (prev: ConversationListItem[]) => {
    for (const g of statusGroups(prev))
      bulk.mutate({ conversation_ids: g.ids, status: g.status, snooze_until: g.until }, { onError: failed })
  }
  const [failures, setFailures] = useState<ConversationBulkFailure[]>([])
  const closeChosen = () => {
    const prev = chosenRows.filter((c) => c.status !== "closed").slice(0, 100)
    if (prev.length === 0) return
    bulk.mutate(
      { conversation_ids: prev.map((c) => c.id), status: "closed" },
      {
        onSuccess: (r) => {
          const done = new Set(r.updated.map((c) => c.id))
          if (done.size > 0) toast(t`${done.size} closed`, () => restoreStatus(prev.filter((c) => done.has(c.id))))
          if (r.failed.length > 0) setFailures(r.failed)
          stopSelecting()
        },
        onError: failed,
      }
    )
  }
  const anyUnread = chosenRows.some((c) => c.unread)
  const readChosen = () => {
    const ids = chosenRows.filter((c) => c.unread === anyUnread).map((c) => c.id)
    const n = ids.length
    for (const id of ids) {
      if (anyUnread) memberActions.markRead(id)
      else memberActions.markUnread(id)
    }
    const undo = () => {
      for (const id of ids) {
        if (anyUnread) memberActions.markUnread(id)
        else memberActions.markRead(id)
      }
    }
    toast(anyUnread ? t`${n} marked as read` : t`${n} marked as unread`, undo)
    stopSelecting()
  }
  const labelChosen = (labelIdToToggle: string) => {
    const all = chosenRows.every((c) => c.labels.includes(labelIdToToggle))
    const targets = chosenRows.filter((c) => c.labels.includes(labelIdToToggle) === all).map((c) => c.id)
    const body = (add: boolean) => (add ? { add_labels: [labelIdToToggle] } : { remove_labels: [labelIdToToggle] })
    bulk.mutate(
      { conversation_ids: targets.slice(0, 100), ...body(!all) },
      {
        onSuccess: () => {
          toast(t`Labels changed`, () =>
            bulk.mutate({ conversation_ids: targets.slice(0, 100), ...body(all) }, { onError: failed })
          )
          stopSelecting()
        },
        onError: failed,
      }
    )
  }
  const allLabels = useLabels().data ?? []
  const allTeams = useTeams().data ?? []
  const bulkTeam = useBulkTeam()
  const teamsForAll = allTeams.filter((team) => chosenRows.every((c) => team.inbox_ids.includes(c.inbox_id)))
  const teamChosen = (teamId: string | null) =>
    bulkTeam.mutate(
      { ids: chosen.slice(0, 100), teamId },
      {
        onSuccess: () => {
          toast(t`Team changed`)
          stopSelecting()
        },
        onError: failed,
      }
    )
  const inTrash = view.base ? !!view.base.trashed : chip === "trash"
  const setTrashed = async (ids: string[], trashed: boolean) => {
    const done: string[] = []
    try {
      for (let at = 0; at < ids.length; at += 100) {
        const r = await bulk.mutateAsync({ conversation_ids: ids.slice(at, at + 100), trashed })
        done.push(...r.updated.map((c) => c.id))
      }
    } catch (e) {
      failed(e)
    }
    return done
  }
  const trashChosen = () => {
    const ids = [...chosen]
    if (ids.length === 0) return
    confirm({
      title: <Trans>Move {ids.length} to the Trash?</Trans>,
      description: (
        <Trans>
          They disappear for your team and for the customers. You can restore them from the Trash for 30 days, then they
          are deleted for good with their messages and files.
        </Trans>
      ),
      confirm: <Trans>Move to the Trash</Trans>,
      run: () =>
        void setTrashed(ids, true).then((done) => {
          if (done.length === 0) return
          toast(t`${done.length} moved to the Trash`, () => void setTrashed(done, false))
          stopSelecting()
        }),
    })
  }
  const restoreChosen = () => {
    const ids = [...chosen]
    if (ids.length === 0) return
    void setTrashed(ids, false).then((done) => {
      if (done.length === 0) return
      toast(t`${done.length} restored`, () => void setTrashed(done, true))
      stopSelecting()
    })
  }
  useHotkeys(
    selecting && canManage && chosen.length > 0
      ? { [inTrash ? SHORTCUTS.restore : SHORTCUTS.trash]: inTrash ? restoreChosen : trashChosen }
      : {},
    active
  )

  const searchBox = (
    <InputGroup className="h-[34px] min-w-0 flex-1 rounded-lg border-input bg-background shadow-none focus-within:border-faint">
      <InputGroupAddon>
        <SearchIcon className="size-[15px] text-faint" />
      </InputGroupAddon>
      <InputGroupInput
        ref={search}
        name="list-search"
        value={query}
        onChange={(e) => {
          setListQuery(e.target.value)
          setCursorId(null)
        }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault()
            moveCursor(e.key === "ArrowDown" ? 1 : -1)
          } else if (e.key === "Enter") {
            e.preventDefault()
            openCursor()
          } else if (e.key === "Escape") {
            if (query) setListQuery("")
            else e.currentTarget.blur()
          }
        }}
        placeholder={t`Search`}
        aria-label={t`Search conversations`}
        data-testid="list-search"
      />
      <InputGroupAddon align="inline-end">
        {query ? (
          <InputGroupButton size="icon-xs" onClick={() => setListQuery("")} aria-label={t`Clear the search`}>
            <XIcon />
          </InputGroupButton>
        ) : (
          <Kbd className="phone:hidden">/</Kbd>
        )}
      </InputGroupAddon>
    </InputGroup>
  )

  const searchRow = (
    <div className="flex shrink-0 items-center gap-1.5 px-3 pt-2.5 pb-1.5">
      {searchBox}
      <Button
        variant="outline"
        className={cn(look.btn, "w-[34px] px-0", selecting && "bg-muted")}
        onClick={() => (selecting ? stopSelecting() : setSelecting(true))}
        aria-pressed={selecting}
        aria-label={t`Select conversations`}
        title={t`Select conversations`}
        data-testid="select-conversations"
      >
        <SquareCheckBigIcon />
      </Button>
    </div>
  )

  const none = picked === 0
  const selection = (
    <div
      className="flex shrink-0 flex-wrap items-center gap-1.5 px-3 pt-1 pb-2"
      role="toolbar"
      aria-label={t`Selection`}
      data-testid="selection-bar"
    >
      <span className="me-1 min-w-0 truncate text-small font-medium" aria-live="polite">
        {none ? <Trans>Pick conversations</Trans> : <Trans>Selected: {picked}</Trans>}
      </span>
      {inTrash ? (
        <Button
          variant="outline"
          className={look.btnSm}
          disabled={none || bulk.isPending}
          onClick={restoreChosen}
          data-testid="restore-selected"
        >
          <Trans>Restore</Trans>
        </Button>
      ) : (
        <>
          <Button
            variant="outline"
            className={look.btnSm}
            disabled={none || bulk.isPending}
            onClick={closeChosen}
            data-testid="close-selected"
          >
            <Trans>Close all</Trans>
          </Button>
          <Button
            variant="outline"
            className={look.btnSm}
            disabled={none}
            onClick={readChosen}
            data-testid="read-selected"
          >
            {anyUnread || none ? <Trans>Mark as read</Trans> : <Trans>Mark as unread</Trans>}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="outline" className={look.btnSm} disabled={none || bulk.isPending} />}
              data-testid="label-selected"
            >
              <Trans>Label</Trans>
              <ChevronDownIcon className="text-faint" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
              {allLabels.length === 0 && (
                <p className="px-2.5 py-1.5 text-body text-muted-foreground">
                  <Trans>No labels yet. Add them in settings.</Trans>
                </p>
              )}
              {allLabels.map((l) => (
                <DropdownMenuCheckboxItem
                  key={l.id}
                  checked={chosenRows.length > 0 && chosenRows.every((c) => c.labels.includes(l.id))}
                  onCheckedChange={() => labelChosen(l.id)}
                >
                  <Dot color={l.color} />
                  {l.name}
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          {allTeams.length > 0 && (
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="outline" className={look.btnSm} disabled={none || bulkTeam.isPending} />}
                data-testid="team-selected"
              >
                <Trans>Team</Trans>
                <ChevronDownIcon className="text-faint" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
                {teamsForAll.length === 0 && (
                  <p className="px-2.5 py-1.5 text-body text-muted-foreground">
                    <Trans>No team covers all the selected inboxes.</Trans>
                  </p>
                )}
                {teamsForAll.map((team) => (
                  <DropdownMenuItem key={team.id} onClick={() => teamChosen(team.id)}>
                    {team.name}
                  </DropdownMenuItem>
                ))}
                {teamsForAll.length > 0 && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={() => teamChosen(null)}>
                      <Trans>Remove the team</Trans>
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
          <Button
            variant="outline"
            className={look.btnSm}
            disabled={none}
            onClick={() => setLinking(true)}
            title={t`Link to an incident (${linkKey})`}
            data-testid="link-selected"
          >
            <Trans>Link to an incident</Trans>
          </Button>
          {canManage && (
            <Button
              variant="outline"
              className={cn(look.btnSm, "text-destructive")}
              disabled={none || bulk.isPending}
              onClick={trashChosen}
              data-testid="trash-selected"
            >
              <Trans>Delete</Trans>
            </Button>
          )}
        </>
      )}
      <Button
        variant="ghost"
        className="h-[30px] rounded-lg px-2"
        onClick={stopSelecting}
        title={t`Clear the selection (Esc)`}
      >
        <Trans>Cancel</Trans>
      </Button>
    </div>
  )

  const chips = (
    <div className="flex shrink-0 flex-wrap items-center gap-1.5 px-3 pb-2.5" data-testid="chips">
      <InboxChip value={sidebarInbox || chipInbox} locked={locked} onChange={setInbox} />
      <SourceFilter inboxId={inboxId} onChange={reset} placeholder={<Trans>Filter</Trans>} />
      <LabelChip
        value={labelId}
        onChange={(id) => {
          setLabelId(id)
          reset()
        }}
      />
      <TeamChip
        value={teamId}
        onChange={(id) => {
          setTeamId(id)
          reset()
        }}
      />
      <OrderChip
        order={sort}
        unreadOnly={unreadOnly}
        onOrder={(o) => {
          setOrder(o)
          reset()
        }}
        onUnread={(on) => {
          setUnreadOnly(on)
          reset()
        }}
      />
      <ViewsMenu filters={filters} active={active} />
    </div>
  )

  const counts = useCounts(["conversation"]).data
  const unassignedHere = useLoadedCount({
    kind: ["conversation"],
    inbox_id: inboxId || undefined,
    status: ["open"],
    assignee: "unassigned",
  })
  const waitingHere = useLoadedCount(
    { kind: ["conversation"], inbox_id: inboxId || undefined, status: ["open"] },
    isWaiting
  )

  return (
    <nav
      aria-label={t`Conversations`}
      className="@container/list flex h-full min-h-0 flex-col bg-background"
      data-testid="chat-list"
    >
      {!embedded && (
        <PhoneHeader>
          <UnderlineTabs
            value={view.id ? null : chip}
            onChange={setChip}
            label={t`Filter`}
            items={tabs.map((k) => ({
              value: k,
              label: labels[k],
              count:
                k === "waiting"
                  ? waitingHere
                  : k === "unassigned"
                    ? inboxId
                      ? unassignedHere
                      : counts?.unassigned
                    : k === "teams"
                      ? counts?.mine_teams
                      : undefined,
              testId: `chip-${k}`,
            }))}
          />
        </PhoneHeader>
      )}
      {searchRow}
      {selecting ? selection : chips}
      <AwayBanner />
      {!view.id && chip === "waiting" && <CleanupBanner inboxId={inboxId} />}
      <ImportBanner />
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto border-t" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1 p-3">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3 py-1.5">
                <Skeleton className="size-9 rounded-full" />
                <div className="flex flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-3 w-3/4" />
                </div>
              </div>
            ))}
          </div>
        ) : rows.length === 0 && !hasNextPage ? (
          <div className="flex flex-col items-center gap-3 px-4 py-10 text-center" data-testid="list-empty">
            <p className="text-body text-faint">
              {q ? (
                <Trans>Nothing matches your search here.</Trans>
              ) : narrowed || unreadOnly ? (
                <Trans>Nothing matches these filters.</Trans>
              ) : (
                empty[view.id ? "all" : chip]
              )}
            </p>
            {filtered && (
              <Button variant="outline" className={look.btnSm} onClick={clearFilters} data-testid="clear-filters">
                <Trans>Clear search and filters</Trans>
              </Button>
            )}
          </div>
        ) : (
          <>
            {pinnedRows.length > 0 && (
              <div
                className="flex items-center gap-1.5 border-b px-4 pt-2 pb-1.5 text-caption font-medium text-faint"
                data-testid="pinned-heading"
              >
                <PinIcon className="size-3 rotate-45" />
                <Trans>Pinned</Trans>
              </div>
            )}
            {pinnedRows.map((c) => (
              <Row
                key={c.id}
                c={c}
                current={c.id === currentId}
                cursor={c.id === cursorId}
                live={live.has(c.id)}
                selecting={selecting}
                checked={selected.has(c.id)}
                onOpen={() => (selecting ? toggle(c.id) : onOpen(c.id))}
              />
            ))}
            {pinnedRows.length > 0 && mainRows.length > 0 && (
              <div className="h-2 border-b bg-surface" role="separator" />
            )}
            {mainRows.map((c) => (
              <Row
                key={c.id}
                c={c}
                current={c.id === currentId}
                cursor={c.id === cursorId}
                live={live.has(c.id)}
                selecting={selecting}
                checked={selected.has(c.id)}
                onOpen={() => (selecting ? toggle(c.id) : onOpen(c.id))}
              />
            ))}
          </>
        )}
        <div ref={sentinel} className="h-px" />
        {isFetchingNextPage && (
          <div className="flex items-center gap-3 px-3 py-2">
            <Skeleton className="size-9 rounded-full" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        )}
        <ErrorLine error={list.error} className="px-4 py-2" />
      </div>
      {confirmDialog}
      <BulkFailuresDialog failures={failures} rows={rows} onClose={() => setFailures([])} />
      {linking && (
        <LinkIncidentDialog conversationIds={chosen} onClose={() => setLinking(false)} onDone={stopSelecting} />
      )}
    </nav>
  )
}
