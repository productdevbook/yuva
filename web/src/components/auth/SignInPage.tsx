import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ArrowLeftIcon, KeyRoundIcon, MailIcon } from "lucide-react"
import { useState } from "react"
import { Navigate, useNavigate, useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { AuthLayout } from "@/components/auth/AuthLayout"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, unwrap } from "@/lib/api"
import { safeNext } from "@/lib/next"
import { signInWithPasskey } from "@/lib/passkey"
import { meKey, useMe } from "@/lib/session"

const pill = "h-11 rounded-full text-[0.95rem]"
const field = "h-11 rounded-xl px-3.5"
const alert = "rounded-xl border border-destructive/25 bg-destructive/5 px-3.5 py-2.5 text-sm text-destructive"

export function SignInPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const me = useMe()
  const [params] = useSearchParams()
  const next = safeNext(params.get("next"))
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const [step, setStep] = useState<"email" | "code">("email")

  const done = (data: unknown) => {
    qc.setQueryData(meKey, data)
    navigate(next, { replace: true })
  }

  const request = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/auth/code", { body: { email } })),
    onSuccess: () => setStep("code"),
  })
  const verify = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/auth/code/verify", { body: { email, code } })),
    onSuccess: done,
  })
  const passkey = useMutation({ mutationFn: signInWithPasskey, onSuccess: done })

  if (me.data) return <Navigate to={next} replace />

  return (
    <AuthLayout>
      <div className="mb-8">
        {step === "code" && (
          <span className="mb-5 grid size-11 place-items-center rounded-full bg-brand-wash text-brand">
            <MailIcon className="size-5" />
          </span>
        )}
        <h1 className="text-[1.75rem] leading-tight font-semibold tracking-[-0.03em] sm:text-[2rem]">
          {step === "email" ? <Trans>Sign in to Yuva</Trans> : <Trans>Check your e-mail</Trans>}
        </h1>
        <p className="mt-3 leading-relaxed text-pretty text-muted-foreground">
          {step === "email" ? (
            <Trans>We e-mail you a one-time code. No password to remember.</Trans>
          ) : (
            <Trans>
              We sent a 6-digit code to <span className="font-medium break-words text-foreground">{email}</span>. It is
              valid for 10 minutes.
            </Trans>
          )}
        </p>
      </div>

      {step === "email" ? (
        <div className="flex flex-col gap-5">
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              request.mutate()
            }}
          >
            <div className="flex flex-col gap-2">
              <Label htmlFor="email">
                <Trans>E-mail address</Trans>
              </Label>
              <Input
                id="email"
                type="email"
                autoComplete="username webauthn"
                required
                autoFocus
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={t`you@company.com`}
                className={field}
              />
            </div>
            <ErrorLine error={request.error} className={alert} />
            <Button type="submit" disabled={request.isPending} className={pill}>
              <MailIcon />
              <Trans>Send me a code</Trans>
            </Button>
          </form>
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span className="h-px flex-1 bg-border" />
            <Trans>or</Trans>
            <span className="h-px flex-1 bg-border" />
          </div>
          <Button variant="outline" onClick={() => passkey.mutate()} disabled={passkey.isPending} className={pill}>
            <KeyRoundIcon />
            <Trans>Sign in with a passkey</Trans>
          </Button>
          {passkey.error && (
            <p role="alert" className={alert}>
              <Trans>The passkey sign-in did not complete. Try again or use an e-mailed code.</Trans>
            </p>
          )}
        </div>
      ) : (
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            verify.mutate()
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="code">
              <Trans>Code</Trans>
            </Label>
            <Input
              id="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              required
              autoFocus
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              className="h-14 rounded-xl text-center font-mono text-2xl tracking-[0.5em] md:text-2xl"
            />
          </div>
          {verify.error && (
            <p role="alert" className={alert}>
              <Trans>The code is wrong or has expired.</Trans>
            </p>
          )}
          <ErrorLine error={request.error} className={alert} />
          <Button type="submit" disabled={verify.isPending || code.length !== 6} className={pill}>
            <Trans>Sign in</Trans>
          </Button>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="-ms-2.5 rounded-full"
              onClick={() => {
                setStep("email")
                setCode("")
                verify.reset()
              }}
            >
              <ArrowLeftIcon />
              <Trans>Change address</Trans>
            </Button>
            <Button
              type="button"
              variant="link"
              size="sm"
              className="-me-2.5"
              disabled={request.isPending}
              onClick={() => request.mutate()}
            >
              <Trans>Send a new code</Trans>
            </Button>
          </div>
        </form>
      )}
    </AuthLayout>
  )
}
