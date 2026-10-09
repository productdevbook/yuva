import { useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { CheckIcon, LanguagesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { activate, locales, type Locale } from "@/i18n"
import { api, unwrap } from "@/lib/api"
import { meKey, useMe } from "@/lib/session"

export function useChangeLocale() {
  const qc = useQueryClient()
  const me = useMe()
  const save = useMutation({
    mutationFn: (locale: Locale) => unwrap(api.PATCH("/v1/me", { body: { locale } })),
    onSuccess: (data) => qc.setQueryData(meKey, data),
    onError: () => {
      if (me.data) activate(me.data.person.locale)
    },
  })
  return (locale: Locale) => {
    activate(locale)
    if (me.data && me.data.person.locale !== locale) save.mutate(locale)
  }
}

export function LanguageMenu() {
  const { t, i18n } = useLingui()
  const choose = useChangeLocale()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="sm" aria-label={t`Language`} />}>
        <LanguagesIcon />
        <span className="eyebrow">{i18n.locale}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-36">
        {Object.entries(locales).map(([k, v]) => (
          <DropdownMenuItem key={k} onClick={() => choose(k as Locale)}>
            <span className="flex-1">{v}</span>
            {i18n.locale === k && <CheckIcon className="text-foreground" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
