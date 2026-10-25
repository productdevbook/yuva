import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  CircleAlertIcon,
  CopyIcon,
  ImageIcon,
  PlusIcon,
  Trash2Icon,
  TriangleAlertIcon,
  XIcon,
} from "lucide-react"

import { Segmented } from "@/components/common"
import { useLanguageName } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { findBlock, hasText, setColumnCount, textOf, type Block, type BlockOf } from "@/features/designs/doc"
import { useDesigner } from "@/features/designs/designer/context"
import { ColorInput, Group, NumberBox, Row, SliderRow, UrlInput } from "@/features/designs/designer/controls"
import { RichEditor } from "@/features/designs/designer/editor"
import { LayoutHint } from "@/features/designs/designer/Languages"
import { BlockIcon } from "@/features/designs/designer/icons"
import { NETWORKS, isWarning, useBlockNames, useIssueText, useNetworkNames } from "@/features/designs/text"
import type { EmailSocialNetwork } from "@/lib/api"
import { cn } from "@/lib/utils"

function Toggle({
  label,
  hint,
  checked,
  onChange,
  testId,
}: {
  label: React.ReactNode
  hint?: React.ReactNode
  checked: boolean
  onChange: (v: boolean) => void
  testId?: string
}) {
  return (
    <Label className="items-start justify-between gap-3 font-normal">
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="text-small font-medium">{label}</span>
        {hint && <span className="text-caption text-faint">{hint}</span>}
      </span>
      <Switch checked={checked} onCheckedChange={onChange} data-testid={testId} />
    </Label>
  )
}

function useSet<T extends Block>(block: T) {
  const { update } = useDesigner()
  return (patch: Partial<T>, key?: string) => update(block.id, patch as Partial<Block>, key && `${key}:${block.id}`)
}

function Issues({ block }: { block: Block }) {
  const { issues } = useDesigner()
  const text = useIssueText()
  const list = issues.get(block.id) ?? []
  if (list.length === 0) return null
  return (
    <ul className="flex flex-col gap-1.5 px-4 pt-3" data-testid="block-issues">
      {list.map((i, n) => (
        <li
          key={n}
          className={cn(
            "flex gap-2 rounded-lg px-2.5 py-2 text-small",
            isWarning(i) ? "bg-warning/10 text-foreground" : "bg-destructive/8 text-foreground"
          )}
        >
          {isWarning(i) ? (
            <TriangleAlertIcon className="mt-0.5 size-3.5 shrink-0 text-warning" />
          ) : (
            <CircleAlertIcon className="mt-0.5 size-3.5 shrink-0 text-destructive" />
          )}
          <span>{text(i)}</span>
        </li>
      ))}
    </ul>
  )
}

function Layout({ block }: { block: Block }) {
  const { t } = useLingui()
  const set = useSet(block)
  const p = block.padding
  const sides = [t`Top`, t`Right`, t`Bottom`, t`Left`]
  return (
    <Group
      title={<Trans>Layout</Trans>}
      action={
        p && (
          <Button variant="link" size="sm" className="text-small" onClick={() => set({ padding: undefined })}>
            <Trans>Default spacing</Trans>
          </Button>
        )
      }
    >
      <div className="grid grid-cols-4 gap-2">
        {sides.map((label, i) => (
          <NumberBox
            key={label}
            label={label}
            value={p?.[i]}
            min={0}
            max={64}
            onChange={(v) => {
              const next = [...(p ?? [0, 0, 0, 0])] as [number, number, number, number]
              next[i] = v
              set({ padding: next }, "padding")
            }}
          />
        ))}
      </div>
      {block.type !== "spacer" && block.type !== "section" && block.type !== "columns" && (
        <Row label={<Trans>Alignment</Trans>}>
          <Segmented
            value={block.align ?? "auto"}
            onChange={(v) => set({ align: v === "auto" ? undefined : v })}
            label={t`Alignment`}
            className="w-full"
            itemClassName="flex-1 px-1"
            items={[
              { value: "auto", label: t`Default` },
              { value: "left", label: t`Start` },
              { value: "center", label: t`Centre` },
              { value: "right", label: t`End` },
            ]}
          />
        </Row>
      )}
      <ColorInput
        label={<Trans>Background</Trans>}
        value={block.background}
        onChange={(v) => set({ background: v }, "background")}
        clearable
      />
      <Row label={<Trans>Show on</Trans>} hint={<Trans>Some mail apps show every block anyway.</Trans>}>
        <Segmented
          value={block.hide ?? "none"}
          onChange={(v) => set({ hide: v === "none" ? undefined : v })}
          label={t`Show on`}
          className="w-full"
          itemClassName="flex-1 px-1"
          items={[
            { value: "none", label: t`Everywhere` },
            { value: "mobile", label: t`Wide screens` },
            { value: "desktop", label: t`Phones` },
          ]}
        />
      </Row>
    </Group>
  )
}

