import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, CopyIcon } from "lucide-react"
import { lazy, Suspense, useCallback, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ContactAvatar, ErrorLine, toast, useCopy, VisitorAvatar, isVisitor } from "@/components/common"
import { useFeedbackCategories } from "@/components/common/feedbackCategories"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText, useErrorText } from "@/components/common/text"
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
  FeedbackList,
  progressTone,
  rememberedFeedbackTab,
  type FeedbackTab,
} from "@/features/feedback/FeedbackList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { Ago, FrameTabs, look, Overline, PageTop } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { useCounts, useLoadedCount } from "@/features/inbox/queries"
import { Lightbox } from "@/features/feedback/Lightbox"
import { RoadmapCard } from "@/features/feedback/RoadmapCard"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { attachmentUrl, type Conversation, type FeedbackProgress } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

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

function Detail({ id, onReply }: { id: string; onReply: () => void }) {
  const { t } = useLingui()
  const conversation = useConversation(id)
  const c = conversation.data
  const contact = useContact(c?.contact_id).data
  const [ideaLinking, setIdeaLinking] = useState<"link" | "new" | null>(null)
  const progress = useRef<HTMLDivElement>(null)
  if (conversation.isPending) {
    return (
      <div className="flex flex-col gap-3 p-5">
        <Skeleton className="h-5 w-32" />
        <Skeleton className="h-6 w-3/4" />
        <Skeleton className="h-16 w-full" />
      </div>
    )
  }
  if (!c) return <ErrorLine error={conversation.error} className="p-5" />
  const f = c.feedback
  const name = contact ? contactName(contact, t`Unnamed contact`) : c.contact_id ? "…" : ""
  const visitor = !!contact && isVisitor(contact.name, contact.emails[0])
  const raw = (f?.text || c.subject).trim()
  const [title, ...rest] = raw.split("\n")
  const body = rest.join("\n").trim()
  return (
    <div className="flex min-h-0 flex-col" data-testid="feedback-detail" data-conversation-id={c.id}>
      <section className="flex flex-col gap-3 border-b p-5">
        <div className="flex items-center gap-2">
          {f && <CategoryPill category={f.category} inboxId={c.inbox_id} />}
          <ProductTag id={c.inbox_id} />
          <span className="flex-1" />
          <Ago iso={c.created_at} className={look.faint} />
        </div>
        <h2 className="text-[18px] leading-[1.35] font-semibold break-words" dir="auto">
          {title}
        </h2>
        {body && (
          <p className="whitespace-pre-line text-muted-foreground" dir="auto">
            {body}
          </p>
        )}
        <Shots c={c} />
        {contact && (
          <Link
            to={`/contacts/${contact.id}`}
            className="flex w-fit items-center gap-2 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {visitor ? (
              <VisitorAvatar className="size-8" />
            ) : (
              <ContactAvatar id={contact.id} name={name} className="size-8 font-semibold" />
            )}
            <span className="flex min-w-0 flex-col leading-[1.3]">
              <span className="truncate font-medium">{name}</span>
              {contact.emails[0] && contact.emails[0] !== name && (
                <span className="truncate text-caption text-faint">{contact.emails[0]}</span>
              )}
            </span>
          </Link>
        )}
      </section>
      <RoadmapCard c={c} onLink={() => setIdeaLinking("link")} onNew={() => setIdeaLinking("new")} />
      {f && <Meta c={c} />}
      <section ref={progress} className="flex flex-col gap-2.5 border-b px-5 py-4">
        <Overline>
          <Trans>Progress · the person sees this</Trans>
        </Overline>
        <ProgressPicker c={c} />
        {c.idea && (
          <p className={look.faint}>
            <Trans>
              Moving the idea to Planned, In progress or Shipped sets this for linked feedback that has no progress yet.
            </Trans>
          </p>
        )}
      </section>
      <Actions c={c} name={name} onReply={onReply} onIdea={() => setIdeaLinking("link")} progress={progress} />
      {ideaLinking && (
        <Suspense fallback={null}>
          <LinkIdeaDialog c={c} startNew={ideaLinking === "new"} onClose={() => setIdeaLinking(null)} />
        </Suspense>
      )}
    </div>
  )
}

