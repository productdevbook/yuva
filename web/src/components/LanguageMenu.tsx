import { useLingui } from "@lingui/react/macro"
import { CheckIcon, LanguagesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { activate, locales, type Locale } from "@/i18n"

export function LanguageMenu() {
  const { t, i18n } = useLingui()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="sm" aria-label={t`Language`} />}>
        <LanguagesIcon />
        <span className="uppercase">{i18n.locale}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-36">
        {Object.entries(locales).map(([k, v]) => (
          <DropdownMenuItem key={k} onClick={() => activate(k as Locale)}>
            <span className="flex-1">{v}</span>
            {i18n.locale === k && <CheckIcon />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
