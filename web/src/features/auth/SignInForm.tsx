import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ArrowLeftIcon, BuildingIcon, KeyRoundIcon, MailIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { AuthDivider, AuthHeading, authAlert as alert, authField as field } from "@/features/auth/AuthLayout"
import { isLocale } from "@/i18n"
import { api, unwrap, type Locale, type Me } from "@/lib/api"
import { safeNext } from "@/lib/next"
import { signInWithPasskey } from "@/lib/passkey"
import { meKey } from "@/lib/session"
import { startSso } from "@/lib/sso"

export function SignInForm({
  title,
  lead,
  reader,
  note,
  banner,
  onSignedIn,
}: {
  banner?: React.ReactNode
  title: React.ReactNode
  lead: React.ReactNode
  reader?: boolean
  note?: React.ReactNode
  onSignedIn?: (me: Me) => void
}) {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const [step, setStep] = useState<"email" | "code" | "sso">("email")
  const [sso, setSso] = useState<{ url: string; required: boolean } | null>(null)
  const [resent, setResent] = useState(false)

  const done = (data: Me) => {
    qc.setQueryData(meKey, data)
    onSignedIn?.(data)
  }

  const request = useMutation({
    mutationFn: async () => {
      if (!reader && step === "email") {
        const start = await startSso(email, safeNext(new URLSearchParams(window.location.search).get("next")))
        if (start.sso && start.authorization_url) {
          setSso({ url: start.authorization_url, required: start.required === true })
          setStep("sso")
          return null
        }
      }
      const locale: Locale = isLocale(i18n.locale) ? i18n.locale : "en"
      return unwrap(api.POST("/v1/auth/code", { body: reader ? { email, reader: true, locale } : { email, locale } }))
    },
    onSuccess: (sent) => {
      if (sent === null) return
      setResent(step === "code")
      setStep("code")
    },
  })
  const verify = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/auth/code/verify", { body: { email, code } })),
    onSuccess: done,
  })
  const passkey = useMutation({ mutationFn: signInWithPasskey, onSuccess: done })

  return (
    <>
      <AuthHeading
        icon={step === "code" ? <MailIcon /> : step === "sso" ? <BuildingIcon /> : undefined}
        title={
          step === "email" ? (
            title
          ) : step === "sso" ? (
            <Trans>Sign in with your organisation</Trans>
          ) : (
            <Trans>Check your e-mail</Trans>
          )
        }
      >
        <p>
          {step === "email" ? (
            lead
          ) : step === "sso" ? (
            <Trans>
              <span className="font-medium break-words text-foreground">{email}</span> signs in through your
              organisation's identity provider.
            </Trans>
          ) : (
            <Trans>
              We sent a 6-digit code to <span className="font-medium break-words text-foreground">{email}</span>. It is
              valid for 10 minutes.
            </Trans>
          )}
        </p>
        {step === "code" && (
          <p className="mt-2 text-small text-faint" data-testid="sign-in-spam">
            {resent ? (
              <Trans>We sent a new code. Only the newest code works.</Trans>
            ) : (
              <Trans>Not there after a minute? Look in your spam or junk folder, or send a new code below.</Trans>
            )}
          </p>
        )}
      </AuthHeading>

      {step === "email" ? (
        <div className="flex flex-col gap-5">
          {banner && (
            <p role="alert" className={alert} data-testid="sign-in-banner">
              {banner}
            </p>
          )}
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
                placeholder={reader ? t`you@example.com` : t`you@company.com`}
                className={field}
                data-testid="sign-in-email"
              />
            </div>
            <ErrorLine error={request.error} className={alert} />
            <Button type="submit" disabled={request.isPending} size="lg" data-testid="sign-in-send">
              <MailIcon />
              <Trans>Send me a code</Trans>
            </Button>
          </form>
          <AuthDivider>
            <Trans>or</Trans>
          </AuthDivider>
          <Button variant="outline" onClick={() => passkey.mutate()} disabled={passkey.isPending} size="lg">
            <KeyRoundIcon />
            <Trans>Sign in with a passkey</Trans>
          </Button>
          {passkey.error && (
            <p role="alert" className={alert}>
              <Trans>The passkey sign-in did not complete. Try again or use an e-mailed code.</Trans>
            </p>
          )}
          {note}
        </div>
      ) : step === "sso" && sso ? (
        <div className="flex flex-col gap-4">
          <Button size="lg" onClick={() => window.location.assign(sso.url)} autoFocus data-testid="sign-in-sso">
            <BuildingIcon />
            <Trans>Continue with your organisation's sign-in</Trans>
          </Button>
          {sso.required && (
            <p className="text-small text-pretty text-faint">
              <Trans>Your organisation requires this sign-in for its workspace.</Trans>
            </p>
          )}
          <ErrorLine error={request.error} className={alert} />
          <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="-ms-3"
              onClick={() => {
                setStep("email")
                setSso(null)
                request.reset()
              }}
            >
              <ArrowLeftIcon />
              <Trans>Change address</Trans>
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="-me-3"
              disabled={request.isPending}
              onClick={() => request.mutate()}
            >
              <Trans>Use a code instead</Trans>
            </Button>
          </div>
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
              className="h-12 text-center font-mono text-title tracking-[0.5em] md:text-title"
              data-testid="sign-in-code"
            />
          </div>
          {verify.error && (
            <p role="alert" className={alert}>
              <Trans>The code is wrong or has expired.</Trans>
            </p>
          )}
          <ErrorLine error={request.error} className={alert} />
          <Button type="submit" disabled={verify.isPending || code.length !== 6} size="lg">
            <Trans>Sign in</Trans>
          </Button>
          <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="-ms-3"
              onClick={() => {
                setStep("email")
                setCode("")
                setResent(false)
                verify.reset()
                request.reset()
              }}
            >
              <ArrowLeftIcon />
              <Trans>Change address</Trans>
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="-me-3"
              disabled={request.isPending}
              onClick={() => request.mutate()}
            >
              <Trans>Send a new code</Trans>
            </Button>
          </div>
        </form>
      )}
    </>
  )
}
