import "@/features/designs/i18n"

import { Trans, useLingui } from "@lingui/react/macro"
import { FilePlusIcon, LanguagesIcon, LoaderIcon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import { CheckItem, CheckList, Dot, ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { designLink, useCreateDesign, useTemplates } from "@/features/designs/queries"
import { useCategory, useTemplateLocales } from "@/features/designs/text"
import type { EmailTemplate, EmailTemplateLocale } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const WORKSPACE = "workspace"

export type NewDesign = { use: EmailTemplate } | "choose"

function Tile({
  tpl,
  locale,
  on,
  onPick,
}: {
  tpl: EmailTemplate | null
  locale: EmailTemplateLocale
  on: boolean
  onPick: () => void
}) {
  const { t } = useLingui()
  const category = useCategory()
  const title = tpl?.name ?? ""
  return (
    <li>
      <button
        type="button"
        onClick={onPick}
        aria-pressed={on}
        className={cn(
          "flex h-full w-full flex-col gap-2 rounded-xl border p-2 text-start outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring",
          on && "border-foreground bg-muted"
        )}
        data-testid={tpl ? "new-design-tile" : "new-design-blank"}
      >
        <span className="relative block h-[84px] overflow-hidden rounded-md border bg-white">
          {tpl ? (
            <img
              src={tpl.preview_url}
              alt={t`Preview of ${title}`}
              loading="lazy"
              className="absolute inset-x-0 top-0 w-full"
            />
          ) : (
            <span className="grid h-full place-items-center border-2 border-dashed text-muted-foreground">
              <FilePlusIcon className="size-5" />
            </span>
          )}
        </span>
        <span className="flex min-w-0 flex-col leading-snug">
          <span className="truncate text-small font-medium">{tpl ? tpl.name : <Trans>Empty page</Trans>}</span>
          <span className="flex min-w-0 items-center gap-1 text-caption text-faint">
            {tpl && !tpl.locales.includes(locale) && <LanguagesIcon className="size-3 shrink-0" />}
            <span className="truncate">
              {tpl ? (
                tpl.locales.includes(locale) ? (
                  category(tpl.category)
                ) : (
                  <Trans>Text in English</Trans>
                )
              ) : (
                <Trans>Build it yourself from blocks</Trans>
              )}
            </span>
          </span>
        </span>
      </button>
    </li>
  )
}

export function NewDesignDialog({
  mode,
  locale,
  onClose,
}: {
  mode: NewDesign
  locale: EmailTemplateLocale
  onClose: () => void
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const inboxes = useInboxes().data ?? []
  const create = useCreateDesign()
  const templates = useTemplates(locale)
  const [picked, setPicked] = useState<EmailTemplate | null>(null)
  const [typed, setTyped] = useState<string | null>(null)
  const [inbox, setInbox] = useState(inboxes.length === 1 ? inboxes[0].id : WORKSPACE)
  const choosing = mode === "choose"
  const tpl = choosing ? picked : mode.use
  const localeNames = useTemplateLocales()
  const offered = tpl ? localeNames.filter((l) => tpl.locales.includes(l.value)) : []
  const [chosen, setChosen] = useState<EmailTemplateLocale[] | null>(null)
  const kept = (chosen ?? []).filter((l) => tpl?.locales.includes(l))
  const langs: EmailTemplateLocale[] = kept.length
    ? kept
    : [tpl?.locales.includes(locale) ? locale : (offered[0]?.value ?? locale)]
  const toggle = (l: EmailTemplateLocale, on: boolean) =>
    setChosen(on ? [...langs.filter((x) => x !== l), l] : langs.filter((x) => x !== l))
  const name = typed ?? (tpl ? tpl.name : t`Untitled design`)
  const items: Record<string, string> = {
    [WORKSPACE]: t`Whole workspace`,
    ...Object.fromEntries(inboxes.map((i) => [i.id, i.name])),
  }
  const submit = () => {
    if (!name.trim()) return
    create.mutate(
      {
        name: name.trim(),
        inbox_id: inbox === WORKSPACE ? undefined : inbox,
        template: tpl
          ? { slug: tpl.slug, locale: langs[0], locales: langs.length > 1 ? langs.slice(1) : undefined }
          : undefined,
      },
      { onSuccess: (d) => navigate(designLink(d.id)) }
    )
  }
  const title = tpl?.name ?? ""
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className={cn(choosing && "sm:max-w-2xl")} data-testid="create-design-dialog">
        <form
          className="flex min-w-0 flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{choosing ? <Trans>New design</Trans> : <Trans>Use “{title}”</Trans>}</DialogTitle>
            <DialogDescription>
              {choosing && !tpl ? (
                <Trans>Start from an empty mail with simple colours.</Trans>
              ) : (
                <Trans>Its images are copied into your workspace’s images. You can change everything after.</Trans>
              )}
            </DialogDescription>
          </DialogHeader>
          {choosing && (
            <ul
              className="grid max-h-[min(22rem,40svh)] grid-cols-2 gap-2.5 overflow-y-auto p-0.5 sm:grid-cols-4"
              aria-label={t`Gallery`}
              data-testid="new-design-tiles"
            >
              <Tile tpl={null} locale={locale} on={!picked} onPick={() => setPicked(null)} />
              {templates.isPending
                ? Array.from({ length: 3 }, (_, i) => (
                    <li key={i}>
                      <Skeleton className="h-[132px] w-full rounded-xl" />
                    </li>
                  ))
                : (templates.data ?? []).map((x) => (
                    <Tile
                      key={x.slug}
                      tpl={x}
                      locale={locale}
                      on={picked?.slug === x.slug}
                      onPick={() => setPicked(x)}
                    />
                  ))}
            </ul>
          )}
          {offered.length > 1 && (
            <CheckList
              labelId="new-design-languages"
              label={<Trans>Languages</Trans>}
              hint={
                <p className="text-small text-faint">
                  <Trans>One design with a version per language. The first one you pick is the base.</Trans>
                </p>
              }
            >
              {offered.map((l) => (
                <CheckItem
                  key={l.value}
                  checked={langs.includes(l.value)}
                  disabled={langs.length === 1 && langs[0] === l.value}
                  onChange={(on) => toggle(l.value, on)}
                  testId="new-design-language"
                >
                  <span className="flex items-center gap-2">
                    {l.label}
                    {langs[0] === l.value && (
                      <span className="text-caption text-faint">
                        <Trans>Base</Trans>
                      </span>
                    )}
                  </span>
                </CheckItem>
              ))}
            </CheckList>
          )}
          <div className="flex flex-col gap-2">
            <Label htmlFor="design-name-new">
              <Trans>Name</Trans>
            </Label>
            <Input
              id="design-name-new"
              value={name}
              maxLength={200}
              autoFocus
              onChange={(e) => setTyped(e.target.value)}
              data-testid="new-design-name"
            />
          </div>
          {inboxes.length > 0 && (
            <div className="flex flex-col gap-2">
              <Label>
                <Trans>Inbox</Trans>
              </Label>
              <Select value={inbox} items={items} onValueChange={(v) => setInbox(String(v ?? WORKSPACE))}>
                <SelectTrigger className="w-full min-w-0" aria-label={t`Inbox`} data-testid="new-design-inbox">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={WORKSPACE}>{t`Whole workspace`}</SelectItem>
                  {inboxes.map((i) => (
                    <SelectItem key={i.id} value={i.id}>
                      <Dot color={i.branding.color} className="mt-1.5 size-2 rounded-[3px]" />
                      {i.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-small text-faint">
                <Trans>The inbox’s sender details fill the footer every mail gets.</Trans>
              </p>
            </div>
          )}
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!name.trim() || create.isPending} data-testid="create-design">
              {create.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Create and edit</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
