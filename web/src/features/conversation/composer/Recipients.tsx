import { Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useState } from "react"

import { AddressChips, type AddressOption } from "@/components/common/AddressChips"
import { useContactSearch } from "@/features/contact/queries"

export function useSuggestions() {
  const [text, setText] = useState("")
  const [query, setQuery] = useState("")
  useEffect(() => {
    const id = setTimeout(() => setQuery(text), 200)
    return () => clearTimeout(id)
  }, [text])
  const results = useContactSearch(query, query.length >= 2)
  const options: AddressOption[] =
    query.length < 2
      ? []
      : (results.data ?? []).flatMap((c) => c.emails.map((email) => ({ email: email.toLowerCase(), name: c.name })))
  return { options, onType: setText }
}

const toggleClass =
  "rounded-md px-1.5 py-0.5 text-caption text-faint outline-none transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-muted aria-expanded:text-foreground"

export function Recipients({
  to,
  cc,
  onCc,
  bcc,
  onBcc,
  ccOpen,
  bccOpen,
  setCcOpen,
  setBccOpen,
  ccRef,
  bccRef,
}: {
  to: string
  cc: string[]
  onCc: (next: string[]) => void
  bcc: string[]
  onBcc: (next: string[]) => void
  ccOpen: boolean
  bccOpen: boolean
  setCcOpen: (open: boolean) => void
  setBccOpen: (open: boolean) => void
  ccRef: React.Ref<HTMLInputElement>
  bccRef: React.Ref<HTMLInputElement>
}) {
  const { t } = useLingui()
  const ccHints = useSuggestions()
  const bccHints = useSuggestions()
  const ccCount = cc.length
  const bccCount = bcc.length
  return (
    <div className="flex flex-col gap-1.5 px-3 pb-1.5" data-testid="composer-recipients">
      <div className="flex min-w-0 flex-wrap items-center gap-x-1 gap-y-0.5 text-small">
        <span className="w-7 shrink-0 text-caption text-faint">
          <Trans>To</Trans>
        </span>
        <span className="min-w-0 truncate" data-testid="composer-email-to">
          {to}
        </span>
        <span className="flex-1" />
        <button
          type="button"
          className={toggleClass}
          aria-expanded={ccOpen}
          onClick={() => setCcOpen(!ccOpen)}
          data-testid="composer-cc-toggle"
        >
          {ccCount > 0 ? <Trans>Cc ({ccCount})</Trans> : <Trans>Cc</Trans>}
        </button>
        <button
          type="button"
          className={toggleClass}
          aria-expanded={bccOpen}
          onClick={() => setBccOpen(!bccOpen)}
          data-testid="composer-bcc-toggle"
        >
          {bccCount > 0 ? <Trans>Bcc ({bccCount})</Trans> : <Trans>Bcc</Trans>}
        </button>
      </div>
      {ccOpen && (
        <div className="flex items-start gap-1">
          <span className="w-7 shrink-0 pt-1.5 text-caption text-faint">
            <Trans>Cc</Trans>
          </span>
          <AddressChips
            ref={ccRef}
            label={t`Cc`}
            value={cc}
            onChange={onCc}
            testId="composer-cc"
            className="flex-1"
            {...ccHints}
          />
        </div>
      )}
      {bccOpen && (
        <div className="flex items-start gap-1">
          <span className="w-7 shrink-0 pt-1.5 text-caption text-faint">
            <Trans>Bcc</Trans>
          </span>
          <AddressChips
            ref={bccRef}
            label={t`Bcc`}
            value={bcc}
            onChange={onBcc}
            testId="composer-bcc"
            className="flex-1"
            {...bccHints}
          />
        </div>
      )}
      {bccOpen && (
        <p className="ps-8 text-caption text-faint">
          <Trans>Bcc recipients get the mail but are hidden from everyone else, including the contact.</Trans>
        </p>
      )}
    </div>
  )
}
