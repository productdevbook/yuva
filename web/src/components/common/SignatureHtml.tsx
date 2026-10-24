import { EmailHtml } from "@/components/common/EmailHtml"
import { cn } from "@/lib/utils"

export function SignatureHtml({ html, className }: { html: string; className?: string }) {
  return (
    <div className={cn("text-small", className)} data-testid="signature-html">
      <EmailHtml html={html} images />
    </div>
  )
}