function SectionFields({ block }: { block: BlockOf<"section"> }) {
  const set = useSet(block)
  const { doc } = useDesigner()
  return (
    <Group title={<Trans>Section</Trans>}>
      <SliderRow
        label={<Trans>Corner radius</Trans>}
        value={block.border_radius ?? doc.theme.radius ?? 0}
        min={0}
        max={24}
        onChange={(v) => set({ border_radius: v }, "radius")}
      />
    </Group>
  )
}

const PRESETS: Record<string, number[]> = {
  "50/50": [50, 50],
  "33/67": [33, 67],
  "67/33": [67, 33],
  "33/34/33": [33, 34, 33],
  "25/50/25": [25, 50, 25],
}

function ColumnsFields({ block }: { block: BlockOf<"columns"> }) {
  const { t } = useLingui()
  const set = useSet(block)
  const { setDoc } = useDesigner()
  const sum = block.columns.reduce((s, c) => s + c.width, 0)
  const n = (i: number) => i + 1
  return (
    <Group title={<Trans>Columns</Trans>}>
      <Row label={<Trans>Number of columns</Trans>}>
        <Segmented
          value={String(block.columns.length) as "2" | "3"}
          onChange={(v) => setDoc((d) => setColumnCount(d, block.id, v === "3" ? 3 : 2))}
          label={t`Number of columns`}
          className="w-full"
          itemClassName="flex-1"
          items={[
            { value: "2", label: t`Two` },
            { value: "3", label: t`Three` },
          ]}
        />
      </Row>
      <Row
        label={<Trans>Widths in percent</Trans>}
        error={sum !== 100 ? <Trans>The widths add up to {sum}, not 100.</Trans> : undefined}
      >
        <div className="flex flex-wrap gap-1">
          {Object.entries(PRESETS)
            .filter(([, w]) => w.length === block.columns.length)
            .map(([name, w]) => (
              <Button
                key={name}
                variant="outline"
                size="sm"
                className="h-7 px-2.5 text-small tabular-nums"
                aria-pressed={w.every((x, i) => block.columns[i].width === x)}
                onClick={() => set({ columns: block.columns.map((c, i) => ({ ...c, width: w[i] })) })}
              >
                {name}
              </Button>
            ))}
        </div>
        <div className="grid grid-cols-3 gap-2">
          {block.columns.map((c, i) => (
            <NumberBox
              key={i}
              label={t`Column ${n(i)}`}
              value={c.width}
              min={10}
              max={90}
              onChange={(v) =>
                set({ columns: block.columns.map((col, j) => (j === i ? { ...col, width: v } : col)) }, `width${i}`)
              }
            />
          ))}
        </div>
      </Row>
      <SliderRow
        label={<Trans>Space between</Trans>}
        value={block.gap ?? 16}
        min={0}
        max={32}
        onChange={(v) => set({ gap: v }, "gap")}
      />
      <Toggle
        label={<Trans>Stack on phones</Trans>}
        hint={<Trans>One under the other on narrow screens.</Trans>}
        checked={block.stack ?? true}
        onChange={(v) => set({ stack: v })}
      />
    </Group>
  )
}

