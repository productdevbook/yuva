import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronDownIcon, MailIcon, MailOpenIcon } from "lucide-react"
import { useCallback } from "react"
import { useDefaultLayout } from "react-resizable-panels"
import { Link, Navigate, useLocation, useNavigate } from "react-router"

import { EmptyState } from "@/components/common"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import { ListHead, UnderlineTabs } from "@/features/inbox/ListHead"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import {
  mailPath,
  useFolderLabels,
  useFolders,
  useMailInboxes,
  useMailRoute,
  type Folder,
} from "@/features/mail/folders"
import { MailList } from "@/features/mail/MailList"
import { MailThread } from "@/features/mail/MailThread"
import { useMailCounts } from "@/features/mail/queries"
import { useIsPhone } from "@/hooks/use-media-query"
import { useViewing } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const handle = cn(
  "split-handle cursor-split bg-border bg-clip-content transition-colors duration-150 after:w-4 data-[separator=hover]:bg-brand/60 data-[separator=active]:bg-brand",
  "data-[separator=active]:z-10 data-[separator=active]:-mx-6 data-[separator=active]:px-6 data-[separator=active]:after:w-full",
  "data-[separator=hover]:z-10 data-[separator=hover]:-mx-6 data-[separator=hover]:px-6 data-[separator=hover]:after:w-full"
)

function useFolderCounts(box: string): Partial<Record<Folder, number>> {
  const counts = useMailCounts()
  if (!counts) return {}
  if (box === "all") return { inbox: counts.all, starred: counts.starred }
  return { inbox: counts.inboxes.find((c) => c.id === box)?.count ?? 0 }
}

function BoxPicker({ box, folder }: { box: string; folder: Folder }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const inboxes = useMailInboxes()
  const name = box === "all" ? t`All mail` : (inboxes.find((i) => i.id === box)?.name ?? t`All mail`)
  const pick = (id: string) => navigate(mailPath(id, folder))
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" className={cn(look.btn, "max-w-64 gap-1.5")} />}
        aria-label={t`Mailbox: ${name}`}
        data-testid="mail-box-picker"
      >
        <MailIcon className="text-faint" />
        <span className="min-w-0 truncate">{name}</span>
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
        {inboxes.map((i) => (
          <DropdownMenuItem key={i.id} onClick={() => pick(i.id)} data-testid="mail-box-option">
            <span className="min-w-0 flex-1 truncate">{i.name}</span>
            <CheckIcon className={cn("size-3.5", box !== i.id && "invisible")} />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function EmptyThread() {
  return (
    <div
      className="flex min-h-0 flex-1 flex-col items-center justify-center bg-surface px-8 text-center"
      data-testid="mail-empty"
    >
      <div className="mb-5 grid size-16 place-items-center rounded-[20px] bg-brand-wash text-brand">
        <MailOpenIcon className="size-7" />
      </div>
      <h1 className="text-title">
        <Trans>Pick a mail</Trans>
      </h1>
      <p className="mt-1.5 text-body text-muted-foreground">
        <Trans>Choose one from the list, or move with ↑ ↓ and open it with Enter.</Trans>
      </p>
    </div>
  )
}

function NoMailInboxes() {
  const { canManage } = useSession()
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="mail-none">
      <EmptyState icon={MailIcon} title={<Trans>No team mail yet</Trans>}>
        <p>
          <Trans>
            A team mail inbox keeps the mail of a shared address, like hello@ or billing@, away from customer support.
          </Trans>
        </p>
        {canManage && (
          <Button variant="outline" size="sm" className="mt-4" render={<Link to="/setup/new" />}>
            <Trans>Add an inbox</Trans>
          </Button>
        )}
      </EmptyState>
    </main>
  )
}

function Thread({ id, onBack, onLeave }: { id: string; onBack?: () => void; onLeave: () => void }) {
  return <MailThread key={id} id={id} onBack={onBack} onLeave={onLeave} />
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
  const layout = useDefaultLayout({ id: "mail-panes", panelIds: ["list", "thread"] })
  const open = useCallback((next: string) => navigate(mailPath(box, folder, next)), [navigate, box, folder])
  const leave = useCallback(() => navigate(mailPath(box, folder)), [navigate, box, folder])
  useViewing(id ?? undefined)

  const canonical = mailPath(box, folders.includes(folder) ? folder : "inbox", id ?? undefined)
  if (inboxes.length === 0) return <NoMailInboxes />
  if (pathname !== canonical) return <Navigate to={canonical} replace />

  const boxName = box === "all" ? t`All mail` : (inboxes.find((i) => i.id === box)?.name ?? "")
  const tabs = folders.map((f) => ({
    key: f,
    label: labels[f],
    count: counts[f],
    to: mailPath(box, f),
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
        <ListHead title={<Trans>Mail</Trans>} sub={boxName} action={<BoxPicker box={box} folder={folder} />}>
          <UnderlineTabs
            value={folder}
            onChange={(f) => navigate(mailPath(box, f))}
            label={t`Folders`}
            items={folders.map((f) => ({
              value: f,
              label: labels[f],
              count: counts[f],
              testId: `mail-folder-${f}`,
            }))}
          />
        </ListHead>
        <MailList box={box} folder={folder} currentId={null} onOpen={open} active />
      </main>
    )
  }

  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="mail">
      <PageTop
        title={<Trans>Mail</Trans>}
        sub={boxName}
        actions={<BoxPicker box={box} folder={folder} />}
        tabs={<FrameTabs label={t`Folders`} value={folder} items={tabs} />}
      />
      <ResizablePanelGroup
        className="min-h-0 flex-1"
        disableCursor
        resizeTargetMinimumSize={{ fine: 16, coarse: 32 }}
        defaultLayout={layout.defaultLayout}
        onLayoutChanged={layout.onLayoutChanged}
      >
        <ResizablePanel
          id="list"
          className="flex flex-col"
          defaultSize={380}
          minSize={280}
          maxSize={600}
          groupResizeBehavior="preserve-pixel-size"
        >
          <MailList box={box} folder={folder} currentId={id} onOpen={open} active />
        </ResizablePanel>
        <ResizableHandle withHandle className={handle} />
        <ResizablePanel id="thread" className="flex flex-col" minSize={440}>
          {id ? <Thread id={id} onLeave={leave} /> : <EmptyThread />}
        </ResizablePanel>
      </ResizablePanelGroup>
    </main>
  )
}
