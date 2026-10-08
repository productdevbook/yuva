import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, CopyIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"

export function useCopy() {
  const [copied, setCopied] = useState(false)
  const copy = async (value: string) => {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return [copied, copy] as const
}

export function CopyButton({ value, className }: { value: string; className?: string }) {
  const { t } = useLingui()
  const [copied, copy] = useCopy()
  return (
    <Button type="button" variant="outline" size="sm" className={className} onClick={() => void copy(value)} aria-label={t`Copy`}>
      {copied ? <CheckIcon /> : <CopyIcon />}
      {copied ? <Trans>Copied</Trans> : <Trans>Copy</Trans>}
    </Button>
  )
}

export function CodeLine({ value, testId }: { value: string; testId?: string }) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      <code className="min-w-0 flex-1 truncate rounded-xl border bg-surface px-3 py-2 font-mono text-xs" data-testid={testId}>
        {value}
      </code>
      <CopyButton value={value} />
    </div>
  )
}
