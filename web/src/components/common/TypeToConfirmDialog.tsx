import { Trans } from "@lingui/react/macro"
import { useId, useState } from "react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export function TypeToConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  label,
  expected,
  ignoreCase = false,
  confirm,
  pending,
  error,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: React.ReactNode
  description: React.ReactNode
  label: React.ReactNode
  expected: string
  ignoreCase?: boolean
  confirm: React.ReactNode
  pending: boolean
  error?: React.ReactNode
  onConfirm: () => void
}) {
  const id = useId()
  const [typed, setTyped] = useState("")
  const matches = ignoreCase ? typed.trim().toLowerCase() === expected.toLowerCase() : typed === expected
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setTyped("")
        onOpenChange(next)
      }}
    >
      <DialogContent>
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (matches && !pending) onConfirm()
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor={id}>{label}</Label>
            <Input
              id={id}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
              spellCheck={false}
              placeholder={expected}
              data-testid="type-to-confirm"
            />
          </div>
          {error}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" variant="destructive" disabled={!matches || pending}>
              {confirm}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
