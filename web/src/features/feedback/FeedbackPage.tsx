import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, CopyIcon } from "lucide-react"
import { lazy, Suspense, useCallback, useLayoutEffect, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import {
  ContactAvatar,
  Dot,
  ErrorLine,
  InfoColumn,
  InfoSection,
  isVisitor,
  KV,
  NavGroup,
  NavList,
  PageHeading,
  PageLayout,
  PageTop,
  SectionHeading,
  toast,
  TopPortal,
  TopSep,
  TopSlotProvider,
  useCopy,
  VisitorAvatar,
} from "@/components/common"
import { useFeedbackCategories } from "@/components/common/feedbackCategories"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDateTime, useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useContact } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useConversationActions } from "@/features/conversation/actions"
import { ChatView } from "@/features/conversation/ChatView"
import { useFeedbackRows } from "@/features/conversation/Feedback"
import { isPreviewable } from "@/features/conversation/messages/Attachments"
import { useConversation, useMessages, useUpdateConversation } from "@/features/conversation/queries"
import {
  CategoryPill,
  feedbackPlace,
  FeedbackFlatList,
  progressTone,
  rememberedFeedbackCategory,
  rememberedFeedbackTab,
  rememberFeedback,
  useFeedbackTypes,
  type FeedbackCategoryFilter,
  type FeedbackTab,
} from "@/features/feedback/FeedbackList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { ConversationEyebrow } from "@/features/inbox/InboxSide"
import { useCounts, useLoadedCount } from "@/features/inbox/queries"
import { Lightbox } from "@/features/feedback/Lightbox"
import { IssuesCard } from "@/features/feedback/IssuesCard"
import { RoadmapCard } from "@/features/feedback/RoadmapCard"
import { feedbackDraft } from "@/features/issues/draft"
import { IssueDialogs, type IssueMode } from "@/features/issues/IssueMenu"
import { ideaLink, useBoards, useScopeIdeas } from "@/features/roadmap/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { attachmentUrl, type Conversation, type Feedback, type FeedbackProgress } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const LinkIdeaDialog = lazy(() => import("@/features/roadmap/LinkIdeaDialog"))
const FeedbackFormDialog = lazy(() => import("@/features/feedback/FeedbackFormDialog"))

const STEPS = ["none", "planned", "in_progress", "done", "wont_do"] as const

function ProgressPicker({ c }: { c: Conversation }) {
  const { t } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const updater = useUpdateConversation(c.id)
  const current = c.feedback?.progress ?? "none"
  const set = (v: (typeof STEPS)[number]) =>
    updater.mutate(
      { feedback_progress: v === "none" ? null : (v as FeedbackProgress) },
      { onError: (e) => toast(errorText(e)) }
    )
  return (
    <div
      role="radiogroup"
      aria-label={t`Progress`}
      className="flex flex-wrap rounded-[10px] border bg-surface p-[3px]"
      data-testid="progress-strip"
    >
      {STEPS.map((p) => {
        const on = current === p
        return (
          <button
            key={p}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => set(p)}
            className={cn(
              "h-[30px] flex-auto rounded-[7px] px-2 text-[13px] whitespace-nowrap outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
              on
                ? cn(
                    "border-0 bg-background font-semibold shadow-[0_1px_3px_rgba(0,0,0,0.1)]",
                    p === "none" ? "text-foreground" : progressTone[p]
                  )
                : "text-muted-foreground hover:text-foreground"
            )}
            data-testid={`progress-step-${p}`}
          >
            {p === "none" ? t`None` : text.progress[p]}
          </button>
        )
      })}
    </div>
  )
}

function Shots({ c }: { c: Conversation }) {
  const { workspaceId } = useSession()
  const messages = useMessages(c.id)
  const [open, setOpen] = useState<number | null>(null)
  const first = (messages.data?.pages ?? [])
    .flatMap((p) => p.items)
    .filter((m) => m.kind === "message" && m.direction !== "out")
    .at(-1)
  const items = (first?.attachments ?? []).filter(isPreviewable)
  if (items.length === 0) return null
  const shots = items.map((a) => ({ id: a.id, url: attachmentUrl(a, workspaceId), name: a.filename }))
  return (
    <div className="flex flex-wrap gap-2" data-testid="attachments">
      {shots.map((a, i) => (
        <button
          key={a.id}
          type="button"
          onClick={() => setOpen(i)}
          className="block h-32 w-[72px] overflow-hidden rounded-[10px] border bg-surface outline-none focus-visible:ring-2 focus-visible:ring-ring"
          title={a.name}
          data-testid="attachment"
        >
          <img src={a.url} alt={a.name} loading="lazy" className="size-full object-cover" />
        </button>
      ))}
      {open !== null && <Lightbox shots={shots} index={open} onIndex={setOpen} onClose={() => setOpen(null)} />}
    </div>
  )
}

