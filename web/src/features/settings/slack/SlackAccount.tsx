import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine, toast, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useMySlack, useMySlackMutations } from "@/features/settings/slack/queries"
import { Card, Field, Section } from "@/features/settings/ui"

export function SlackAccount() {
  const { t, i18n } = useLingui()
  const q = useMySlack()
  const { start, confirm: confirmCode, unlink } = useMySlackMutations()
  const [confirm, confirmDialog] = useConfirm()
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const link = q.data
  if (!link) return null
  const pending = !link.linked && !!link.code_expires_at
  const who = link.slack_user_name ?? link.slack_user_id ?? ""
  return (
    <Section
      title={<Trans>Slack account</Trans>}
      description={<Trans>Link your Slack user so replies you write in Slack threads are written as you.</Trans>}
    >
      <Card data-testid="slack-account">
        {link.linked ? (
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex min-w-0 flex-1 basis-48 flex-col">
              <span className="truncate text-reading font-semibold">{who}</span>
              {link.linked_at && (
                <span className="text-body text-muted-foreground">
                  <Trans>Linked {formatDateTime(link.linked_at, i18n.locale)}</Trans>
                </span>
              )}
            </div>
            <Button
              variant="outline"
              size="sm"
              className="text-destructive"
              disabled={unlink.isPending}
              data-testid="slack-unlink"
              onClick={() =>
                confirm({
                  title: <Trans>Unlink your Slack account?</Trans>,
                  description: <Trans>Replies you write in Slack threads no longer reach Yuva.</Trans>,
                  confirm: <Trans>Unlink</Trans>,
                  run: () => unlink.mutate(undefined, { onSuccess: () => toast(t`Slack account unlinked`) }),
                })
              }
            >
              <Trans>Unlink</Trans>
            </Button>
          </div>
        ) : (
          <>
            {pending && (
              <form
                className="flex flex-col gap-2"
                onSubmit={(e) => {
                  e.preventDefault()
                  if (code.trim() && !confirmCode.isPending)
                    confirmCode.mutate(code.trim(), {
                      onSuccess: () => {
                        setCode("")
                        toast(t`Slack account linked`)
                      },
                    })
                }}
              >
                <Field
                  label={<Trans>Code from Slack</Trans>}
                  htmlFor="slack-code"
                  hint={<Trans>Slack sent a 6-digit code to {who} as a direct message.</Trans>}
                >
                  <div className="flex flex-wrap gap-2">
                    <Input
                      className="min-w-40 flex-1"
                      id="slack-code"
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      maxLength={6}
                      value={code}
                      onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
                      data-testid="slack-code"
                    />
                    <Button type="submit" disabled={code.length !== 6 || confirmCode.isPending}>
                      <Trans>Link</Trans>
                    </Button>
                  </div>
                </Field>
                <ErrorLine error={confirmCode.error} className="text-small" />
              </form>
            )}
            <form
              className="flex flex-col gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                if (!start.isPending) start.mutate(email.trim() || undefined)
              }}
            >
              <Field
                label={<Trans>Slack e-mail address</Trans>}
                htmlFor="slack-email"
                hint={<Trans>Only if your Slack account uses another address than this one. Optional.</Trans>}
              >
                <div className="flex flex-wrap gap-2">
                  <Input
                    className="min-w-40 flex-1"
                    id="slack-email"
                    type="email"
                    autoComplete="off"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    data-testid="slack-email"
                  />
                  <Button type="submit" variant="outline" disabled={start.isPending} data-testid="slack-link-start">
                    {pending ? <Trans>Send a new code</Trans> : <Trans>Send me a code</Trans>}
                  </Button>
                </div>
              </Field>
              <ErrorLine error={start.error} className="text-small" />
            </form>
          </>
        )}
        <ErrorLine error={unlink.error} className="text-small" />
      </Card>
      {confirmDialog}
    </Section>
  )
}