function Meta({ c }: { c: Conversation }) {
  const { t } = useLingui()
  const { find } = useFeedbackCategories()
  const rows = useFeedbackRows(c.feedback!)
  const [copied, copy] = useCopy()
  const copyAll = () =>
    copy(
      [`${t`Category`}: ${find(c.feedback!.category, c.inbox_id).name}`, ...rows.map(([k, v]) => `${k}: ${v}`)].join(
        "\n"
      )
    )
  useHotkeys({ [SHORTCUTS.copyDetails]: () => void copyAll() })
  return (
    <section className="relative border-b px-5 py-4" data-testid="feedback-details">
      <Overline className="mb-2.5">
        <Trans>Details</Trans>
      </Overline>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2.5">
        {rows.map(([k, v, mono]) => (
          <div key={k} className="flex min-w-0 flex-col leading-[1.3]">
            <dt className={look.faint}>{k}</dt>
            <dd className={cn("truncate", mono && "font-mono text-small")} title={v}>
              {v}
            </dd>
          </div>
        ))}
      </dl>
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        className="absolute end-3 top-3 text-faint"
        onClick={() => void copyAll()}
        aria-label={t`Copy the feedback details`}
        title={t`Copy the feedback details (Y)`}
        data-testid="copy-feedback"
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </Button>
    </section>
  )
}

function Actions({
  c,
  name,
  onReply,
  onIdea,
  progress,
}: {
  c: Conversation
  name: string
  onReply: () => void
  onIdea: () => void
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
  })
  return (
    <div className="flex flex-wrap gap-2 px-5 py-4">
      <Button
        className={cn(look.btnPSm, "h-[34px] px-3.5")}
        onClick={onReply}
        title={t`Reply (R)`}
        data-testid="feedback-reply"
      >
        <Trans>Reply</Trans>
      </Button>
      <Button
        variant="outline"
        className={look.btn}
        onClick={toggle}
        title={closed ? undefined : t`Resolve (E)`}
        data-testid={closed ? "reopen-conversation" : "close-conversation"}
      >
        {closed ? <Trans>Reopen</Trans> : <Trans>Resolve</Trans>}
      </Button>
    </div>
  )
}

export function FeedbackPage({ id }: { id: string | null }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [inboxId] = useInboxFilter()
  const counts = useCounts().data
  const resolved = useLoadedCount({ kind: ["feedback"], status: ["closed"], inbox_id: inboxId || undefined })
  const [tab, setTab] = useState<FeedbackTab>(rememberedFeedbackTab)
  const [first, setFirst] = useState<string | null>(null)
  const [form, setForm] = useState(false)
  const thread = !!id && params.get("thread") === "1"
  const shown = id ?? first
  const open = useCallback((next: string) => navigate(`/feedback/${next}`), [navigate])
  const onRows = useCallback((ids: string[]) => setFirst(ids[0] ?? null), [])
  const tabs = useMemo(
    () => [
      {
        key: "open" as const,
        label: <Trans>Open</Trans>,
        count: inboxId ? undefined : counts?.feedback,
        testId: "feedback-tab-open",
      },
      { key: "resolved" as const, label: <Trans>Resolved</Trans>, count: resolved, testId: "feedback-tab-resolved" },
    ],
    [inboxId, counts?.feedback, resolved]
  )
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="feedback-page">
      <PageTop
        title={<Trans>Feedback</Trans>}
        sub={<Trans>Bugs, ideas and praise sent from your apps and pages</Trans>}
        actions={
          <Button variant="outline" className={look.btn} onClick={() => setForm(true)} data-testid="feedback-form">
            <Trans>Feedback form</Trans>
          </Button>
        }
        tabs={
          <div data-testid="feedback-tabs">
            <FrameTabs label={t`Status`} value={tab} onChange={setTab} items={tabs} />
          </div>
        }
      />
      <div className="flex min-h-0 flex-1">
        <div className={cn("flex min-w-0 flex-col border-e", thread ? "w-[400px] shrink-0" : "flex-1")}>
          <FeedbackList currentId={shown} onOpen={open} embedded tab={tab} onRows={onRows} />
        </div>
        {thread && id ? (
          <div className="flex min-w-0 flex-1 flex-col">
            <ChatView
              key={id}
              id={id}
              onBack={() => navigate(`/feedback/${id}`)}
              panel={false}
              inlinePanel={false}
              onPanel={() => {}}
            />
          </div>
        ) : (
          <aside className="flex w-[400px] shrink-0 flex-col overflow-y-auto" aria-label={t`Feedback details`}>
            {shown ? (
              <Detail key={shown} id={shown} onReply={() => navigate(`/feedback/${shown}?thread=1`)} />
            ) : (
              <p className="p-10 text-center text-faint" data-testid="feedback-empty">
                <Trans>Pick a piece of feedback to see it here.</Trans>
              </p>
            )}
          </aside>
        )}
      </div>
      {form && (
        <Suspense fallback={null}>
          <FeedbackFormDialog onClose={() => setForm(false)} />
        </Suspense>
      )}
    </main>
  )
}
