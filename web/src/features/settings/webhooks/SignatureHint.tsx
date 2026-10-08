import { Trans } from "@lingui/react/macro"
import { InfoIcon } from "lucide-react"

import { Notice } from "@/components/common"

const STANDARD_WEBHOOKS = "https://www.standardwebhooks.com"

export function SignatureHint() {
  return (
    <Notice icon={InfoIcon} className="text-xs" data-testid="signature-hint">
      <p>
        <Trans>
          Verify every request before trusting it: the <code className="font-mono">webhook-signature</code> header
          follows the{" "}
          <a href={STANDARD_WEBHOOKS} target="_blank" rel="noreferrer" className="font-medium text-foreground underline underline-offset-2">
            Standard Webhooks
          </a>{" "}
          specification, so any of its libraries can check it with the endpoint's secret. Go backends can use the{" "}
          <code className="font-mono">sdk/go/webhook</code> package of the Yuva repository.
        </Trans>
      </p>
    </Notice>
  )
}
