import { useId, useState } from "react"

import { useErrorText } from "@/components/common/text"
import { Label } from "@/components/ui/label"
import { FieldInput } from "@/features/fields/FieldInput"
import { byPosition, type FieldValue } from "@/features/fields/value"
import { useConversation, useUpdateConversation } from "@/features/conversation/queries"
import { ApiError } from "@/lib/api"
import { useCustomFields } from "@/lib/workspace"

export function useConversationFieldList() {
  return [...(useCustomFields("conversation").data ?? [])].sort(byPosition)
}

export function ConversationFields({ conversationId }: { conversationId: string }) {
  const errorText = useErrorText()
  const uid = useId()
  const fields = useConversationFieldList()
  const conversation = useConversation(conversationId).data
  const update = useUpdateConversation(conversationId)
  const [errors, setErrors] = useState<Record<string, string>>({})
  if (fields.length === 0 || !conversation) return null

  const save = (key: string, v: FieldValue | null) => {
    setErrors(({ [key]: _, ...rest }) => rest)
    update.mutate(
      { fields: { [key]: v } },
      {
        onError: (err) => {
          const named = err instanceof ApiError && err.code === "field_invalid" ? (err.field ?? key) : key
          setErrors((e) => ({ ...e, [named]: errorText(err) }))
        },
      }
    )
  }
  const stray = Object.entries(errors).filter(([key]) => !fields.some((f) => f.key === key))

  return (
    <div className="flex flex-col gap-3" data-testid="conversation-fields">
      {fields.map((f) => {
        const id = `${uid}-${f.key}`
        const error = errors[f.key]
        return (
          <div key={f.id} className="flex min-w-0 flex-col gap-1.5" data-testid="conversation-field" data-key={f.key}>
            <Label htmlFor={id} className="text-caption font-normal text-muted-foreground">
              {f.name}
            </Label>
            <FieldInput
              field={f}
              id={id}
              value={conversation.fields?.[f.key]}
              onValue={(v) => save(f.key, v)}
              invalid={!!error}
              describedBy={error ? `${id}-error` : undefined}
            />
            {error && (
              <p id={`${id}-error`} role="alert" className="text-caption text-destructive">
                {error}
              </p>
            )}
          </div>
        )
      })}
      {stray.map(([key, message]) => (
        <p key={key} role="alert" className="text-caption text-destructive">
          {message}
        </p>
      ))}
    </div>
  )
}
