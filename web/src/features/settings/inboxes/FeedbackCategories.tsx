import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowDownIcon, ArrowUpIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { useConfirm } from "@/components/common/ConfirmDialog"
import { CATEGORY_ICONS, CategoryIcon, categoryTone } from "@/components/common/feedbackCategories"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Block, ChoiceSelect } from "@/features/settings/ui"
import type { InboxFeedbackCategory } from "@/lib/api"
import { cn } from "@/lib/utils"

export const MAX_CATEGORIES = 20
export const CATEGORY_KEY = /^[a-z][a-z0-9_]{0,31}$/
const NAME_MAX = 60

function languageName(tag: string, locale: string) {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(tag) ?? tag
  } catch {
    return tag
  }
}

export function categoriesInvalid(list: InboxFeedbackCategory[], locale: string) {
  return (
    list.length < 1 ||
    list.length > MAX_CATEGORIES ||
    !list.some((c) => c.key === "other") ||
    list.some((c) => !c.names[locale]?.trim())
  )
}

export function cleanCategories(list: InboxFeedbackCategory[]): InboxFeedbackCategory[] {
  return list.map((c) => ({
    key: c.key,
    icon: c.icon,
    names: Object.fromEntries(
      Object.entries(c.names)
        .map(([l, n]) => [l, n.trim()] as const)
        .filter(([, n]) => n)
    ),
  }))
}

function keyFor(name: string, taken: Set<string>) {
  const base =
    name
      .normalize("NFKD")
      .replace(/[\u0300-\u036f]/g, "")
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^[^a-z]+/, "")
      .replace(/_+$/, "")
      .slice(0, 28) || "category"
  let key = base
  for (let i = 2; taken.has(key); i++) key = `${base}_${i}`
  return key
}

