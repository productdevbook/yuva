import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronDownIcon } from "lucide-react"
import { useState } from "react"

import { Dot, Segmented } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { derivedDark, textOn } from "@/features/designs/colors"
import { hasMessageBlock, setTranslation } from "@/features/designs/doc"
import { useDesigner } from "@/features/designs/designer/context"
import { LayoutHint } from "@/features/designs/designer/Languages"
import { useLanguageName } from "@/components/common/text"
import { ColorInput, ContrastBadge, Group, Row, SliderRow } from "@/features/designs/designer/controls"
import { FONTS } from "@/features/designs/designer/Canvas"
import { useFontNames } from "@/features/designs/text"
import type { EmailFont, EmailTheme, EmailThemeColors } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type ColorKey = keyof EmailThemeColors

const FONT_KEYS: EmailFont[] = ["system", "humanist", "geometric", "serif", "mono"]
const WORKSPACE = "workspace"
const SAME = "same"

function useColorNames(): { key: ColorKey; label: string; on?: ColorKey }[] {
  const { t } = useLingui()
  return [
    { key: "page", label: t`Page background` },
    { key: "card", label: t`Content background` },
    { key: "text", label: t`Text`, on: "card" },
    { key: "muted", label: t`Secondary text`, on: "card" },
    { key: "link", label: t`Links`, on: "card" },
    { key: "accent", label: t`Buttons` },
    { key: "border", label: t`Lines` },
  ]
}

