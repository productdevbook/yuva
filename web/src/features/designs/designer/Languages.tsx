import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { LanguagesIcon, PlusIcon, Settings2Icon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { Notice, Segmented, useConfirm } from "@/components/common"
import { LanguagePicker } from "@/components/common/LanguagePicker"
import { canonicalTag, useLanguageName } from "@/components/common/text"
import { useScrollFade } from "@/components/common/useScrollFade"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import {
  fallbackOf,
  MAX_VERSIONS,
  missing,
  removeVersion,
  sameTag,
  setTranslation,
  versions,
  type Doc,
} from "@/features/designs/doc"
import { useDesigner } from "@/features/designs/designer/context"
import { cn } from "@/lib/utils"

export function useMissingText() {
  return (count: number) => plural(count, { one: "# text not translated", other: "# texts not translated" })
}

export function LayoutHint({ className }: { className?: string }) {
  const d = useDesigner()
  const name = useLanguageName()
  if (!d.locale) return null
  const language = name(d.original.locale ?? "")
  return (
    <Notice icon={LanguagesIcon} className={cn("text-small", className)} data-testid="layout-hint">
      <p>
        <Trans>Layout is shared by all languages. Switch to {language} to change it.</Trans>
      </p>
      <button
        type="button"
        className="w-fit text-brand underline-offset-4 hover:underline"
        onClick={() => d.setLocale(null)}
        data-testid="layout-hint-base"
      >
        {language}
      </button>
    </Notice>
  )
}

export function LanguageBar({
  doc,
  current,
  onSelect,
  onAdd,
  onChange,
  readOnly,
}: {
  doc: Doc
  current: string | null
  onSelect: (tag: string | null) => void
  onAdd: () => void
  onChange: (fn: (d: Doc) => Doc) => void
  readOnly: boolean
}) {
  const { t } = useLingui()
  const name = useLanguageName()
  const missingText = useMissingText()
  const tags = versions(doc)
  const base = tags[0]
  const scroll = useScrollFade<HTMLDivElement>(current, tags.length)
  return (
    <div
      className="flex shrink-0 items-center gap-1.5 border-b px-3 py-1.5 phone:px-2"
      role="toolbar"
      aria-label={t`Languages`}
      data-testid="language-bar"
    >
      <LanguagesIcon className="size-4 shrink-0 text-faint" aria-hidden />
      <div
        {...scroll}
        className="flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {tags.map((tag) => {
          const on = tag === base ? current === null : current === tag
          const gaps = tag === base ? 0 : missing(doc, tag).length
          return (
            <button
              key={tag}
              type="button"
              onClick={() => onSelect(tag === base ? null : tag)}
              aria-pressed={on}
              className={cn(
                "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border px-2.5 text-small whitespace-nowrap transition-colors",
                on ? "border-foreground bg-foreground text-background" : "hover:border-faint"
              )}
              data-testid="language-chip"
              data-locale={tag}
            >
              {name(tag)}
              {tag === base && (
                <span className={cn("text-caption", on ? "opacity-70" : "text-faint")}>
                  · <Trans>Base</Trans>
                </span>
              )}
              {gaps > 0 && (
                <span
                  className="inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-warning px-1 text-[10px] font-semibold text-white tabular-nums"
                  title={missingText(gaps)}
                  aria-label={missingText(gaps)}
                  data-testid="language-missing"
                >
                  {gaps}
                </span>
              )}
            </button>
          )
        })}
      </div>
      {!readOnly && (
        <>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 shrink-0 gap-1 px-2 text-small"
            onClick={onAdd}
            disabled={tags.length > MAX_VERSIONS}
            data-testid="language-add"
          >
            <PlusIcon />
            <span className="phone:sr-only">
              <Trans>Language</Trans>
            </span>
          </Button>
          <LanguageSettings doc={doc} onChange={onChange} onRemoved={(tag) => tag === current && onSelect(null)} />
        </>
      )}
    </div>
  )
}

