import { Trans, useLingui } from "@lingui/react/macro"
import {
  CheckIcon,
  ChevronDownIcon,
  LayoutTemplateIcon,
  MonitorIcon,
  MoonIcon,
  SmartphoneIcon,
  UserRoundXIcon,
  XIcon,
} from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ErrorLine, Notice, Segmented } from "@/components/common"
import {
  blocksSend,
  useDesignCheck,
  useDesignPreview,
  useOfferedDesigns,
  type DesignPick,
} from "@/components/common/designChoice"
import { MailFrame } from "@/components/common/MailFrame"
import { popupClass } from "@/components/ui/dropdown-menu"
import { useLanguageName } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Command, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { versions } from "@/features/designs/doc"
import { Preview } from "@/features/designs/designer/Preview"
import { useDesign } from "@/features/designs/queries"
import { useContact } from "@/features/contact/queries"
import { useDebounced } from "@/hooks/use-debounced"
import { useNear } from "@/hooks/use-near"
import { cn } from "@/lib/utils"
import { useSession } from "@/lib/session"

const THUMB_SCALE = 0.1

function Thumb({ conversationId, id }: { conversationId: string | undefined; id: string }) {
  const { t } = useLingui()
  const [el, setEl] = useState<HTMLDivElement | null>(null)
  const near = useNear(el, !!conversationId, "0px")
  const check = useDesignCheck(near ? conversationId : undefined, near ? { id, name: "" } : undefined)
  return (
    <div
      ref={setEl}
      className="relative h-20 w-[60px] shrink-0 overflow-hidden rounded-md border bg-white"
      data-testid="design-option-thumb"
    >
      {check.data ? (
        <div
          className="pointer-events-none absolute top-0 start-0 origin-top-left rtl:origin-top-right"
          style={{ width: 600, transform: `scale(${THUMB_SCALE})` }}
          aria-hidden
        >
          <MailFrame html={check.data.html} width={600} title={t`Preview of the design`} />
        </div>
      ) : check.isError || !conversationId ? (
        <div className="grid h-full place-items-center text-faint">
          <LayoutTemplateIcon className="size-5" />
        </div>
      ) : (
        <Skeleton className="h-full w-full rounded-none" />
      )}
    </div>
  )
}

function DesignList({
  inboxId,
  conversationId,
  chosen,
  onPick,
}: {
  inboxId: string
  conversationId: string | undefined
  chosen: DesignPick | undefined
  onPick: (d: DesignPick | undefined) => void
}) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const [q, setQ] = useState("")
  const query = useDebounced(q.trim(), 250)
  const list = useOfferedDesigns(inboxId, query, true)
  const items = list.data ?? []
  return (
    <Command shouldFilter={false} className="bg-transparent p-1.5" label={t`Designs`}>
      <CommandInput value={q} onValueChange={setQ} placeholder={t`Search designs…`} data-testid="design-search" />
      <CommandList className="max-h-80">
        {chosen && (
          <CommandItem
            value="none"
            onSelect={() => onPick(undefined)}
            className="rounded-lg px-2.5 py-2 data-selected:bg-muted"
            data-testid="design-none"
          >
            <XIcon className="text-faint" />
            <Trans>No design</Trans>
          </CommandItem>
        )}
        {list.isPending ? (
          <div className="flex flex-col gap-2 p-2" data-testid="design-loading">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : list.isError ? (
          <ErrorLine error={list.error} className="px-3 py-4 text-caption" />
        ) : items.length === 0 ? (
          <div className="px-3 py-6 text-center text-small text-faint" data-testid="design-empty">
            {query ? (
              <Trans>No design matches.</Trans>
            ) : (
              <>
                <Trans>No designs are offered in conversations yet. Owners and admins offer one in the designer.</Trans>
                {canManage && (
                  <Link
                    to="/broadcasts/designs"
                    className="mt-2 block font-medium text-foreground underline underline-offset-2"
                  >
                    <Trans>Open the designs</Trans>
                  </Link>
                )}
              </>
            )}
          </div>
        ) : (
          items.map((d) => (
            <CommandItem
              key={d.id}
              value={d.id}
              onSelect={() => onPick({ id: d.id, name: d.name })}
              className="gap-3 rounded-lg px-2.5 py-2 data-selected:bg-muted"
              data-testid="design-option"
            >
              <Thumb conversationId={conversationId} id={d.id} />
              <span className="min-w-0 flex-1 truncate font-medium">{d.name}</span>
              {chosen?.id === d.id && <CheckIcon className="size-4 text-faint" />}
            </CommandItem>
          ))
        )}
      </CommandList>
    </Command>
  )
}

export function DesignButton({
  open,
  onOpenChange,
  inboxId,
  conversationId,
  chosen,
  onPick,
  shortcut,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  inboxId: string
  conversationId: string | undefined
  chosen: DesignPick | undefined
  onPick: (d: DesignPick | undefined) => void
  shortcut: string
}) {
  const { t } = useLingui()
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={<Button variant="ghost" size="sm" className="text-faint hover:bg-muted aria-expanded:bg-muted" />}
        title={`${t`Send in an e-mail design`} (${shortcut})`}
        data-testid="design-button"
      >
        <LayoutTemplateIcon />
        <span className="@max-md/composer:hidden">
          <Trans>Design</Trans>
        </span>
      </PopoverTrigger>
      <PopoverContent
        side="top"
        align="start"
        className={cn(popupClass, "w-[min(380px,calc(100vw-48px))] gap-0 rounded-xl p-0 ring-0")}
      >
        <DesignList
          inboxId={inboxId}
          conversationId={conversationId}
          chosen={chosen}
          onPick={(d) => {
            onPick(d)
            onOpenChange(false)
          }}
        />
      </PopoverContent>
    </Popover>
  )
}

