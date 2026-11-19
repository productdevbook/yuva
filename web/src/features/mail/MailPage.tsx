import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronDownIcon, FilterIcon, MailIcon, PenSquareIcon, PlusIcon } from "lucide-react"
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react"
import { Link, Navigate, useLocation, useNavigate } from "react-router"

import {
  Dot,
  EmptyState,
  InfoColumn,
  Kbd,
  FlatEmpty,
  PageHeading,
  PageLayout,
  PageTop,
  TopSep,
  TopSlotProvider,
} from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { useConversation } from "@/features/conversation/queries"
import { MailListInfo, MailThreadInfo, useFolderCounts } from "@/features/mail/MailSide"
import { MAIL_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { ListHead, ViewChips } from "@/features/inbox/ListHead"
import {
  mailPath,
  useFilterAccess,
  useFolderLabels,
  useFolders,
  useMailInboxes,
  useMailRoute,
  type Folder,
} from "@/features/mail/folders"
import { Compose } from "@/features/mail/Compose"
import { COMPOSE_EVENT } from "@/features/mail/compose"
import { MailDrafts } from "@/features/mail/MailDrafts"
import { MailList } from "@/features/mail/MailList"
import { MailThread } from "@/features/mail/MailThread"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { DraftItem, Inbox } from "@/lib/api"
import { useIsPhone } from "@/hooks/use-media-query"
import { useViewing } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const chrome = "h-8 rounded-[9px] border-input bg-card font-normal"

function BoxPicker({ box, folder, compact }: { box: string; folder: Folder; compact?: boolean }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const inboxes = useMailInboxes()
  const name = box === "all" ? t`All mail` : (inboxes.find((i) => i.id === box)?.name ?? t`All mail`)
  const pick = (id: string) => navigate(mailPath(id, folder))
  const own = inboxes.filter((i) => i.mailbox?.access === "owner")
  const shared = inboxes.filter((i) => i.mailbox && i.mailbox.access !== "owner")
  const team = inboxes.filter((i) => !i.mailbox)
  const grouped = own.length + shared.length > 0
  const option = (i: Inbox) => (
    <DropdownMenuItem key={i.id} onClick={() => pick(i.id)} data-testid="mail-box-option">
      <span className="min-w-0 flex-1 truncate">{i.name}</span>
      {i.mailbox?.access === "read" && (
        <span className="text-caption text-faint">
          <Trans context="mailbox access">Read</Trans>
        </span>
      )}
      {i.mailbox?.access === "send" && (
        <span className="text-caption text-faint">
          <Trans>Read and send</Trans>
        </span>
      )}
      <CheckIcon className={cn("size-3.5", box !== i.id && "invisible")} />
    </DropdownMenuItem>
  )
  const group = (title: React.ReactNode, items: Inbox[], testId: string) =>
    items.length > 0 && (
      <DropdownMenuGroup data-testid={testId}>
        {grouped && <DropdownMenuLabel>{title}</DropdownMenuLabel>}
        {items.map(option)}
      </DropdownMenuGroup>
    )
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" className={cn(chrome, compact ? "gap-0.5 px-2" : "max-w-40 gap-1.5 px-2.5")} />
        }
        aria-label={t`Mailbox: ${name}`}
        title={name}
        data-testid="mail-box-picker"
      >
        <MailIcon className="text-faint" />
        {!compact && <span className="min-w-0 truncate">{name}</span>}
        <ChevronDownIcon className="text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <DropdownMenuItem onClick={() => pick("all")} data-testid="mail-box-all">
          <span className="flex-1">
            <Trans>All mail</Trans>
          </span>
          <CheckIcon className={cn("size-3.5", box !== "all" && "invisible")} />
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {group(<Trans>Your mailboxes</Trans>, own, "mail-box-own")}
        {group(<Trans>Shared with you</Trans>, shared, "mail-box-delegated")}
        {group(<Trans>Team mail</Trans>, team, "mail-box-team")}
        <DropdownMenuSeparator />
        <DropdownMenuItem render={<Link to="/setup/mail" state={{ back: "/mail" }} />} data-testid="mail-box-new">
          <PlusIcon className="size-3.5" />
          <span className="flex-1">
            <Trans>New mailbox</Trans>
          </span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function FiltersLink({ box, compact }: { box: string; compact?: boolean }) {
  const { t } = useLingui()
  const allowed = useFilterAccess(box)
  if (!allowed) return null
  if (compact)
    return (
      <Button
        variant="outline"
        size="icon"
        className={cn(chrome, "w-8 px-0")}
        render={<Link to={`/mail/${box}/filters`} />}
        aria-label={t`Filters`}
        data-testid="mail-filters-link"
      >
        <FilterIcon className="text-faint" />
      </Button>
    )
  return (
    <Button
      variant="outline"
      className={cn(chrome, "gap-1.5 px-2.5")}
      render={<Link to={`/mail/${box}/filters`} />}
      data-testid="mail-filters-link"
    >
      <FilterIcon className="text-faint" />
      <Trans>Filters</Trans>
    </Button>
  )
}

function ComposeButton({ onClick }: { onClick: () => void }) {
  return (
    <Button
      className="h-8 gap-1.5 rounded-[9px] bg-foreground px-3 text-background hover:bg-foreground/90"
      onClick={onClick}
      data-testid="mail-compose"
    >
      <PenSquareIcon />
      <Trans>New mail</Trans>
      <Kbd className="phone:hidden bg-background/15 text-background">{MAIL_KEYS.compose}</Kbd>
    </Button>
  )
}

function NoMailText() {
  return (
    <Trans>
      A team mail inbox keeps the mail of a shared address, like hello@ or billing@, away from customer support.
    </Trans>
  )
}

function NoMailAdd({ className }: { className?: string }) {
  const { canManage } = useSession()
  return (
    <Button
      variant="outline"
      size="sm"
      className={className}
      render={<Link to={canManage ? "/setup/new" : "/setup/mail"} state={{ back: "/mail" }} />}
      data-testid="mail-none-add"
    >
      {canManage ? <Trans>Add an inbox</Trans> : <Trans>Create your own mailbox</Trans>}
    </Button>
  )
}

function NoMailInboxes() {
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="mail-none">
      <EmptyState icon={MailIcon} title={<Trans>No team mail yet</Trans>}>
        <p>
          <NoMailText />
        </p>
        <NoMailAdd className="mt-4" />
      </EmptyState>
    </main>
  )
}

