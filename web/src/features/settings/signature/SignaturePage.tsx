import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine, InboxMark, toast } from "@/components/common"
import { SignatureEditor } from "@/components/common/SignatureEditor"
import { SignatureHtml } from "@/components/common/SignatureHtml"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useMySignatures, useResetSignature, useSetSignature } from "@/features/settings/signature/queries"
import { PageHeader, Section, StatusTag } from "@/features/settings/ui"
import type { Inbox, MemberSignature } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

function SignatureBlock({ inbox, sig }: { inbox: Inbox; sig: MemberSignature }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const own = sig.body !== undefined
  const [draft, setDraft] = useState(sig.body ?? inbox.signature)
  const save = useSetSignature(inbox.id)
  const reset = useResetSignature(inbox.id)
  const busy = save.isPending || reset.isPending
  const error = save.error ?? reset.error
  const done = (text: string) => ({
    onSuccess: (saved?: MemberSignature) => {
      if (saved) setDraft(saved.body ?? "")
      toast(text)
    },
    onError: (e: unknown) => toast(errorText(e)),
  })
  return (
    <section className="flex flex-col overflow-hidden rounded-xl border bg-card" data-testid="signature-inbox">
      <div className="flex items-center gap-3 border-b px-[18px] py-3.5">
        <InboxMark name={inbox.name} color={inbox.branding.color} className="size-7 rounded-lg text-[13px]" />
        <h2 className="min-w-0 flex-1 truncate font-semibold">{inbox.name}</h2>
        {own ? (
          sig.body === "" ? (
            <StatusTag tone="muted">
              <Trans>No signature</Trans>
            </StatusTag>
          ) : (
            <StatusTag tone="success">
              <Trans>Your own</Trans>
            </StatusTag>
          )
        ) : (
          <StatusTag tone="muted">
            <Trans>Inbox default</Trans>
          </StatusTag>
        )}
      </div>
      <div className="flex flex-col gap-1.5 border-b px-[18px] py-3.5">
        <p className="text-caption text-faint">
          <Trans>Your replies by e-mail end with</Trans>
        </p>
        {sig.html ? (
          <SignatureHtml html={sig.html} />
        ) : (
          <p className="text-small text-muted-foreground" data-testid="signature-empty">
            <Trans>Nothing. Replies from this inbox go out without a signature.</Trans>
          </p>
        )}
      </div>
      <form
        className="flex flex-col gap-3 px-[18px] py-3.5"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate(draft, done(t`Saved`))
        }}
      >
        <label htmlFor={`signature-${inbox.id}`} className="text-small font-medium">
          <Trans>Your signature</Trans>
        </label>
        <SignatureEditor
          id={`signature-${inbox.id}`}
          value={draft}
          onChange={setDraft}
          disabled={busy}
          placeholder={t`Best regards,`}
        />
        <ErrorLine error={error} className="text-caption" />
        <div className="flex flex-wrap items-center gap-2">
          <Button type="submit" size="sm" disabled={busy || (own && draft === sig.body)} data-testid="signature-save">
            <Trans>Save</Trans>
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={busy || (own && sig.body === "" && draft === "")}
            onClick={() => {
              setDraft("")
              save.mutate("", done(t`Saved`))
            }}
            data-testid="signature-none"
          >
            <Trans>No signature</Trans>
          </Button>
          {own && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={busy}
              onClick={() =>
                reset.mutate(undefined, {
                  onSuccess: () => {
                    setDraft(inbox.signature)
                    toast(t`Using the inbox default`)
                  },
                  onError: (e) => toast(errorText(e)),
                })
              }
              data-testid="signature-default"
            >
              <Trans>Use the inbox default</Trans>
            </Button>
          )}
        </div>
      </form>
    </section>
  )
}

export function SignaturePage() {
  const inboxes = useInboxes()
  const sigs = useMySignatures()
  const byId = new Map((inboxes.data ?? []).map((i) => [i.id, i]))
  return (
    <>
      <PageHeader
        title={<Trans>Signature</Trans>}
        description={<Trans>What your e-mailed replies end with, per inbox. Chat replies and notes carry none.</Trans>}
      />
      {(inboxes.isPending || sigs.isPending) && <Skeleton className="h-48 rounded-xl" />}
      <ErrorLine error={inboxes.error ?? sigs.error} />
      <Section>
        <div className="flex max-w-[760px] flex-col gap-4">
          {(sigs.data ?? []).map((s) => {
            const inbox = byId.get(s.inbox_id)
            return inbox && <SignatureBlock key={`${inbox.id}:${s.body ?? ""}`} inbox={inbox} sig={s} />
          })}
        </div>
      </Section>
    </>
  )
}
