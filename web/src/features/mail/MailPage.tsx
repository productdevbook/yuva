import { Trans, useLingui } from "@lingui/react/macro"
import { PenSquareIcon } from "lucide-react"
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react"
import { Link, Navigate, useLocation, useNavigate } from "react-router"

import { Kbd, PageLayout, PageTop } from "@/components/common"
import { ListBox, ListBoxEmpty, PageTitle } from "@/components/common/ListBox"
import { MAIL_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import {
  mailPath,
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
import type { DraftItem } from "@/lib/api"
import { useIsPhone, useMediaQuery } from "@/hooks/use-media-query"
import { useViewing } from "@/lib/realtime"
import { useSession } from "@/lib/session"

function ComposeButton({ onClick }: { onClick: () => void }) {
  return (
    <Button onClick={onClick} data-testid="mail-compose">
      <PenSquareIcon />
      <Trans>New mail</Trans>
      <Kbd className="phone:hidden bg-primary-foreground/20 text-primary-foreground">{MAIL_KEYS.compose}</Kbd>
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

function NoMailInboxesPage() {
  const { t } = useLingui()
  const labels = useFolderLabels()
  return (
    <PageLayout testId="mail" label={t`Mail`}>
      <PageTitle title={labels.inbox} sub={t`All mail`} />
      <ListBox>
        <ListBoxEmpty testId="mail-none" action={<NoMailAdd />}>
          <span className="block font-medium text-foreground">
            <Trans>No team mail yet</Trans>
          </span>
          <NoMailText />
        </ListBoxEmpty>
      </ListBox>
    </PageLayout>
  )
}

function Thread({
  id,
  onBack,
  onLeave,
  box,
  folder,
  panel,
  inlinePanel,
  onPanel,
}: {
  id: string
  onBack?: () => void
  onLeave: () => void
  box: string
  folder: Folder
  panel: boolean
  inlinePanel: boolean
  onPanel: (open: boolean) => void
}) {
  const state = useLocation().state as { editDraft?: string } | null
  return (
    <MailThread
      key={id}
      id={id}
      onBack={onBack}
      onLeave={onLeave}
      openDraft={state?.editDraft}
      box={box}
      folder={folder}
      panel={panel}
      inlinePanel={inlinePanel}
      onPanel={onPanel}
    />
  )
}

const PANEL_KEY = "mail-panel"

function usePanel() {
  const [open, setOpen] = useState(() => {
    try {
      return localStorage.getItem(PANEL_KEY) !== "closed"
    } catch {
      return true
    }
  })
  const set = useCallback((v: boolean) => {
    setOpen(v)
    try {
      localStorage.setItem(PANEL_KEY, v ? "open" : "closed")
    } catch {
      // Storage can be blocked; the panel then stays as chosen for this page only.
    }
  }, [])
  return [open, set] as const
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
  const [pager, setPager] = useState<HTMLElement | null>(null)
  const wide = useMediaQuery("(min-width: 1180px)")
  const [panel, setPanel] = usePanel()
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
  if (inboxes.length === 0) return <NoMailInboxesPage />
  if (pathname !== canonical) return <Navigate to={canonical} replace />

  const compose = () => setComposing({})
  if (composing)
    return (
      <PageLayout testId="mail" label={t`Mail`}>
        <Compose
          key={composing.draft?.draft.id}
          box={box}
          draft={composing.draft}
          onClose={() => setComposing(null)}
          backLabel={labels[folder]}
        />
      </PageLayout>
    )

  if (phone && id) {
    return (
      <main className="flex min-h-0 flex-1 flex-col" data-testid="mail">
        <div className="flex min-h-0 flex-1 flex-col px-3 pt-2.5 pb-2">
          <Thread
            id={id}
            onBack={leave}
            onLeave={leave}
            box={box}
            folder={folder}
            panel={false}
            inlinePanel={false}
            onPanel={setPanel}
          />
        </div>
      </main>
    )
  }

  const mailbox = inboxes.find((i) => i.id === box)
  const address = mailbox?.mailbox?.addresses[0]
  const boxName = box === "all" ? t`All mail` : (mailbox?.name ?? "")
  const heading = (
    <PageTitle
      title={labels[folder]}
      sub={
        <span data-testid="list-lede">
          {boxName}
          {address && address !== boxName && <span className="text-faint"> · {address}</span>}
        </span>
      }
      actions={<ComposeButton onClick={compose} />}
    />
  )
  const list =
    folder === "drafts" ? (
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
      <MailList
        box={box}
        folder={folder}
        currentId={id}
        onOpen={open}
        active
        hidden={!!id}
        head={heading}
        pagerSlot={pager}
        place={labels[folder]}
      />
    )
  if (id)
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="mail">
        <div className="mx-auto flex min-h-0 w-full max-w-[1440px] flex-1 flex-col ps-8 pe-7 pb-4 max-md:px-6">
          <PageTop
            back={{ label: labels[folder], onClick: leave }}
            slot={setPager}
            actions={<ComposeButton onClick={compose} />}
          />
          {list}
          <Thread
            id={id}
            onLeave={leave}
            box={box}
            folder={folder}
            panel={panel}
            inlinePanel={wide}
            onPanel={setPanel}
          />
        </div>
      </div>
    )
  return (
    <PageLayout testId="mail" label={t`Mail`}>
      {list}
    </PageLayout>
  )
}
