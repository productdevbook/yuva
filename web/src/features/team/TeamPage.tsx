import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeftIcon, ArrowUpRightIcon, MailIcon, UsersIcon } from "lucide-react"
import { useLayoutEffect, useRef, useState } from "react"
import { Link, useNavigate, useParams } from "react-router"

import { byPresence, useMemberStatus } from "@/app/TeamMenu"
import { colorFor, Dot, ErrorLine } from "@/components/common"
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
import { KpiTile } from "@/features/analytics/Overview"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, Overline, PageTop } from "@/features/inbox/PageFrame"
import { LoadProblem, Tiles } from "@/features/reports/parts"
import { useConversations, useCounts, useStats } from "@/features/inbox/queries"
import { Chip, Field } from "@/features/settings/ui"
import { MemberAddresses } from "@/features/team/MemberAddresses"
import { RemoveMemberDialog } from "@/features/team/RemoveMemberDialog"
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
        "relative grid size-9 shrink-0 place-items-center rounded-full text-[12px] font-semibold text-white",
        className
      )}
      style={{ backgroundColor: colorFor(m.id) }}
    >
      {initials(name)}
      <span
        className={cn(
          "absolute -end-px -bottom-px size-[11px] rounded-full border-2 border-card",
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
  const hint = useRoleHint()
  return (
    <div role="group" aria-label={label} className="flex flex-col gap-1.5">
      {ROLES.map((r) => {
        const on = r === value
        return (
          <button
            key={r}
            type="button"
            aria-pressed={on}
            disabled={disabled(r)}
            onClick={() => !on && onChange(r)}
            className={cn(
              "flex items-start gap-2.5 rounded-[10px] border bg-card px-3 py-2.5 text-start outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-55 disabled:hover:border-border",
              on && "border-brand/40 bg-brand-wash hover:border-brand/40"
            )}
            data-testid={`role-${r}`}
          >
            <span
              aria-hidden
              className={cn(
                "mt-0.5 grid size-4 shrink-0 place-items-center rounded-full border border-input bg-card",
                on && "border-brand"
              )}
            >
              {on && <span className="size-2 rounded-full bg-brand" />}
            </span>
            <span className="flex min-w-0 flex-col leading-[1.35]">
              <span className="text-body font-semibold">{text.role[r]}</span>
              <span className="text-caption text-faint">{hint(r)}</span>
            </span>
          </button>
        )
      })}
    </div>
  )
}

function InviteDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
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
          <Field label={<Trans>Role</Trans>}>
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
  const sent = formatDate(invite.created_at, i18n.locale)
  const expires = formatDate(invite.expires_at, i18n.locale)
  const role = text.role[invite.role]
  const language = locales[invite.locale]
  const expired = new Date(invite.expires_at).getTime() < now
  return (
    <li
      className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-3 last:border-b-0"
      data-testid="invite-row"
    >
      <span aria-hidden className="grid size-9 shrink-0 place-items-center rounded-full bg-muted text-muted-foreground">
        <MailIcon className="size-4" />
      </span>
      <span className="flex min-w-0 flex-1 basis-56 flex-col leading-[1.35]">
        <span className="flex min-w-0 items-center gap-1.5">
          <bdi className="truncate text-[13.5px] font-semibold">{invite.email}</bdi>
          {expired && (
            <span className="inline-flex h-5 shrink-0 items-center rounded-md bg-warning/10 px-[7px] text-[11px] font-semibold text-warning">
              <Trans>Expired</Trans>
            </span>
          )}
        </span>
        <span className="text-caption text-faint">
          <Trans>
            {role} · {language} · sent {sent} · expires {expires}
          </Trans>
        </span>
      </span>
      {asking ? (
        <span className="flex flex-wrap items-center gap-2">
          <span className="text-small">
            <Trans>Withdraw this invite?</Trans>
          </span>
          <Button
            variant="destructive"
            className={look.btnSm}
            onClick={() => withdraw.mutate()}
            disabled={withdraw.isPending}
            aria-label={t`Withdraw the invite for ${invite.email}`}
          >
            <Trans>Withdraw</Trans>
          </Button>
          <Button variant="outline" className={look.btnSm} onClick={() => setAsking(false)}>
            <Trans>Cancel</Trans>
          </Button>
        </span>
      ) : (
        <span className="flex gap-1.5">
          <Button
            variant="outline"
            className={cn(look.btnSm, "bg-card")}
            onClick={() => resend.mutate()}
            disabled={resend.isPending || resend.isSuccess}
            aria-label={t`Send the invite for ${invite.email} again`}
          >
            {resend.isSuccess ? <Trans>Sent</Trans> : <Trans>Resend</Trans>}
          </Button>
          <Button
            variant="outline"
            className={cn(look.btnSm, "bg-card")}
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
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="flex w-full flex-col gap-3 px-4 pt-3 pb-16">
        {invites.isPending ? (
          <div className="overflow-hidden rounded-[14px] border bg-card" aria-busy="true" data-testid="invites-loading">
            {[0, 1, 2].map((i) => (
              <div key={i} className="flex h-[62px] items-center gap-3 border-b px-4 last:border-b-0">
                <Skeleton className="size-9 shrink-0 rounded-full" />
                <div className="flex flex-1 flex-col gap-1.5">
                  <Skeleton className="h-3 w-48 max-w-full" />
                  <Skeleton className="h-2.5 w-64 max-w-full" />
                </div>
              </div>
            ))}
          </div>
        ) : invites.error && !invites.data ? (
          <LoadProblem error={invites.error} onRetry={() => void invites.refetch()} testId="invites-error" />
        ) : (
          <>
            <ErrorLine error={invites.error} />
            {invites.data?.length === 0 ? (
              <div
                className="mx-auto flex max-w-sm flex-col items-center gap-3 px-2 py-14 text-center"
                data-testid="invites"
              >
                <span className="grid size-12 place-items-center rounded-xl bg-brand-wash text-brand">
                  <MailIcon className="size-5" aria-hidden />
                </span>
                <p className="text-body font-semibold">
                  <Trans>No pending invites.</Trans>
                </p>
                <Button className={look.btnInkSm} onClick={onInvite}>
                  <Trans>Invite a teammate</Trans>
                </Button>
              </div>
            ) : (
              <ul className="overflow-hidden rounded-[14px] border bg-card" data-testid="invites">
                {(invites.data ?? []).map((inv) => (
                  <InviteRow key={inv.id} invite={inv} />
                ))}
              </ul>
            )}
            <p className="max-w-2xl px-1 text-caption text-faint">
              <Trans>
                An invite is accepted when the person signs in with a code sent to that address. It expires after 7
                days.
              </Trans>
            </p>
          </>
        )}
      </div>
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

function DetailSection({
  title,
  count,
  children,
  last,
}: {
  title: React.ReactNode
  count?: number
  children: React.ReactNode
  last?: boolean
}) {
  return (
    <section className={cn("flex flex-col gap-2.5 px-5 py-4", !last && "border-b")}>
      <Overline className="flex items-center gap-2">
        {title}
        {count !== undefined && (
          <span className="rounded-full bg-muted px-1.5 text-[11px] leading-4 font-medium tracking-normal text-muted-foreground tabular-nums">
            {count}
          </span>
        )}
      </Overline>
      {children}
    </section>
  )
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
    <div className="flex min-w-0 flex-col" data-testid="member-page">
      <div className="flex flex-col gap-4 border-b px-5 pt-5 pb-4">
        <div className="flex items-start gap-3.5">
          <PresenceFace m={m} className="size-14 text-[18px]" />
          <div className="flex min-w-0 flex-1 flex-col gap-0.5 leading-[1.35]">
            <h2 className="text-[18px] leading-6 font-semibold break-words" data-testid="member-name">
              {you ? t`${name} (you)` : name}
            </h2>
            {m.title && (
              <span className="text-small text-muted-foreground" data-testid="member-title">
                {m.title}
              </span>
            )}
            <bdi className="text-small break-words text-faint">{m.email}</bdi>
            <span className={cn("text-caption text-faint", s.conversationId && "text-success")}>
              {text.role[m.role]} · {s.text}
            </span>
          </div>
        </div>
        {s.conversationId && !you && (
          <Button
            variant="outline"
            className={cn(look.btn, "justify-between bg-card")}
            render={<Link to={`/conversations/${s.conversationId}`} />}
            data-testid="member-viewing"
          >
            <Trans>Open what they see</Trans>
            <ArrowUpRightIcon />
          </Button>
        )}
        <div
          className="grid grid-cols-3 divide-x rounded-xl border bg-card rtl:divide-x-reverse"
          data-testid="member-tiles"
        >
          {[
            { v: fmt.format(load), l: <Trans>open, assigned</Trans> },
            { v: today ? fmt.format(today.replies) : "–", l: <Trans>replies today</Trans> },
            { v: today ? fmt.format(today.closed) : "–", l: <Trans>closed today</Trans> },
          ].map((x, i) => (
            <div key={i} className="flex min-w-0 flex-col gap-0.5 px-3 py-2.5">
              <b className="text-[18px] leading-6 font-semibold tabular-nums">{x.v}</b>
              <span className="text-caption text-faint">{x.l}</span>
            </div>
          ))}
        </div>
      </div>
      <DetailSection title={<Trans>Role</Trans>}>
        {canManage ? (
          <RoleChips
            value={m.role}
            onChange={(r) => setRole.mutate(r)}
            disabled={(r) => rolesLocked || setRole.isPending || (r === "owner" && !isOwner)}
            label={t`Role of ${name}`}
          />
        ) : (
          <div className="flex flex-col gap-1">
            <Chip className="w-fit">{text.role[m.role]}</Chip>
            <span className="text-caption text-faint">{hint(m.role)}</span>
          </div>
        )}
        {canManage && roleNote && <span className="text-caption text-faint">{roleNote}</span>}
        <ErrorLine error={setRole.error} />
      </DetailSection>
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
      <DetailSection
        title={<Trans>Open conversations assigned</Trans>}
        count={open.isPending ? undefined : items.length}
      >
        <ConversationRows items={items} roomy pending={open.isPending} />
        {open.hasNextPage && (
          <Button
            variant="outline"
            className={cn(look.btnSm, "self-center bg-card")}
            onClick={() => void open.fetchNextPage()}
            disabled={open.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
      </DetailSection>
      {canManage && <MemberAddresses memberId={m.id} name={name} />}
      {canManage && (
        <section className="px-5 py-4">
          <div className="flex flex-wrap items-center gap-3 rounded-xl border border-destructive/30 bg-card px-4 py-3.5">
            <span className="flex min-w-48 flex-1 flex-col leading-[1.35]">
              <span className="text-[13.5px] font-semibold">
                <Trans>Remove from the workspace</Trans>
              </span>
              <span className="text-caption text-faint">{removeNote}</span>
            </span>
            {asking ? (
              <span className="flex flex-wrap items-center gap-2">
                <span className="text-small">
                  <Trans>Remove {name}?</Trans>
                </span>
                <Button
                  variant="destructive"
                  className={look.btnSm}
                  onClick={() => remove.mutate()}
                  disabled={remove.isPending}
                  aria-label={t`Remove ${name}`}
                >
                  <Trans>Remove</Trans>
                </Button>
                <Button variant="outline" className={look.btnSm} onClick={() => setAsking(false)}>
                  <Trans>Cancel</Trans>
                </Button>
              </span>
            ) : (
              <Button
                variant="outline"
                className={cn(look.btnSm, "bg-card text-destructive")}
                disabled={!removable}
                onClick={() => (owned.length > 0 ? setHandling(true) : setAsking(true))}
                aria-label={t`Remove ${name}`}
              >
                <Trans>Remove</Trans>
              </Button>
            )}
            <ErrorLine error={remove.error} className="basis-full" />
          </div>
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

function useNarrow(ref: React.RefObject<HTMLElement | null>, limit: number) {
  const [narrow, setNarrow] = useState(false)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setNarrow(el.clientWidth < limit)
    measure()
    const watch = new ResizeObserver(measure)
    watch.observe(el)
    return () => watch.disconnect()
  }, [ref, limit])
  return narrow
}

const GRID =
  "grid grid-cols-[minmax(0,1fr)_72px] items-center gap-3 px-3 @min-[640px]:grid-cols-[minmax(200px,2fr)_minmax(110px,1.3fr)_120px_80px] @min-[900px]:grid-cols-[minmax(0,1fr)_110px] @min-[1240px]:grid-cols-[minmax(230px,2.2fr)_minmax(130px,1.3fr)_150px_96px]"
const wideCol = "hidden @min-[640px]:flex @min-[900px]:hidden @min-[1240px]:flex"

function MemberRow({
  m,
  load,
  peak,
  replies,
  current,
  products,
}: {
  m: Member
  load: number
  peak: number
  replies: number | undefined
  current: boolean
  products: { id: string; name: string; color?: string }[]
}) {
  const { t, i18n } = useLingui()
  const status = useMemberStatus()
  const { membership } = useSession()
  const text = useEnumText()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const name = m.name || m.email
  const s = status(m)
  return (
    <li data-testid="team-member">
      <Link
        to={`/team/${m.id}`}
        aria-current={current ? "page" : undefined}
        className={cn(
          GRID,
          "relative min-h-[60px] border-b border-b-muted py-2 outline-none transition-colors hover:bg-muted/60 focus-visible:bg-muted/60",
          current &&
            "rounded-[10px] border-b-transparent bg-card shadow-[0_0_0_1px_var(--border)] before:absolute before:inset-y-3 before:start-1 before:w-[3px] before:rounded-[3px] before:bg-brand hover:bg-card"
        )}
      >
        <span className="flex min-w-0 items-center gap-3">
          <PresenceFace m={m} />
          <span className="flex min-w-0 flex-col leading-[1.35]">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="truncate text-[13.5px] font-semibold">
                {m.id === membership.member_id ? t`${name} (you)` : name}
              </span>
              <Chip>{text.role[m.role]}</Chip>
            </span>
            <span className={cn("truncate text-caption text-faint", s.conversationId && "text-success")}>{s.text}</span>
          </span>
        </span>
        <span className={cn(wideCol, "min-w-0 flex-wrap gap-x-2.5 gap-y-1 overflow-hidden")}>
          {products.slice(0, 3).map((p) => (
            <span key={p.id} className="inline-flex min-w-0 items-center gap-[5px] text-caption text-muted-foreground">
              <Dot color={p.color} className="size-2 rounded-[3px]" />
              <span className="truncate">{p.name}</span>
            </span>
          ))}
        </span>
        <span
          className={cn("flex items-center gap-2 text-body tabular-nums", !load && "text-faint")}
          data-testid="team-load"
        >
          <span className="min-w-6">{fmt.format(load)}</span>
          <span
            className="hidden h-1.5 max-w-20 flex-1 overflow-hidden rounded-full bg-muted @min-[640px]:block"
            aria-hidden
          >
            <i
              className="block h-full rounded-full bg-brand"
              style={{ width: `${peak ? Math.round((load / peak) * 100) : 0}%` }}
            />
          </span>
        </span>
        <span className={cn(wideCol, "text-small text-muted-foreground tabular-nums")}>
          {replies === undefined ? "–" : fmt.format(replies)}
        </span>
      </Link>
    </li>
  )
}

function ListSkeleton() {
  return (
    <div className="flex flex-col" aria-busy="true" data-testid="team-loading">
      {[62, 48, 70, 54, 66].map((w, i) => (
        <div key={i} className="flex h-[60px] items-center gap-3 border-b border-b-muted px-3">
          <Skeleton className="size-9 shrink-0 rounded-full" />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-3" style={{ width: `${w}%`, maxWidth: 240 }} />
            <Skeleton className="h-2.5 w-35" />
          </div>
        </div>
      ))}
    </div>
  )
}

export function TeamPage({ tab = "members" }: { tab?: "members" | "invites" }) {
  const { t, i18n } = useLingui()
  const frame = useRef<HTMLElement>(null)
  const single = useNarrow(frame, 900)
  const { memberId } = useParams()
  const { canManage, workspaceId: ws } = useSession()
  const members = useMembers()
  const counts = useCounts().data
  const [inboxId] = useInboxFilter()
  const stats = useStats(inboxId).data
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
  const peak = Math.max(0, ...order.map((m) => load.get(m.id) ?? 0))
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
  const current = memberId ? order.find((m) => m.id === memberId) : single ? undefined : order[0]
  const missing = !!memberId && members.data !== undefined && !current

  const tiles = (
    <Tiles className="px-6 pt-4 phone:px-4" testId="team-summary">
      <KpiTile
        label={
          <span className="block first-letter:uppercase">
            <Trans>available now</Trans>
          </span>
        }
        value={
          members.data ? (
            <Trans>
              {available} of {total}
            </Trans>
          ) : (
            "–"
          )
        }
        wrapLabel
      />
      <KpiTile
        label={
          <span className="block first-letter:uppercase">
            <Trans>open, unassigned</Trans>
          </span>
        }
        value={num(unassigned)}
        hint={
          <Link
            to="/conversations"
            onClick={() => rememberChip("unassigned")}
            className="text-brand underline-offset-2 outline-none hover:underline focus-visible:underline"
            data-testid="team-queue"
          >
            <Trans>Open the unassigned queue</Trans>
          </Link>
        }
        wrapLabel
      />
      <KpiTile
        label={
          <span className="block first-letter:uppercase">
            <Trans>open, assigned</Trans>
          </span>
        }
        value={num(assigned)}
        wrapLabel
      />
    </Tiles>
  )

  const table = (
    <div className="min-h-0 flex-1 overflow-auto px-4 pt-3 pb-6" data-testid="team-panel">
      <div
        className={cn(
          GRID,
          "sticky top-0 z-[2] hidden h-[34px] border-b bg-surface text-[11px] font-semibold tracking-[0.05em] text-faint uppercase @min-[640px]:grid"
        )}
        aria-hidden
      >
        <span>
          <Trans>Person</Trans>
        </span>
        <span className={wideCol}>
          <Trans>Products</Trans>
        </span>
        <span>
          <Trans>open, assigned</Trans>
        </span>
        <span className={wideCol}>
          <Trans>replies today</Trans>
        </span>
      </div>
      {members.isPending ? (
        <ListSkeleton />
      ) : members.error && !members.data ? (
        <LoadProblem error={members.error} onRetry={() => void members.refetch()} testId="team-error" />
      ) : order.length === 0 ? (
        <div className="mx-auto flex max-w-sm flex-col items-center gap-3 px-2 py-14 text-center">
          <span className="grid size-12 place-items-center rounded-xl bg-brand-wash text-brand">
            <UsersIcon className="size-5" aria-hidden />
          </span>
          <p className="text-body text-faint">
            <Trans>Nobody on the team works on this product yet.</Trans>
          </p>
        </div>
      ) : (
        <>
          <ErrorLine error={members.error} className="py-2" />
          <ul className="flex flex-col" data-testid="team-members">
            {order.map((m) => (
              <MemberRow
                key={m.id}
                m={m}
                load={load.get(m.id) ?? 0}
                peak={peak}
                replies={stats?.members.find((x) => x.member_id === m.id)?.replies}
                current={m.id === current?.id}
                products={where(m)}
              />
            ))}
          </ul>
        </>
      )}
      <p className="mx-3 mt-3.5 text-caption text-faint">
        <Trans>
          Owners and admins see every inbox; agents see the inboxes they were given. Only owners and admins can invite,
          change roles or remove members.
        </Trans>
      </p>
    </div>
  )

  const detail = current ? (
    <MemberDetail key={current.id} member={current} load={load.get(current.id) ?? 0} products={where(current)} />
  ) : missing ? (
    <p className="p-5 text-body text-muted-foreground">
      <Trans>This member is not in the workspace any more.</Trans>
    </p>
  ) : members.isPending ? (
    <Skeleton className="m-5 h-40 rounded-xl" />
  ) : null

  return (
    <main
      ref={frame}
      className="@container flex h-svh min-h-0 min-w-0 flex-1 flex-col bg-surface max-md:h-[calc(100svh-49px)] phone:h-auto"
      data-testid="team-home"
    >
      <PageTop
        title={<Trans>Team</Trans>}
        sub={<Trans>Who answers, where, and who is online</Trans>}
        actions={
          canManage && (
            <Button className={look.btnInk} onClick={() => setInviting(true)} data-testid="team-invite-open">
              <Trans>Invite</Trans>
            </Button>
          )
        }
        tabs={
          canManage && (
            <FrameTabs
              label={t`Team`}
              value={tabKey}
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
      ) : single ? (
        memberId ? (
          <div className="flex min-h-0 flex-1 flex-col">
            <Link
              to="/team"
              className="inline-flex w-fit items-center gap-1.5 px-4 pt-3 text-body text-faint hover:text-foreground"
            >
              <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
              <Trans>Team</Trans>
            </Link>
            <div className="min-h-0 flex-1 overflow-y-auto">{detail}</div>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col">
            {tiles}
            {table}
          </div>
        )
      ) : (
        <div className="flex min-h-0 flex-1">
          <section className="flex min-h-0 min-w-0 flex-1 flex-col" aria-label={t`Team`}>
            {tiles}
            {table}
          </section>
          <aside
            className="flex min-h-0 w-[440px] shrink-0 flex-col overflow-y-auto border-s bg-card @max-[1240px]:w-[380px]"
            aria-label={t`Member`}
          >
            {detail}
          </aside>
        </div>
      )}
      {canManage && <InviteDialog open={inviting} onOpenChange={setInviting} />}
    </main>
  )
}