function IconPicker({
  value,
  onChange,
  disabled,
  label,
}: {
  value: string
  onChange: (v: string) => void
  disabled: boolean
  label: string
}) {
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={<Button type="button" variant="outline" size="icon-sm" className="rounded-lg" />}
        disabled={disabled}
        aria-label={label}
        title={label}
      >
        <CategoryIcon name={value} className="size-4" />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 gap-0 p-2">
        <div className="grid grid-cols-5 gap-1" role="group" aria-label={label}>
          {Object.keys(CATEGORY_ICONS).map((name) => (
            <Button
              key={name}
              type="button"
              variant={name === value ? "secondary" : "ghost"}
              size="icon-sm"
              className="rounded-lg"
              aria-pressed={name === value}
              title={name}
              aria-label={name}
              onClick={() => {
                onChange(name)
                setOpen(false)
              }}
            >
              <CategoryIcon name={name} className="size-4" />
            </Button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}

export function FeedbackCategoriesBlock({
  list,
  saved,
  locale,
  langs,
  locked,
  onChange,
}: {
  list: InboxFeedbackCategory[]
  saved: InboxFeedbackCategory[]
  locale: string
  langs: string[]
  locked: boolean
  onChange: (list: InboxFeedbackCategory[]) => void
}) {
  const { t, i18n } = useLingui()
  const [confirm, dialog] = useConfirm()
  const [extra, setExtra] = useState<string[]>([])
  const shown = [...new Set([locale, ...list.flatMap((c) => Object.keys(c.names)), ...extra])]
  const addable = langs.filter((l) => !shown.includes(l))
  const nameIn = (c: InboxFeedbackCategory) => c.names[locale] || c.names.en || Object.values(c.names)[0] || c.key

  const patch = (i: number, p: Partial<InboxFeedbackCategory>) =>
    onChange(list.map((c, j) => (j === i ? { ...c, ...p } : c)))
  const rename = (i: number, lang: string, value: string) => {
    const c = list[i]!
    const fresh = !saved.some((s) => s.key === c.key)
    const key =
      fresh && lang === locale && value.trim()
        ? keyFor(value, new Set(list.filter((_, j) => j !== i).map((x) => x.key)))
        : c.key
    patch(i, { key, names: { ...c.names, [lang]: value } })
  }
  const move = (i: number, by: number) => {
    const next = [...list]
    const [item] = next.splice(i, 1)
    next.splice(i + by, 0, item!)
    onChange(next)
  }
  const remove = (i: number) => onChange(list.filter((_, j) => j !== i))
  const ask = (i: number) => {
    const c = list[i]!
    if (!saved.some((s) => s.key === c.key)) return remove(i)
    const name = nameIn(c)
    confirm({
      title: t`Remove ${name}?`,
      description: t`Feedback already sent as ${name} moves to Other when you save.`,
      confirm: t`Remove`,
      run: () => remove(i),
    })
  }
  const add = () => {
    const taken = new Set(list.map((c) => c.key))
    const key = keyFor("category", taken)
    const at = list.findIndex((c) => c.key === "other")
    const next = [...list]
    next.splice(at < 0 ? next.length : at, 0, { key, icon: "tag", names: { [locale]: "" } })
    onChange(next)
  }

  return (
    <Block
      title={<Trans>Feedback categories</Trans>}
      description={<Trans>What people can pick when they send feedback from your apps and pages</Trans>}
      testId="feedback-categories-block"
      footer={
        locked ? undefined : (
          <>
            {addable.length > 0 && (
              <ChoiceSelect
                value=""
                placeholder={t`Add a language`}
                label={t`Add a language`}
                options={addable.map((l) => [l, languageName(l, i18n.locale)] as const)}
                onChange={(l) => setExtra((x) => [...x, l])}
                className="w-48"
              />
            )}
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={list.length >= MAX_CATEGORIES}
              onClick={add}
              data-testid="category-add"
            >
              <PlusIcon />
              <Trans>Add a category</Trans>
            </Button>
          </>
        )
      }
    >
      <ul className="flex flex-col" data-testid="category-list">
        {list.map((c, i) => {
          const isOther = c.key === "other"
          return (
            <li
              key={i}
              className="flex items-start gap-3 border-b px-[18px] py-3 phone:flex-wrap"
              data-testid="category-row"
              data-category={c.key}
            >
              <IconPicker value={c.icon} disabled={locked} label={t`Icon`} onChange={(icon) => patch(i, { icon })} />
              <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                {shown.map((lang) => (
                  <div key={lang} className="flex items-center gap-2">
                    <span className="w-24 shrink-0 truncate text-caption text-faint">
                      {languageName(lang, i18n.locale)}
                    </span>
                    <Input
                      value={c.names[lang] ?? ""}
                      maxLength={NAME_MAX}
                      disabled={locked}
                      aria-label={`${languageName(lang, i18n.locale)}: ${c.key}`}
                      aria-invalid={(lang === locale && !c.names[lang]?.trim()) || undefined}
                      onChange={(e) => rename(i, lang, e.target.value)}
                      className="h-8 max-w-64 rounded-lg bg-background"
                      data-testid={`category-name-${lang}`}
                    />
                  </div>
                ))}
                <span className={cn("font-mono text-caption", categoryTone(c.key).icon)}>{c.key}</span>
              </div>
              {!locked && (
                <div className="flex shrink-0 items-center gap-0.5">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={i === 0}
                    onClick={() => move(i, -1)}
                    aria-label={t`Move up`}
                  >
                    <ArrowUpIcon />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={i === list.length - 1}
                    onClick={() => move(i, 1)}
                    aria-label={t`Move down`}
                  >
                    <ArrowDownIcon />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={isOther}
                    title={isOther ? t`Other stays: removed categories move here` : undefined}
                    onClick={() => ask(i)}
                    aria-label={t`Remove`}
                    data-testid="category-remove"
                  >
                    <Trash2Icon />
                  </Button>
                </div>
              )}
            </li>
          )
        })}
      </ul>
      {dialog}
    </Block>
  )
}
