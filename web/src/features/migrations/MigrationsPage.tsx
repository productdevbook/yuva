import { Trans, useLingui } from "@lingui/react/macro"
import { BookOpenIcon, CheckIcon, InboxIcon, MailIcon, MoreHorizontalIcon, PlusIcon, XIcon } from "lucide-react"
import { Link, Navigate, useNavigate, useSearchParams } from "react-router"

import { ErrorLine, Notice, TopPortal, useConfirm } from "@/components/common"
import { boxRowClass, FilterMenu, FilterRow, ListBox, ListBoxEmpty, ListBoxSkeleton } from "@/components/common/ListBox"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import {
  MigrationBar,
  MigrationStateTag,
  useMigrationConfirms,
  useStateAnnouncement,
  WaitingLine,
} from "@/features/migrations/parts"
import { useDeleteMigration, useMigrations, useMigrationStep, usePresets } from "@/features/migrations/queries"
import { guideUrl, isHelpdesk } from "@/features/migrations/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { GeneralHeader } from "@/features/settings/GeneralTabs"
import type { Migration } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMemberMap } from "@/lib/workspace"

const PROGRESS_STATES = ["copying", "paused", "needs_attention", "syncing"]
const FINISHED_STATES = ["completed", "cancelled", "removed"]

type Show = "active" | "finished"

export function MigrationsPage() {
  const { t } = useLingui()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const list = useMigrations()
  const [filter] = useInboxFilter()
  const [params, setParams] = useSearchParams()
  const show: Show | "" =
    params.get("show") === "active" || params.get("show") === "finished" ? (params.get("show") as Show) : ""
  const all = (list.data?.pages.flatMap((p) => p.items) ?? []).filter((m) => !filter || m.inbox_id === filter)
  const finished = (m: Migration) => FINISHED_STATES.includes(m.state)
  const items = all.filter((m) => !show || (show === "finished") === finished(m))
  const announcement = useStateAnnouncement(all)
  const num = useNumber()
  if (!canManage) return <Navigate to="/settings/profile" replace />
  const start = () => navigate(filter ? `/migrations/new?inbox=${filter}` : "/migrations/new")
  const count = (n: number) => (list.data ? ` · ${num(n)}` : "")
  return (
    <>
      <GeneralHeader current="migrations" plain />
      <TopPortal>
        <Button onClick={start} data-testid="migration-new">
          <PlusIcon />
          <Trans>New import</Trans>
        </Button>
      </TopPortal>
      <p className="m-0 max-w-[680px] text-body text-muted-foreground">
        <Trans>
          Bring the history of a mailbox or help desk you answer from today, then switch its mail over. Yuva only reads
          the source; nothing there is changed.
        </Trans>
      </p>
      {announcement}
      {params.get("result") === "expired" && (
        <Notice tone="danger" icon={XIcon} role="alert" data-testid="migration-sign-in-result">
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
        <Notice icon={CheckIcon} role="status" data-testid="migration-admin-consented">
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
      {all.length > 0 && (
        <FilterRow testId="migrations-filters">
          <FilterMenu
            value={show}
            onChange={(v) =>
              setParams(
                (p) => {
                  if (v) p.set("show", v)
                  else p.delete("show")
                  return p
                },
                { replace: true }
              )
            }
            options={[
              { value: "active", label: t`Active` + count(all.filter((m) => !finished(m)).length) },
              { value: "finished", label: t`Finished` + count(all.filter(finished).length) },
            ]}
            anyLabel={t`All imports` + count(all.length)}
            label={t`Show`}
            testId="migrations-show"
          />
        </FilterRow>
      )}
      <ListBox testId="migrations-list">
        {list.isPending ? (
          <ListBoxSkeleton rows={3} />
        ) : all.length === 0 && !list.error ? (
          <Empty onStart={start} />
        ) : items.length === 0 && !list.error ? (
          <ListBoxEmpty testId="migrations-none">
            <Trans>No imports match this filter.</Trans>
          </ListBoxEmpty>
        ) : (
          items.map((m) => <MigrationRow key={m.id} m={m} />)
        )}
        <ErrorLine error={list.error} className="px-4 py-3" />
        {list.hasNextPage && (
          <div data-row className="px-3 py-2.5">
            <Button variant="ghost" size="sm" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
              <Trans>Show more</Trans>
            </Button>
          </div>
        )}
      </ListBox>
    </>
  )
}

function Empty({ onStart }: { onStart: () => void }) {
  return (
    <ListBoxEmpty
      testId="migrations-empty"
      action={
        <div className="flex flex-wrap justify-center gap-2">
          <Button size="sm" onClick={onStart}>
            <Trans>Start an import</Trans>
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
      }
    >
      <Trans>
        Moving from Gmail, Google Workspace, Yandex, iCloud, Zoho, Yahoo, Fastmail, GMX, any IMAP server or another help
        desk? Yuva copies the history into an inbox, newest first, while your team keeps working.
      </Trans>
    </ListBoxEmpty>
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
    <div
      data-row
      className={cn(boxRowClass({}), "flex-col gap-2 py-3 pe-3.5")}
      data-testid="migration-row"
      data-id={m.id}
    >
      <div className="flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg border bg-surface text-muted-foreground [&_svg]:size-4">
          {isHelpdesk(m.source) ? <InboxIcon /> : <MailIcon />}
        </span>
        <div className="min-w-0 flex-1 basis-48">
          <Link
            to={`/migrations/${m.id}`}
            className="block truncate text-body font-semibold outline-none hover:underline focus-visible:underline"
          >
            {m.source_address}
          </Link>
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
      {showBar && (
        <div className="w-full">
          <MigrationBar m={m} thin />
        </div>
      )}
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
    </div>
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
