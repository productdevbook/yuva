import { Trans } from "@lingui/react/macro"
import { useEffect, useId, useRef, useState } from "react"

import { ErrorLine } from "@/components/common"
import { useErrorText } from "@/components/common/text"
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
import { useConversation, useUpdateConversation } from "@/features/conversation/queries"
import { FieldInput } from "@/features/fields/FieldInput"
import { setRequiredAsker } from "@/features/fields/required"
import { byPosition, isSet, type FieldValue } from "@/features/fields/value"
import { ApiError } from "@/lib/api"
import { useCustomFields } from "@/lib/workspace"

type Request = { conversationId: string; field?: string; resolve: (done: boolean) => void }

function Missing({ request, onDone }: { request: Request; onDone: (done: boolean) => void }) {
  const errorText = useErrorText()
  const uid = useId()
  const conversation = useConversation(request.conversationId).data
  const definitions = useCustomFields("conversation").data
  const update = useUpdateConversation(request.conversationId)
  const [values, setValues] = useState<Record<string, FieldValue | null>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [other, setOther] = useState<unknown>(null)
  const empty = useRef(false)

  const missing = conversation
    ? [...(definitions ?? [])]
        .sort(byPosition)
        .filter((f) => (f.required_on_close && !isSet(conversation.fields?.[f.key])) || f.key === request.field)
    : []

  useEffect(() => {
    if (conversation && definitions && missing.length === 0 && !empty.current) {
      empty.current = true
      onDone(true)
    }
  })

  if (!conversation || !definitions || missing.length === 0) return null

  const valueOf = (key: string, type: string) => (key in values ? values[key] : type === "checkbox" ? false : null)
  const complete = missing.every((f) => isSet(valueOf(f.key, f.type)))
  const submit = () => {
    setErrors({})
    setOther(null)
    const body: Record<string, FieldValue | null> = {}
    for (const f of missing) body[f.key] = valueOf(f.key, f.type)
    update.mutate(
      { fields: body },
      {
        onSuccess: () => onDone(true),
        onError: (err) => {
          if (err instanceof ApiError && err.code === "field_invalid" && err.field && err.field in body)
            setErrors({ [err.field]: errorText(err) })
          else setOther(err)
        },
      }
    )
  }

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (complete) submit()
      }}
    >
      {missing.map((f, i) => {
        const id = `${uid}-${f.key}`
        return (
          <div key={f.id} className="flex flex-col gap-1.5" data-testid="required-field" data-key={f.key}>
            <Label htmlFor={id}>{f.name}</Label>
            <FieldInput
              field={f}
              id={id}
              immediate
              autoFocus={i === 0}
              value={valueOf(f.key, f.type)}
              onValue={(v) => setValues((x) => ({ ...x, [f.key]: v }))}
              invalid={!!errors[f.key]}
            />
            {errors[f.key] && (
              <p role="alert" className="text-small text-destructive">
                {errors[f.key]}
              </p>
            )}
          </div>
        )
      })}
      <ErrorLine error={other} />
      <DialogFooter>
        <Button type="button" variant="ghost" onClick={() => onDone(false)}>
          <Trans>Cancel</Trans>
        </Button>
        <Button type="submit" disabled={!complete || update.isPending} data-testid="required-save">
          <Trans>Save and close</Trans>
        </Button>
      </DialogFooter>
    </form>
  )
}

export function RequiredFieldsHost() {
  const [request, setRequest] = useState<Request | null>(null)
  useEffect(() => {
    setRequiredAsker(
      (conversationId, field) =>
        new Promise<boolean>((resolve) => {
          setRequest((prev) => {
            prev?.resolve(false)
            return { conversationId, field, resolve }
          })
        })
    )
    return () => setRequiredAsker(null)
  }, [])
  const finish = (done: boolean) => {
    request?.resolve(done)
    setRequest(null)
  }
  return (
    <Dialog open={!!request} onOpenChange={(o) => !o && finish(false)}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-md" data-testid="required-fields">
        <DialogHeader>
          <DialogTitle>
            <Trans>Fill in before closing</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Your workspace asks for these fields on every conversation it closes.</Trans>
          </DialogDescription>
        </DialogHeader>
        {request && <Missing key={request.conversationId} request={request} onDone={finish} />}
      </DialogContent>
    </Dialog>
  )
}
