import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { formatBytes } from "@/components/common/text"
import { Textarea } from "@/components/ui/textarea"
import { Input } from "@/components/ui/input"
import { PageSection, usePatchChannel } from "@/features/settings/channels/sections"
import { Chip, Field, FormActions, FormCard } from "@/features/settings/ui"
import { ApiError, type Channel, type ChannelAttachmentsInput } from "@/lib/api"

const MIB = 1024 * 1024
const MAX_TYPES = 100
const TYPE = /^[a-z0-9][a-z0-9!#$&^_.+-]*\/(\*|[a-z0-9][a-z0-9!#$&^_.+-]*)$/

function mibText(bytes: number | undefined, locale: string) {
  return bytes === undefined
    ? ""
    : new Intl.NumberFormat(locale, { maximumFractionDigits: 2, useGrouping: false }).format(bytes / MIB)
}

function parseMib(raw: string) {
  const text = raw.trim()
  if (!/^\d+(?:[.,]\d+)?$/.test(text)) return Number.NaN
  return Math.round(Number(text.replace(",", ".")) * MIB)
}

function parseTypes(raw: string) {
  const out: string[] = []
  for (const x of raw.toLowerCase().split(/[\s,;]+/)) if (x && !out.includes(x)) out.push(x)
  return out
}

function serverDetail(err: unknown) {
  return err instanceof ApiError && err.status === 400 && err.detail ? err.detail : null
}

export function AttachmentsSection({ channel }: { channel: Channel }) {
  const { i18n } = useLingui()
  const patch = usePatchChannel(channel)
  const own = channel.attachments
  const [draft, setDraft] = useState<{ size?: string; types?: string }>({})
  const baseSize = mibText(own.max_bytes, i18n.locale)
  const baseTypes = (own.types ?? []).join("\n")
  const size = draft.size ?? baseSize
  const typesRaw = draft.types ?? baseTypes
  const types = parseTypes(typesRaw)
  const bytes = size.trim() === "" ? null : parseMib(size)
  const sizeBad = bytes !== null && !(Number.isFinite(bytes) && bytes >= 1)
  const badTypes = types.filter((x) => !TYPE.test(x))
  const tooMany = types.length > MAX_TYPES
  const sizeDirty = draft.size !== undefined && size.trim() !== baseSize
  const typesDirty = draft.types !== undefined && types.join("\n") !== (own.types ?? []).join("\n")
  const dirty = sizeDirty || typesDirty
  const invalid = sizeBad || badTypes.length > 0 || tooMany
  const set = (p: { size?: string; types?: string }) => {
    patch.reset()
    setDraft((d) => ({ ...d, ...p }))
  }
  const submit = () => {
    if (invalid || !dirty) return
    const body: ChannelAttachmentsInput = {}
    if (sizeDirty) body.max_bytes = bytes
    if (typesDirty) body.types = types.length > 0 ? types : null
    patch.mutate({ attachments: body }, { onSuccess: () => setDraft({}) })
  }
  const detail = serverDetail(patch.error)
  const effectiveSize = formatBytes(own.effective_max_bytes, i18n.locale)
  const sizeError = sizeBad ? <Trans>Enter a size in MB, such as 10.</Trans> : null
  const badList = badTypes.join(", ")
  const typesError =
    badTypes.length > 0 ? (
      <Trans>Not a content type: {badList}. Use forms such as application/pdf or image/*.</Trans>
    ) : tooMany ? (
      <Trans>List at most 100 types.</Trans>
    ) : null
  return (
    <PageSection
      id="attachments"
      title={<Trans>Attachments</Trans>}
      description={
        <Trans>
          Narrow what people can attach in this channel. The server’s own limits stay the ceiling: a channel can lower
          them, never raise them.
        </Trans>
      }
    >
      <FormCard
        onSubmit={submit}
        footer={
          <FormActions
            pending={patch.isPending}
            saved={patch.isSuccess && !dirty}
            error={detail ? undefined : patch.error}
            disabled={!dirty || invalid}
          />
        }
      >
        <Field
          label={<Trans>Largest file (MB)</Trans>}
          htmlFor="attachments-max"
          error={sizeError}
          hint={<Trans>In effect: {effectiveSize}. Leave empty to follow the server.</Trans>}
        >
          <Input
            id="attachments-max"
            inputMode="decimal"
            value={size}
            placeholder={mibText(own.effective_max_bytes, i18n.locale)}
            onChange={(e) => set({ size: e.target.value })}
            aria-invalid={sizeBad || undefined}
            className="max-w-40"
            data-testid="attachments-max"
          />
        </Field>
        <Field
          label={<Trans>Allowed file types</Trans>}
          htmlFor="attachments-types"
          error={typesError}
          hint={<Trans>One per line, such as application/pdf or image/*. Leave empty to follow the server.</Trans>}
        >
          <Textarea
            id="attachments-types"
            rows={3}
            spellCheck={false}
            className="font-mono text-caption"
            value={typesRaw}
            placeholder={"application/pdf\nimage/*"}
            onChange={(e) => set({ types: e.target.value })}
            aria-invalid={!!typesError || undefined}
            data-testid="attachments-types"
          />
          <div
            className="flex flex-wrap items-center gap-1.5 text-small text-faint"
            data-testid="attachments-effective"
          >
            <span>
              <Trans>In effect:</Trans>
            </span>
            {own.effective_types.length === 0 ? (
              <span className="text-destructive">
                <Trans>No file is accepted.</Trans>
              </span>
            ) : (
              own.effective_types.map((x) => (
                <Chip key={x} className="font-mono">
                  {x}
                </Chip>
              ))
            )}
          </div>
        </Field>
        {detail && (
          <p role="alert" className="text-small text-destructive" data-testid="attachments-error">
            {detail}
          </p>
        )}
      </FormCard>
    </PageSection>
  )
}
