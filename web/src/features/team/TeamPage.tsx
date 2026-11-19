import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowUpRightIcon, UserPlusIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Link, useNavigate, useParams } from "react-router"

import { byPresence, useMemberStatus } from "@/app/TeamMenu"
import { colorFor, Dot, ErrorLine, InfoColumn, InfoSection, KV, PageLayout, PageTop } from "@/components/common"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import { KeyHints, ListBox, ListBoxEmpty, ListBoxSkeleton, PageTitle } from "@/components/common/ListBox"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDate, initials, ROLES, useEnumText } from "@/components/common/text"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConversationRows } from "@/features/contact/ContactDetails"
import { rememberChip } from "@/features/inbox/ChatList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useListCursor } from "@/features/inbox/listCursor"
import { useConversations, useCounts, useStats } from "@/features/inbox/queries"
import { LoadProblem } from "@/features/reports/parts"
import { MemberAddresses } from "@/features/team/MemberAddresses"
import { RemoveMemberDialog } from "@/features/team/RemoveMemberDialog"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import { locales } from "@/i18n"
import { api, ApiError, unwrap, type Invite, type Locale, type Member, type Role } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useAllInboxes, useInboxes, useMembers } from "@/lib/workspace"

type Presence = "here" | "away" | "offline"

function presenceOf(m: Member): Presence {
  return m.availability === "away" ? "away" : m.online ? "here" : "offline"
}

function PresenceFace({ m, className }: { m: Member; className?: string }) {
  const name = m.name || m.email
  const presence = presenceOf(m)
  return (
    <span
      aria-hidden
      className={cn(
        "relative grid size-8 shrink-0 place-items-center rounded-full text-[11px] font-semibold text-white",
        className
      )}
      style={{ backgroundColor: colorFor(m.id) }}
    >
      {initials(name)}
      <span
        className={cn(
          "absolute -end-px -bottom-px size-[10px] rounded-full border-2 border-card",
          presence === "here" && "bg-success",
          presence === "away" && "bg-warning",
          presence === "offline" && "bg-faint/60"
        )}
      />
    </span>
  )
}

function useRoleHint() {
  const { t } = useLingui()
  return (role: Role) =>
    role === "owner"
      ? t`Owners can do everything, including billing and deleting the workspace.`
      : role === "admin"
        ? t`Admins see every inbox and manage products, channels and members.`
        : t`Agents see the inboxes they were given and answer in them.`
}

function RoleBadge({ role }: { role: Role }) {
  const text = useEnumText()
  return (
    <Badge
      variant="secondary"
      className={cn(
        "rounded-md font-medium",
        role === "owner" && "bg-success/10 text-success",
        role === "admin" && "bg-warning/10 text-warning"
      )}
    >
      {text.role[role]}
    </Badge>
  )
}

function RoleChoice({
  value,
  onChange,
  disabled,
  label,
}: {
  value: Role
  onChange: (r: Role) => void
  disabled: (r: Role) => boolean
  label: string
}) {
  const text = useEnumText()
  const hint = useRoleHint()
  return (
    <RadioGroup
      value={value}
      onValueChange={(v) => v !== value && onChange(v as Role)}
      aria-label={label}
      className="gap-2"
    >
      {ROLES.map((r) => (
        <FieldLabel key={r} htmlFor={`role-${r}`} className="w-full">
          <Field orientation="horizontal" data-disabled={disabled(r) || undefined}>
            <RadioGroupItem value={r} id={`role-${r}`} disabled={disabled(r)} data-testid={`role-${r}`} />
            <FieldContent>
              <FieldTitle>{text.role[r]}</FieldTitle>
              <FieldDescription className="text-caption">{hint(r)}</FieldDescription>
            </FieldContent>
          </Field>
        </FieldLabel>
      ))}
    </RadioGroup>
  )
}

