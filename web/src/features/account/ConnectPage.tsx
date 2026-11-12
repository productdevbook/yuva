import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { CircleCheckIcon, TriangleAlertIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { InitialsAvatar } from "@/components/common/DiscussionAuthor"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Unreachable, usePersonLocale } from "@/features/account/AccountLayout"
import { accountKeys, useReaderAccount } from "@/features/account/queries"
import { AuthHeading, AuthLayout, authField } from "@/features/auth/AuthLayout"
import { SignInForm } from "@/features/auth/SignInForm"
import { Field } from "@/features/settings/ui"
import { api, ApiError, unwrap, type ReaderAccount, type ReaderToken } from "@/lib/api"
import { useMe, useSignOut } from "@/lib/session"

const REMEMBERED = "yuva-reader-connections"

function remembered(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(REMEMBERED) ?? "[]")
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : []
  } catch {
    return []
  }
}

function remember(id: string) {
  try {
    localStorage.setItem(REMEMBERED, JSON.stringify([id, ...remembered().filter((x) => x !== id)].slice(0, 50)))
  } catch {
    // Storage can be blocked; the person is then asked again next time.
  }
}

function forget(id: string) {
  try {
    localStorage.setItem(REMEMBERED, JSON.stringify(remembered().filter((x) => x !== id)))
  } catch {
    // Nothing to forget without storage.
  }
}

function normalOrigin(raw: string | null) {
  if (!raw) return ""
  try {
    const u = new URL(raw)
    return u.protocol === "https:" || u.protocol === "http:" ? u.origin : ""
  } catch {
    return ""
  }
}

function useTarget() {
  const [params] = useSearchParams()
  const channel = params.get("channel") ?? ""
  const origin = normalOrigin(params.get("origin")) || normalOrigin(document.referrer)
  return { channel, origin, host: origin ? new URL(origin).host : "" }
}

function Message({
  icon: Icon,
  tone,
  title,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>
  tone: "success" | "danger"
  title: React.ReactNode
  children?: React.ReactNode
}) {
  return (
    <div data-testid="connect-message" data-tone={tone}>
      <AuthHeading icon={<Icon />} tone={tone} title={title} className="mb-0">
        {children}
      </AuthHeading>
    </div>
  )
}

