import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowRightLeftIcon,
  BookOpenIcon,
  CheckIcon,
  MailIcon,
  MoreHorizontalIcon,
  PlusIcon,
  XIcon,
} from "lucide-react"
import { useState } from "react"
import { Link, Navigate, useNavigate, useSearchParams } from "react-router"

import { ErrorLine, Notice, useConfirm } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import {
  MigrationBar,
  MigrationStateTag,
  useMigrationConfirms,
  useStateAnnouncement,
  WaitingLine,
} from "@/features/migrations/parts"
import { useDeleteMigration, useMigrations, useMigrationStep, usePresets } from "@/features/migrations/queries"
import { guideUrl } from "@/features/migrations/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { Card, ChoiceSelect, Field, headButton, RowIcon, Rows, Section } from "@/features/settings/ui"
import type { Migration } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes, useMemberMap } from "@/lib/workspace"

const PROGRESS_STATES = ["copying", "paused", "needs_attention", "syncing"]

function NewMigrationDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const inboxes = useInboxes().data ?? []
  const [filter] = useInboxFilter()
  const [inboxId, setInboxId] = useState(filter || inboxes[0]?.id || "")
  const sources: [React.ReactNode, React.ReactNode][] = [
    [<Trans key="m">Mailboxes</Trans>, <Trans key="mt">Gmail, Outlook, Yandex, any IMAP server</Trans>],
    [<Trans key="h">Help desks</Trans>, <Trans key="ht">Zendesk, Help Scout, Front, Freshdesk, Intercom</Trans>],
    [<Trans key="f">Files</Trans>, <Trans key="ft">An mbox export of a mailbox</Trans>],
  ]
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="migration-new-dialog">
        <DialogHeader>
          <DialogTitle>
            <Trans>New migration</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Yuva only reads the source; nothing there is changed.</Trans>
          </DialogDescription>
        </DialogHeader>
        <ul className="flex flex-col divide-y rounded-xl border">
          {sources.map(([title, hint], i) => (
            <li key={i} className="flex flex-col px-3.5 py-2.5 leading-[1.35]">
              <span className="font-medium">{title}</span>
              <span className="text-caption text-faint">{hint}</span>
            </li>
          ))}
        </ul>
        {inboxes.length > 0 && (
          <Field label={<Trans>Product</Trans>} htmlFor="migration-product">
            <ChoiceSelect
              id="migration-product"
              label={t`Product`}
              value={inboxId}
              onChange={setInboxId}
              options={inboxes.map((i) => [i.id, i.name] as const)}
              className="w-full"
            />
          </Field>
        )}
        <p className="text-caption text-faint">
          <Trans>Next you pick the source and connect it. Nothing is copied until you start.</Trans>
        </p>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            <Trans>Cancel</Trans>
          </Button>
          <Button
            onClick={() => navigate(inboxId ? `/migrations/new?inbox=${inboxId}` : "/migrations/new")}
            data-testid="migration-new-continue"
          >
            <Trans>Continue</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function MigrationsPage() {
  const { canManage } = useSession()
  const list = useMigrations()
  const [filter] = useInboxFilter()
  const [creating, setCreating] = useState(false)
  const items = (list.data?.pages.flatMap((p) => p.items) ?? []).filter((m) => !filter || m.inbox_id === filter)
  const announcement = useStateAnnouncement(items)
  const [params] = useSearchParams()
  if (!canManage) return <Navigate to="/settings/profile" replace />
  return (
    <>
      <GeneralHeader current="migrations" />
      <Intro
        action={
          <Button className={headButton.primary} onClick={() => setCreating(true)} data-testid="migration-new">
            <PlusIcon />
            <Trans>New migration</Trans>
          </Button>
        }
      >
        <Trans>
          Bring the history of a mailbox you answer from today, then switch its mail over. Yuva only reads the mailbox;
          nothing there is changed.
        </Trans>
      </Intro>
      {announcement}
      {params.get("result") === "expired" && (
        <Notice tone="danger" icon={XIcon} className="mb-6" role="alert" data-testid="migration-sign-in-result">
          <p className="font-medium">
            <Trans>The sign-in expired</Trans>
          </p>
          <p>
            <Trans>
              A sign-in must come back within 10 minutes and works once. Open the migration and sign in again.
            </Trans>
          </p>
        </Notice>
      )}
      {params.get("result") === "admin_consented" && (
        <Notice icon={CheckIcon} className="mb-6" role="status" data-testid="migration-admin-consented">
          <p className="font-medium text-foreground">
            <Trans>Consent granted</Trans>
          </p>
          <p>
            <Trans>
              Yuva may now read your organization's mailboxes over IMAP, each once its owner signs in. Whoever started
              the migration can open it and sign in with Microsoft again.
            </Trans>
          </p>
        </Notice>
      )}
      <Section>
        <Card flush>
          {list.isPending ? (
            <Skeleton className="m-5 h-10" />
          ) : items.length === 0 && !list.error ? (
            <Empty />
          ) : (
            <Rows>
              {items.map((m) => (
                <MigrationRow key={m.id} m={m} />
              ))}
            </Rows>
          )}
          <ErrorLine error={list.error} className="px-5 py-4" />
          {list.hasNextPage && (
            <div className="border-t px-4 py-3">
              <Button variant="ghost" size="sm" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
                <Trans>Show more</Trans>
              </Button>
            </div>
          )}
        </Card>
      </Section>
      {creating && <NewMigrationDialog onClose={() => setCreating(false)} />}
    </>
  )
}