function LanguageSettings({
  doc,
  onChange,
  onRemoved,
}: {
  doc: Doc
  onChange: (fn: (d: Doc) => Doc) => void
  onRemoved: (tag: string) => void
}) {
  const { t } = useLingui()
  const name = useLanguageName()
  const [confirm, confirmDialog] = useConfirm()
  const tags = versions(doc)
  const base = tags[0] ?? ""
  const items = Object.fromEntries(tags.map((tag) => [tag, name(tag)]))
  const directions = [
    { value: "same", label: t`As the base` },
    { value: "ltr", label: t`Left to right` },
    { value: "rtl", label: t`Right to left` },
  ] as const
  return (
    <>
      <Popover>
        <PopoverTrigger
          render={<Button variant="ghost" size="icon-sm" className="shrink-0" />}
          aria-label={t`Language settings`}
          data-testid="language-settings"
        >
          <Settings2Icon />
        </PopoverTrigger>
        <PopoverContent align="end" className="flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="design-base-language">
              <Trans>Base language</Trans>
            </Label>
            <LanguagePicker
              id="design-base-language"
              value={base}
              label={t`Base language`}
              exclude={tags.slice(1)}
              onChange={(tag) =>
                onChange((d) => ({
                  ...d,
                  locale: tag,
                  fallback_locale: sameTag(d.fallback_locale, d.locale) ? tag : d.fallback_locale,
                }))
              }
              testId="design-base-language"
            />
            <p className="text-caption text-faint">
              <Trans>The language the blocks are written in. Layout, images and links come from it.</Trans>
            </p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>
              <Trans>Fallback language</Trans>
            </Label>
            <Select
              value={fallbackOf(doc) ?? base}
              items={items}
              onValueChange={(v) =>
                v && onChange((d) => ({ ...d, fallback_locale: v === d.locale ? undefined : String(v) }))
              }
            >
              <SelectTrigger className="w-full min-w-0" aria-label={t`Fallback language`} data-testid="design-fallback">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {tags.map((tag) => (
                  <SelectItem key={tag} value={tag}>
                    {name(tag)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-caption text-faint">
              <Trans>For contacts without a language, or whose language has no version.</Trans>
            </p>
          </div>
          <ul className="flex flex-col gap-3 border-t pt-3">
            {tags.slice(1).map((tag) => {
              const language = name(tag)
              const dir = doc.translations?.[tag]?.direction
              return (
                <li key={tag} className="flex flex-col gap-1.5" data-testid="language-row" data-locale={tag}>
                  <div className="flex items-center gap-2">
                    <span className="min-w-0 flex-1 truncate text-small font-medium">{language}</span>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      className="hover:text-destructive"
                      aria-label={t`Remove ${language}`}
                      onClick={() =>
                        confirm({
                          title: t`Remove ${language} and its texts?`,
                          description: t`Contacts who would get it get the fallback language instead.`,
                          confirm: t`Remove ${language}`,
                          run: () => {
                            onChange((d) => removeVersion(d, tag))
                            onRemoved(tag)
                          },
                        })
                      }
                      data-testid="language-remove"
                    >
                      <Trash2Icon />
                    </Button>
                  </div>
                  <Segmented
                    value={dir === "ltr" || dir === "rtl" ? dir : "same"}
                    onChange={(v) =>
                      onChange((d) =>
                        setTranslation(d, tag, (x) => ({ ...x, direction: v === "same" ? undefined : v }))
                      )
                    }
                    label={t`Writing direction`}
                    className="w-full"
                    itemClassName="flex-1 px-1 text-small"
                    items={directions}
                  />
                </li>
              )
            })}
          </ul>
        </PopoverContent>
      </Popover>
      {confirmDialog}
    </>
  )
}

export function AddLanguageDialog({
  open,
  onOpenChange,
  doc,
  suggestedBase,
  onAdd,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  doc: Doc
  suggestedBase: string
  onAdd: (tag: string, base: string) => void
}) {
  const { t } = useLingui()
  const needBase = !doc.locale
  const [base, setBase] = useState(() => canonicalTag(suggestedBase) ?? "")
  const [tag, setTag] = useState("")
  const taken = [doc.locale ?? base, ...Object.keys(doc.translations ?? {})].filter(Boolean)
  const valid = !!tag && (!needBase || (!!base && !sameTag(base, tag))) && !taken.some((x) => sameTag(x, tag))
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o)
        if (!o) setTag("")
      }}
    >
      <DialogContent data-testid="add-language-dialog">
        <form
          className="flex min-w-0 flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!valid) return
            onAdd(tag, base)
            setTag("")
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Add a language</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                The new version starts with the texts of the base language. Layout, images and links stay shared; you
                translate only the texts.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          {needBase && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="add-language-base">
                <Trans>Base language</Trans>
              </Label>
              <LanguagePicker
                id="add-language-base"
                value={base}
                label={t`Base language`}
                onChange={setBase}
                testId="add-language-base"
              />
              <p className="text-small text-faint">
                <Trans>The language the design is written in now.</Trans>
              </p>
            </div>
          )}
          <div className="flex flex-col gap-2">
            <Label htmlFor="add-language-tag">
              <Trans>Language</Trans>
            </Label>
            <LanguagePicker
              id="add-language-tag"
              value={tag}
              label={t`Language`}
              exclude={taken}
              onChange={setTag}
              testId="add-language-tag"
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!valid} data-testid="add-language-submit">
              <Trans>Add</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