function Consent({ account, channel, origin, host }: { account: ReaderAccount } & ReturnType<typeof useTarget>) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const errorText = useErrorText()
  const signOut = useSignOut()
  const id = `${channel} ${origin}`
  const [name, setName] = useState("")
  const [auto] = useState(() => remembered().includes(id))
  const connect = useMutation({
    mutationFn: async (): Promise<ReaderToken> => {
      if (!account.name && name.trim()) {
        const updated = await unwrap(api.PATCH("/v1/account", { body: { name: name.trim() } }))
        qc.setQueryData(accountKeys.account, updated)
      }
      return unwrap(api.POST("/v1/account/reader-tokens", { body: { channel_key: channel, origin } }))
    },
    onSuccess: (token) => {
      remember(id)
      window.opener?.postMessage(token, token.origin)
      window.close()
    },
    onError: () => forget(id),
  })
  const run = connect.mutate
  useEffect(() => {
    if (auto && window.opener) run()
  }, [auto, run])

  const err = connect.error
  if (err instanceof ApiError && (err.status === 403 || err.status === 404)) {
    return (
      <Message icon={TriangleAlertIcon} tone="danger" title={<Trans>This site cannot connect</Trans>}>
        <p>
          {err.code === "discussions_off" ? (
            <Trans>Discussions are turned off for {host}.</Trans>
          ) : err.code === "origin_not_allowed" ? (
            <Trans>{host} is not one of the sites this documentation channel allows.</Trans>
          ) : err.status === 404 ? (
            <Trans>The documentation channel of {host} no longer exists.</Trans>
          ) : (
            errorText(err)
          )}
        </p>
      </Message>
    )
  }
  if (connect.isSuccess)
    return (
      <Message icon={CircleCheckIcon} tone="success" title={<Trans>Connected</Trans>}>
        <p>
          <Trans>You can close this window and go back to {host}.</Trans>
        </p>
      </Message>
    )
  if (auto && (connect.isPending || connect.isIdle)) return <Skeleton className="h-40 rounded-xl" />

  const shown = account.name || name.trim() || t`Reader`
  const email = account.email
  return (
    <div className="flex flex-col" data-testid="connect-consent">
      <AuthHeading title={<Trans>Connect to {host}</Trans>} titleTestId="connect-title" className="mb-6">
        <Trans>
          You can then write, vote and follow discussions there. The team behind {host} sees your name and e-mail
          address when you write; other readers see only your name.
        </Trans>
      </AuthHeading>
      <div className="flex items-center gap-3 border-y border-line-soft px-1 py-3">
        <InitialsAvatar initials={account.initials} />
        <span className="flex min-w-0 flex-col">
          <span className="truncate text-body font-medium">{shown}</span>
          <span className="truncate font-mono text-caption text-muted-foreground">{account.email}</span>
        </span>
      </div>
      {!account.name && (
        <Field
          label={<Trans>Your name</Trans>}
          htmlFor="connect-name"
          hint={<Trans>Shown on your posts. Leave it empty to show as “Reader”.</Trans>}
          className="mt-5"
        >
          <Input
            id="connect-name"
            value={name}
            maxLength={200}
            autoComplete="name"
            onChange={(e) => setName(e.target.value)}
            className={authField}
            data-testid="connect-name"
          />
        </Field>
      )}
      {!window.opener && (
        <p role="alert" className="mt-5 text-body text-destructive">
          <Trans>This window lost the page that opened it. Close it and sign in again from {host}.</Trans>
        </p>
      )}
      <ErrorLine error={err} className="mt-5" />
      <div className="mt-7 flex flex-col gap-2">
        <Button
          size="lg"
          onClick={() => run()}
          disabled={connect.isPending || !window.opener}
          data-testid="connect-confirm"
        >
          <Trans>Connect</Trans>
        </Button>
        <Button size="lg" variant="ghost" onClick={() => window.close()}>
          <Trans>Cancel</Trans>
        </Button>
      </div>
      <p className="mt-6 border-t border-line-soft pt-4 text-center text-small break-words text-muted-foreground">
        <Trans>Not {email}?</Trans>{" "}
        <button
          type="button"
          className="text-muted-foreground underline underline-offset-4 hover:text-foreground"
          onClick={() => void signOut()}
        >
          <Trans>Use another address</Trans>
        </button>
      </p>
    </div>
  )
}

function Connected({ target }: { target: ReturnType<typeof useTarget> }) {
  const account = useReaderAccount()
  if (account.isPending) return <Skeleton className="h-40 rounded-xl" />
  if (!account.data) return <ErrorLine error={account.error} />
  return <Consent account={account.data} {...target} />
}

export function ConnectPage() {
  const me = useMe()
  const target = useTarget()
  const host = target.host
  usePersonLocale()
  if (me.isPending) return <div className="min-h-svh bg-background" />
  if (me.error && !me.data) return <Unreachable onRetry={() => void me.refetch()} />
  return (
    <AuthLayout>
      {!target.channel || !target.origin ? (
        <Message icon={TriangleAlertIcon} tone="danger" title={<Trans>This link is incomplete</Trans>}>
          <p>
            <Trans>Open it from the sign-in button of a documentation page.</Trans>
          </p>
        </Message>
      ) : !me.data ? (
        <SignInForm
          reader
          title={<Trans>Sign in to join the discussion</Trans>}
          lead={
            <Trans>
              {host} uses Yuva for its discussions. Sign in or create an account with your e-mail address; we e-mail you
              a one-time code.
            </Trans>
          }
        />
      ) : (
        <Connected target={target} />
      )}
    </AuthLayout>
  )
}