function Empty() {
  return (
    <div className="flex flex-col items-start gap-3 px-4 py-6" data-testid="migrations-empty">
      <span className="grid size-10 place-items-center rounded-full bg-brand-wash text-brand [&_svg]:size-5">
        <ArrowRightLeftIcon />
      </span>
      <p className="max-w-xl text-body text-muted-foreground">
        <Trans>
          Moving from Gmail, Google Workspace, Yandex, iCloud, Zoho, Yahoo, Fastmail, GMX or any IMAP server? Yuva
          copies the mailbox's history into an inbox, quietly and newest first, while your team keeps working. Then you
          forward the mailbox here and nothing is lost in between.
        </Trans>
      </p>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" render={<Link to="/migrations/new" />}>
          <Trans>Start</Trans>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          render={<a href={guideUrl("migrate")} target="_blank" rel="noreferrer" />}
          data-testid="migrations-guide"
        >
          <BookOpenIcon />
          <Trans>Read the guide</Trans>
        </Button>
      </div>
    </div>
  )
}

function MigrationRow({ m }: { m: Migration }) {
  const { i18n } = useLingui()
  const num = useNumber()
  const inboxes = useInboxes().data ?? []
  const members = useMemberMap()
  const presets = usePresets().data ?? []
  const inbox = m.inbox_id ? inboxes.find((i) => i.id === m.inbox_id) : undefined
  const preset = presets.find((p) => p.id === m.preset)?.name ?? m.preset
  const creator = m.member_id ? members.get(m.member_id) : undefined
  const by = creator ? creator.name || creator.email : m.via
  const date = formatDate(m.created_at, i18n.locale)
  const c = m.counts
  const imported = num(c.imported)
  const skipped = num(c.skipped + c.duplicate)
  const failed = num(c.failed)
  const showBar = PROGRESS_STATES.includes(m.state) && c.total > 0
  const actions = useRowActions(m)
  return (
    <li className="flex flex-col gap-2.5 px-4 py-3" data-testid="migration-row" data-id={m.id}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <RowIcon>
          <MailIcon />
        </RowIcon>
        <div className="min-w-0 flex-1 basis-48">
          <p className="truncate text-body">{m.source_address}</p>
          <p className="mt-px truncate text-small text-faint">
            {preset}
            {inbox && (
              <>
                {" → "}
                {inbox.name}
              </>
            )}
            {" · "}
            {by ? (
              <Trans>
                {by}, {date}
              </Trans>
            ) : (
              date
            )}
          </p>
        </div>
        <MigrationStateTag m={m} />
        {actions.buttons}
      </div>
      <ErrorLine error={actions.error} className="text-small" />
      {actions.dialog}
      {showBar && <MigrationBar m={m} thin />}
      <WaitingLine m={m} />
      {m.state === "needs_attention" && m.state_reason && (
        <p className="text-small text-destructive">{m.state_reason}</p>
      )}
      {(c.imported > 0 || c.failed > 0 || c.skipped > 0) && (
        <p className="text-small text-faint" data-testid="migration-counts">
          <Trans>
            {imported} imported · {skipped} skipped · {failed} failed
          </Trans>
        </p>
      )}
    </li>
  )
}

