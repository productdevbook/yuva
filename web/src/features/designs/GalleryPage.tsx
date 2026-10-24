import "@/features/designs/i18n"

import { Trans, useLingui } from "@lingui/react/macro"
import { ArchiveIcon, LanguagesIcon, LayoutTemplateIcon, SearchIcon, TriangleAlertIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Link, useLocation, useNavigate, useSearchParams } from "react-router"

import { ErrorLine, Notice, Segmented, toast } from "@/components/common"
import { PaneEmpty } from "@/components/common/Column"
import { MailFrame } from "@/components/common/MailFrame"
import { DESIGN_KEYS } from "@/components/common/ShortcutSheet"
import { formatShort } from "@/components/common/text"
import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { keyHint } from "@/features/broadcasts/BroadcastParts"
import { DesignBulkBar } from "@/features/designs/Bulk"
import { NewDesignDialog, type NewDesign } from "@/features/designs/NewDesignDialog"
import {
  designLink,
  useDesign,
  useDesigns,
  useRender,
  useRestoreDesign,
  useShowArchivedDesigns,
  useTemplates,
} from "@/features/designs/queries"
import { useCategory, useTemplateLocales } from "@/features/designs/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { look } from "@/features/inbox/PageFrame"
import { FrameButton, FrameTabs, framePad, PageFrame } from "@/features/status/PageFrame"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection, type Selection } from "@/hooks/use-selection"
import { useRowFocus } from "@/hooks/use-row-focus"
import type { EmailDesignSummary, EmailTemplate, EmailTemplateLocale } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const LOCALES: EmailTemplateLocale[] = ["en", "tr", "de"]
const KINDS = ["newsletter", "product", "announcement", "onboarding", "event", "digest"]

const sheet = "relative h-[260px] w-[184px] overflow-hidden rounded-t-md bg-white shadow-[0_4px_14px_rgb(0_0_0/0.08)]"
const box =
  "relative flex h-[297px] justify-center overflow-hidden rounded-xl border bg-surface pt-[18px] transition-colors group-hover:border-faint"
const grid = "grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-x-4 gap-y-5"

function TemplateCard({ tpl, locale, onUse }: { tpl: EmailTemplate; locale: EmailTemplateLocale; onUse: () => void }) {
  const { t } = useLingui()
  const category = useCategory()
  const own = tpl.locales.includes(locale)
  const title = tpl.name
  const kind = category(tpl.category)
  return (
    <li>
      <button
        type="button"
        onClick={onUse}
        title={tpl.description}
        className="group flex w-full flex-col gap-2.5 rounded-xl text-start outline-none focus-visible:ring-2 focus-visible:ring-ring"
        data-testid="template-card"
        data-slug={tpl.slug}
      >
        <span className={box}>
          <span className={sheet}>
            <img
              src={tpl.preview_url}
              alt={t`Preview of ${title}`}
              loading="lazy"
              className="absolute inset-x-0 top-0 w-full transition-transform duration-500 group-hover:-translate-y-[6%]"
            />
          </span>
        </span>
        <span className="flex min-w-0 flex-col leading-snug">
          <span className="truncate text-body font-semibold">{tpl.name}</span>
          <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint">
            <span className="truncate">
              <Trans>Gallery · {kind}</Trans>
            </span>
            {!own && (
              <span className="inline-flex shrink-0 items-center gap-1">
                <LanguagesIcon className="size-3" />
                <Trans>Text in English</Trans>
              </span>
            )}
          </span>
        </span>
      </button>
    </li>
  )
}

const THUMB_WIDTH = 600
const THUMB_SCALE = 184 / 600

