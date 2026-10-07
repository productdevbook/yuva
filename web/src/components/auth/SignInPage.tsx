import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ArrowLeftIcon, KeyRoundIcon, MailIcon } from "lucide-react"
import { useState } from "react"
import { Navigate, useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { LanguageMenu } from "@/components/LanguageMenu"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, unwrap } from "@/lib/api"
import { signInWithPasskey } from "@/lib/passkey"
import { meKey, useMe } from "@/lib/session"

export function SignInPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const me = useMe()
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const [step, setStep] = useState<"email" | "code">("email")

  const done = (data: unknown) => {
    qc.setQueryData(meKey, data)
    navigate("/", { replace: true })
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

  if (me.data) return <Navigate to="/" replace />

  return (
    <div className="flex min-h-svh flex-col bg-background">
      <header className="flex h-12 items-center justify-end px-3">
        <LanguageMenu />
      </header>
      <main className="flex flex-1 items-start justify-center px-4 pt-[12vh] pb-10">
        <div className="w-full max-w-sm">
          <div className="mb-8 flex flex-col items-center gap-3 text-center">
            <img src="/favicon.svg" alt="" className="size-10 rounded-lg" />
            <h1 className="text-xl font-semibold">
              {step === "email" ? <Trans>Sign in to Yuva</Trans> : <Trans>Check your e-mail</Trans>}
            </h1>
            {step === "code" && (
              <p className="text-sm text-muted-foreground">
                <Trans>
                  We sent a 6-digit code to <span className="font-medium text-foreground">{email}</span>. It is
                  valid for 10 minutes.
                </Trans>
              </p>
            )}
          </div>

          {step === "email" ? (
            <div className="flex flex-col gap-4">
              <form
                className="flex flex-col gap-3"
                onSubmit={(e) => {
                  e.preventDefault()
                  request.mutate()
                }}
              >
                <div className="flex flex-col gap-1.5">
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
                  />
                </div>
                <ErrorLine error={request.error} />
                <Button type="submit" disabled={request.isPending}>
                  <MailIcon />
                  <Trans>Send me a code</Trans>
                </Button>
              </form>
              <div className="flex items-center gap-3 text-xs text-muted-foreground">
                <span className="h-px flex-1 bg-border" />
                <Trans>or</Trans>
                <span className="h-px flex-1 bg-border" />
              </div>
              <Button variant="outline" onClick={() => passkey.mutate()} disabled={passkey.isPending}>
                <KeyRoundIcon />
                <Trans>Sign in with a passkey</Trans>
              </Button>
              {passkey.error && (
                <p role="alert" className="text-sm text-destructive">
                  <Trans>The passkey sign-in did not complete. Try again or use an e-mailed code.</Trans>
                </p>
              )}
            </div>
          ) : (
            <form
              className="flex flex-col gap-3"
              onSubmit={(e) => {
                e.preventDefault()
                verify.mutate()
              }}
            >
              <div className="flex flex-col gap-1.5">
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
                  className="text-center font-mono text-lg tracking-[0.5em]"
                />
              </div>
              {verify.error && (
                <p role="alert" className="text-sm text-destructive">
                  <Trans>The code is wrong or has expired.</Trans>
                </p>
              )}
              <Button type="submit" disabled={verify.isPending || code.length !== 6}>
                <Trans>Sign in</Trans>
              </Button>
              <div className="flex items-center justify-between">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
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
                  disabled={request.isPending}
                  onClick={() => request.mutate()}
                >
                  <Trans>Send a new code</Trans>
                </Button>
              </div>
            </form>
          )}
        </div>
      </main>
    </div>
  )
}
