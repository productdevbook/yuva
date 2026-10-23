import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useFieldTypeText } from "@/features/fields/value"
import { ChoiceSelect, Field, ToggleRow } from "@/features/settings/ui"
import { api, ApiError, unwrap, type CustomField, type CustomFieldObject, type CustomFieldType } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

const KEY = /^[a-z][a-z0-9_]{0,39}$/
const TYPES: CustomFieldType[] = ["text", "number", "date", "select", "checkbox"]

function slug(name: string) {
  const s = name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .replace(/^[0-9_]+/, "")
  return s.slice(0, 40)
}

function parseOptions(text: string) {
  return text
    .split("\n")
    .map((o) => o.trim())
    .filter(Boolean)
}

function Form({
  field,
  object,
  nextPosition,
  onClose,
}: {
  field: CustomField | null
  object: CustomFieldObject
  nextPosition: number
  onClose: () => void
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const typeText = useFieldTypeText()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [name, setName] = useState(field?.name ?? "")
  const [key, setKey] = useState(field?.key ?? "")
  const [keyTouched, setKeyTouched] = useState(false)
  const [type, setType] = useState<CustomFieldType>(field?.type ?? "text")
  const [options, setOptions] = useState((field?.options ?? []).join("\n"))
  const [required, setRequired] = useState(field?.required_on_close ?? false)
  const list = parseOptions(options)
  const removed = field ? field.options.filter((o) => !list.includes(o)) : []

  const save = useMutation({
    mutationFn: () =>
      field
        ? unwrap(
            api.PATCH("/v1/custom-fields/{customFieldId}", {
              params: { path: { customFieldId: field.id } },
              body: {
                name: name.trim(),
                ...(field.type === "select" ? { options: list } : {}),
                ...(field.object === "conversation" ? { required_on_close: required } : {}),
              },
            })
          )
        : unwrap(
            api.POST("/v1/custom-fields", {
              body: {
                object,
                key,
                name: name.trim(),
                type,
                ...(type === "select" ? { options: list } : {}),
                required_on_close: object === "conversation" ? required : false,
                position: nextPosition,
              },
            })
          ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.customFields(ws) })
      onClose()
    },
  })

  const error = save.error
  const keyError = error instanceof ApiError && (error.code === "field_taken" || error.code === "field_clearing")
  const keyValid = !!field || KEY.test(key)
  const optionsValid =
    type !== "select" || (list.length >= 1 && list.length <= 100 && new Set(list).size === list.length)
  const changed =
    !field ||
    name.trim() !== field.name ||
    required !== field.required_on_close ||
    (field.type === "select" && list.join("\n") !== field.options.join("\n"))

  return (
    <form
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <Field label={<Trans>Name</Trans>} htmlFor="field-name" hint={<Trans>What the team sees.</Trans>}>
        <Input
          id="field-name"
          required
          maxLength={100}
          value={name}
          placeholder={t`Order number`}
          onChange={(e) => {
            setName(e.target.value)
            if (!field && !keyTouched) setKey(slug(e.target.value))
          }}
          data-testid="field-name"
        />
      </Field>
      <Field
        label={<Trans>Key</Trans>}
        htmlFor="field-key"
        hint={
          field ? (
            <Trans>The key, the object and the type cannot change after creation.</Trans>
          ) : (
            <Trans>Where the API stores the value. Lowercase letters, digits and underscores; it cannot change.</Trans>
          )
        }
        error={keyError ? errorText(error) : undefined}
      >
        <Input
          id="field-key"
          required
          disabled={!!field}
          maxLength={40}
          className="font-mono"
          value={key}
          aria-invalid={keyError || (!keyValid && key !== "") || undefined}
          onChange={(e) => {
            setKey(e.target.value)
            setKeyTouched(true)
          }}
          data-testid="field-key"
        />
      </Field>
      <Field label={<Trans>Type</Trans>} htmlFor="field-type">
        <ChoiceSelect<CustomFieldType>
          id="field-type"
          value={type}
          disabled={!!field}
          onChange={setType}
          options={TYPES.map((x) => [x, typeText[x]] as const)}
        />
      </Field>
      {type === "select" && (
        <Field
          label={<Trans>Options</Trans>}
          htmlFor="field-options"
          hint={<Trans>One per line, all different, at most 100.</Trans>}
        >
          <Textarea
            id="field-options"
            rows={5}
            value={options}
            placeholder={t`Refund`}
            onChange={(e) => setOptions(e.target.value)}
            data-testid="field-options"
          />
          {removed.length > 0 && (
            <p className="text-small text-warning" data-testid="field-options-warning">
              <Trans>
                Removed options are cleared from every conversation that holds them. This runs in the background.
              </Trans>
            </p>
          )}
        </Field>
      )}
      {object === "conversation" && (
        <ToggleRow
          title={<Trans>Required to close</Trans>}
          hint={<Trans>A conversation cannot be closed until this field has a value. Spam is exempt.</Trans>}
        >
          <Switch
            checked={required}
            onCheckedChange={setRequired}
            aria-label={t`Required to close`}
            data-testid="field-required"
          />
        </ToggleRow>
      )}
      {!keyError && <ErrorLine error={error} />}
      <DialogFooter>
        <Button
          type="submit"
          disabled={save.isPending || name.trim() === "" || !keyValid || !optionsValid || !changed}
          data-testid="field-save"
        >
          {field ? <Trans>Save</Trans> : <Trans>Create field</Trans>}
        </Button>
      </DialogFooter>
    </form>
  )
}

export function FieldDialog({
  field,
  object,
  nextPosition,
  onClose,
}: {
  field: CustomField | null
  object: CustomFieldObject
  nextPosition: number
  onClose: () => void
}) {
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{field ? <Trans>Edit field</Trans> : <Trans>New field</Trans>}</DialogTitle>
        </DialogHeader>
        <Form field={field} object={object} nextPosition={nextPosition} onClose={onClose} />
      </DialogContent>
    </Dialog>
  )
}
