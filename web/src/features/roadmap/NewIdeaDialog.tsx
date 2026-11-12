import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
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
import { Textarea } from "@/components/ui/textarea"
import { ideaLink, useCreateIdea } from "@/features/roadmap/queries"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import type { RoadmapBoard } from "@/lib/api"

export function NewIdeaDialog({
  boards,
  initialBoardId,
  onClose,
}: {
  boards: RoadmapBoard[]
  initialBoardId?: string
  onClose: () => void
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const create = useCreateIdea()
  const [boardId, setBoardId] = useState(boards.find((b) => b.id === initialBoardId)?.id ?? boards[0]?.id ?? "")
  const [title, setTitle] = useState("")
  const [body, setBody] = useState("")
  const priv = boards.find((b) => b.id === boardId)?.public === false
  const ok = !!boardId && title.trim().length >= 3 && body.trim().length > 0
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="new-idea-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (ok)
              create.mutate(
                { board_id: boardId, title: title.trim(), body: body.trim() },
                {
                  onSuccess: (d) => {
                    onClose()
                    navigate(ideaLink(boardId, d.id))
                  },
                }
              )
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>New idea</Trans>
            </DialogTitle>
            <DialogDescription>
              {priv ? (
                <Trans>It goes into Ideas on the board. The board is private, so only your team sees it.</Trans>
              ) : (
                <Trans>It goes into Ideas on the board, where customers can vote on it.</Trans>
              )}
            </DialogDescription>
          </DialogHeader>
          {boards.length > 1 && (
            <Field label={<Trans>Board</Trans>} htmlFor="new-idea-board">
              <ChoiceSelect
                id="new-idea-board"
                value={boardId}
                onChange={setBoardId}
                options={boards.map((b) => [b.id, b.name] as const)}
                label={t`Board`}
                className="w-64"
              />
            </Field>
          )}
          <Field label={<Trans>Title</Trans>} htmlFor="new-idea-title" hint={<Trans>At least 3 characters.</Trans>}>
            <Input
              id="new-idea-title"
              value={title}
              maxLength={200}
              autoFocus
              required
              onChange={(e) => setTitle(e.target.value)}
              data-testid="new-idea-title"
            />
          </Field>
          <Field label={<Trans>Description</Trans>} htmlFor="new-idea-body">
            <Textarea
              id="new-idea-body"
              value={body}
              rows={4}
              required
              onChange={(e) => setBody(e.target.value)}
              data-testid="new-idea-body"
            />
          </Field>
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ok || create.isPending} data-testid="create-idea">
              {create.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Create idea</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