function HeadingFields({ block }: { block: BlockOf<"heading"> }) {
  const { t } = useLingui()
  const set = useSet(block)
  const { doc } = useDesigner()
  return (
    <Group title={<Trans>Heading</Trans>}>
      <Row label={<Trans>Size</Trans>}>
        <Segmented
          value={String(block.level) as "1" | "2" | "3"}
          onChange={(v) => set({ level: Number(v) as 1 | 2 | 3 })}
          label={t`Size`}
          className="w-full"
          itemClassName="flex-1"
          items={[
            { value: "1", label: t`Large` },
            { value: "2", label: t`Medium` },
            { value: "3", label: t`Small` },
          ]}
        />
      </Row>
      <ColorInput
        label={<Trans>Colour</Trans>}
        value={block.color}
        placeholder={doc.theme.colors.text}
        onChange={(v) => set({ color: v }, "color")}
        clearable
        contrastWith={block.background ?? doc.theme.colors.card}
      />
    </Group>
  )
}

function TextFields({ block }: { block: BlockOf<"text"> }) {
  const set = useSet(block)
  const { doc } = useDesigner()
  return (
    <Group title={<Trans>Text</Trans>}>
      <SliderRow
        label={<Trans>Text size</Trans>}
        value={block.size ?? 16}
        min={12}
        max={22}
        onChange={(v) => set({ size: v }, "size")}
      />
      <ColorInput
        label={<Trans>Colour</Trans>}
        value={block.color}
        placeholder={doc.theme.colors.text}
        onChange={(v) => set({ color: v }, "color")}
        clearable
        contrastWith={block.background ?? doc.theme.colors.card}
      />
    </Group>
  )
}

function ImageFields({ block }: { block: BlockOf<"image"> }) {
  const set = useSet(block)
  const d = useDesigner()
  const image = block.image_id ? d.images.get(block.image_id) : undefined
  const max = Math.min(720, d.doc.theme.width)
  const altMissing = !block.decorative && !(block.alt ?? "").trim()
  return (
    <Group title={<Trans>Image</Trans>}>
      <button
        type="button"
        className="group relative grid aspect-[16/9] w-full place-items-center overflow-hidden rounded-xl border bg-surface text-small text-faint transition-colors hover:border-faint"
        onClick={() => d.openLibrary(block.id)}
        data-testid="inspector-image"
      >
        {image ? (
          <img src={image.url} alt="" className="size-full object-contain" />
        ) : (
          <span className="flex flex-col items-center gap-1.5">
            <ImageIcon className="size-5" />
            <Trans>Choose an image</Trans>
          </span>
        )}
        {image && (
          <span className="absolute inset-x-2 bottom-2 rounded-full bg-black/60 px-2 py-1 text-caption text-white opacity-0 transition-opacity group-hover:opacity-100">
            <Trans>Replace</Trans>
          </span>
        )}
      </button>
      <Row
        label={<Trans>Description for screen readers</Trans>}
        htmlFor={`alt-${block.id}`}
        error={altMissing ? <Trans>Describe the image, or mark it as decorative.</Trans> : undefined}
        hint={<Trans>Also shown when a mail app blocks images.</Trans>}
      >
        <Textarea
          id={`alt-${block.id}`}
          rows={2}
          maxLength={300}
          value={block.alt ?? ""}
          disabled={block.decorative}
          aria-invalid={altMissing || undefined}
          onChange={(e) => set({ alt: e.target.value }, "alt")}
          data-testid="image-alt"
        />
      </Row>
      <Toggle
        label={<Trans>Decorative</Trans>}
        hint={<Trans>It adds nothing to the message, so screen readers skip it.</Trans>}
        checked={!!block.decorative}
        onChange={(v) => set({ decorative: v || undefined })}
      />
      <Toggle
        label={<Trans>Full width</Trans>}
        checked={block.width === undefined}
        onChange={(v) => set({ width: v ? undefined : Math.min(max, image?.width ?? 300, 300) })}
      />
      {block.width !== undefined && (
        <SliderRow
          label={<Trans>Width</Trans>}
          value={block.width}
          min={16}
          max={max}
          onChange={(v) => set({ width: v }, "width")}
        />
      )}
      <UrlInput label={<Trans>Link</Trans>} value={block.link} onChange={(v) => set({ link: v }, "link")} />
      <Toggle
        label={<Trans>Light plate in dark mode</Trans>}
        hint={<Trans>For dark logos on a transparent background.</Trans>}
        checked={!!block.plate}
        onChange={(v) => set({ plate: v || undefined })}
      />
    </Group>
  )
}

