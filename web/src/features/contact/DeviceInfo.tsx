import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, CopyIcon } from "lucide-react"

import { useCopy } from "@/components/common/CopyButton"
import { formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { languageName } from "@/features/contact/ContactDetails"
import { look } from "@/features/inbox/PageFrame"
import { useConversation } from "@/features/conversation/queries"
import { cn } from "@/lib/utils"
import type { Contact } from "@/lib/api"

type Info = NonNullable<Contact["client_info"]>

const PLATFORMS: Record<string, string> = { ios: "iOS", android: "Android", web: "Web" }

function join(parts: (string | undefined)[], sep = " ") {
  return parts.filter(Boolean).join(sep)
}

function storage(mb: number) {
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`
}

export function DeviceInfo({ contact, conversationId }: { contact: Contact; conversationId?: string }) {
  const { t, i18n } = useLingui()
  const conversation = useConversation(conversationId).data
  const info: Info | undefined = conversation?.client_info ?? contact.client_info
  const at = conversation?.client_info ? conversation.client_info_at : contact.client_info_at
  const [copied, copy] = useCopy()
  if (!info) return null

  const platform = info.platform ? PLATFORMS[info.platform] : undefined
  const locale = (code?: string) => (code ? languageName(code, i18n.locale) : undefined)
  const rows: [string, string | undefined][] = [
    [t`App`, join([info.app_version, info.build && `(${info.build})`])],
    [t`System`, join([info.os ?? platform, info.os_version])],
    [t`Device`, join([info.device_manufacturer, info.device_model])],
    [t`SDK`, join([info.sdk_name, info.sdk_version])],
    [t`App language`, locale(info.app_locale)],
    [t`Device language`, locale(info.device_locale)],
    [t`Time zone`, info.timezone],
    [t`Screen`, info.screen],
    [t`Free storage`, info.free_storage_mb === undefined ? undefined : storage(info.free_storage_mb)],
  ]
  const shown = rows.filter((r): r is [string, string] => !!r[1])
  if (shown.length === 0) return null
  const seen = at ? formatRelative(at, i18n.locale) : ""
  const text = shown.map(([k, v]) => `${k}: ${v}`).join("\n")

  return (
    <div data-testid="device-info">
      <dl className="flex flex-col gap-2">
        {shown.map(([k, v]) => (
          <div key={k} className="flex min-w-0 items-baseline gap-3">
            <dt className="w-24 shrink-0 text-faint">{k}</dt>
            <dd className="min-w-0 flex-1 break-words" title={v}>
              {v}
            </dd>
          </div>
        ))}
      </dl>
      <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2">
        {at && (
          <span className="min-w-0 flex-1 text-caption text-faint">
            <Trans>Updated {seen}</Trans>
          </span>
        )}
        <Button
          type="button"
          variant="outline"
          className={cn(look.btnSm, "ms-auto")}
          onClick={() => void copy(text)}
          aria-label={t`Copy device details`}
          data-testid="device-info-copy"
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? <Trans>Copied</Trans> : <Trans>Copy</Trans>}
        </Button>
      </div>
    </div>
  )
}
