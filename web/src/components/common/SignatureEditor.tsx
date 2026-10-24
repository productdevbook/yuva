import { Trans, useLingui } from "@lingui/react/macro"
import { ImageIcon } from "lucide-react"
import { useRef } from "react"

import { ErrorLine } from "@/components/common/ErrorLine"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useUploadImage } from "@/features/designs/queries"

const NAME = "{{member.name}}"
const TITLE = "{{member.title}}"

export function SignatureEditor({
  id,
  value,
  onChange,
  disabled,
  placeholder,
}: {
  id: string
  value: string
  onChange: (v: string) => void
  disabled?: boolean
  placeholder?: string
}) {
  const { t } = useLingui()
  const area = useRef<HTMLTextAreaElement>(null)
  const file = useRef<HTMLInputElement>(null)
  const upload = useUploadImage()
  const insert = (f: File) =>
    upload.mutate(f, {
      onSuccess: (image) => {
        const el = area.current
        const start = el?.selectionStart ?? value.length
        const end = el?.selectionEnd ?? start
        const text = `![](${image.url})`
        onChange(value.slice(0, start) + text + value.slice(end))
        requestAnimationFrame(() => {
          el?.focus()
          el?.setSelectionRange(start + text.length, start + text.length)
        })
      },
    })
  return (
    <div className="flex flex-col gap-2">
      <Textarea
        ref={area}
        id={id}
        rows={5}
        maxLength={4000}
        value={value}
        disabled={disabled}
        placeholder={placeholder}
        className="rounded-[10px] bg-background font-mono text-small"
        onChange={(e) => onChange(e.target.value)}
      />
      <div className="flex flex-wrap items-center gap-2">
        <input
          ref={file}
          type="file"
          hidden
          accept="image/png,image/jpeg,image/gif"
          onChange={(e) => {
            const f = e.target.files?.[0]
            e.target.value = ""
            if (f) insert(f)
          }}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || upload.isPending}
          onClick={() => file.current?.click()}
          title={t`Upload an image and put it at the cursor`}
          data-testid="signature-add-image"
        >
          <ImageIcon />
          <Trans>Add image</Trans>
        </Button>
        <span className="text-caption text-faint">
          <Trans>
            Markdown. {NAME} and {TITLE} are replaced with the author's name and title.
          </Trans>
        </span>
      </div>
      <ErrorLine error={upload.error} className="text-caption" />
    </div>
  )
}