function NoMailInboxesPage() {
  const { t } = useLingui()
  const labels = useFolderLabels()
  return (
    <PageLayout
      testId="mail"
      label={t`Mail`}
      aside={
        <InfoColumn label={t`Details`}>
          <MailListInfo box="all" folder="inbox" />
        </InfoColumn>
      }
    >
      <PageTop>
        <span className="font-medium text-foreground">
          <Trans>Mail</Trans>
        </span>
        <TopSep />
        <span className="truncate">{labels.inbox}</span>
      </PageTop>
      <div className="max-w-[860px]">
        <PageHeading title={labels.inbox} lede={t`All mail`} />
        <FlatEmpty testId="mail-none">
          <p className="m-0 font-medium text-foreground">
            <Trans>No team mail yet</Trans>
          </p>
          <p className="m-0 max-w-[560px]">
            <NoMailText />
          </p>
          <NoMailAdd />
        </FlatEmpty>
      </div>
    </PageLayout>
  )
}

function Thread({
  id,
  flat,
  onBack,
  onLeave,
}: {
  id: string
  flat?: boolean
  onBack?: () => void
  onLeave: () => void
}) {
  const state = useLocation().state as { editDraft?: string } | null
  return <MailThread key={id} id={id} flat={flat} onBack={onBack} onLeave={onLeave} openDraft={state?.editDraft} />
}

function MailEyebrow({ id }: { id: string }) {
  const text = useEnumText()
  const c = useConversation(id).data
  const inbox = useMailInboxes().find((i) => i.id === c?.inbox_id)
  if (!c) return null
  return (
    <>
      {inbox && (
        <>
          <Dot color={inbox.branding.color} className="size-2 rounded-[2px]" />
          <span className="truncate font-medium text-foreground">{inbox.name}</span>
          <TopSep />
        </>
      )}
      <span className="truncate">{text.statusOf(c.status, c.kind)}</span>
    </>
  )
}