function DesignCard({ d, selection }: { d: EmailDesignSummary; selection: Selection }) {
  const { t, i18n } = useLingui()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === d.inbox_id)
  const design = useDesign(d.id)
  const restore = useRestoreDesign()
  const render = useRender(
    design.data ? { document: design.data.document, ...(d.inbox_id ? { inbox_id: d.inbox_id } : {}) } : null
  )
  const name = d.name
  const archived = !!d.archived_at
  return (
    <li className="relative flex flex-col gap-2">
      {selection.selecting && !archived && (
        <RowCheck
          className="absolute top-2.5 start-2.5 z-10"
          checked={selection.has(d.id)}
          name={name}
          onToggle={(range) => selection.toggle(d.id, range)}
        />
      )}
      <Link
        to={designLink(d.id)}
        className={cn(
          "group flex flex-col gap-2.5 rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring",
          archived && "opacity-60"
        )}
        data-testid="design-card"
      >
        <span className={box}>
          <span className={sheet}>
            {render.data ? (
              <span
                className="pointer-events-none absolute top-0 start-0 origin-top-left rtl:origin-top-right"
                style={{ width: THUMB_WIDTH, transform: `scale(${THUMB_SCALE})` }}
                aria-hidden
              >
                <MailFrame html={render.data.html} width={THUMB_WIDTH} title={t`Preview of ${name}`} />
              </span>
            ) : render.isError || design.isError ? (
              <span className="grid h-full place-items-center text-faint">
                <LayoutTemplateIcon className="size-6" />
              </span>
            ) : (
              <Skeleton className="h-full w-full rounded-none" />
            )}
          </span>
          {archived && (
            <span className="absolute top-2.5 end-2.5 inline-flex items-center gap-1 rounded-md bg-background/90 px-1.5 py-0.5 text-caption text-muted-foreground">
              <ArchiveIcon className="size-3" />
              <Trans>Archived</Trans>
            </span>
          )}
        </span>
        <span className="flex min-w-0 flex-col leading-snug">
          <span className="truncate text-body font-semibold">{d.name}</span>
          <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint">
            {inbox && (
              <>
                <span className="truncate">{inbox.name}</span>
                <span aria-hidden>·</span>
              </>
            )}
            <span className="shrink-0">{formatShort(d.updated_at, i18n.locale)}</span>
          </span>
        </span>
      </Link>
      {archived && (
        <div className="flex justify-end">
          <Button
            variant="outline"
            className={look.btnSm}
            disabled={restore.isPending}
            onClick={() => restore.mutate(d.id, { onSuccess: () => toast(t`Design restored`) })}
            data-testid="design-restore"
          >
            <Trans>Restore</Trans>
          </Button>
        </div>
      )}
      <ErrorLine error={restore.error} />
    </li>
  )
}