function ButtonFields({ block }: { block: BlockOf<"button"> }) {
  const set = useSet(block)
  const { doc } = useDesigner()
  const bg = block.color ?? doc.theme.colors.accent
  return (
    <Group title={<Trans>Button</Trans>}>
      <UrlInput
        label={<Trans>Link</Trans>}
        value={block.url}
        onChange={(v) => v && set({ url: v }, "url")}
        required
        testId="button-url"
      />
      <ColorInput
        label={<Trans>Button colour</Trans>}
        value={block.color}
        placeholder={doc.theme.colors.accent}
        onChange={(v) => set({ color: v }, "color")}
        clearable
      />
      <ColorInput
        label={<Trans>Text colour</Trans>}
        value={block.text_color}
        placeholder="#ffffff"
        onChange={(v) => set({ text_color: v }, "text_color")}
        clearable
        contrastWith={bg}
      />
      <Toggle
        label={<Trans>Full width</Trans>}
        checked={!!block.full_width}
        onChange={(v) => set({ full_width: v || undefined })}
      />
    </Group>
  )
}

function DividerFields({ block }: { block: BlockOf<"divider"> }) {
  const set = useSet(block)
  const { doc } = useDesigner()
  return (
    <Group title={<Trans>Divider</Trans>}>
      <ColorInput
        label={<Trans>Colour</Trans>}
        value={block.color}
        placeholder={doc.theme.colors.border}
        onChange={(v) => set({ color: v }, "color")}
        clearable
      />
      <SliderRow
        label={<Trans>Thickness</Trans>}
        value={block.thickness ?? 1}
        min={1}
        max={4}
        onChange={(v) => set({ thickness: v }, "thickness")}
      />
      <SliderRow
        label={<Trans>Width</Trans>}
        value={block.width ?? 100}
        min={10}
        max={100}
        unit="%"
        onChange={(v) => set({ width: v }, "width")}
      />
    </Group>
  )
}

function SpacerFields({ block }: { block: BlockOf<"spacer"> }) {
  const set = useSet(block)
  return (
    <Group title={<Trans>Space</Trans>}>
      <SliderRow
        label={<Trans>Height</Trans>}
        value={block.height}
        min={4}
        max={96}
        step={4}
        onChange={(v) => set({ height: v }, "height")}
      />
    </Group>
  )
}