export function MailPage() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const phone = useIsPhone()
  const inboxes = useMailInboxes()
  const { box, folder, id } = useMailRoute()
  const folders = useFolders()
  const labels = useFolderLabels()
  const counts = useFolderCounts(box)
  const [slot, setSlot] = useState<HTMLElement | null>(null)
  const listY = useRef(0)
  const open = useCallback(
    (next: string, editDraft?: string) => {
      if (!id) listY.current = window.scrollY
      navigate(mailPath(box, folder, next), editDraft ? { state: { editDraft } } : undefined)
    },
    [navigate, box, folder, id]
  )
  useLayoutEffect(() => {
    if (!phone) window.scrollTo({ top: id ? 0 : listY.current })
  }, [id, phone])
  const leave = useCallback(() => navigate(mailPath(box, folder)), [navigate, box, folder])
  useViewing(id ?? undefined)
  const [composing, setComposing] = useState<{ draft?: DraftItem } | null>(null)
  useEffect(() => {
    const on = () => setComposing({})
    window.addEventListener(COMPOSE_EVENT, on)
    return () => window.removeEventListener(COMPOSE_EVENT, on)
  }, [])
  useHotkeys({ [MAIL_KEYS.compose]: () => setComposing({}) }, inboxes.length > 0 && !composing)
  useHotkeys({ Escape: leave }, !phone && !!id && !composing)

  const canonical = mailPath(box, folders.includes(folder) ? folder : "inbox", id ?? undefined)
  if (inboxes.length === 0) return phone ? <NoMailInboxes /> : <NoMailInboxesPage />
  if (pathname !== canonical) return <Navigate to={canonical} replace />

  const list = (currentId: string | null, head?: React.ReactNode) =>
    folder === "drafts" ? (
      <MailDrafts
        box={box}
        currentId={currentId}
        onOpen={open}
        onCompose={(draft) => setComposing({ draft })}
        active={!composing}
        head={head}
      />
    ) : (
      <MailList box={box} folder={folder} currentId={currentId} onOpen={open} active head={head} />
    )

  const boxName = box === "all" ? t`All mail` : (inboxes.find((i) => i.id === box)?.name ?? "")
  const chips = folders.map((f) => ({
    value: f,
    label: labels[f],
    count: counts[f],
    testId: `mail-folder-${f}`,
  }))

  if (phone) {
    if (id) {
      return (
        <main className="flex min-h-0 flex-1 flex-col" data-testid="mail">
          <Thread id={id} onBack={leave} onLeave={leave} />
        </main>
      )
    }
    return (
      <main className="flex min-h-0 flex-1 flex-col" data-testid="mail">
        <ListHead
          title={<Trans>Mail</Trans>}
          sub={boxName}
          action={
            <>
              <BoxPicker box={box} folder={folder} />
              <FiltersLink box={box} compact />
              <ComposeButton onClick={() => setComposing({})} />
            </>
          }
        >
          <ViewChips
            value={folder}
            onChange={(f) => navigate(mailPath(box, f))}
            label={t`Folders`}
            className="-mx-4 mt-2.5 px-4"
            items={chips}
          />
        </ListHead>
        {list(null)}
        {composing && (
          <Compose
            key={composing.draft?.draft.id}
            box={box}
            draft={composing.draft}
            onClose={() => setComposing(null)}
          />
        )}
      </main>
    )
  }

  const heading = <PageHeading title={labels[folder]} lede={<span data-testid="list-lede">{boxName}</span>} />
  return (
    <PageLayout
      testId="mail"
      label={t`Mail`}
      aside={
        <InfoColumn label={t`Details`}>
          {id ? <MailThreadInfo id={id} box={box} folder={folder} /> : <MailListInfo box={box} folder={folder} />}
        </InfoColumn>
      }
    >
      <PageTop
        back={id ? { label: labels[folder], onClick: leave } : undefined}
        slot={id ? setSlot : undefined}
        actions={<ComposeButton onClick={() => setComposing({})} />}
      >
        {id ? (
          <MailEyebrow id={id} />
        ) : (
          <>
            <span className="font-medium text-foreground">
              <Trans>Mail</Trans>
            </span>
            <TopSep />
            <span className="truncate">{labels[folder]}</span>
            <TopSep />
            <span className="truncate">{boxName}</span>
          </>
        )}
      </PageTop>
      <div className="max-w-[860px]">
        {folder === "drafts" ? (
          <MailDrafts
            box={box}
            currentId={id}
            onOpen={open}
            onCompose={(draft) => setComposing({ draft })}
            active={!composing}
            flat
            hidden={!!id}
            head={heading}
          />
        ) : (
          <MailList box={box} folder={folder} currentId={id} onOpen={open} active flat hidden={!!id} head={heading} />
        )}
        {id && (
          <TopSlotProvider value={slot}>
            <Thread id={id} flat onLeave={leave} />
          </TopSlotProvider>
        )}
      </div>
      {composing && (
        <Compose key={composing.draft?.draft.id} box={box} draft={composing.draft} onClose={() => setComposing(null)} />
      )}
    </PageLayout>
  )
}