function useCopyDetails(c: Conversation, f: Feedback) {
  const { t } = useLingui()
  const { find } = useFeedbackCategories()
  const rows = useFeedbackRows(f)
  const [copied, copy] = useCopy()
  const copyAll = () =>
    copy([`${t`Category`}: ${find(f.category, c.inbox_id).name}`, ...rows.map(([k, v]) => `${k}: ${v}`)].join("\n"))
  useHotkeys({ [SHORTCUTS.copyDetails]: () => void copyAll() })
  return { rows, copied, copyAll }
}

function Meta({ c, f }: { c: Conversation; f: Feedback }) {
  const { t } = useLingui()
  const { rows, copied, copyAll } = useCopyDetails(c, f)
  return (
    <section data-testid="feedback-details">
      <SectionHeading
        title={<Trans>Device and app</Trans>}
        more={
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="text-muted-foreground"
            onClick={() => void copyAll()}
            aria-label={t`Copy the feedback details`}
            title={t`Copy the feedback details (Y)`}
            data-testid="copy-feedback"
          >
            {copied ? <CheckIcon /> : <CopyIcon />}
          </Button>
        }
      />
      <dl className="m-0 grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-x-6 border-t border-line-soft">
        {rows.map(([k, v, mono]) => (
          <div key={k} className="flex min-w-0 justify-between gap-3 border-b border-line-soft py-2 text-body">
            <dt className="shrink-0 text-muted-foreground">{k}</dt>
            <dd className={cn("m-0 min-w-0 truncate text-end", mono && "font-mono text-xs leading-5")} title={v}>
              {v}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

function Actions({
  c,
  name,
  onReply,
  onIdea,
  onIssue,
  progress,
}: {
  c: Conversation
  name: string
  onReply: () => void
  onIdea: () => void
  onIssue: (mode: IssueMode) => void
  progress: React.RefObject<HTMLElement | null>
}) {
  const { t } = useLingui()
  const actions = useConversationActions(c, name)
  const errorText = useErrorText()
  const updater = useUpdateConversation(c.id)
  const closed = c.status === "closed"
  const resolve = () =>
    updater.mutate(
      { status: "closed" },
      {
        onSuccess: () => toast(t`Resolved. It moved to the Resolved tab.`, () => updater.mutate({ status: "open" })),
        onError: (e) => toast(errorText(e)),
      }
    )
  const toggle = closed ? actions.reopen : resolve
  useHotkeys({
    [SHORTCUTS.reply]: onReply,
    [SHORTCUTS.close]: toggle,
    [SHORTCUTS.progress]: () => progress.current?.querySelector<HTMLElement>("[aria-checked=true]")?.focus(),
    [SHORTCUTS.idea]: onIdea,
    [SHORTCUTS.createIssue]: () => onIssue("create"),
    [SHORTCUTS.linkIssue]: () => onIssue("link"),
  })
  return (
    <TopPortal>
      <Button
        variant="outline"
        className="h-8 rounded-[9px] border-input bg-card px-3 font-normal"
        onClick={toggle}
        title={closed ? undefined : t`Resolve (E)`}
        data-testid={closed ? "reopen-conversation" : "close-conversation"}
      >
        {closed ? <Trans>Reopen</Trans> : <Trans>Resolve</Trans>}
      </Button>
      <Button
        className="h-8 rounded-[9px] bg-foreground px-3 text-background hover:bg-foreground/90"
        onClick={onReply}
        title={t`Reply (R)`}
        data-testid="feedback-reply"
      >
        <Trans>Reply</Trans>
      </Button>
    </TopPortal>
  )
}

function Detail({ c, onReply }: { c: Conversation; onReply: () => void }) {
  const { t, i18n } = useLingui()
  const contact = useContact(c.contact_id).data
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const [ideaLinking, setIdeaLinking] = useState<"link" | "new" | null>(null)
  const [issueMode, setIssueMode] = useState<IssueMode | null>(null)
  const progress = useRef<HTMLElement>(null)
  const f = c.feedback
  const name = contact ? contactName(contact, t`Unnamed contact`) : c.contact_id ? "…" : ""
  const raw = (f?.text || c.subject).trim()
  const [title, ...rest] = raw.split("\n")
  const body = rest.join("\n").trim()
  const where = feedbackPlace(f)
  return (
    <article className="max-w-[720px]" data-testid="feedback-detail" data-conversation-id={c.id}>
      <div className="mt-3 flex min-w-0 flex-wrap items-center gap-2 text-small text-muted-foreground">
        {f && <CategoryPill category={f.category} inboxId={c.inbox_id} />}
        {inbox && (
          <>
            <Dot color={inbox.branding.color} className="size-[7px] rounded-full" />
            <span className="truncate">{inbox.name}</span>
          </>
        )}
        {where && (
          <>
            <span aria-hidden>·</span>
            <span className="min-w-0 truncate">{where}</span>
          </>
        )}
      </div>
      <h1 className="m-0 mt-2.5 text-page leading-[34px] text-balance break-words" dir="auto">
        {title}
      </h1>
      <p className="m-0 mt-1.5 text-reading text-muted-foreground">
        {contact && (
          <>
            <Link
              to={`/contacts/${contact.id}`}
              className="rounded-sm text-fg2 underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
            >
              {name}
            </Link>
            {" · "}
          </>
        )}
        <time dateTime={c.created_at}>{formatDateTime(c.created_at, i18n.locale)}</time>
      </p>
      {body && (
        <p className="m-0 mt-6 text-base leading-7 whitespace-pre-line text-fg2" dir="auto">
          {body}
        </p>
      )}
      <div className="mt-4 empty:hidden">
        <Shots c={c} />
      </div>
      <section ref={progress}>
        <SectionHeading
          title={<Trans>Progress</Trans>}
          lead={
            c.idea ? (
              <Trans>
                Moving the idea to Planned, In progress or Shipped sets this for linked feedback that has no progress
                yet.
              </Trans>
            ) : (
              <Trans>Progress · the person sees this</Trans>
            )
          }
        />
        <ProgressPicker c={c} />
      </section>
      <RoadmapCard c={c} onLink={() => setIdeaLinking("link")} onNew={() => setIdeaLinking("new")} />
      <IssuesCard c={c} onCreate={() => setIssueMode("create")} onLink={() => setIssueMode("link")} />
      {f && <Meta c={c} f={f} />}
      <Actions
        c={c}
        name={name}
        onReply={onReply}
        onIdea={() => setIdeaLinking("link")}
        onIssue={setIssueMode}
        progress={progress}
      />
      <IssueDialogs
        subject={{ kind: "conversation", id: c.id }}
        mode={issueMode}
        draft={feedbackDraft(c)}
        onClose={() => setIssueMode(null)}
      />
      {ideaLinking && (
        <Suspense fallback={null}>
          <LinkIdeaDialog c={c} startNew={ideaLinking === "new"} onClose={() => setIdeaLinking(null)} />
        </Suspense>
      )}
    </article>
  )
}

function DetailInfo({ c }: { c: Conversation }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const contact = useContact(c.contact_id).data
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const name = contact ? contactName(contact, t`Unnamed contact`) : ""
  const visitor = !!contact && isVisitor(contact.name, contact.emails[0])
  const progress = c.feedback?.progress
  return (
    <>
      {contact && (
        <Link
          to={`/contacts/${contact.id}`}
          className="mb-4 flex min-w-0 items-center gap-2.5 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="feedback-contact"
        >
          {visitor ? (
            <VisitorAvatar className="size-9" />
          ) : (
            <ContactAvatar id={contact.id} name={name} className="size-9 text-small font-semibold" />
          )}
          <span className="flex min-w-0 flex-col">
            <span className="truncate text-reading font-semibold">{name}</span>
            {contact.emails[0] && contact.emails[0] !== name && (
              <span className="truncate font-mono text-xs text-muted-foreground">{contact.emails[0]}</span>
            )}
          </span>
        </Link>
      )}
      <InfoSection title={t`Feedback`} testId="feedback-info">
        <KV label={t`Status`} value={text.statusOf(c.status, c.kind)} />
        <KV label={t`Progress`} value={progress ? text.progress[progress] : t`None`} />
        {inbox && <KV label={t`Product`} value={inbox.name} />}
        <KV label={t`Started`} value={formatDateTime(c.created_at, i18n.locale)} />
      </InfoSection>
      {contact?.activity && (
        <InfoSection title={t`From this person`} testId="feedback-person">
          <KV
            label={t`Conversations`}
            value={fmt.format(contact.activity.conversations)}
            to={`/contacts/${contact.id}`}
          />
          <KV
            label={t`Open`}
            value={fmt.format(contact.activity.open_conversations)}
            tone={contact.activity.open_conversations > 0 ? "brand" : undefined}
          />
        </InfoSection>
      )}
    </>
  )
}

function ListInfo({ title, tab }: { title: string; tab: FeedbackTab }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const [inboxId] = useInboxFilter()
  const counts = useCounts().data
  const resolved = useLoadedCount({ kind: ["feedback"], status: ["closed"], inbox_id: inboxId || undefined })
  const { types, open } = useFeedbackTypes(inboxId, "all")
  const boards = useBoards()
  const ideas = (useScopeIdeas(undefined, { state: "published" }).data?.pages[0]?.items ?? [])
    .filter((i) => i.stage !== "shipped" && i.stage !== "closed" && (!inboxId || i.inbox_id === inboxId))
    .slice(0, 4)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const byType = types.filter((x) => x.key !== "all" && open(x.key) > 0)
  return (
    <>
      <InfoSection title={title} testId="feedback-summary">
        {!inboxId && (
          <KV
            label={t`Open`}
            value={fmt.format(counts?.feedback ?? 0)}
            tone={tab === "open" && counts?.feedback ? "brand" : undefined}
          />
        )}
        <KV label={t`Resolved`} value={resolved === undefined ? "–" : fmt.format(resolved)} />
      </InfoSection>
      {!inboxId && byType.length > 0 && (
        <InfoSection title={t`Type`}>
          {byType.map((x) => (
            <KV key={x.key} label={x.name} value={fmt.format(open(x.key))} />
          ))}
        </InfoSection>
      )}
      {(boards.data?.length ?? 0) > 0 && ideas.length > 0 && (
        <InfoSection
          title={t`Most asked for`}
          action={
            <Link
              to="/board"
              className="rounded-sm font-normal text-fg2 underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Trans>Roadmap</Trans>
            </Link>
          }
          testId="feedback-asks"
        >
          {ideas.map((i) => (
            <KV
              key={i.id}
              label={<span title={i.stage && text.stage[i.stage]}>{i.title}</span>}
              value={<span className="font-mono text-[11px] text-muted2">{`▲ ${fmt.format(i.votes)}`}</span>}
              to={i.board_id ? ideaLink(i.board_id, i.id) : undefined}
            />
          ))}
        </InfoSection>
      )}
    </>
  )
}

function FeedbackFilters({
  tab,
  category,
  onTab,
  onCategory,
}: {
  tab: FeedbackTab
  category: FeedbackCategoryFilter
  onTab: (tab: FeedbackTab) => void
  onCategory: (category: FeedbackCategoryFilter) => void
}) {
  const { t } = useLingui()
  const [inboxId] = useInboxFilter()
  const counts = useCounts().data
  const resolved = useLoadedCount({ kind: ["feedback"], status: ["closed"], inbox_id: inboxId || undefined })
  const { types, open } = useFeedbackTypes(inboxId, category)
  const showCount = !inboxId && tab === "open"
  return (
    <NavList label={t`Feedback`}>
      <div data-testid="feedback-tabs">
        <NavGroup
          title={t`Status`}
          items={[
            {
              key: "open",
              label: t`Open`,
              count: inboxId ? undefined : counts?.feedback,
              hot: true,
              current: tab === "open",
              onSelect: () => onTab("open"),
              testId: "feedback-tab-open",
            },
            {
              key: "resolved",
              label: t`Resolved`,
              count: resolved,
              current: tab === "resolved",
              onSelect: () => onTab("resolved"),
              testId: "feedback-tab-resolved",
            },
          ]}
        />
      </div>
      <div data-testid="feedback-categories">
        <NavGroup
          title={t`Type`}
          items={types.map((x) => ({
            key: x.key,
            label: x.key === "all" ? t`Every type` : x.name,
            count: showCount ? open(x.key) : undefined,
            current: category === x.key,
            onSelect: () => onCategory(x.key),
            testId: `feedback-category-${x.key}`,
          }))}
        />
      </div>
    </NavList>
  )
}

export function FeedbackPage({ id }: { id: string | null }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [inboxId] = useInboxFilter()
  const { find } = useFeedbackCategories()
  const product = useInboxes().data?.find((i) => i.id === inboxId)?.name
  const [tab, setTab] = useState<FeedbackTab>(rememberedFeedbackTab)
  const [category, setCategory] = useState<FeedbackCategoryFilter>(rememberedFeedbackCategory)
  const [form, setForm] = useState(false)
  const [slot, setSlot] = useState<HTMLElement | null>(null)
  const thread = !!id && params.get("thread") === "1"
  const conversation = useConversation(id ?? undefined)
  const c = id ? conversation.data : undefined
  const listY = useRef(0)
  const open = useCallback(
    (next: string) => {
      listY.current = window.scrollY
      navigate(`/feedback/${next}`)
    },
    [navigate]
  )
  useLayoutEffect(() => {
    window.scrollTo({ top: id ? 0 : listY.current })
  }, [id, thread])
  const viewName = tab === "open" ? t`Open` : t`Resolved`
  const typeName = category === "all" ? t`Every type` : find(category, inboxId || undefined).name
  const back = thread ? { label: t`Feedback`, to: `/feedback/${id}` } : { label: viewName, to: "/feedback" }
  useHotkeys({ Escape: () => navigate(back.to) }, !!id)
  const toList = () => {
    if (id) navigate("/feedback")
  }
  const pickTab = (next: FeedbackTab) => {
    rememberFeedback(next, category)
    setTab(next)
    toList()
  }
  const pickCategory = (next: FeedbackCategoryFilter) => {
    rememberFeedback(tab, next)
    setCategory(next)
    toList()
  }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const where = product ?? t`every product`
  const heading = ({ count, more }: { count: number; more: boolean }) => {
    const n = `${fmt.format(count)}${more ? "+" : ""}`
    return (
      <PageHeading
        title={category === "all" ? viewName : `${viewName} · ${typeName}`}
        lede={
          <span data-testid="list-lede">
            {more ? <Trans>{n} reports</Trans> : <Plural value={count} one="# report" other="# reports" />}
            {" · "}
            {where}
            {". "}
            <Trans>Bugs, ideas and praise sent from your apps and pages</Trans>
          </span>
        }
      />
    )
  }
  return (
    <PageLayout
      testId="feedback-page"
      label={t`Feedback`}
      nav={<FeedbackFilters tab={tab} category={category} onTab={pickTab} onCategory={pickCategory} />}
      aside={
        <InfoColumn label={t`Details`}>
          {c ? <DetailInfo c={c} /> : !id && <ListInfo title={viewName} tab={tab} />}
        </InfoColumn>
      }
    >
      <PageTop
        back={id ? back : undefined}
        slot={id ? setSlot : undefined}
        actions={
          !id && (
            <Button
              variant="outline"
              className="h-8 rounded-[9px] border-input bg-card px-3 font-normal"
              onClick={() => setForm(true)}
              data-testid="feedback-form"
            >
              <Trans>Feedback form</Trans>
            </Button>
          )
        }
      >
        {thread && id ? (
          <ConversationEyebrow id={id} />
        ) : (
          <>
            <span className="font-medium text-foreground">
              <Trans>Feedback</Trans>
            </span>
            <TopSep />
            <span className="truncate">{viewName}</span>
            <TopSep />
            <span className="truncate">{typeName}</span>
            <TopSep />
            <span className="truncate">{product ?? t`All products`}</span>
          </>
        )}
      </PageTop>
      <div className="max-w-[980px]">
        <FeedbackFlatList tab={tab} category={category} onOpen={open} hidden={!!id} head={heading} />
        <TopSlotProvider value={slot}>
          {thread && id ? (
            <ChatView key={id} id={id} flat panel={false} inlinePanel={false} onPanel={() => {}} />
          ) : id && conversation.isPending ? (
            <div className="mt-3 flex max-w-[720px] flex-col gap-3">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-8 w-3/4" />
              <Skeleton className="h-16 w-full" />
            </div>
          ) : c ? (
            <Detail key={c.id} c={c} onReply={() => navigate(`/feedback/${c.id}?thread=1`)} />
          ) : (
            id && <ErrorLine error={conversation.error} className="mt-3" />
          )}
        </TopSlotProvider>
      </div>
      {form && (
        <Suspense fallback={null}>
          <FeedbackFormDialog onClose={() => setForm(false)} />
        </Suspense>
      )}
    </PageLayout>
  )
}