function Mine({ onNew }: { onNew: () => void }) {
  const { t } = useLingui()
  const [inboxId] = useInboxFilter()
  const [archived, setArchived] = useShowArchivedDesigns()
  const [q, setQ] = useState("")
  const query = useDebounced(q.trim(), 250)
  const live = useDesigns(inboxId, query, false)
  const gone = useDesigns(inboxId, query, true, archived)
  const lists = archived ? [live, gone] : [live]
  const items = lists
    .flatMap((l) => l.data?.pages.flatMap((p) => p.items) ?? [])
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
  const selection = useSelection(
    items.filter((d) => !d.archived_at).map((d) => d.id),
    `${inboxId}|${query}|${archived}`
  )
  const pending = lists.some((l) => l.isPending)
  const error = lists.find((l) => l.error)?.error
  const more = lists.filter((l) => l.hasNextPage)
  const list = useRowFocus<HTMLUListElement>()
  const search = useRef<HTMLInputElement>(null)
  useHotkeys({
    [DESIGN_KEYS.search]: () => search.current?.focus(),
    [DESIGN_KEYS.archived]: () => setArchived(!archived),
  })
  return (
    <>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="relative min-w-0 flex-1 basis-60 sm:max-w-[300px] sm:flex-none">
          <SearchIcon className="pointer-events-none absolute top-1/2 start-3 size-4 -translate-y-1/2 text-faint" />
          <Input
            ref={search}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && search.current?.blur()}
            placeholder={t`Search designs`}
            aria-label={t`Search designs`}
            title={keyHint(t`Search designs`, DESIGN_KEYS.search)}
            className="ps-9"
            data-testid="designs-search"
          />
        </div>
        <div className="ms-auto flex items-center gap-3">
          {items.some((d) => !d.archived_at) && !selection.selecting && <SelectToggle selection={selection} />}
          <Label
            className="gap-3 font-normal text-muted-foreground"
            title={keyHint(t`Show archived designs`, DESIGN_KEYS.archived)}
          >
            <Trans>Show archived designs</Trans>
            <Switch checked={archived} onCheckedChange={setArchived} data-testid="designs-archived" />
          </Label>
        </div>
      </div>
      <DesignBulkBar items={items} selection={selection} />
      <ErrorLine error={error} />
      {pending ? (
        <div className={grid}>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-[297px] rounded-xl" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div
          className="flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-12 text-center text-body text-muted-foreground"
          data-testid="designs-home-empty"
        >
          <LayoutTemplateIcon className="size-6 text-faint" />
          <p className="font-semibold text-foreground">
            {query ? <Trans>No designs match.</Trans> : <Trans>No designs yet</Trans>}
          </p>
          <p className="max-w-md">
            {query ? (
              <Trans>Try another word, or show archived designs.</Trans>
            ) : (
              <Trans>A design is the look of a mail: start from a template, then use it for your broadcasts.</Trans>
            )}
          </p>
          {!query && (
            <div className="mt-1 flex gap-2">
              <FrameButton render={<Link to="/broadcasts/designs/new" />}>
                <Trans>Browse the gallery</Trans>
              </FrameButton>
              <FrameButton ink onClick={onNew}>
                <Trans>New design</Trans>
              </FrameButton>
            </div>
          )}
        </div>
      ) : (
        <ul ref={list} className={grid} aria-label={t`Designs`} data-testid="design-grid">
          {items.map((d) => (
            <DesignCard key={d.id} d={d} selection={selection} />
          ))}
        </ul>
      )}
      {more.length > 0 && (
        <Button
          variant="ghost"
          size="sm"
          className="self-center"
          onClick={() => more.forEach((l) => void l.fetchNextPage())}
          disabled={more.some((l) => l.isFetchingNextPage)}
        >
          <Trans>Load more</Trans>
        </Button>
      )}
      <p className="text-small text-faint">
        <Trans>Owners and admins make e-mail designs and send broadcasts. Archiving keeps the versions.</Trans>
      </p>
    </>
  )
}

function Gallery({ locale, onUse }: { locale: EmailTemplateLocale; onUse: (tpl: EmailTemplate) => void }) {
  const { t } = useLingui()
  const category = useCategory()
  const templates = useTemplates(locale)
  const [kind, setKind] = useState("all")
  const all = templates.data ?? []
  const kinds = [
    ...KINDS.filter((k) => all.some((x) => x.category === k)),
    ...new Set(all.map((x) => x.category).filter((c) => !KINDS.includes(c))),
  ]
  const shown = kind === "all" ? all : all.filter((x) => x.category === kind)
  const english = shown.some((x) => !x.locales.includes(locale))
  return (
    <>
      <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t`Kind`}>
        {["all", ...kinds].map((k) => (
          <button
            key={k}
            type="button"
            aria-pressed={kind === k}
            onClick={() => setKind(k)}
            className={kind === k ? look.chipOn : look.chip}
            data-testid={`template-kind-${k}`}
          >
            {k === "all" ? <Trans>All</Trans> : category(k)}
          </button>
        ))}
        {english && (
          <span className="ms-auto inline-flex items-center gap-1.5 text-small text-faint">
            <LanguagesIcon className="size-3.5" />
            <Trans>Text in English</Trans>
          </span>
        )}
      </div>
      <ErrorLine error={templates.error} />
      {!templates.isPending && shown.length === 0 && !templates.error ? (
        <div
          className="flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-12 text-center text-body text-muted-foreground"
          data-testid="templates-empty"
        >
          <p className="font-semibold text-foreground">
            <Trans>No templates of this kind</Trans>
          </p>
          <p>
            <Trans>Try another kind.</Trans>
          </p>
        </div>
      ) : (
        <ul className={grid} data-testid="template-grid">
          {templates.isPending
            ? Array.from({ length: 6 }, (_, i) => (
                <li key={i}>
                  <Skeleton className="h-[297px] w-full rounded-xl" />
                </li>
              ))
            : shown.map((tpl) => <TemplateCard key={tpl.slug} tpl={tpl} locale={locale} onUse={() => onUse(tpl)} />)}
        </ul>
      )}
      <p className="text-small text-faint">
        <Trans>Templates are copied into your workspace, images included. You can change everything after.</Trans>
      </p>
    </>
  )
}

