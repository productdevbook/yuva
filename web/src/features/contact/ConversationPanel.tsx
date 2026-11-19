import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ContactAvatar, Dot, ErrorLine, Segmented, sourceOf, useSourceText } from "@/components/common"
import { Fold, FoldRow, FoldSub } from "@/components/common/Fold"
import { formatDateTime, PRIORITIES, useEnumText } from "@/components/common/text"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { BeforeRows, DetailRows, ProductRows, Seen } from "@/features/contact/ContactCard"
import { NewConversationDialog, useContactConversations } from "@/features/contact/ContactDetails"
import { ContactNotes, useNoteCount } from "@/features/contact/ContactNotes"
import { ConversationMedia } from "@/features/contact/ConversationMedia"
import { DeviceInfo } from "@/features/contact/DeviceInfo"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { useContact } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useInboxTeams } from "@/features/conversation/HeaderActions"
import { Participants } from "@/features/conversation/Participants"
import { useUpdateConversation } from "@/features/conversation/queries"
import { ConversationFields, useConversationFieldList } from "@/features/fields/ConversationFields"
import type { Channel, Conversation, ConversationUpdate, Priority } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes, useLabels, useMemberMap, useTeams } from "@/lib/workspace"

export type PanelPart = "person" | "conv" | "past" | "notes"

const FOLDS_KEY = "conversation-folds"
const DEFAULT_FOLDS: Record<PanelPart, boolean> = { person: true, conv: true, past: true, notes: false }

function readFolds(): Record<PanelPart, boolean> {
  try {
    const saved = JSON.parse(localStorage.getItem(FOLDS_KEY) ?? "{}") as Partial<Record<PanelPart, boolean>>
    return { ...DEFAULT_FOLDS, ...saved }
  } catch {
    return DEFAULT_FOLDS
  }
}

export function usePanelFolds(reveal?: { part: PanelPart; n: number }) {
  const [folds, setFolds] = useState(readFolds)
  const set = (part: PanelPart, open: boolean) =>
    setFolds((f) => {
      const next = { ...f, [part]: open }
      try {
        localStorage.setItem(FOLDS_KEY, JSON.stringify(next))
      } catch {
        // Storage can be blocked; the folds then stay as chosen for this page only.
      }
      return next
    })
  const n = reveal?.n ?? 0
  const [seen, setSeen] = useState(n)
  if (reveal && n !== seen) {
    setSeen(n)
    if (!folds[reveal.part]) setFolds({ ...folds, [reveal.part]: true })
  }
  return [folds, set] as const
}

export function LabelEditor({ c, update }: { c: Conversation; update: (body: ConversationUpdate) => void }) {
  const { t } = useLingui()
  const labels = useLabels().data ?? []
  const [picking, setPicking] = useState(false)
  const chosen = labels.filter((l) => c.labels.includes(l.id))
  const rest = labels.filter((l) => !c.labels.includes(l.id))
  return (
    <div data-testid="panel-labels">
      <div className="flex flex-wrap items-center gap-1">
        {chosen.map((l) => (
          <Badge
            key={l.id}
            variant="secondary"
            render={<button type="button" />}
            className="h-6 gap-1 rounded-[5px] ps-2 pe-1.5 font-medium"
            onClick={() => update({ labels: c.labels.filter((id) => id !== l.id) })}
            aria-label={t`Remove label ${l.name}`}
            data-testid="panel-label"
          >
            <Dot color={l.color} className="size-1.5" />
            {l.name}
            <XIcon className="size-3 text-muted-foreground" />
          </Badge>
        ))}
        {labels.length === 0 ? (
          <span className="text-muted-foreground">
            <Trans>No labels yet. Add them in settings.</Trans>
          </span>
        ) : (
          rest.length > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-1.5 text-small"
              aria-expanded={picking}
              onClick={() => setPicking((p) => !p)}
              data-testid="panel-label-add"
            >
              <PlusIcon />
              <Trans>Label</Trans>
            </Button>
          )
        )}
      </div>
      {picking && rest.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {rest.map((l) => (
            <Button
              key={l.id}
              variant="outline"
              size="sm"
              className="h-[26px] gap-1.5 px-2 text-small font-normal"
              onClick={() => update({ labels: [...c.labels, l.id] })}
              data-testid="panel-label-option"
            >
              <Dot color={l.color} className="size-1.5" />
              {l.name}
            </Button>
          ))}
        </div>
      )}
    </div>
  )
}

