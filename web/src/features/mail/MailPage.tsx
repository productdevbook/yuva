import { Trans, useLingui } from "@lingui/react/macro"
import { PenSquareIcon } from "lucide-react"
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react"
import { Link, Navigate, useLocation, useNavigate } from "react-router"

import { Dot, InfoColumn, Kbd, PageLayout, PageTop, TopSep, TopSlotProvider } from "@/components/common"
import { ListBox, ListBoxEmpty, PageTitle } from "@/components/common/ListBox"
import { useEnumText } from "@/components/common/text"
import { useConversation } from "@/features/conversation/queries"
import { MailThreadInfo } from "@/features/mail/MailSide"
import { MAIL_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { mailPath, useFolderLabels, useFolders, useMailInboxes, useMailRoute } from "@/features/mail/folders"
import { Compose } from "@/features/mail/Compose"
import { COMPOSE_EVENT } from "@/features/mail/compose"
import { MailDrafts } from "@/features/mail/MailDrafts"
import { MailList } from "@/features/mail/MailList"
import { MailThread } from "@/features/mail/MailThread"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { DraftItem } from "@/lib/api"
import { useIsPhone } from "@/hooks/use-media-query"
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
  if (inboxes.length === 0) return <NoMailInboxesPage />
  if (pathname !== canonical) return <Navigate to={canonical} replace />

  const compose = () => setComposing({})
  const composer = composing && (
    <Compose key={composing.draft?.draft.id} box={box} draft={composing.draft} onClose={() => setComposing(null)} />
  )

  if (phone && id) {
    return (
      <main className="flex min-h-0 flex-1 flex-col" data-testid="mail">
        <Thread id={id} onBack={leave} onLeave={leave} />
        {composer}
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
  return (
    <PageLayout
      testId="mail"
      label={t`Mail`}
      aside={
        id ? (
          <InfoColumn label={t`Details`}>
            <MailThreadInfo id={id} box={box} folder={folder} />
          </InfoColumn>
        ) : undefined
      }
    >
      {id && (
        <PageTop
          back={{ label: labels[folder], onClick: leave }}
          slot={setSlot}
          actions={<ComposeButton onClick={compose} />}
        >
          <MailEyebrow id={id} />
        </PageTop>
      )}
      <div className={id ? "max-w-[860px]" : undefined}>
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
          <MailList box={box} folder={folder} currentId={id} onOpen={open} active hidden={!!id} head={heading} />
        )}
        {id && (
          <TopSlotProvider value={slot}>
            <Thread id={id} flat onLeave={leave} />
          </TopSlotProvider>
        )}
      </div>
      {composer}
    </PageLayout>
  )
}