function InviteForm({ email, setEmail }: { email: string; setEmail: (v: string) => void }) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const text = useEnumText()
  const hint = useRoleHint()
  const { membership, workspaceId: ws } = useSession()
  const [role, setRole] = useState<Role>("agent")
  const [locale, setLocale] = useState<Locale>(i18n.locale === "tr" ? "tr" : "en")
  const [sent, setSent] = useState("")
  const create = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/invites", { body: { email: email.trim(), role, locale } })),
    onSuccess: () => {
      setSent(email.trim())
      setEmail("")
      setRole("agent")
      void qc.invalidateQueries({ queryKey: keys.invites(ws) })
    },
  })
  const isOwner = membership.role === "owner"
  const clear = () => {
    setEmail("")
    setRole("agent")
    create.reset()
  }
  return (
    <FormCard
      title={<Trans>Invite a teammate</Trans>}
      submit={<Trans>Send invite</Trans>}
      cancel={<Trans>Clear</Trans>}
      onCancel={clear}
      onSubmit={() => create.mutate()}
      ready={!!email.trim()}
      busy={create.isPending}
      error={create.error}
      testId="invite-form"
    >
      <FormGrid>
        <Field>
          <FieldLabel htmlFor="invite-email">
            <Trans>E-mail address</Trans>
          </FieldLabel>
          <Input
            id="invite-email"
            type="email"
            required
            value={email}
            onChange={(e) => {
              setEmail(e.target.value)
              setSent("")
            }}
            placeholder={t`teammate@company.com`}
            className="h-9"
          />
          <FieldDescription className="text-caption" aria-live="polite">
            {sent ? (
              <span className="text-success" data-testid="invite-sent">
                <Trans>Invite sent to {sent}.</Trans>
              </span>
            ) : (
              <Trans>We send a sign-in link that is valid for 7 days.</Trans>
            )}
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="invite-role">
            <Trans>Role</Trans>
          </FieldLabel>
          <NativeSelect
            id="invite-role"
            value={role}
            onChange={(e) => setRole(e.target.value as Role)}
            className="w-full"
            data-testid="invite-role"
          >
            {ROLES.map((r) => (
              <NativeSelectOption key={r} value={r} disabled={r === "owner" && !isOwner}>
                {text.role[r]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <FieldDescription className="text-caption">{hint(role)}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="invite-locale">
            <Trans>Language of the invite</Trans>
          </FieldLabel>
          <NativeSelect
            id="invite-locale"
            value={locale}
            onChange={(e) => setLocale(e.target.value as Locale)}
            className="w-full"
          >
            {Object.entries(locales).map(([k, v]) => (
              <NativeSelectOption key={k} value={k}>
                {v}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
      </FormGrid>
    </FormCard>
  )
}

function InviteRow({ invite }: { invite: Invite }) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const now = useNow(60_000)
  const [asking, setAsking] = useState(false)
  const refresh = () => qc.invalidateQueries({ queryKey: keys.invites(ws) })
  const resend = useMutation({
    mutationFn: () =>
      unwrap(api.POST("/v1/invites", { body: { email: invite.email, role: invite.role, locale: invite.locale } })),
    onSuccess: refresh,
  })
  const withdraw = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/invites/{inviteId}", { params: { path: { inviteId: invite.id } } })),
    onSuccess: refresh,
  })
  const expired = new Date(invite.expires_at).getTime() < now
  const error = resend.error ?? withdraw.error
  return (
    <TableRow className="border-line-soft hover:bg-subtle" data-testid="invite-row">
      <TableCell className="min-w-0 overflow-hidden px-4 py-2.5">
        <span className="flex min-w-0 flex-col leading-5">
          <bdi className="truncate font-medium text-foreground">{invite.email}</bdi>
          <span className="truncate text-small text-muted-foreground">
            {locales[invite.locale]}
            <span className="@3xl/list:hidden">
              {" · "}
              <Trans>sent {formatDate(invite.created_at, i18n.locale)}</Trans>
            </span>
          </span>
          {error && <ErrorLine error={error} className="text-small whitespace-normal" />}
        </span>
      </TableCell>
      <TableCell className="px-3 py-2.5 @max-md/list:hidden">
        <RoleBadge role={invite.role} />
      </TableCell>
      <TableCell className="px-3 py-2.5 text-small text-muted-foreground @max-3xl/list:hidden">
        {formatDate(invite.created_at, i18n.locale)}
      </TableCell>
      <TableCell className="px-3 py-2.5 @max-xl/list:hidden">
        {expired ? (
          <Badge variant="destructive" className="rounded-md">
            <Trans>Expired</Trans>
          </Badge>
        ) : (
          <Badge variant="secondary" className="rounded-md" title={formatDate(invite.expires_at, i18n.locale)}>
            <Trans>Pending</Trans>
          </Badge>
        )}
      </TableCell>
      <TableCell className="px-3 py-2 pe-4 text-end">
        {asking ? (
          <span className="inline-flex items-center justify-end gap-1.5 @max-md/list:flex-col @max-md/list:items-stretch">
            <Button
              variant="destructive"
              size="sm"
              onClick={() => withdraw.mutate()}
              disabled={withdraw.isPending}
              aria-label={t`Withdraw the invite for ${invite.email}`}
            >
              <Trans>Withdraw</Trans>
            </Button>
            <Button variant="outline" size="sm" onClick={() => setAsking(false)}>
              <Trans>Cancel</Trans>
            </Button>
          </span>
        ) : (
          <span className="inline-flex items-center justify-end gap-1.5 @max-md/list:flex-col @max-md/list:items-stretch">
            <Button
              variant="outline"
              size="sm"
              onClick={() => resend.mutate()}
              disabled={resend.isPending || resend.isSuccess}
              aria-label={t`Send the invite for ${invite.email} again`}
            >
              {resend.isSuccess ? <Trans>Sent</Trans> : <Trans>Resend</Trans>}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setAsking(true)}
              aria-label={t`Withdraw the invite for ${invite.email}`}
            >
              <Trans>Withdraw</Trans>
            </Button>
          </span>
        )}
      </TableCell>
    </TableRow>
  )
}

export function useInvites(enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.invites(ws),
    queryFn: () => unwrap(api.GET("/v1/invites")).then((r) => r.items),
    enabled,
  })
}

