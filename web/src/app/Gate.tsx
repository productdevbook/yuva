import { Trans } from "@lingui/react/macro"
import { BuildingIcon, ChevronRightIcon, LogOutIcon, RefreshCwIcon } from "lucide-react"
import { useCallback, useEffect } from "react"
import { Navigate, useLocation, useNavigate } from "react-router"

import { AppShell } from "@/app/AppShell"
import { EmptyState } from "@/components/common"
import { Button } from "@/components/ui/button"
import { AuthLayout } from "@/features/auth/AuthLayout"
import { DeleteAccountButton } from "@/features/auth/DeleteAccountButton"
import { activate, i18n } from "@/i18n"
import type { Me } from "@/lib/api"
import { signInPath } from "@/lib/next"
import { pickMembership, SessionProvider, useMe, useSignOut, useWorkspaceChoice } from "@/lib/session"

const heading = "text-[1.75rem] leading-tight font-semibold tracking-[-0.03em] sm:text-[2rem]"

function NoWorkspace({ email }: { email: string }) {
  const signOut = useSignOut()
  const navigate = useNavigate()
  return (
    <AuthLayout>
      <span className="mb-5 grid size-11 place-items-center rounded-full border text-faint">
        <BuildingIcon className="size-5" />
      </span>
      <h1 className={heading}>
        <Trans>You are not a member of any workspace</Trans>
      </h1>
      <p className="mt-3 leading-relaxed text-muted-foreground">
        <Trans>Ask an owner or admin of your workspace to invite you.</Trans>
      </p>
      <p className="mt-4 rounded-xl bg-surface px-3.5 py-2.5 text-sm break-words">{email}</p>
      <div className="mt-8 flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="lg"
          onClick={async () => {
            await signOut()
            navigate("/sign-in", { replace: true })
          }}
        >
          <LogOutIcon />
          <Trans>Sign out</Trans>
        </Button>
        <DeleteAccountButton email={email} size="lg" />
      </div>
    </AuthLayout>
  )
}

function WorkspacePicker({ me, onChoose }: { me: Me; onChoose: (id: string) => void }) {
  return (
    <AuthLayout>
      <h1 className={`mb-6 ${heading}`}>
        <Trans>Choose a workspace</Trans>
      </h1>
      <ul className="flex flex-col gap-2">
        {me.memberships.map((m) => (
          <li key={m.workspace.id}>
            <Button
              variant="plain"
              size="auto"
              onClick={() => onChoose(m.workspace.id)}
              className="flex w-full gap-3 rounded-2xl border-border px-4 py-3.5 text-base hover:border-input hover:bg-surface"
            >
              <span className="flex-1 truncate font-medium">{m.workspace.name}</span>
              <ChevronRightIcon className="size-4 text-faint" />
            </Button>
          </li>
        ))}
      </ul>
    </AuthLayout>
  )
}

export function Gate() {
  const me = useMe()
  const location = useLocation()
  const navigate = useNavigate()
  const [chosen, choose] = useWorkspaceChoice()
  const linked = new URLSearchParams(location.search).get("workspace_id")
  const linkedMember = !!linked && !!me.data?.memberships.some((m) => m.workspace.id === linked)
  useEffect(() => {
    if (linkedMember && linked !== chosen) choose(linked)
  }, [linkedMember, linked, chosen, choose])
  const personLocale = me.data?.person.locale
  useEffect(() => {
    if (personLocale && personLocale !== i18n.locale) activate(personLocale)
  }, [personLocale])
  const onSwitch = useCallback(
    (id: string) => {
      choose(id)
      navigate("/")
    },
    [choose, navigate],
  )

  if (me.isPending) return <div className="min-h-svh bg-background" />
  if (me.error && !me.data) {
    return (
      <div className="flex min-h-svh flex-col bg-background">
        <EmptyState icon={RefreshCwIcon} title={<Trans>Could not reach the server</Trans>}>
          <Button className="mt-3" onClick={() => me.refetch()}>
            <Trans>Try again</Trans>
          </Button>
        </EmptyState>
      </div>
    )
  }
  if (!me.data) return <Navigate to={signInPath(location.pathname, location.search)} replace />
  if (me.data.memberships.length === 0) return <NoWorkspace email={me.data.person.email} />
  const membership = pickMembership(me.data, linkedMember ? linked : chosen)
  if (!membership) return <WorkspacePicker me={me.data} onChoose={choose} />
  return (
    <SessionProvider key={membership.workspace.id} me={me.data} membership={membership} onSwitch={onSwitch}>
      <AppShell />
    </SessionProvider>
  )
}
