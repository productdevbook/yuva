import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, PaperclipIcon, RotateCcwIcon, Trash2Icon } from "lucide-react"
import { lazy, Suspense, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, toast, useConfirm, useContactName } from "@/components/common"
import {
  boxRowClass,
  BulkBar,
  bulkButton,
  FilterMenu,
  FilterRow,
  FilterSearch,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  PageTitle,
  RowCheckbox,
} from "@/components/common/ListBox"
import { PageLayout } from "@/components/common/PageLayout"
import { ProductFilter, useProductScope } from "@/components/common/ProductFilter"
import { FEEDBACK_KEYS, keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { ScoreMark } from "@/features/conversation/Feedback"
import { IdeaMark } from "@/features/conversation/IdeaChip"
import {
  CategoryPill,
  feedbackPlace,
  ProgressPill,
  useFeedbackTypes,
  type FeedbackCategoryFilter,
} from "@/features/feedback/FeedbackList"
import { useListCursor } from "@/features/inbox/listCursor"
import { Ago } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { useBulkUpdateConversations, useConversations } from "@/features/inbox/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import type { ConversationListItem } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useChannelMap } from "@/lib/workspace"

const FeedbackFormDialog = lazy(() => import("@/features/feedback/FeedbackFormDialog"))

let remembered = { search: "", y: 0 }

export function feedbackBack() {
  return `/feedback${remembered.search}`
}

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

function Row({
  c,
  cursor,
  checked,
  onToggle,
  onOpen,
}: {
  c: ConversationListItem
  cursor: boolean
  checked: boolean
  onToggle: (range: boolean) => void
  onOpen: () => void
}) {
  const { t } = useLingui()
  const channel = useChannelMap().get(c.channel_id ?? "")
  const contactName = useContactName()
  const name = contactName(c, channel, c.contact.name, c.contact.email)
  const f = c.feedback
  const where = feedbackPlace(f)
  const answered = c.last_message?.author_type === "member" || c.last_message?.author_type === "bot"
  const text = (f?.text || c.subject).split("\n")[0]
  return (
    <div
      data-row
      className={boxRowClass({ cursor, checked })}
      data-conversation-id={c.id}
      data-checked={checked || undefined}
    >
      <RowCheckbox checked={checked} name={text} onToggle={onToggle} />
      <button
        type="button"
        onClick={(e) => (e.shiftKey ? onToggle(true) : onOpen())}
        className="flex min-w-0 flex-1 items-center gap-3 py-2.5 pe-3 text-start text-body leading-5 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
        data-testid="feedback-row"
        data-unread={c.unread || undefined}
      >
        <span
          className={cn("size-2 shrink-0 rounded-full bg-brand", !c.unread && "invisible")}
          aria-label={c.unread ? t`Unread` : undefined}
          data-testid={c.unread ? "row-unread" : undefined}
        />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex min-w-0 items-center gap-1.5">
            <span
              className={cn("min-w-0 truncate text-foreground", c.unread ? "font-semibold" : "font-medium")}
              dir="auto"
            >
              {text}
            </span>
            {!!f?.attachment_count && (
              <span
                className="inline-flex shrink-0 items-center gap-0.5 text-caption text-muted-foreground"
                title={t`Screenshots attached`}
                data-testid="feedback-attachments"
              >
                <PaperclipIcon className="size-3" aria-label={t`Screenshots attached`} />
                {f.attachment_count}
              </span>
            )}
            {f?.score ? <ScoreMark score={f.score} /> : null}
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-caption text-muted-foreground">
            <span className="max-w-40 shrink-0 truncate">{name}</span>
            {where && (
              <>
                <span aria-hidden>·</span>
                <span className="min-w-0 truncate">{where}</span>
              </>
            )}
            {answered && (
              <span className="inline-flex shrink-0 items-center gap-0.5 text-success" data-testid="feedback-answered">
                <CheckIcon className="size-3" />
                <Trans>answered</Trans>
              </span>
            )}
          </span>
        </span>
        <span className="flex shrink-0 items-center gap-2">
          <ProductTag id={c.inbox_id} className="@max-xl/list:hidden" />
          {f && <CategoryPill category={f.category} inboxId={c.inbox_id} className="@max-md/list:hidden" />}
          {c.idea && <IdeaMark idea={c.idea} pill className="max-w-44 @max-3xl/list:hidden" />}
          {f?.progress && (
            <span className="@max-2xl/list:hidden">
              <ProgressPill progress={f.progress} />
            </span>
          )}
          <Ago iso={c.last_activity_at} className="w-12 text-end text-caption text-muted-foreground tabular-nums" />
        </span>
      </button>
    </div>
  )
}

export function FeedbackHome() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const errorText = useErrorText()
  const { chosen, inboxId, params, set } = useProductScope()
  const q = params.get("q") ?? ""
  const resolved = params.get("status") === "resolved"
  const type = params.get("type") ?? ""
  const query = useDebounced(q.trim(), 250)
  const [form, setForm] = useState(false)
  const list = useConversations({
    kind: ["feedback"],
    status: resolved ? ["closed"] : ["open", "pending"],
    category: type || undefined,
    inbox_id: inboxId || undefined,
    q: query || undefined,
  })
  const rows = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const scope = `${inboxId}|${query}|${resolved}|${type}`
  const selection = useSelection(
    rows.map((c) => c.id),
    scope
  )
  const bulk = useBulkUpdateConversations()
  const [confirm, confirmDialog] = useConfirm()
  const { types, open: openCount } = useFeedbackTypes(inboxId, (type || "all") as FeedbackCategoryFilter)
  const listRef = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const open = (id: string) => {
    remembered = { search: params.toString() ? `?${params.toString()}` : "", y: window.scrollY }
    navigate(`/feedback/${id}`)
  }
  const { cursorId, setCursorId } = useListCursor(rows, null, open, !form, listRef, true, true)
  const [seen, setSeen] = useState(scope)
  if (seen !== scope) {
    setSeen(scope)
    setCursorId(null)
  }
  useLayoutEffect(() => {
    if (remembered.y) window.scrollTo({ top: remembered.y })
    remembered = { ...remembered, y: 0 }
  }, [])
  const failed = (e: unknown) => toast(errorText(e))
  const formKey = keyLabel(FEEDBACK_KEYS.form).join("")
  const closeKey = keyLabel(SHORTCUTS.close).join("")
  const setStatus = (ids: string[], status: "open" | "closed") =>
    bulk.mutate(
      { conversation_ids: ids, status },
      {
        onSuccess: (r) => {
          const done = r.updated.map((c) => c.id)
          selection.stop()
          toast(status === "closed" ? t`${done.length} resolved` : t`${done.length} reopened`, () =>
            bulk.mutate(
              { conversation_ids: done, status: status === "closed" ? "open" : "closed" },
              { onError: failed }
            )
          )
        },
        onError: failed,
      }
    )
  const setTrashed = (ids: string[], trashed: boolean, after?: (done: string[]) => void) =>
    bulk.mutate(
      { conversation_ids: ids, trashed },
      { onSuccess: (r) => after?.(r.updated.map((c) => c.id)), onError: failed }
    )
  const chosenIds = selection.chosen
  const n = chosenIds.length
  const toggleStatus = () => n > 0 && setStatus(chosenIds, resolved ? "open" : "closed")
  const trash = () => {
    if (n === 0) return
    const ids = [...chosenIds]
    confirm({
      title: <Trans>Move {n} to the Trash?</Trans>,
      description: (
        <Trans>
          They disappear for your team. You can restore them from the Trash for 30 days, then they are deleted for good
          with their messages and files.
        </Trans>
      ),
      confirm: <Trans>Move to the Trash</Trans>,
      run: () =>
        setTrashed(ids, true, (done) => {
          selection.stop()
          toast(t`${done.length} moved to the Trash`, () => setTrashed(done, false))
        }),
    })
  }
  useHotkeys(
    {
      [SHORTCUTS.search]: () => search.current?.focus(),
      [SHORTCUTS.select]: () => cursorId && selection.toggle(cursorId),
      [FEEDBACK_KEYS.form]: () => setForm(true),
      ...(n > 0
        ? { [SHORTCUTS.close]: toggleStatus, [SHORTCUTS.trash]: trash, Escape: selection.stop }
        : selection.selecting
          ? { Escape: selection.stop }
          : {}),
    },
    !form
  )
  const anyFilter = !!(q || chosen || resolved || type)
  const clearFilters = () => {
    set("q", "")
    set("inbox", "")
    set("status", "")
    set("type", "")
  }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shown = `${fmt.format(rows.length)}${list.hasNextPage ? "+" : ""}`
  return (
    <PageLayout testId="feedback-page" label={t`Feedback`}>
      <section aria-label={t`Feedback`} className="@container/list min-w-0" data-testid="feedback-list">
        <PageTitle
          title={<Trans>Feedback</Trans>}
          sub={<Trans>Bugs, ideas and praise sent from your apps and pages</Trans>}
          actions={
            <Button
              variant="outline"
              onClick={() => setForm(true)}
              title={t`Feedback form (${formKey})`}
              data-testid="feedback-form"
            >
              <Trans>Feedback form</Trans>
            </Button>
          }
          testId="feedback-head"
        />
        <FilterRow testId="feedback-filters">
          <FilterSearch
            inputRef={search}
            value={q}
            onChange={(v) => set("q", v)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && rows[0]) {
                e.preventDefault()
                open(rows[0].id)
              }
            }}
            placeholder={t`Search feedback…`}
            label={t`Search feedback`}
            testId="list-search"
          />
          <ProductFilter value={chosen} onChange={(id) => set("inbox", id)} testId="feedback-product" />
          <FilterMenu
            value={resolved ? "resolved" : ""}
            onChange={(v) => set("status", v)}
            options={[{ value: "resolved", label: t`Resolved` }]}
            anyLabel={t`Open`}
            label={t`Filter by status`}
            testId="feedback-status"
          />
          <FilterMenu
            value={type}
            onChange={(v) => set("type", v)}
            options={types
              .filter((x) => x.key !== "all")
              .map((x) => ({
                value: x.key,
                label: resolved || inboxId ? x.name : `${x.name} · ${fmt.format(openCount(x.key))}`,
              }))}
            anyLabel={t`Every type`}
            label={t`Filter by type`}
            testId="feedback-type"
          />
          {anyFilter && (
            <Button variant="ghost" size="sm" className="h-8" onClick={clearFilters} data-testid="feedback-clear">
              <Trans>Clear filters</Trans>
            </Button>
          )}
        </FilterRow>
        <div ref={listRef}>
          <ListBox
            testId="list"
            bar={
              selection.selecting && (
                <BulkBar
                  picked={n}
                  total={rows.length}
                  onToggleAll={selection.toggleAll}
                  onCancel={selection.stop}
                  busy={bulk.isPending}
                >
                  <Button
                    variant="outline"
                    className={bulkButton()}
                    disabled={n === 0 || bulk.isPending}
                    onClick={toggleStatus}
                    title={resolved ? t`Reopen (${closeKey})` : t`Resolve (${closeKey})`}
                    data-testid={resolved ? "reopen-selected" : "resolve-selected"}
                  >
                    {resolved ? <RotateCcwIcon /> : <CheckIcon />}
                    {resolved ? <Trans>Reopen</Trans> : <Trans>Resolve</Trans>}
                  </Button>
                  <Button
                    variant="outline"
                    className={bulkButton(true)}
                    disabled={n === 0 || bulk.isPending}
                    onClick={trash}
                    title={t`Move to the Trash (#)`}
                    data-testid="trash-selected"
                  >
                    <Trash2Icon />
                    <Trans>Move to the Trash</Trans>
                  </Button>
                </BulkBar>
              )
            }
          >
            {list.isPending ? (
              <ListBoxSkeleton />
            ) : list.error && rows.length === 0 ? (
              <ListBoxEmpty
                action={
                  <Button variant="outline" size="sm" onClick={() => void list.refetch()}>
                    <Trans>Try again</Trans>
                  </Button>
                }
              >
                <ErrorLine error={list.error} />
              </ListBoxEmpty>
            ) : rows.length === 0 ? (
              <ListBoxEmpty
                testId="list-empty"
                action={
                  anyFilter && (
                    <Button variant="outline" size="sm" onClick={clearFilters}>
                      <Trans>Clear filters</Trans>
                    </Button>
                  )
                }
              >
                {query ? (
                  <Trans>Nothing matches your search here.</Trans>
                ) : resolved ? (
                  <Trans>Nothing is resolved yet.</Trans>
                ) : (
                  <Trans>No open feedback.</Trans>
                )}
              </ListBoxEmpty>
            ) : (
              rows.map((c) => (
                <Row
                  key={c.id}
                  c={c}
                  cursor={c.id === cursorId}
                  checked={selection.has(c.id)}
                  onToggle={(range) => selection.toggle(c.id, range)}
                  onOpen={() => (selection.selecting ? selection.toggle(c.id) : open(c.id))}
                />
              ))
            )}
          </ListBox>
        </div>
        <ErrorLine error={list.data ? list.error : null} className="py-2" />
        {list.hasNextPage && (
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => void list.fetchNextPage()}
            disabled={list.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
        <p className="m-0 mt-3 text-small text-muted-foreground" data-testid="list-lede">
          {plural(rows.length, { one: `${shown} report`, other: `${shown} reports` })}
        </p>
        <KeyHints
          items={[
            [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
            [
              ["↵"],
              <Trans key="open" context="keyboard hint">
                open
              </Trans>,
            ],
            [[SHORTCUTS.select], <Trans key="select">select</Trans>],
            [[SHORTCUTS.search], <Trans key="search">search</Trans>],
          ]}
        />
      </section>
      {confirmDialog}
      {form && (
        <Suspense fallback={null}>
          <FeedbackFormDialog onClose={() => setForm(false)} />
        </Suspense>
      )}
    </PageLayout>
  )
}