type Product = { id: string; name: string; color?: string }

function DetailSection({
  title,
  count,
  children,
}: {
  title: React.ReactNode
  count?: number
  children: React.ReactNode
}) {
  return (
    <section className="mt-7 flex flex-col gap-2.5">
      <h2 className="m-0 flex items-center gap-2 text-[15px] font-semibold">
        {title}
        {count !== undefined && (
          <Badge variant="secondary" className="rounded-full px-1.5 tabular-nums">
            {count}
          </Badge>
        )}
      </h2>
      {children}
    </section>
  )
}

function MemberDetail({ member: m, products }: { member: Member; products: Product[] }) {
  const { t } = useLingui()
  const text = useEnumText()
  const hint = useRoleHint()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const status = useMemberStatus()
  const { membership, canManage, workspaceId: ws } = useSession()
  const open = useConversations({ assignee: m.id, status: ["open"], purpose: "support" })
  const [asking, setAsking] = useState(false)
  const [handling, setHandling] = useState(false)
  const owned = (useAllInboxes().data ?? []).filter((i) => i.owner_member_id === m.id)
  const isOwner = membership.role === "owner"
  const you = m.id === membership.member_id
  const name = m.name || m.email
  const s = status(m)
  const items = open.data?.pages.flatMap((p) => p.items) ?? []
  const refresh = () => qc.invalidateQueries({ queryKey: keys.members(ws) })
  const setRole = useMutation({
    mutationFn: (role: Role) =>
      unwrap(api.PATCH("/v1/members/{memberId}", { params: { path: { memberId: m.id } }, body: { role } })),
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/members/{memberId}", { params: { path: { memberId: m.id } } })),
    onSuccess: () => {
      void refresh()
      navigate("/team", { replace: true })
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === "personal_mailbox") {
        void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
        setAsking(false)
        setHandling(true)
      }
    },
  })
  const roleNote = you
    ? t`You cannot change your own role.`
    : m.role === "owner" && !isOwner
      ? t`Only an owner can change an owner's role.`
      : ""
  const rolesLocked = !canManage || you || (m.role === "owner" && !isOwner)
  const removeNote = you
    ? t`You cannot remove yourself.`
    : m.role === "owner"
      ? t`An owner cannot be removed. Change their role first.`
      : t`Their conversations stay; they lose access at once.`
  const removable = canManage && !you && m.role !== "owner"
  return (
    <div className="flex min-w-0 flex-col" data-testid="member-page">
      <div className="mt-3 flex flex-col gap-3">
        <div className="flex items-start gap-3.5">
          <PresenceFace m={m} className="size-12 text-[16px]" />
          <div className="flex min-w-0 flex-1 flex-col">
            <h1 className="m-0 text-[22px] leading-7 font-semibold break-words" data-testid="member-name">
              {you ? t`${name} (you)` : name}
            </h1>
            <p className="m-0 mt-0.5 text-body text-muted-foreground">
              {m.title && <span data-testid="member-title">{m.title} · </span>}
              <bdi>{m.email}</bdi>
            </p>
            <span className={cn("text-small text-muted-foreground", s.conversationId && "text-ok")}>
              {text.role[m.role]} · {s.text}
            </span>
          </div>
        </div>
        {s.conversationId && !you && (
          <Button
            variant="outline"
            size="sm"
            className="self-start"
            render={<Link to={`/conversations/${s.conversationId}`} />}
            data-testid="member-viewing"
          >
            <Trans>Open what they see</Trans>
            <ArrowUpRightIcon />
          </Button>
        )}
      </div>
      <DetailSection title={<Trans>Role</Trans>}>
        {canManage ? (
          <RoleChoice
            value={m.role}
            onChange={(r) => setRole.mutate(r)}
            disabled={(r) => rolesLocked || setRole.isPending || (r === "owner" && !isOwner)}
            label={t`Role of ${name}`}
          />
        ) : (
          <div className="flex flex-col gap-1">
            <RoleBadge role={m.role} />
            <span className="text-caption text-muted-foreground">{hint(m.role)}</span>
          </div>
        )}
        {canManage && roleNote && <span className="text-caption text-muted-foreground">{roleNote}</span>}
        <ErrorLine error={setRole.error} />
      </DetailSection>
      {products.length > 0 && (
        <DetailSection title={<Trans>Products</Trans>}>
          <div className="flex flex-wrap gap-x-3.5 gap-y-1.5">
            {products.map((p) => (
              <span
                key={p.id}
                className="inline-flex min-w-0 items-center gap-[5px] text-small"
                data-testid="product-tag"
              >
                <Dot color={p.color} className="size-2 rounded-[3px]" />
                <span className="truncate">{p.name}</span>
              </span>
            ))}
          </div>
        </DetailSection>
      )}
      <DetailSection
        title={<Trans>Open conversations assigned</Trans>}
        count={open.isPending ? undefined : items.length}
      >
        <ConversationRows items={items} roomy pending={open.isPending} />
        {open.hasNextPage && (
          <Button
            variant="outline"
            size="sm"
            className="self-center"
            onClick={() => void open.fetchNextPage()}
            disabled={open.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
      </DetailSection>
      {canManage && (
        <div data-flat="">
          <MemberAddresses memberId={m.id} name={name} />
        </div>
      )}
      {canManage && (
        <section className="mt-8 flex flex-wrap items-center gap-3 rounded-xl border border-destructive/30 bg-card px-4 py-3.5">
          <span className="flex min-w-48 flex-1 flex-col leading-[1.35]">
            <span className="text-body font-semibold">
              <Trans>Remove from the workspace</Trans>
            </span>
            <span className="text-caption text-muted-foreground">{removeNote}</span>
          </span>
          {asking ? (
            <span className="flex flex-wrap items-center gap-2">
              <span className="text-small">
                <Trans>Remove {name}?</Trans>
              </span>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => remove.mutate()}
                disabled={remove.isPending}
                aria-label={t`Remove ${name}`}
              >
                <Trans>Remove</Trans>
              </Button>
              <Button variant="outline" size="sm" onClick={() => setAsking(false)}>
                <Trans>Cancel</Trans>
              </Button>
            </span>
          ) : (
            <Button
              variant="outline"
              size="sm"
              className="text-destructive"
              disabled={!removable}
              onClick={() => (owned.length > 0 ? setHandling(true) : setAsking(true))}
              aria-label={t`Remove ${name}`}
            >
              <Trans>Remove</Trans>
            </Button>
          )}
          <ErrorLine error={remove.error} className="basis-full" />
        </section>
      )}
      {handling && owned.length > 0 && (
        <RemoveMemberDialog
          member={m}
          mailboxes={owned}
          onClose={() => setHandling(false)}
          onRemoved={() => {
            setHandling(false)
            void refresh()
            void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
            navigate("/team", { replace: true })
          }}
        />
      )}
    </div>
  )
}

function useTeam() {
  const { i18n } = useLingui()
  const { workspaceId: ws } = useSession()
  const members = useMembers()
  const counts = useCounts().data
  const [inboxId] = useInboxFilter()
  const stats = useStats(inboxId).data
  const inboxes = useInboxes().data ?? []
  const queue = useConversations(
    { status: ["open"], assignee: "unassigned", purpose: "support", inbox_id: inboxId || undefined },
    !!inboxId
  )
  const granted = useQueries({
    queries: inboxes.map((inbox) => ({
      queryKey: keys.inboxMembers(ws, inbox.id),
      queryFn: () =>
        unwrap(api.GET("/v1/inboxes/{inboxId}/members", { params: { path: { inboxId: inbox.id } } })).then(
          (r) => r.items
        ),
      staleTime: 60_000,
    })),
  })
  const load = new Map((counts?.assignees ?? []).map((a) => [a.id, a.count]))
  const where = (m: Member): Product[] =>
    (m.role === "agent" ? inboxes.filter((_, i) => granted[i]?.data?.some((g) => g.id === m.id)) : inboxes).map(
      (i) => ({ id: i.id, name: i.name, color: i.branding.color })
    )
  const scopedIndex = inboxes.findIndex((i) => i.id === inboxId)
  const inScope = (m: Member) =>
    !inboxId || m.role !== "agent" || !!granted[scopedIndex]?.data?.some((g) => g.id === m.id)
  const order = [...(members.data ?? [])]
    .filter(inScope)
    .sort((a, b) => byPresence(a, b) || (load.get(b.id) ?? 0) - (load.get(a.id) ?? 0))
  const fmt = new Intl.NumberFormat(i18n.locale)
  const available = fmt.format(order.filter((m) => m.online && m.availability === "auto").length)
  const total = fmt.format(order.length)
  const unassigned = inboxId
    ? queue.data && !queue.hasNextPage
      ? queue.data.pages.flatMap((p) => p.items).length
      : undefined
    : counts?.unassigned
  const openTotal = inboxId ? (counts?.inboxes.find((i) => i.id === inboxId)?.count ?? 0) : counts?.all
  const assigned = unassigned !== undefined && openTotal !== undefined ? Math.max(0, openTotal - unassigned) : undefined
  return { members, stats, load, where, order, available, total, unassigned, assigned }
}

const head = "h-9 px-3 text-start text-caption font-medium text-muted-foreground"

function MemberRow({
  m,
  load,
  replies,
  cursor,
  products,
  onOpen,
}: {
  m: Member
  load: number
  replies: number | undefined
  cursor: boolean
  products: Product[]
  onOpen: () => void
}) {
  const { t, i18n } = useLingui()
  const status = useMemberStatus()
  const { membership } = useSession()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const name = m.name || m.email
  const s = status(m)
  const presence = presenceOf(m)
  return (
    <TableRow
      onClick={onOpen}
      className={cn(
        "cursor-pointer border-line-soft hover:bg-subtle",
        cursor && "bg-subtle [&>td:first-child]:shadow-[inset_2px_0_0_var(--brand)]"
      )}
      data-testid="team-member"
      data-row-id={m.id}
    >
      <TableCell className="min-w-0 overflow-hidden px-4 py-2.5">
        <Link
          to={`/team/${m.id}`}
          onClick={(e) => e.stopPropagation()}
          className="flex min-w-0 items-center gap-3 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <PresenceFace m={m} />
          <span className="flex min-w-0 flex-col leading-5">
            <span className="truncate font-medium text-foreground">
              {m.id === membership.member_id ? t`${name} (you)` : name}
            </span>
            <bdi className="truncate text-small text-muted-foreground">{m.email}</bdi>
          </span>
        </Link>
      </TableCell>
      <TableCell className="px-3 py-2.5 @max-md/list:hidden">
        <RoleBadge role={m.role} />
      </TableCell>
      <TableCell className="px-3 py-2.5 @max-xl/list:hidden">
        <span
          className={cn(
            "flex min-w-0 items-center gap-1.5 text-small",
            s.conversationId ? "text-ok" : "text-muted-foreground"
          )}
        >
          <span
            aria-hidden
            className={cn(
              "size-[7px] shrink-0 rounded-full",
              presence === "here" ? "bg-success" : presence === "away" ? "bg-warning" : "bg-faint/60"
            )}
          />
          <span className="truncate">{s.text}</span>
        </span>
      </TableCell>
      <TableCell className="px-3 py-2.5 @max-3xl/list:hidden">
        <span className="flex min-w-0 items-center gap-2.5 overflow-hidden">
          {products.slice(0, 3).map((p) => (
            <span key={p.id} className="inline-flex min-w-0 items-center gap-[5px] text-small text-muted-foreground">
              <Dot color={p.color} className="size-[7px] rounded-full" />
              <span className="truncate">{p.name}</span>
            </span>
          ))}
          {products.length > 3 && <span className="text-small text-faint">+{products.length - 3}</span>}
        </span>
      </TableCell>
      <TableCell
        className={cn("px-3 py-2.5 text-end tabular-nums", !load && "text-muted-foreground")}
        data-testid="team-load"
      >
        {fmt.format(load)}
      </TableCell>
      <TableCell className="px-3 py-2.5 pe-4 text-end text-muted-foreground tabular-nums @max-md/list:hidden">
        {replies === undefined ? "–" : fmt.format(replies)}
      </TableCell>
    </TableRow>
  )
}

function MembersList({ team }: { team: ReturnType<typeof useTeam> }) {
  const navigate = useNavigate()
  const { members, stats, load, where, order } = team
  const listRef = useRef<HTMLDivElement>(null)
  const { cursorId } = useListCursor(order, null, (id) => navigate(`/team/${id}`), true, listRef, true, true)
  return (
    <div ref={listRef}>
      <ListBox testId="team-panel">
        {members.isPending ? (
          <ListBoxSkeleton rows={3} />
        ) : members.error && !members.data ? (
          <LoadProblem error={members.error} onRetry={() => void members.refetch()} testId="team-error" />
        ) : order.length === 0 ? (
          <ListBoxEmpty>
            <Trans>Nobody on the team works on this product yet.</Trans>
          </ListBoxEmpty>
        ) : (
          <Table className="table-fixed text-body" data-testid="team-members">
            <TableHeader>
              <TableRow className="border-line-soft hover:bg-transparent">
                <TableHead className={cn(head, "ps-4")}>
                  <Trans>Member</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-24 @max-md/list:hidden")}>
                  <Trans>Role</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-[22%] @max-xl/list:hidden")}>
                  <Trans>Status</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-[22%] @max-3xl/list:hidden")}>
                  <Trans>Products</Trans>
                </TableHead>
                <TableHead
                  className={cn(
                    head,
                    "w-32 text-end leading-tight whitespace-normal first-letter:uppercase @max-md/list:w-24"
                  )}
                >
                  <Trans>open, assigned</Trans>
                </TableHead>
                <TableHead
                  className={cn(
                    head,
                    "w-32 pe-4 text-end leading-tight whitespace-normal first-letter:uppercase @max-md/list:hidden"
                  )}
                >
                  <Trans>replies today</Trans>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {order.map((m) => (
                <MemberRow
                  key={m.id}
                  m={m}
                  load={load.get(m.id) ?? 0}
                  replies={stats?.members.find((x) => x.member_id === m.id)?.replies}
                  cursor={m.id === cursorId}
                  products={where(m)}
                  onOpen={() => navigate(`/team/${m.id}`)}
                />
              ))}
            </TableBody>
          </Table>
        )}
      </ListBox>
      <ErrorLine error={members.data ? members.error : null} className="py-2" />
      <p className="m-0 mt-3 max-w-[720px] text-caption text-muted-foreground">
        <Trans>
          Owners and admins see every inbox; agents see the inboxes they were given. Only owners and admins can invite,
          change roles or remove members.
        </Trans>
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
        ]}
      />
    </div>
  )
}