function SocialFields({ block }: { block: BlockOf<"social"> }) {
  const { t } = useLingui()
  const set = useSet(block)
  const names = useNetworkNames()
  const items = block.items
  const setItem = (i: number, patch: Partial<(typeof items)[number]>) =>
    set({ items: items.map((it, j) => (j === i ? { ...it, ...patch } : it)) }, `item${i}`)
  return (
    <Group title={<Trans>Social links</Trans>}>
      <ul className="flex flex-col gap-3">
        {items.map((it, i) => (
          <li key={i} className="flex flex-col gap-1.5 rounded-xl border p-2.5">
            <div className="flex items-center gap-2">
              <Select
                value={it.network}
                items={names}
                onValueChange={(v) => v && setItem(i, { network: v as EmailSocialNetwork })}
              >
                <SelectTrigger size="sm" className="min-w-0 flex-1" aria-label={t`Network`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {NETWORKS.map((n) => (
                    <SelectItem key={n} value={n}>
                      {names[n]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t`Remove`}
                disabled={items.length === 1}
                onClick={() => set({ items: items.filter((_, j) => j !== i) })}
              >
                <XIcon />
              </Button>
            </div>
            <UrlInput
              label={<Trans>Address</Trans>}
              value={it.url}
              required
              onChange={(v) => v && setItem(i, { url: v })}
            />
          </li>
        ))}
      </ul>
      <Button
        variant="outline"
        size="sm"
        className="w-fit"
        disabled={items.length >= 10}
        onClick={() => set({ items: [...items, { network: "website", url: "https://example.com" }] })}
      >
        <PlusIcon />
        <Trans>Add a link</Trans>
      </Button>
      <Row label={<Trans>Show as</Trans>}>
        <Segmented
          value={block.style ?? "icons"}
          onChange={(v) => set({ style: v })}
          label={t`Show as`}
          className="w-full"
          itemClassName="flex-1"
          items={[
            { value: "icons", label: t`Icons` },
            { value: "text", label: t`Text` },
          ]}
        />
      </Row>
      {(block.style ?? "icons") === "icons" && (
        <Row label={<Trans>Icon colour</Trans>}>
          <Segmented
            value={block.color ?? "dark"}
            onChange={(v) => set({ color: v })}
            label={t`Icon colour`}
            className="w-full"
            itemClassName="flex-1"
            items={[
              { value: "dark", label: t`Dark` },
              { value: "light", label: t`Light` },
              { value: "brand", label: t`Brand` },
            ]}
          />
        </Row>
      )}
    </Group>
  )
}

function FooterFields() {
  return (
    <Group title={<Trans>Footer text</Trans>}>
      <p className="text-small text-muted-foreground">
        <Trans>
          These lines go above the footer every mail gets: the inbox’s legal name and postal address, why the person
          gets the mail, and the unsubscribe and preference links. Set the sender’s details in the inbox settings.
        </Trans>
      </p>
    </Group>
  )
}

function Fields({ block }: { block: Block }) {
  switch (block.type) {
    case "section":
      return <SectionFields block={block} />
    case "columns":
      return <ColumnsFields block={block} />
    case "heading":
      return <HeadingFields block={block} />
    case "text":
      return <TextFields block={block} />
    case "image":
      return <ImageFields block={block} />
    case "button":
      return <ButtonFields block={block} />
    case "divider":
      return <DividerFields block={block} />
    case "spacer":
      return <SpacerFields block={block} />
    case "social":
      return <SocialFields block={block} />
    case "footer":
      return <FooterFields />
  }
}

const boxClass =
  "min-w-0 rounded-lg border px-3 py-2 text-body [&_a]:underline [&_ol]:list-decimal [&_ol]:ps-5 [&_p+p]:mt-2 [&_ul]:list-disc [&_ul]:ps-5"

function TranslationFields({ block }: { block: Block }) {
  const { t } = useLingui()
  const d = useDesigner()
  const name = useLanguageName()
  const language = name(d.original.locale ?? "")
  const target = name(d.locale ?? "")
  const orig = findBlock(d.original, block.id)
  if (!orig || !hasText(orig.type) || (orig.type === "image" && orig.decorative))
    return (
      <p className="px-4 py-4 text-small text-faint" data-testid="translation-none">
        <Trans>This block has no text to translate.</Trans>
      </p>
    )
  const mode = orig.type === "heading" ? "inline" : orig.type === "text" || orig.type === "footer" ? "rich" : "plain"
  const source = textOf(orig)
  const value = textOf(block)
  const untranslated = d.missing.has(block.id)
  const set = (v: string) => d.setText(block.id, v, `text:${block.id}`)
  const field = `translation-${block.id}`
  return (
    <Group title={<Trans>Translation</Trans>}>
      <Row label={<Trans>Original ({language})</Trans>}>
        <div className={cn(boxClass, "bg-surface text-muted-foreground")} data-testid="translation-original">
          {mode === "plain" ? (
            <p className="break-words whitespace-pre-wrap">{source}</p>
          ) : (
            <RichEditor
              key={`${d.locale}:${source}`}
              mode={mode}
              owner={`original:${block.id}`}
              value={source}
              onChange={() => undefined}
              placeholder=""
              label={t`Original (${language})`}
              readOnly
            />
          )}
        </div>
      </Row>
      <Row
        label={target}
        htmlFor={mode === "plain" ? field : undefined}
        hint={untranslated ? <Trans>Not translated: the original shows until you write this one.</Trans> : undefined}
      >
        {mode === "plain" && orig.type === "image" ? (
          <Textarea
            id={field}
            rows={2}
            maxLength={300}
            value={value}
            readOnly={d.readOnly}
            onChange={(e) => set(e.target.value)}
            data-testid="translation-input"
          />
        ) : mode === "plain" ? (
          <Input
            id={field}
            maxLength={80}
            value={value}
            readOnly={d.readOnly}
            onChange={(e) => set(e.target.value)}
            data-testid="translation-input"
          />
        ) : (
          <div className={cn(boxClass, "bg-background focus-within:ring-2 focus-within:ring-ring/40")}>
            <RichEditor
              mode={mode}
              owner={block.id}
              value={value}
              onChange={set}
              placeholder={t`Write the translation`}
              label={target}
              readOnly={d.readOnly}
              testId="translation-editor"
            />
          </div>
        )}
      </Row>
    </Group>
  )
}

export function Inspector({ block }: { block: Block }) {
  const { t } = useLingui()
  const d = useDesigner()
  const names = useBlockNames()
  return (
    <div className="flex flex-col" data-testid="inspector">
      <div className="flex items-center gap-2 border-b px-4 py-3">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-brand-wash text-brand">
          <BlockIcon type={block.type} className="size-4" />
        </span>
        <h2 className="min-w-0 flex-1 truncate text-body font-semibold">{names[block.type]}</h2>
        {!d.locked && (
          <div className="flex items-center">
            <Button variant="ghost" size="icon-sm" aria-label={t`Move up`} onClick={() => d.nudge(block.id, -1)}>
              <ArrowUpIcon />
            </Button>
            <Button variant="ghost" size="icon-sm" aria-label={t`Move down`} onClick={() => d.nudge(block.id, 1)}>
              <ArrowDownIcon />
            </Button>
          </div>
        )}
      </div>
      <Issues block={block} />
      {d.locale ? (
        <>
          <LayoutHint className="mx-4 mt-3" />
          <TranslationFields block={block} />
        </>
      ) : (
        <fieldset disabled={d.readOnly} className="flex min-w-0 flex-col">
          <Fields block={block} />
          <Layout block={block} />
          {!d.readOnly && (
            <div className="flex gap-2 px-4 py-4">
              <Button
                variant="outline"
                className="flex-1"
                onClick={() => d.duplicate(block.id)}
                data-testid="inspector-duplicate"
              >
                <CopyIcon />
                <Trans>Duplicate</Trans>
              </Button>
              <Button
                variant="outline"
                className="flex-1 hover:text-destructive"
                onClick={() => d.remove(block.id)}
                data-testid="inspector-delete"
              >
                <Trash2Icon />
                <Trans>Delete block</Trans>
              </Button>
            </div>
          )}
        </fieldset>
      )}
    </div>
  )
}
