import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeftIcon, ArrowUpRightIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useParams } from "react-router"

import { byPresence, useMemberStatus } from "@/app/TeamMenu"
import { colorFor, ErrorLine } from "@/components/common"
import { formatDate, initials, ROLES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { ConversationRows } from "@/features/contact/ContactDetails"
import { rememberChip } from "@/features/inbox/ChatList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useConversations, useCounts, useStats } from "@/features/inbox/queries"
import {
  Chip,
  Field,
  headButton,
  Page,
  PageHeader,
  PageTabs,
  ProductTag,
  Section,
  StatTile,
} from "@/features/settings/ui"
import { MemberAddresses } from "@/features/team/MemberAddresses"
import { RemoveMemberDialog } from "@/features/team/RemoveMemberDialog"
import { useIsPhone } from "@/hooks/use-media-query"
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
        "relative grid size-10 shrink-0 place-items-center rounded-full text-[14px] font-semibold text-white",
        presence === "offline" && "opacity-70",
        className
      )}
      style={{ backgroundColor: colorFor(m.id) }}
    >
      {initials(name)}
      <span
        className={cn(
          "absolute -end-px -bottom-px size-[11px] rounded-full border-2 border-background",
          presence === "here" && "bg-success",
          presence === "away" && "bg-warning",
          presence === "offline" && "bg-input"
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

function RoleChips({
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
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-1.5">
      {ROLES.map((r) => (
        <Button
          key={r}
          type="button"
          variant="outline"
          size="sm"
          aria-pressed={r === value}
          disabled={disabled(r)}
          className={cn("rounded-lg px-3 font-normal", r === value && "border-foreground bg-muted font-medium")}
          onClick={() => r !== value && onChange(r)}
          data-testid={`role-${r}`}
        >
          {text.role[r]}
        </Button>
      ))}
    </div>
  )
}

function InviteDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const hint = useRoleHint()
  const { membership, workspaceId: ws } = useSession()
  const [email, setEmail] = useState("")
  const [role, setRole] = useState<Role>("agent")
  const [locale, setLocale] = useState<Locale>(i18n.locale === "tr" ? "tr" : "en")
  const create = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/invites", { body: { email: email.trim(), role, locale } })),
    onSuccess: () => {
      setEmail("")
      setRole("agent")
      void qc.invalidateQueries({ queryKey: keys.invites(ws) })
      onOpenChange(false)
    },
  })
  const isOwner = membership.role === "owner"
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="invite-dialog">
        <form
          className="grid gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Invite a teammate</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                An invite is accepted when the person signs in with a code sent to that address. It expires after 7
                days.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>E-mail address</Trans>} htmlFor="invite-email">
            <Input
              id="invite-email"
              type="email"
              required
              autoFocus
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={t`teammate@company.com`}
            />
          </Field>
          <Field label={<Trans>Role</Trans>} hint={hint(role)}>
            <RoleChips value={role} onChange={setRole} disabled={(r) => r === "owner" && !isOwner} label={t`Role`} />
          </Field>
          <Field label={<Trans>Language of the invite</Trans>}>
            <Select value={locale} onValueChange={(v) => setLocale(v as Locale)} items={locales}>
              <SelectTrigger aria-label={t`Invite language`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(locales).map(([k, v]) => (
                  <SelectItem key={k} value={k}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={create.isPending || !email.trim()} data-testid="invite-send">
              <Trans>Send invite</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function InviteRow({ invite }: { invite: Invite }) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const text = useEnumText()
  const { workspaceId: ws } = useSession()
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
  const sent = formatDate(invite.created_at, i18n.locale)
  const expires = formatDate(invite.expires_at, i18n.locale)
  const role = text.role[invite.role]
  const language = locales[invite.locale]
  return (
    <li className="flex flex-wrap items-center gap-3 border-b px-4 py-3 last:border-b-0" data-testid="invite-row">
      <span
        aria-hidden
        className="grid size-9 shrink-0 place-items-center rounded-full text-[13px] font-semibold text-white"
        style={{ backgroundColor: colorFor(invite.email) }}
      >
        {initials(invite.email.split("@")[0])}
      </span>
      <span className="flex min-w-0 flex-1 basis-56 flex-col leading-[1.35]">
        <span className="truncate font-medium">{invite.email}</span>
        <span className="text-caption text-faint">
          <Trans>
            {role} · {language} · sent {sent} · expires {expires}
          </Trans>
        </span>
      </span>
      {asking ? (
        <span className="flex flex-wrap items-center gap-2">
          <span>
            <Trans>Withdraw this invite?</Trans>
          </span>
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
        <span className="flex gap-2">
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
      <ErrorLine error={resend.error ?? withdraw.error} className="basis-full" />
    </li>
  )
}

function InvitesTab({ invites, onInvite }: { invites: ReturnType<typeof useInvites>; onInvite: () => void }) {
  return (
    <div className="flex max-w-[880px] flex-col gap-4">
      <p className="max-w-2xl text-muted-foreground">
        <Trans>
          An invite is accepted when the person signs in with a code sent to that address. It expires after 7 days.
        </Trans>
      </p>
      <ErrorLine error={invites.error} />
      {invites.isPending ? (
        <Skeleton className="h-20 rounded-xl" />
      ) : (
        <ul className="overflow-hidden rounded-xl border bg-card" data-testid="invites">
          {(invites.data ?? []).map((inv) => (
            <InviteRow key={inv.id} invite={inv} />
          ))}
          {invites.data?.length === 0 && (
            <li className="flex flex-col items-center gap-3 px-4 py-10 text-center text-muted-foreground">
              <Trans>No pending invites.</Trans>
              <Button size="sm" onClick={onInvite}>
                <Trans>Invite a teammate</Trans>
              </Button>
            </li>
          )}
        </ul>
      )}
    </div>
  )
}

function useInvites(enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.invites(ws),
    queryFn: () => unwrap(api.GET("/v1/invites")).then((r) => r.items),
    enabled,
  })
}

type Detail = {
  member: Member
  load: number
  products: { id: string; name: string; color?: string }[]
}

function MemberDetail({ member: m, load, products }: Detail) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const hint = useRoleHint()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const status = useMemberStatus()
  const { membership, canManage, workspaceId: ws } = useSession()
  const [inboxId] = useInboxFilter()
  const stats = useStats(inboxId).data
  const open = useConversations({ assignee: m.id, status: ["open"], purpose: "support" })
  const [asking, setAsking] = useState(false)
  const [handling, setHandling] = useState(false)
  const owned = (useAllInboxes().data ?? []).filter((i) => i.owner_member_id === m.id)
  const isOwner = membership.role === "owner"
  const you = m.id === membership.member_id
  const name = m.name || m.email
  const s = status(m)
  const today = stats?.members.find((x) => x.member_id === m.id)
  const fmt = new Intl.NumberFormat(i18n.locale)
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
    <div className="flex min-w-0 flex-col gap-5" data-testid="member-page">
      <div className="flex flex-wrap items-center gap-4">
        <PresenceFace m={m} className="size-14 text-[18px]" />
        <div className="flex min-w-48 flex-1 flex-col leading-[1.35]">
          <h2 className="text-title break-words" data-testid="member-name">
            {you ? t`${name} (you)` : name}
          </h2>
          {m.title && (
            <span className="text-muted-foreground" data-testid="member-title">
              {m.title}
            </span>
          )}
          <span className="text-muted-foreground">{m.email}</span>
          <span className="text-caption text-faint">
            {text.role[m.role]} · {s.text}
          </span>
        </div>
        {s.conversationId && !you && (
          <Button
            variant="outline"
            render={<Link to={`/conversations/${s.conversationId}`} />}
            data-testid="member-viewing"
          >
            <Trans>Open what they see</Trans>
            <ArrowUpRightIcon />
          </Button>
        )}
      </div>
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span className="w-16 text-caption text-faint">
            <Trans>Role</Trans>
          </span>
          {canManage ? (
            <RoleChips
              value={m.role}
              onChange={(r) => setRole.mutate(r)}
              disabled={(r) => rolesLocked || setRole.isPending || (r === "owner" && !isOwner)}
              label={t`Role of ${name}`}
            />
          ) : (
            <Chip>{text.role[m.role]}</Chip>
          )}
          {canManage && roleNote && <span className="text-caption text-faint">{roleNote}</span>}
          {canManage && !rolesLocked && (
            <span className="basis-full text-caption text-faint ps-19">{hint(m.role)}</span>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span className="w-16 text-caption text-faint">
            <Trans>Products</Trans>
          </span>
          {products.map((p) => (
            <ProductTag key={p.id} name={p.name} color={p.color} />
          ))}
        </div>
        <ErrorLine error={setRole.error} />
      </div>
      <div className="grid grid-cols-3 gap-3 phone:grid-cols-1" data-testid="member-tiles">
        <StatTile label={<Trans>open, assigned</Trans>} value={fmt.format(load)} />
        <StatTile label={<Trans>replies today</Trans>} value={today ? fmt.format(today.replies) : "–"} />
        <StatTile label={<Trans>closed today</Trans>} value={today ? fmt.format(today.closed) : "–"} />
      </div>
      <Section title={<Trans>Open conversations assigned</Trans>}>
        <ConversationRows items={items} roomy pending={open.isPending} />
        {open.hasNextPage && (
          <Button
            variant="ghost"
            size="sm"
            className="self-center"
            onClick={() => void open.fetchNextPage()}
            disabled={open.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
      </Section>
      {canManage && <MemberAddresses memberId={m.id} name={name} />}
      {canManage && (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card px-4 py-3.5">
          <span className="flex min-w-48 flex-1 flex-col leading-[1.35]">
            <span className="font-medium">
              <Trans>Remove from the workspace</Trans>
            </span>
            <span className="text-caption text-faint">{removeNote}</span>
          </span>
          {asking ? (
            <span className="flex flex-wrap items-center gap-2">
              <span>
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
        </div>
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

export function TeamPage({ tab = "members" }: { tab?: "members" | "invites" }) {
  const { t, i18n } = useLingui()
  const phone = useIsPhone()
  const { memberId } = useParams()
  const { membership, canManage, workspaceId: ws } = useSession()
  const members = useMembers()
  const status = useMemberStatus()
  const counts = useCounts().data
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const [inviting, setInviting] = useState(false)
  const invites = useInvites(canManage)
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
  const where = (m: Member) =>
    (m.role === "agent" ? inboxes.filter((_, i) => granted[i]?.data?.some((g) => g.id === m.id)) : inboxes).map(
      (i) => ({
        id: i.id,
        name: i.name,
        color: i.branding.color,
      })
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
  const num = (n: number | undefined) => (n === undefined ? "–" : fmt.format(n))
  const tabKey = canManage ? tab : "members"
  const current = memberId ? order.find((m) => m.id === memberId) : phone ? undefined : order[0]
  const missing = !!memberId && members.data !== undefined && !current

  const list = (
    <div className="flex min-w-0 flex-col overflow-hidden rounded-xl border bg-card" data-testid="team-panel">
      <div className="flex flex-col gap-3 border-b px-4 py-4">
        <span className="text-caption font-medium tracking-wider text-faint uppercase">
          <Trans>Who is here</Trans>
        </span>
        <div className="grid grid-cols-3 gap-2" data-testid="team-summary">
          <span className="flex flex-col leading-[1.2]">
            <b className="text-title tabular-nums">
              {members.data ? (
                <Trans>
                  {available} of {total}
                </Trans>
              ) : (
                "–"
              )}
            </b>
            <span className="text-caption text-faint">
              <Trans>available now</Trans>
            </span>
          </span>
          <span className="flex flex-col leading-[1.2]">
            <b className="text-title tabular-nums">{num(unassigned)}</b>
            <span className="text-caption text-faint">
              <Trans>open, unassigned</Trans>
            </span>
          </span>
          <span className="flex flex-col leading-[1.2]">
            <b className="text-title tabular-nums">{num(assigned)}</b>
            <span className="text-caption text-faint">
              <Trans>open, assigned</Trans>
            </span>
          </span>
        </div>
        <Link
          to="/conversations"
          onClick={() => rememberChip("unassigned")}
          className="w-fit text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
          data-testid="team-queue"
        >
          <Trans>Open the unassigned queue</Trans>
        </Link>
      </div>
      {members.isPending && <Skeleton className="m-3 h-14" />}
      <ErrorLine error={members.error} className="px-4 py-2" />
      <ul className="flex flex-col" data-testid="team-members">
        {order.map((m) => {
          const name = m.name || m.email
          const open = load.get(m.id) ?? 0
          const on = m.id === current?.id
          return (
            <li key={m.id} className="border-b last:border-b-0" data-testid="team-member">
              <Link
                to={`/team/${m.id}`}
                aria-current={on ? "page" : undefined}
                className="flex items-center gap-3 px-4 py-3 outline-none transition-colors hover:bg-surface focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset aria-[current=page]:bg-muted aria-[current=page]:shadow-[inset_2px_0_0_var(--brand)]"
              >
                <PresenceFace m={m} />
                <span className="flex min-w-0 flex-1 flex-col leading-[1.35]">
                  <span className="flex items-center gap-2">
                    <span className="truncate font-medium">
                      {m.id === membership.member_id ? t`${name} (you)` : name}
                    </span>
                    <RoleLabel role={m.role} />
                  </span>
                  <span className="truncate text-caption text-faint">{status(m).text}</span>
                </span>
                <span className="flex flex-col items-end leading-[1.2]" data-testid="team-load">
                  <b className={cn("tabular-nums", !open && "font-normal text-faint")}>{fmt.format(open)}</b>
                  <span className="text-caption text-faint">
                    <Trans>open</Trans>
                  </span>
                </span>
              </Link>
            </li>
          )
        })}
        {members.data && order.length === 0 && (
          <li className="px-4 py-8 text-center text-faint">
            <Trans>Nobody on the team works on this product yet.</Trans>
          </li>
        )}
      </ul>
      <p className="border-t px-4 py-2.5 text-caption text-faint">
        <Trans>
          Owners and admins see every inbox; agents see the inboxes they were given. Only owners and admins can invite,
          change roles or remove members.
        </Trans>
      </p>
    </div>
  )

  const detail = current ? (
    <MemberDetail member={current} load={load.get(current.id) ?? 0} products={where(current)} />
  ) : missing ? (
    <p className="text-body text-muted-foreground">
      <Trans>This member is not in the workspace any more.</Trans>
    </p>
  ) : null

  return (
    <Page testId="team-home">
      <PageHeader
        back={false}
        title={<Trans>Team</Trans>}
        description={<Trans>Who answers, where, and who is online</Trans>}
        action={
          canManage && (
            <Button className={headButton.primary} onClick={() => setInviting(true)} data-testid="team-invite-open">
              <Trans>Invite</Trans>
            </Button>
          )
        }
        tabs={
          canManage && (
            <PageTabs
              label={t`Team`}
              current={tabKey}
              items={[
                { key: "members", to: "/team", label: <Trans>Members</Trans>, testId: "team-tab-members" },
                {
                  key: "invites",
                  to: "/team/invites",
                  label: <Trans>Invites</Trans>,
                  count: invites.data?.length,
                  testId: "team-tab-invites",
                },
              ]}
            />
          )
        }
      />
      {tabKey === "invites" ? (
        <InvitesTab invites={invites} onInvite={() => setInviting(true)} />
      ) : phone ? (
        memberId ? (
          <div className="flex flex-col gap-3">
            <Link
              to="/team"
              className="inline-flex w-fit items-center gap-1.5 text-body text-faint hover:text-foreground"
            >
              <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
              <Trans>Team</Trans>
            </Link>
            {members.isPending ? <Skeleton className="h-24 rounded-xl" /> : detail}
          </div>
        ) : (
          list
        )
      ) : (
        <div className="grid grid-cols-[minmax(320px,400px)_minmax(0,1fr)] items-start gap-6 max-xl:grid-cols-[320px_minmax(0,1fr)]">
          {list}
          {members.isPending ? <Skeleton className="h-40 rounded-xl" /> : detail}
        </div>
      )}
      {canManage && <InviteDialog open={inviting} onOpenChange={setInviting} />}
    </Page>
  )
}

function RoleLabel({ role }: { role: Role }) {
  const text = useEnumText()
  return <Chip>{text.role[role]}</Chip>
}