function InvitesList() {
  const { canManage } = useSession()
  const invites = useInvites(canManage)
  return (
    <>
      <ListBox testId="invites">
        {invites.isPending ? (
          <ListBoxSkeleton rows={2} />
        ) : invites.error && !invites.data ? (
          <LoadProblem error={invites.error} onRetry={() => void invites.refetch()} testId="invites-error" />
        ) : invites.data?.length === 0 ? (
          <ListBoxEmpty>
            <Trans>No pending invites.</Trans>
          </ListBoxEmpty>
        ) : (
          <Table className="table-fixed text-body">
            <TableHeader>
              <TableRow className="border-line-soft hover:bg-transparent">
                <TableHead className={cn(head, "ps-4")}>
                  <Trans>E-mail</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-24 @max-md/list:hidden")}>
                  <Trans>Role</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-28 @max-3xl/list:hidden")}>
                  <Trans>Sent</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-24 @max-xl/list:hidden")}>
                  <Trans>Status</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-60 pe-4 @max-md/list:w-40")}>
                  <span className="sr-only">
                    <Trans>Actions</Trans>
                  </span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(invites.data ?? []).map((inv) => (
                <InviteRow key={inv.id} invite={inv} />
              ))}
            </TableBody>
          </Table>
        )}
      </ListBox>
      <ErrorLine error={invites.data ? invites.error : null} className="py-2" />
      <p className="m-0 mt-3 max-w-[720px] text-caption text-muted-foreground">
        <Trans>
          An invite is accepted when the person signs in with a code sent to that address. It expires after 7 days.
        </Trans>
      </p>
    </>
  )
}