function TeamPick({ c }: { c: Conversation }) {
  const { t } = useLingui()
  const teams = useInboxTeams(c.inbox_id)
  const current = (useTeams().data ?? []).find((x) => x.id === c.team_id)
  const updater = useUpdateConversation(c.id)
  if (teams.length === 0 && !current) return null
  const items = [
    ...teams.map((x) => ({ value: x.id, label: x.name })),
    ...(current && !teams.some((x) => x.id === current.id) ? [{ value: current.id, label: current.name }] : []),
    { value: "none", label: t`None` },
  ]
  return (
    <>
      <FoldSub>
        <Trans>Team</Trans>
      </FoldSub>
      <Segmented
        value={c.team_id ?? "none"}
        onChange={(v) => updater.mutate({ team_id: v === "none" ? null : v })}
        items={items}
        label={t`Team`}
        className="max-w-full flex-wrap"
        itemClassName="h-6 px-2 text-small"
      />
      <ErrorLine error={updater.error} className="mt-1 text-caption" />
    </>
  )
}

export function ConversationPanel({
  c,
  channel,
  update,
  reveal,
}: {
  c: Conversation
  channel?: Channel
  update: (body: ConversationUpdate) => void
  reveal?: { part: PanelPart; n: number }
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const sourceText = useSourceText()
  const { canManage } = useSession()
  const members = useMemberMap()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === c.inbox_id)
  const contact = useContact(c.contact_id)
  const conversations = useContactConversations(c.contact_id)
  const noteCount = useNoteCount(c.contact_id)
  const hasFields = useConversationFieldList().length > 0
  const [folds, setFold] = usePanelFolds(reveal)
  const [merging, setMerging] = useState(false)
  const [starting, setStarting] = useState(false)
  const [media, setMedia] = useState(false)
  const person = contact.data
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const source = sourceOf(c, channel)
  const email = c.channel_kind === "email"
  const others = conversations.items.filter((o) => o.id !== c.id)
  const name = person ? contactName(person, t`Unnamed contact`) : "…"
  const company = person?.attributes?.company
  const personSum = typeof company === "string" && company ? company : (person?.emails[0] ?? "")
  const who = assignee ? (assignee.name || assignee.email).split(/\s+/)[0] : t`Unassigned`
  const convSum = c.status === "closed" ? text.statusOf(c.status, c.kind) : who
  const priorities = [...PRIORITIES].reverse()

  return (
    <>
      <Fold
        title={<Trans>Person</Trans>}
        summary={personSum}
        open={folds.person}
        onOpenChange={(o) => setFold("person", o)}
        testId="panel-person"
      >
        {contact.isPending ? (
          <Skeleton className="h-10 w-full" />
        ) : !person ? (
          <ErrorLine error={contact.error} />
        ) : (
          <>
            <div className="flex min-w-0 items-center gap-2.5">
              <ContactAvatar id={person.id} name={name} className="size-7 text-caption" />
              <div className="min-w-0">
                <b className="block truncate text-body font-semibold" data-testid="contact-sheet-name">
                  {name}
                </b>
                {person.emails[0] && person.emails[0] !== name && (
                  <span className="block truncate text-muted-foreground">{person.emails[0]}</span>
                )}
              </div>
            </div>
            <div className="mt-2.5">
              <Seen contactId={person.id} />
            </div>
            {person.blocked && (
              <Badge variant="destructive" className="mt-2">
                <Trans>Blocked</Trans>
              </Badge>
            )}
            <div className="mt-2.5 empty:hidden">
              <DetailRows contact={person} />
            </div>
            {person.inboxes.length > 1 && (
              <div className="mt-2.5">
                <ProductRows contact={person} />
              </div>
            )}
            <div className="mt-3 flex flex-wrap gap-1.5">
              <Button
                variant="outline"
                size="sm"
                className="h-[30px]"
                onClick={() => setStarting(true)}
                data-testid="new-conversation-open"
              >
                <Trans>New conversation</Trans>
              </Button>
              {canManage && (
                <Button
                  variant="outline"
                  size="sm"
                  className="h-[30px]"
                  onClick={() => setMerging(true)}
                  data-testid="merge-contact"
                >
                  <Trans>Merge…</Trans>
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                className="h-[30px]"
                render={<Link to={`/contacts/${person.id}`} />}
                data-testid="open-contact-page"
              >
                <Trans>Open in People</Trans>
              </Button>
            </div>
            {canManage && <MergeContactDialog target={person} open={merging} onOpenChange={setMerging} />}
            <NewConversationDialog contact={person} open={starting} onOpenChange={setStarting} />
          </>
        )}
      </Fold>
      <Fold
        title={<Trans>This conversation</Trans>}
        summary={convSum}
        open={folds.conv}
        onOpenChange={(o) => setFold("conv", o)}
        testId="panel-conversation"
      >
        <div data-testid="conversation-info">
          <FoldRow label={<Trans>Status</Trans>} value={text.statusOf(c.status, c.kind)} />
          {inbox && <FoldRow label={<Trans>Product</Trans>} value={inbox.name} />}
          <FoldRow
            label={<Trans>Assignee</Trans>}
            value={assignee ? assignee.name || assignee.email : <Trans>Unassigned</Trans>}
          />
          {source && <FoldRow label={<Trans>Channel</Trans>} value={sourceText[source]} />}
          <FoldRow label={<Trans>Started</Trans>} value={formatDateTime(c.created_at, i18n.locale)} />
        </div>
        <TeamPick c={c} />
        <FoldSub>
          <Trans>Priority</Trans>
        </FoldSub>
        <Segmented
          value={c.priority}
          onChange={(p) => update({ priority: p as Priority })}
          items={priorities.map((p) => ({ value: p, label: text.priority[p], testId: `panel-priority-${p}` }))}
          label={t`Priority`}
          className="max-w-full flex-wrap"
          itemClassName="h-6 px-2 text-small"
        />
        <FoldSub>
          <Trans>Labels</Trans>
        </FoldSub>
        <LabelEditor c={c} update={update} />
        {hasFields && (
          <>
            <FoldSub>
              <Trans>Fields</Trans>
            </FoldSub>
            <div data-testid="fields-section">
              <ConversationFields conversationId={c.id} />
            </div>
          </>
        )}
        {email && (
          <>
            <FoldSub>
              <Trans>Participants</Trans>
            </FoldSub>
            <Participants conversationId={c.id} bare />
          </>
        )}
        {person && (person.client_info || c.client_info) && (
          <>
            <FoldSub>
              <Trans>Device</Trans>
            </FoldSub>
            <div data-testid="device-section">
              <DeviceInfo contact={person} conversationId={c.id} />
            </div>
          </>
        )}
        <FoldSub>
          <Trans>Media, links and docs</Trans>
        </FoldSub>
        {media ? (
          <ConversationMedia conversationId={c.id} />
        ) : (
          <Button
            variant="ghost"
            size="sm"
            className="-ms-2 h-7 px-2 text-small"
            onClick={() => setMedia(true)}
            data-testid="contact-tab-media"
          >
            <Trans>Show</Trans>
          </Button>
        )}
      </Fold>
      <Fold
        title={<Trans>Earlier conversations</Trans>}
        summary={conversations.isPending ? "" : others.length > 0 ? String(others.length) : t`None`}
        open={folds.past}
        onOpenChange={(o) => setFold("past", o)}
        testId="panel-earlier"
      >
        {conversations.isPending ? (
          <Skeleton className="h-10 w-full" />
        ) : (
          <BeforeRows items={conversations.items} currentId={c.id} dots limit={8} />
        )}
      </Fold>
      <Fold
        title={<Trans>Notes</Trans>}
        summary={noteCount === undefined ? "" : noteCount === "0" ? t`None` : noteCount}
        open={folds.notes}
        onOpenChange={(o) => setFold("notes", o)}
        testId="panel-notes"
      >
        <ContactNotes contactId={c.contact_id} />
      </Fold>
    </>
  )
}