export default function GalleryPage() {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const localeNames = useTemplateLocales()
  const wanted = params.get("locale") as EmailTemplateLocale | null
  const fallback: EmailTemplateLocale = LOCALES.includes(i18n.locale as EmailTemplateLocale)
    ? (i18n.locale as EmailTemplateLocale)
    : "en"
  const [locale, setLocale] = useState<EmailTemplateLocale>(wanted && LOCALES.includes(wanted) ? wanted : fallback)
  const templates = useTemplates(locale)
  const [choice, setChoice] = useState<NewDesign | null>(null)
  const slug = params.get("template")
  const asked = slug ? templates.data?.find((x) => x.slug === slug) : undefined
  const unknown = !!slug && !!templates.data && !asked
  const gallery = useLocation().pathname.endsWith("/new")
  useHotkeys(
    {
      [DESIGN_KEYS.create]: () => setChoice("choose"),
      [DESIGN_KEYS.mine]: () => navigate("/broadcasts/designs"),
      [DESIGN_KEYS.gallery]: () => navigate("/broadcasts/designs/new"),
    },
    canManage
  )
  if (!canManage)
    return (
      <PaneEmpty icon={LayoutTemplateIcon} title={<Trans>E-mail designs</Trans>} testId="designs-read-only">
        <Trans>Owners and admins make e-mail designs and send broadcasts.</Trans>
      </PaneEmpty>
    )
  const open: NewDesign | null = choice ?? (asked ? { use: asked } : null)
  const close = () => {
    setChoice(null)
    if (slug) setParams({}, { replace: true })
  }
  const tabs = [
    { key: "mine", to: "/broadcasts/designs", label: t`Mine`, testId: "designs-tab-mine" },
    { key: "gallery", to: "/broadcasts/designs/new", label: t`Gallery`, testId: "designs-tab-gallery" },
  ]
  return (
    <PageFrame
      testId="design-gallery"
      title={<Trans>Designs</Trans>}
      description={<Trans>E-mail layouts you reuse across broadcasts</Trans>}
      actions={
        <>
          {gallery && (
            <Segmented
              value={locale}
              onChange={setLocale}
              label={t`Template language`}
              items={localeNames.map((l) => ({ ...l, testId: `template-locale-${l.value}` }))}
            />
          )}
          <FrameButton
            ink
            onClick={() => setChoice("choose")}
            title={keyHint(t`New design`, DESIGN_KEYS.create)}
            data-testid="designs-home-new"
          >
            <Trans>New design</Trans>
          </FrameButton>
        </>
      }
      tabs={<FrameTabs label={t`Designs`} items={tabs} current={gallery ? "gallery" : "mine"} />}
    >
      <div className={framePad}>
        {unknown && (
          <Notice tone="warning" icon={TriangleAlertIcon} data-testid="template-unknown">
            <p>
              <Trans>This server does not have the template “{slug}”. It may need a newer Yuva.</Trans>
            </p>
          </Notice>
        )}
        {gallery ? (
          <Gallery locale={locale} onUse={(tpl) => setChoice({ use: tpl })} />
        ) : (
          <Mine onNew={() => setChoice("choose")} />
        )}
      </div>
      {open && (
        <NewDesignDialog
          key={open === "choose" ? "choose" : open.use.slug}
          mode={open}
          locale={locale}
          onClose={close}
        />
      )}
    </PageFrame>
  )
}