function FontSelect({
  value,
  onChange,
  label,
  same,
}: {
  value: EmailFont | undefined
  onChange: (v: EmailFont | undefined) => void
  label: string
  same?: string
}) {
  const names = useFontNames()
  const items: Record<string, string> = { ...(same ? { [SAME]: same } : {}), ...names }
  return (
    <Select
      value={value ?? (same ? SAME : "system")}
      items={items}
      onValueChange={(v) => onChange(!v || v === SAME ? undefined : (v as EmailFont))}
    >
      <SelectTrigger className="w-full min-w-0" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {same && <SelectItem value={SAME}>{same}</SelectItem>}
        {FONT_KEYS.map((f) => (
          <SelectItem key={f} value={f}>
            <span style={{ fontFamily: FONTS[f] }}>{names[f]}</span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function ThemePanel({
  inboxId,
  onInbox,
  offered,
  onOffered,
}: {
  inboxId: string | undefined
  onInbox: (id: string | undefined) => void
  offered: boolean
  onOffered: (v: boolean) => void
}) {
  const { t } = useLingui()
  const d = useDesigner()
  const inboxes = useInboxes().data ?? []
  const colors = useColorNames()
  const theme = d.doc.theme
  const derived = derivedDark(theme.colors)
  const dark = theme.dark ?? {}
  const [darkOpen, setDarkOpen] = useState(Object.keys(dark).length > 0)
  const setTheme = (patch: Partial<EmailTheme>, key?: string) =>
    d.setDoc((doc) => ({ ...doc, theme: { ...doc.theme, ...patch } }), key && `theme:${key}`)
  const setDark = (k: ColorKey, v: string | undefined) => {
    const next = { ...dark, [k]: v }
    if (!v) delete next[k]
    setTheme({ dark: Object.keys(next).length ? next : undefined }, `dark.${k}`)
  }
  const inboxItems: Record<string, string> = {
    [WORKSPACE]: t`Whole workspace`,
    ...Object.fromEntries(inboxes.map((i) => [i.id, i.name])),
  }
  const darkCard = dark.card ?? derived.card
  const hasMessage = hasMessageBlock(d.original)
  if (d.locale) return <VersionPanel tag={d.locale} />
  return (
    <fieldset disabled={d.readOnly} className="flex min-w-0 flex-col" data-testid="theme-panel">
      <div className="border-b px-4 py-3">
        <h2 className="text-body font-semibold">
          <Trans>Design</Trans>
        </h2>
        <p className="text-small text-faint">
          <Trans>Select a block to change it. These settings apply to the whole mail.</Trans>
        </p>
      </div>
      <Group title={<Trans>Mail</Trans>}>
        <Row label={<Trans>Inbox</Trans>} hint={<Trans>Its sender details fill the required footer.</Trans>}>
          <Select
            value={inboxId ?? WORKSPACE}
            items={inboxItems}
            onValueChange={(v) => onInbox(!v || v === WORKSPACE ? undefined : String(v))}
          >
            <SelectTrigger className="w-full min-w-0" aria-label={t`Inbox`} data-testid="design-inbox">
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
        </Row>
        <Row
          label={<Trans>Offer in conversations</Trans>}
          hint={
            hasMessage ? (
              <Trans>Members who see this inbox can send replies and new mail in this design.</Trans>
            ) : (
              <Trans>Add a message block first: it marks where the text a member writes goes.</Trans>
            )
          }
        >
          <Switch
            checked={offered && hasMessage}
            disabled={!hasMessage && !offered}
            onCheckedChange={onOffered}
            aria-label={t`Offer in conversations`}
            data-testid="design-offered"
          />
        </Row>
        <Row
          label={<Trans>Preview text</Trans>}
          htmlFor="design-preheader"
          hint={<Trans>Shown after the subject in most inboxes. A broadcast can set its own.</Trans>}
        >
          <Input
            id="design-preheader"
            value={d.doc.preheader ?? ""}
            maxLength={300}
            onChange={(e) => d.setDoc((doc) => ({ ...doc, preheader: e.target.value || undefined }), "preheader")}
            data-testid="design-preheader"
          />
        </Row>
      </Group>
      <Group title={<Trans>Colours</Trans>}>
        {colors.map((c) => (
          <ColorInput
            key={c.key}
            label={c.label}
            value={theme.colors[c.key]}
            onChange={(v) => v && setTheme({ colors: { ...theme.colors, [c.key]: v } }, `color.${c.key}`)}
            contrastWith={c.on ? theme.colors[c.on] : undefined}
            testId={`theme-color-${c.key}`}
          />
        ))}
        <p className="flex items-center gap-2 text-caption text-faint">
          <Trans>Button text</Trans>
          <ContrastBadge fg={textOn(theme.colors.accent)} bg={theme.colors.accent} />
        </p>
      </Group>
      <Group
        title={<Trans>Dark mode</Trans>}
        action={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-expanded={darkOpen}
            aria-label={darkOpen ? t`Hide dark colours` : t`Show dark colours`}
            onClick={() => setDarkOpen((o) => !o)}
          >
            <ChevronDownIcon className={cn("transition-transform", darkOpen && "rotate-180")} />
          </Button>
        }
      >
        <p className="text-small text-muted-foreground">
          <Trans>
            Mail apps in dark mode get darker colours made from yours, with text kept easy to read. Set any of them
            yourself.
          </Trans>
        </p>
        <div className="flex gap-1" aria-hidden>
          {colors.map((c) => (
            <span
              key={c.key}
              className="h-5 flex-1 rounded border"
              style={{ background: dark[c.key] ?? derived[c.key] }}
            />
          ))}
        </div>
        {darkOpen && (
          <>
            {colors.map((c) => (
              <ColorInput
                key={c.key}
                label={c.label}
                value={dark[c.key]}
                placeholder={derived[c.key]}
                clearable
                onChange={(v) => setDark(c.key, v)}
                contrastWith={c.on ? darkCard : undefined}
                testId={`theme-dark-${c.key}`}
              />
            ))}
            {theme.dark && (
              <Button
                variant="link"
                size="sm"
                className="w-fit text-small"
                onClick={() => setTheme({ dark: undefined })}
              >
                <Trans>Make them all from the light colours</Trans>
              </Button>
            )}
          </>
        )}
      </Group>
      <Group title={<Trans>Type</Trans>}>
        <Row
          label={<Trans>Text font</Trans>}
          hint={<Trans>Fonts already on people’s devices; mails load no web fonts.</Trans>}
        >
          <FontSelect value={theme.font} onChange={(v) => setTheme({ font: v ?? "system" })} label={t`Text font`} />
        </Row>
        <Row label={<Trans>Heading font</Trans>}>
          <FontSelect
            value={theme.heading_font}
            onChange={(v) => setTheme({ heading_font: v })}
            label={t`Heading font`}
            same={t`Same as the text`}
          />
        </Row>
      </Group>
      <Group title={<Trans>Shape</Trans>}>
        <SliderRow
          label={<Trans>Width</Trans>}
          value={theme.width}
          min={480}
          max={720}
          step={10}
          onChange={(v) => setTheme({ width: v }, "width")}
          testId="theme-width"
        />
        <SliderRow
          label={<Trans>Corner radius</Trans>}
          value={theme.radius ?? 0}
          min={0}
          max={24}
          onChange={(v) => setTheme({ radius: v }, "radius")}
        />
        <Row label={<Trans>Direction</Trans>} hint={<Trans>Right to left for Arabic, Hebrew and Persian.</Trans>}>
          <Segmented
            value={theme.direction ?? "auto"}
            onChange={(v) => setTheme({ direction: v })}
            label={t`Direction`}
            className="w-full"
            itemClassName="flex-1 px-1"
            items={[
              { value: "auto", label: t`Automatic` },
              { value: "ltr", label: t`Left to right` },
              { value: "rtl", label: t`Right to left` },
            ]}
          />
        </Row>
      </Group>
    </fieldset>
  )
}

function VersionPanel({ tag }: { tag: string }) {
  const d = useDesigner()
  const name = useLanguageName()
  const language = name(tag)
  const value = d.original.translations?.[tag]?.preheader ?? ""
  return (
    <fieldset disabled={d.readOnly} className="flex min-w-0 flex-col" data-testid="theme-panel">
      <div className="border-b px-4 py-3">
        <h2 className="text-body font-semibold">
          <Trans>Design</Trans>
        </h2>
        <LayoutHint className="mt-2" />
      </div>
      <Group title={<Trans>Mail</Trans>}>
        <Row
          label={<Trans>Preview text ({language})</Trans>}
          htmlFor="design-preheader"
          hint={<Trans>Shown after the subject in most inboxes. Empty uses the base language’s.</Trans>}
        >
          <Input
            id="design-preheader"
            value={value}
            maxLength={300}
            placeholder={d.original.preheader ?? ""}
            onChange={(e) =>
              d.setDoc(
                (doc) => setTranslation(doc, tag, (x) => ({ ...x, preheader: e.target.value || undefined })),
                `preheader:${tag}`
              )
            }
            data-testid="design-preheader"
          />
        </Row>
      </Group>
    </fieldset>
  )
}