function MemberAside({ m, load }: { m: Member; load: number }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const status = useMemberStatus()
  const [inboxId] = useInboxFilter()
  const today = useStats(inboxId).data?.members.find((x) => x.member_id === m.id)
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <>
      <InfoSection title={m.name || m.email} testId="member-info">
        <KV label={t`Role`} value={text.role[m.role]} />
        <KV label={t`Status`} value={status(m).text} tone={presenceOf(m) === "here" ? "ok" : undefined} />
      </InfoSection>
      <InfoSection title={t`Today`} testId="member-tiles">
        <KV label={<span className="first-letter:uppercase">{t`open, assigned`}</span>} value={fmt.format(load)} />
        <KV
          label={<span className="first-letter:uppercase">{t`replies today`}</span>}
          value={today ? fmt.format(today.replies) : "–"}
        />
        <KV
          label={<span className="first-letter:uppercase">{t`closed today`}</span>}
          value={today ? fmt.format(today.closed) : "–"}
        />
      </InfoSection>
    </>
  )
}

export function TeamPage({ tab = "members" }: { tab?: "members" | "invites" }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { memberId } = useParams()
  const { canManage, membership } = useSession()
  const team = useTeam()
  const [email, setEmail] = useState("")
  const { members, load, where, order, available, total, unassigned } = team
  const current = memberId ? order.find((m) => m.id === memberId) : undefined
  const missing = !!memberId && members.data !== undefined && !current
  const view = canManage ? tab : "members"
  const step = (d: 1 | -1) => {
    const at = order.findIndex((m) => m.id === memberId)
    const next = order[at + d]
    if (at >= 0 && next) navigate(`/team/${next.id}`)
  }
  useHotkeys(
    { Escape: () => navigate("/team"), [SHORTCUTS.next]: () => step(1), [SHORTCUTS.previous]: () => step(-1) },
    !!memberId
  )

  if (memberId) {
    return (
      <PageLayout
        testId="team-home"
        label={t`Team`}
        aside={
          current && (
            <InfoColumn label={t`Details`}>
              <MemberAside m={current} load={load.get(current.id) ?? 0} />
            </InfoColumn>
          )
        }
      >
        <PageTop back={{ label: t`Members`, to: "/team" }} />
        <div className="max-w-[720px]">
          {current ? (
            <MemberDetail key={current.id} member={current} products={where(current)} />
          ) : missing ? (
            <p className="mt-6 text-body text-muted-foreground">
              <Trans>This member is not in the workspace any more.</Trans>
            </p>
          ) : (
            <Skeleton className="mt-6 h-40 w-full" />
          )}
        </div>
      </PageLayout>
    )
  }

  const fmt = new Intl.NumberFormat(i18n.locale)
  const queue = unassigned !== undefined && (
    <>
      {" · "}
      <Link
        to="/conversations"
        onClick={() => rememberChip("unassigned")}
        className="underline-offset-4 hover:text-foreground hover:underline"
        data-testid="team-queue"
      >
        <Trans>{fmt.format(unassigned)} open, unassigned</Trans>
      </Link>
    </>
  )

  return (
    <PageLayout testId="team-home" label={t`Team`}>
      <section className="@container/list min-w-0" aria-label={view === "invites" ? t`Invites` : t`Members`}>
        {view === "invites" ? (
          <>
            <PageTitle title={t`Invites`} sub={t`People asked to join who have not signed in yet`} testId="team-head" />
            <InviteForm email={email} setEmail={setEmail} />
            <InvitesList />
          </>
        ) : (
          <>
            <PageTitle
              title={t`Members`}
              sub={
                <span data-testid="team-lede">
                  {members.data ? (
                    <Trans>
                      {available} of {total} available now
                    </Trans>
                  ) : (
                    <Trans>Who answers, where, and who is online</Trans>
                  )}
                  {" · "}
                  {membership.workspace.name}
                  {queue}
                </span>
              }
              actions={
                canManage && (
                  <Button render={<Link to="/team/invites" />} data-testid="team-invite-open">
                    <UserPlusIcon />
                    <Trans>Invite a teammate</Trans>
                  </Button>
                )
              }
              testId="team-head"
            />
            <MembersList team={team} />
          </>
        )}
      </section>
    </PageLayout>
  )
}