function sameLanguage(a: string, b: string) {
  return a.split("-")[0].toLowerCase() === b.split("-")[0].toLowerCase()
}

export function DesignBar({
  pick,
  onChange,
  conversationId,
  contactId,
  body,
  className,
}: {
  pick: DesignPick
  onChange: (d: DesignPick | undefined) => void
  conversationId: string | undefined
  contactId?: string
  body: string
  className?: string
}) {
  const { t } = useLingui()
  const name = useLanguageName()
  const [previewing, setPreviewing] = useState(false)
  const check = useDesignCheck(conversationId, pick)
  const design = useDesign(pick.id)
  const contact = useContact(contactId)
  const tags = design.data ? versions(design.data.document) : []
  const tag = pick.locale ?? check.data?.locale
  const fallback =
    !pick.locale && !!tag && !!contact.data && !(contact.data.locale && sameLanguage(contact.data.locale, tag))
  const tagName = tag ? name(tag) : ""
  const language = tags.length > 0 && tag ? (fallback ? t`Fallback: ${tagName}` : tagName) : ""
  const failed = check.isError && blocksSend(check.error)
  return (
    <div className={cn("flex flex-col gap-1.5 px-4 pb-2", className)} data-testid="composer-design">
      <div className="flex flex-wrap items-center gap-1.5">
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <button
                type="button"
                className="flex max-w-full items-center gap-1.5 rounded-lg border bg-background py-1 ps-2 pe-1.5 text-caption outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                data-testid="design-chip"
              />
            }
          >
            <LayoutTemplateIcon className="size-3 shrink-0 text-faint" />
            <span className="max-w-56 truncate">{pick.name}</span>
            {language && <span className="shrink-0 text-faint">· {language}</span>}
            {tags.length > 0 && <ChevronDownIcon className="size-3 shrink-0 text-faint" />}
          </DropdownMenuTrigger>
          {tags.length > 0 && (
            <DropdownMenuContent align="start" className="w-64 max-w-[calc(100vw-32px)]">
              <DropdownMenuGroup>
                <DropdownMenuLabel>
                  <Trans>Language of the design</Trans>
                </DropdownMenuLabel>
                <DropdownMenuItem onClick={() => onChange({ ...pick, locale: undefined })}>
                  <span className="flex-1">
                    <Trans>The contact’s language</Trans>
                  </span>
                  {!pick.locale && <CheckIcon className="size-4" />}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                {tags.map((tg) => (
                  <DropdownMenuItem key={tg} onClick={() => onChange({ ...pick, locale: tg })}>
                    <span className="flex-1">{name(tg)}</span>
                    {pick.locale === tg && <CheckIcon className="size-4" />}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          )}
        </DropdownMenu>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 px-2 text-caption text-muted-foreground"
          onClick={() => setPreviewing(true)}
          data-testid="design-preview"
        >
          <Trans>Preview</Trans>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 px-2 text-caption text-faint"
          onClick={() => onChange(undefined)}
          data-testid="design-remove"
        >
          <Trans>Remove</Trans>
        </Button>
      </div>
      {failed && <ErrorLine error={check.error} className="text-caption" />}
      {check.data?.footer === "outreach" && (
        <Notice icon={UserRoundXIcon} className="px-3 py-2 text-caption" data-testid="design-outreach">
          <Trans>This first mail carries an unsubscribe link.</Trans>
        </Notice>
      )}
      <DesignPreviewSheet
        open={previewing}
        onOpenChange={setPreviewing}
        conversationId={conversationId}
        pick={pick}
        body={body}
      />
    </div>
  )
}

function DesignPreviewSheet({
  open,
  onOpenChange,
  conversationId,
  pick,
  body,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  conversationId: string | undefined
  pick: DesignPick
  body: string
}) {
  const { t } = useLingui()
  const [phone, setPhone] = useState(false)
  const [dark, setDark] = useState(false)
  const title = pick.name
  const typed = useDebounced(body.trim(), 800)
  const preview = useDesignPreview(conversationId, pick, typed, dark, open)
  const stale = typed !== body.trim() || preview.isPlaceholderData || preview.isFetching
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:w-[min(1040px,96vw)]" data-testid="design-preview-sheet">
        <SheetHeader>
          <SheetTitle className="text-title">
            <Trans>Preview of {title}</Trans>
          </SheetTitle>
          <SheetDescription>
            {preview.data ? (
              preview.data.subject
            ) : conversationId ? (
              <Trans>What the contact would get.</Trans>
            ) : (
              <Trans>Add a recipient, a subject and some text: the preview appears once the draft is saved.</Trans>
            )}
          </SheetDescription>
        </SheetHeader>
        <div className="flex flex-wrap items-center gap-2 border-b px-5 py-2">
          <Segmented
            value={phone ? "phone" : "desktop"}
            onChange={(v) => setPhone(v === "phone")}
            label={t`Screen size`}
            items={[
              {
                value: "desktop",
                label: (
                  <>
                    <MonitorIcon className="size-3.5" />
                    <Trans>Desktop</Trans>
                  </>
                ),
              },
              {
                value: "phone",
                label: (
                  <>
                    <SmartphoneIcon className="size-3.5" />
                    <Trans>Phone</Trans>
                  </>
                ),
              },
            ]}
          />
          <Button
            variant="outline"
            size="sm"
            aria-pressed={dark}
            onClick={() => setDark((x) => !x)}
            data-testid="design-preview-dark"
          >
            <MoonIcon />
            <Trans>Dark</Trans>
          </Button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          <Preview
            phone={phone}
            dark={dark}
            html={preview.data?.html}
            loading={preview.isFetching}
            error={preview.error}
            stale={stale && !!preview.data}
          />
        </div>
      </SheetContent>
    </Sheet>
  )
}