function useRowActions(m: Migration) {
  const { t } = useLingui()
  const [confirm, dialog] = useConfirm()
  const ask = useMigrationConfirms(m)
  const pause = useMigrationStep(m.id, "pause")
  const resume = useMigrationStep(m.id, "resume")
  const retry = useMigrationStep(m.id, "retry")
  const del = useDeleteMigration(m.id)
  const can = (a: Migration["actions"][number]) => m.actions.includes(a)
  const error = pause.error ?? resume.error ?? retry.error ?? ask.error ?? del.error
  const reconnect = m.state === "needs_attention" && (can("set_credentials") || can("sign_in"))
  const early = ["draft", "connected", "scanning", "ready"].includes(m.state)
  const address = m.source_address
  const menu = [
    can("pause") && (
      <DropdownMenuItem key="pause" onClick={() => pause.mutate()} data-testid="migration-pause">
        <Trans>Pause</Trans>
      </DropdownMenuItem>
    ),
    can("resume") && !reconnect && (
      <DropdownMenuItem key="resume" onClick={() => resume.mutate()} data-testid="migration-resume">
        <Trans>Resume</Trans>
      </DropdownMenuItem>
    ),
    can("retry") && (
      <DropdownMenuItem key="retry" onClick={() => retry.mutate()} data-testid="migration-retry">
        <Trans>Retry failed items</Trans>
      </DropdownMenuItem>
    ),
    can("finish") && (
      <DropdownMenuItem key="finish" onClick={ask.finish} data-testid="migration-finish">
        <Trans>Finish</Trans>
      </DropdownMenuItem>
    ),
  ].filter(Boolean)
  const danger = [
    can("cancel") && (
      <DropdownMenuItem key="cancel" variant="destructive" onClick={ask.cancel} data-testid="migration-cancel">
        <Trans>Cancel migration</Trans>
      </DropdownMenuItem>
    ),
    can("remove_history") && (
      <DropdownMenuItem
        key="remove"
        variant="destructive"
        onClick={ask.removeHistory}
        data-testid="migration-remove-history"
      >
        <Trans>Remove imported history</Trans>
      </DropdownMenuItem>
    ),
    can("delete") && (
      <DropdownMenuItem
        key="delete"
        variant="destructive"
        onClick={() =>
          confirm({
            title: t`Delete this migration?`,
            description: t`Its settings, credentials and progress are deleted. Imported conversations stay.`,
            confirm: t`Delete`,
            run: () => del.mutate(),
          })
        }
        data-testid="migration-delete"
      >
        <Trans>Delete</Trans>
      </DropdownMenuItem>
    ),
  ].filter(Boolean)
  const buttons = (
    <div className="flex shrink-0 items-center gap-1.5">
      {reconnect ? (
        <Button size="sm" render={<Link to={`/migrations/${m.id}/connect`} />} data-testid="migration-reconnect">
          {m.preset === "microsoft" ? <Trans>Sign in again</Trans> : <Trans>Reconnect</Trans>}
        </Button>
      ) : (
        m.state !== "removed" && (
          <Button variant="outline" size="sm" render={<Link to={`/migrations/${m.id}`} />} data-testid="migration-open">
            {early ? <Trans>Continue</Trans> : <Trans>Open</Trans>}
          </Button>
        )
      )}
      {(menu.length > 0 || danger.length > 0) && (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon-sm" />}
            aria-label={t`More actions for ${address}`}
            data-testid="migration-menu"
          >
            <MoreHorizontalIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            {menu}
            {menu.length > 0 && danger.length > 0 && <DropdownMenuSeparator />}
            {danger}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  )
  return {
    buttons,
    error,
    dialog: (
      <>
        {dialog}
        {ask.dialog}
      </>
    ),
  }
}
