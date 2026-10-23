import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { CircleCheckIcon, TriangleAlertIcon } from "lucide-react"
import { useState } from "react"

import { CodeLine, ErrorLine, Notice } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { PageSection } from "@/features/settings/channels/sections"
import { Card, Field } from "@/features/settings/ui"
import { api, ApiError, unwrap, type Channel } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function useProof(channel: Channel) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const path = { channelId: channel.id }
  const saved = (data: Channel) => {
    qc.setQueryData(keys.channel(ws, data.id), data)
    void qc.invalidateQueries({ queryKey: keys.channels(ws, channel.inbox_id) })
  }
  const stale = (err: unknown) => {
    if (err instanceof ApiError && err.code === "email_verified") {
      void qc.invalidateQueries({ queryKey: keys.channel(ws, channel.id) })
    }
  }
  const send = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/channels/{channelId}/verification-code", { params: { path } })),
    onSuccess: saved,
    onError: stale,
  })
  const verify = useMutation({
    mutationFn: (code?: string) =>
      unwrap(api.POST("/v1/channels/{channelId}/verify", { params: { path }, body: code ? { code } : {} })),
    onSuccess: saved,
    onError: stale,
  })
  return { send, verify }
}

function Verified({ channel }: { channel: Channel }) {
  const { i18n } = useLingui()
  const v = channel.email?.verification
  const address = channel.email?.address ?? ""
  const when = v?.verified_at ? formatDateTime(v.verified_at, i18n.locale) : ""
  return (
    <Card>
      <div className="flex items-start gap-2.5" data-testid="address-proof-verified">
        <CircleCheckIcon className="mt-0.5 size-4 shrink-0 text-success" />
        <div className="flex min-w-0 flex-col gap-1">
          <p className="text-body font-medium">
            <Trans>
              Receives mail at <span dir="ltr">{address}</span>
            </Trans>
          </p>
          <p className="text-small text-faint">
            {v?.method === "dns" ? (
              <Trans>Proven with the DNS record on {when}.</Trans>
            ) : v?.method === "operator" ? (
              <Trans>Vouched for by the server's operator on {when}.</Trans>
            ) : (
              <Trans>Proven with a code on {when}.</Trans>
            )}
          </p>
        </div>
      </div>
    </Card>
  )
}

export function AddressProofSection({ channel }: { channel: Channel }) {
  const { i18n, t } = useLingui()
  const [code, setCode] = useState("")
  const { send, verify } = useProof(channel)
  const v = channel.email?.verification
  if (!v) return null
  const title = <Trans>Address proof</Trans>
  if (v.state === "verified")
    return (
      <PageSection id="address-proof" title={title}>
        <Verified channel={channel} />
      </PageSection>
    )
  const address = channel.email?.address ?? ""
  const taken = v.problem === "taken"
  const codeOpen = !!v.code_sent_at && v.code_allowed
  const dnsError = !taken && v.problem ? v.problem : undefined
  const expires = v.code_expires_at ? formatDateTime(v.code_expires_at, i18n.locale) : ""
  const checked = v.checked_at ? formatDateTime(v.checked_at, i18n.locale) : ""
  return (
    <PageSection
      id="address-proof"
      title={title}
      description={
        <Trans>Until the address is proven, this channel receives mail only at its forwarding address.</Trans>
      }
    >
      <Card>
        <Notice
          tone="warning"
          icon={TriangleAlertIcon}
          role={taken ? "alert" : undefined}
          data-testid="address-proof-unverified"
        >
          <p className="font-medium">
            {taken ? (
              <Trans>Another workspace proved this address later</Trans>
            ) : (
              <Trans>This address does not receive mail yet</Trans>
            )}
          </p>
          <p>
            {taken ? (
              <Trans>
                Mail to <span dir="ltr">{address}</span> goes to that workspace now. Prove the address again to take it
                back, or talk to whoever manages the domain.
              </Trans>
            ) : (
              <Trans>
                Mail forwarded to the forwarding address arrives. Mail sent to <span dir="ltr">{address}</span> itself
                is not routed here until you prove it.
              </Trans>
            )}
          </p>
        </Notice>

        {v.code_allowed ? (
          <Field
            label={<Trans>Prove it with a code</Trans>}
            hint={
              codeOpen ? (
                <Trans>
                  The code is valid until {expires}. A mailbox that forwards here may prove the address by itself.
                </Trans>
              ) : (
                <Trans>Yuva mails an 8-digit code to the address. Type it here.</Trans>
              )
            }
          >
            <div className="flex flex-wrap items-center gap-2">
              {codeOpen && (
                <>
                  <Input
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 8))}
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={8}
                    dir="ltr"
                    className="w-40 font-mono"
                    aria-label={t`Code`}
                    data-testid="address-proof-code"
                  />
                  <Button
                    type="button"
                    disabled={code.length !== 8 || verify.isPending}
                    onClick={() => verify.mutate(code)}
                    data-testid="address-proof-verify"
                  >
                    <Trans>Verify</Trans>
                  </Button>
                </>
              )}
              <Button
                type="button"
                variant={codeOpen ? "ghost" : "outline"}
                disabled={send.isPending}
                onClick={() => {
                  setCode("")
                  verify.reset()
                  send.mutate()
                }}
                data-testid="address-proof-send"
              >
                {codeOpen ? <Trans>Send a new code</Trans> : <Trans>Send a code</Trans>}
              </Button>
            </div>
            <ErrorLine error={verify.error} className="text-caption" />
            <ErrorLine error={send.error} className="text-caption" />
          </Field>
        ) : (
          <p className="text-body text-muted-foreground">
            <Trans>A catch-all, or an address on the forwarding domain, can be proven only with a DNS record.</Trans>
          </p>
        )}

        <Field
          label={<Trans>Prove it with a DNS record</Trans>}
          hint={<Trans>One record proves every channel of this workspace on the domain.</Trans>}
        >
          <CodeLine value={v.txt_name} testId="address-proof-txt-name" />
          <CodeLine value={v.txt_value} testId="address-proof-txt-value" />
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              variant="outline"
              disabled={verify.isPending}
              onClick={() => verify.mutate(undefined)}
              data-testid="address-proof-check-dns"
            >
              <Trans>Check DNS</Trans>
            </Button>
            {v.checked_at && (
              <span className="text-small text-faint">
                <Trans>Last checked {checked}</Trans>
              </span>
            )}
          </div>
          {dnsError && (
            <p role="alert" className="text-body text-destructive" data-testid="address-proof-problem">
              {dnsError}
            </p>
          )}
        </Field>
      </Card>
    </PageSection>
  )
}
