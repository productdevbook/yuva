import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, CopyIcon } from "lucide-react"
import { useState } from "react"

import { initials, useErrorText } from "@/components/common/text"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { cn } from "@/lib/utils"

export function EmptyState({
  icon: Icon,
  title,
  children,
  className,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: React.ReactNode
  children?: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn("flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center", className)}>
      <div className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Icon className="size-6" />
      </div>
      <p className="text-sm font-medium">{title}</p>
      {children && <div className="max-w-sm text-sm text-muted-foreground">{children}</div>}
    </div>
  )
}

export function ErrorLine({ error, className }: { error: unknown; className?: string }) {
  const text = useErrorText()
  if (!error) return null
  return (
    <p role="alert" className={cn("text-sm text-destructive", className)}>
      {text(error)}
    </p>
  )
}

export function Kbd({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        "inline-flex h-5 min-w-5 items-center justify-center rounded border bg-muted px-1 font-sans text-[11px] font-medium text-muted-foreground",
        className,
      )}
    >
      {children}
    </kbd>
  )
}

export function PersonAvatar({ name, className }: { name: string; className?: string }) {
  let hash = 0
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) | 0
  const hue = Math.abs(hash) % 360
  return (
    <span
      aria-hidden
      className={cn(
        "inline-flex size-7 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold text-white",
        className,
      )}
      style={{ backgroundColor: `oklch(0.6 0.13 ${hue})` }}
    >
      {initials(name)}
    </span>
  )
}

export function TypingDots({ className }: { className?: string }) {
  return (
    <span aria-hidden className={cn("inline-flex items-center gap-0.5", className)}>
      {[0, 150, 300].map((d) => (
        <span key={d} className="size-1 animate-bounce rounded-full bg-current" style={{ animationDelay: `${d}ms` }} />
      ))}
    </span>
  )
}

export function LabelChip({ name, color, className }: { name: string; color: string; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex max-w-40 items-center gap-1 rounded-full border px-2 py-0.5 text-xs leading-4 text-foreground",
        className,
      )}
    >
      <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: color }} />
      <span className="truncate">{name}</span>
    </span>
  )
}

export function CopyButton({ value, className }: { value: string; className?: string }) {
  const { t } = useLingui()
  const [copied, setCopied] = useState(false)
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className={className}
      onClick={async () => {
        await navigator.clipboard.writeText(value)
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      }}
      aria-label={t`Copy`}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
      {copied ? <Trans>Copied</Trans> : <Trans>Copy</Trans>}
    </Button>
  )
}

export function SecretDialog({
  secret,
  title,
  description,
  onClose,
}: {
  secret: string | null
  title: React.ReactNode
  description: React.ReactNode
  onClose: () => void
}) {
  return (
    <Dialog open={secret !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 rounded-md border bg-muted px-2 py-1.5 font-mono text-xs break-all">
            {secret}
          </code>
          {secret && <CopyButton value={secret} />}
        </div>
        <DialogFooter>
          <Button onClick={onClose}>
            <Trans>I saved it</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirm,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: React.ReactNode
  description?: React.ReactNode
  confirm: React.ReactNode
  onConfirm: () => void
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && <AlertDialogDescription>{description}</AlertDialogDescription>}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            <Trans>Cancel</Trans>
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={() => {
              onConfirm()
              onOpenChange(false)
            }}
          >
            {confirm}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

export function useConfirm() {
  const [state, setState] = useState<{
    title: React.ReactNode
    description?: React.ReactNode
    confirm: React.ReactNode
    run: () => void
  } | null>(null)
  const dialog = (
    <ConfirmDialog
      open={state !== null}
      onOpenChange={(open) => !open && setState(null)}
      title={state?.title}
      description={state?.description}
      confirm={state?.confirm}
      onConfirm={() => state?.run()}
    />
  )
  return [setState, dialog] as const
}
