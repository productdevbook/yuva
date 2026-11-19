import { Trans, useLingui } from "@lingui/react/macro"
import { MailWarningIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router"

import { Dot, ErrorLine, InboxMark, InfoSection, KV, Notice, toast } from "@/components/common"
import { formatShort } from "@/components/common/text"
import { LIST_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { keyHint } from "@/features/broadcasts/BroadcastParts"
import { backOf, DetailFrame, TopTrail, useFlat, useViewNames } from "@/features/broadcasts/BroadcastsFrame"
import { FormSnippets } from "@/features/broadcasts/FormSnippets"
import { Growth } from "@/features/broadcasts/Growth"
import { ListImports } from "@/features/broadcasts/Import"
import { ListExport, ListSettings } from "@/features/broadcasts/ListSettings"
import { listLink, useArchiveList, useList, useSender } from "@/features/broadcasts/queries"
import { Subscribers } from "@/features/broadcasts/Subscribers"
import { Chip } from "@/features/settings/ui"
import { FrameButton, FrameTabs } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { MailingList } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const TABS = ["subscribers", "import", "growth", "form", "settings"] as const
type Tab = (typeof TABS)[number]

export { ListsOverview as ListsHome } from "@/features/broadcasts/ListsHome"

export function Tiles({ list, flat }: { list: MailingList; flat?: boolean }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const tiles: [string, string, string?][] = [
    [fmt.format(list.counts.subscribed), t`subscribed now`, "text-success"],
    [fmt.format(list.counts.pending), t`waiting for confirmation`, list.counts.pending ? "text-warning" : undefined],
    [fmt.format(list.counts.unsubscribed), t`unsubscribed now`],
  ]
  return (
    <div
      className={cn("grid grid-cols-3", flat ? "gap-6 border-y border-line-soft py-3" : "gap-2")}
      data-testid="list-tiles"
    >
      {tiles.map(([value, label, tone]) => (
        <div key={label} className={cn("min-w-0", !flat && "rounded-2xl border bg-card px-4 py-3.5 phone:px-3")}>
          <b className={cn("block text-page tabular-nums", tone)}>{value}</b>
          <span className="block text-small break-words text-faint">{label}</span>
        </div>
      ))}
    </div>
  )
}

function ExportDialog({ list, onClose }: { list: MailingList; onClose: () => void }) {
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl" data-testid="list-export-dialog">
        <DialogHeader>
          <DialogTitle className="sr-only">
            <Trans>Consent export</Trans>
          </DialogTitle>
        </DialogHeader>
        <ListExport list={list} />
      </DialogContent>
    </Dialog>
  )
}

function ListInfo({ list, from }: { list: MailingList; from?: string }) {
  const { t, i18n } = useLingui()
  return (
    <InfoSection title={t`List`} testId="list-info">
      <KV label={t`Sender`} value={from || t`No sender yet`} tone={list.sender_ready ? undefined : "warn"} />
      <KV label={t`Public`} value={list.public ? t`Yes` : t`No`} />
      <KV label={t`Double opt-in`} value={list.double_opt_in ? t`Yes` : t`No`} />
      <KV label={t`Created`} value={formatShort(list.created_at, i18n.locale)} />
    </InfoSection>
  )
}

export function ListPage() {
  const { t } = useLingui()
  const views = useViewNames()
  const flat = useFlat()
  const navigate = useNavigate()
  const { listId = "" } = useParams()
  const [params] = useSearchParams()
  const list = useList(listId)
  const { canManage } = useSession()
  const archive = useArchiveList(listId)
  const [exporting, setExporting] = useState(false)
  const l = list.data
  const inbox = (useInboxes().data ?? []).find((i) => i.id === l?.inbox_id)
  const from = useSender(l?.inbox_id ?? "", l?.channel_id)
  const raw = params.get("tab")
  const tab: Tab = TABS.includes(raw as Tab) ? (raw as Tab) : "subscribers"
  const go = (v: Tab) => navigate(listLink(listId, v === "subscribers" ? undefined : v), { replace: true })
  useHotkeys(
    {
      [LIST_KEYS.subscribers]: () => go("subscribers"),
      [LIST_KEYS.importTab]: () => go("import"),
      [LIST_KEYS.growth]: () => go("growth"),
      [LIST_KEYS.form]: () => go("form"),
      [LIST_KEYS.settings]: () => go("settings"),
      ...(canManage ? { [LIST_KEYS.export]: () => setExporting(true) } : {}),
    },
    !!l && !exporting
  )
  const names: Record<Tab, string> = {
    subscribers: t`Subscribers`,
    import: t`Import`,
    growth: t`Growth`,
    form: t`Form`,
    settings: t`Settings`,
  }
  if (!l) {
    return (
      <DetailFrame
        testId="list-loading"

        back={backOf("lists", views)}
        bar={{
          back: "/broadcasts/lists",
          label: t`Lists`,
          content: list.isPending ? <Skeleton className="h-7 w-60" /> : <ErrorLine error={list.error} />,
        }}
        heading={list.isPending ? <Skeleton className="h-8 w-2/3" /> : <ErrorLine error={list.error} />}
        eyebrow={<TopTrail place="lists" />}
      >
        {list.isPending && <Skeleton className="h-24 w-full rounded-2xl" />}
      </DetailFrame>
    )
  }
  const archived = !!l.archived_at
  const tabs = TABS.map((v) => ({
    key: v,
    to: listLink(l.id, v === "subscribers" ? undefined : v),
    label: names[v],
    testId: `list-tab-${v}`,
  }))
  const chips = (
    <>
      <Chip>{l.public ? <Trans>Public</Trans> : <Trans>Private</Trans>}</Chip>
      <Chip>{l.double_opt_in ? <Trans>Confirmation asked</Trans> : <Trans>No confirmation</Trans>}</Chip>
      {archived && (
        <Chip>
          <Trans>Archived</Trans>
        </Chip>
      )}
    </>
  )
  const senderLine = (
    <>
      {l.sender_ready && from ? (
        <Trans>From {from}</Trans>
      ) : (
        <Link to={listLink(l.id, "settings")} className="text-warning underline-offset-4 hover:underline">
          <Trans>Pick a sender in the settings</Trans>
        </Link>
      )}
      {l.description && ` · ${l.description}`}
    </>
  )
  const exportButton = canManage && (
    <FrameButton
      onClick={() => setExporting(true)}
      title={keyHint(t`Export`, LIST_KEYS.export)}
      data-testid="list-export-open"
    >
      <Trans>Export</Trans>
    </FrameButton>
  )
  return (
    <DetailFrame
      testId="list-page"

      wide
      back={backOf("lists", views)}
      bar={{
        back: "/broadcasts/lists",
        label: t`Lists`,
        content: (
          <>
            {inbox && <InboxMark name={inbox.name} color={inbox.branding.color} className="size-7 text-small" />}
            <div className="flex min-w-48 flex-1 basis-64 flex-col leading-snug">
              <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                <h1 className="min-w-0 truncate text-[17px] font-semibold" data-testid="list-name-heading">
                  {l.name}
                </h1>
                {chips}
              </span>
              <span className="truncate text-small text-muted-foreground">{senderLine}</span>
            </div>
            {exportButton}
          </>
        ),
      }}
      heading={
        <header className="flex flex-col">
          <div className="flex min-w-0 flex-wrap items-center gap-2 text-small text-muted-foreground">
            {inbox && (
              <>
                <Dot color={inbox.branding.color} className="size-[7px] rounded-full" />
                <span className="truncate">{inbox.name}</span>
              </>
            )}
            {chips}
          </div>
          <h1
            className="m-0 mt-2.5 text-page leading-[34px] text-balance break-words"
            dir="auto"
            data-testid="list-name-heading"
          >
            {l.name}
          </h1>
          <p className="m-0 mt-1.5 text-reading text-muted-foreground">{senderLine}</p>
        </header>
      }
      eyebrow={<TopTrail place="lists" />}
      actions={exportButton}
      aside={<ListInfo list={l} from={from} />}
    >
      {archived && (
        <Notice icon={MailWarningIcon} data-testid="list-archived-banner">
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-48 flex-1">
              <Trans>This list is archived. It takes no new subscribers and broadcasts cannot pick it.</Trans>
            </p>
            {canManage && (
              <Button
                variant="outline"
                size="sm"
                disabled={archive.isPending}
                onClick={() => archive.mutate(false, { onSuccess: () => toast(t`List restored`) })}
                data-testid="list-restore"
              >
                <Trans>Restore</Trans>
              </Button>
            )}
          </div>
          <ErrorLine error={archive.error} />
        </Notice>
      )}
      {!l.sender_ready && !archived && (
        <Notice tone="warning" icon={MailWarningIcon} data-testid="list-needs-sender">
          <p>
            <Trans>
              This list has no e-mail channel that can send, so forms send no confirmation, new form subscribers keep
              waiting and nobody gets a welcome mail.
            </Trans>
          </p>
          {tab !== "settings" && (
            <Link to={listLink(l.id, "settings")} className="w-fit text-brand underline-offset-4 hover:underline">
              <Trans>Pick a sender in the settings</Trans>
            </Link>
          )}
        </Notice>
      )}
      <Tiles list={l} flat={flat} />
      <div className="border-b">
        <FrameTabs label={t`List`} items={tabs} current={tab} />
      </div>
      {tab === "subscribers" && <Subscribers list={l} />}
      {tab === "import" && <ListImports list={l} />}
      {tab === "growth" && <Growth list={l} />}
      {tab === "form" && <FormSnippets list={l} />}
      {tab === "settings" && <ListSettings list={l} />}
      {exporting && <ExportDialog list={l} onClose={() => setExporting(false)} />}
    </DetailFrame>
  )
}
