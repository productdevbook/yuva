import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react"
import { useMemo, useState } from "react"

import { canonicalTag, languageName } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { localeCodes } from "@/localeList"
import { cn } from "@/lib/utils"

const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase()

export function LanguagePicker({
  value,
  onChange,
  exclude = [],
  id,
  label,
  className,
  testId,
}: {
  value: string
  onChange: (tag: string) => void
  exclude?: string[]
  id?: string
  label: string
  className?: string
  testId?: string
}) {
  const { t, i18n } = useLingui()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const rows = useMemo(
    () =>
      [...localeCodes]
        .filter((c) => !exclude.some((x) => same(x, c)))
        .map((c) => ({ tag: c, name: languageName(c, i18n.locale), own: languageName(c, c) }))
        .sort((a, b) => a.name.localeCompare(b.name, i18n.locale)),
    [exclude, i18n.locale]
  )
  const typed = canonicalTag(search)
  const custom = typed && !rows.some((r) => same(r.tag, typed)) && !exclude.some((x) => same(x, typed)) ? typed : null
  const pick = (tag: string) => {
    onChange(tag)
    setOpen(false)
    setSearch("")
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            id={id}
            type="button"
            variant="outline"
            aria-label={label}
            className={cn("h-9 w-full justify-between rounded-xl px-3 font-normal", className)}
            data-testid={testId}
          />
        }
      >
        <span className="truncate">
          {value ? (
            <>
              {languageName(value, i18n.locale)} <span className="text-faint">· {value}</span>
            </>
          ) : (
            <span className="text-faint">
              <Trans>Pick a language</Trans>
            </span>
          )}
        </span>
        <ChevronsUpDownIcon className="size-3.5 shrink-0 text-faint" />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--anchor-width) min-w-64 gap-0 p-0">
        <Command label={label}>
          <CommandInput placeholder={t`Search languages or type a tag`} value={search} onValueChange={setSearch} />
          <CommandList>
            <CommandEmpty>
              <Trans>No language matches</Trans>
            </CommandEmpty>
            {custom && (
              <CommandItem value={`${custom} ${search}`} onSelect={() => pick(custom)}>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">{languageName(custom, i18n.locale)}</span>
                  <small className="truncate text-caption text-faint" dir="ltr">
                    {custom}
                  </small>
                </span>
              </CommandItem>
            )}
            {rows.map((r) => (
              <CommandItem
                key={r.tag}
                value={`${r.name} ${r.own} ${r.tag}`}
                onSelect={() => pick(r.tag)}
                className="flex items-start gap-2"
                data-testid="language-option"
              >
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">{r.name}</span>
                  <small className="truncate text-caption text-faint">
                    {r.own !== r.name ? `${r.own} · ${r.tag}` : r.tag}
                  </small>
                </span>
                {same(r.tag, value) && <CheckIcon className="mt-0.5 size-4 shrink-0" />}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
