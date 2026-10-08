import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine, Kbd } from "@/components/common"
import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { DraftControls } from "@/features/conversation/messages/context"
import type { Message } from "@/lib/api"

const mod = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl"

export function DraftEditor({ m, drafts }: { m: Message; drafts: DraftControls }) {
  const { t } = useLingui()
  const [body, setBody] = useState(m.body)
  const submit = () => body.trim() && drafts.save(m, body)
  return (
    <form
      className="flex w-full max-w-xl flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Textarea
        autoFocus
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault()
            submit()
          } else if (e.key === "Escape") {
            e.preventDefault()
            drafts.setEditing(null)
          }
        }}
        aria-label={t`Draft text`}
        className="min-h-28 text-sm"
        data-testid="draft-editor"
      />
      <div className="flex items-center justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={() => drafts.setEditing(null)}>
          <Trans>Cancel</Trans>
        </Button>
        <span className="hidden items-center gap-1 sm:flex">
          <Kbd>{mod}</Kbd>
          <Kbd>Enter</Kbd>
        </span>
        <Button type="submit" size="sm" disabled={drafts.pending || !body.trim()}>
          <Trans>Save draft</Trans>
        </Button>
      </div>
    </form>
  )
}

function hint(keys: string[]) {
  return keys.join("+")
}

export function DraftActions({ m, drafts }: { m: Message; drafts: DraftControls }) {
  const latest = drafts.latest === m.id
  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex items-center gap-1" data-testid="draft-actions">
        <Button
          size="sm"
          onClick={() => drafts.send(m)}
          disabled={drafts.pending}
          title={latest ? hint(keyLabel(SHORTCUTS.sendDraft)) : undefined}
          data-testid="draft-send"
        >
          <Trans>Send</Trans>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => drafts.setEditing(m.id)}
          disabled={drafts.pending}
          title={latest ? hint(keyLabel(SHORTCUTS.editDraft)) : undefined}
          data-testid="draft-edit"
        >
          <Trans>Edit</Trans>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => drafts.discard(m)}
          disabled={drafts.pending}
          title={latest ? hint(keyLabel(SHORTCUTS.discardDraft)) : undefined}
          data-testid="draft-discard"
        >
          <Trans>Discard</Trans>
        </Button>
      </div>
      {latest && <ErrorLine error={drafts.error} />}
    </div>
  )
}
