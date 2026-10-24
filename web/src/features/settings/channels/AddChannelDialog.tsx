import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useLocation, useNavigate } from "react-router"

import { ChannelIcon, toast } from "@/components/common"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { ApiChannelDialog } from "@/features/settings/channels/ApiChannelDialog"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import type { ChannelKind, Inbox } from "@/lib/api"
import { cn } from "@/lib/utils"

type Kind = ChannelKind | "docs"

export function AddChannelDialog({
  inboxes,
  initial,
  onClose,
}: {
  inboxes: Inbox[]
  initial?: string
  onClose: () => void
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { pathname, search } = useLocation()
  const [inboxId, setInboxId] = useState(
    initial && inboxes.some((i) => i.id === initial) ? initial : (inboxes[0]?.id ?? "")
  )
  const [picked, setKind] = useState<Kind>("chat")
  const [apiOpen, setApiOpen] = useState(false)
  const kinds: { value: Kind; label: string; icon: ChannelKind }[] = [
    { value: "chat", label: t`Website chat`, icon: "chat" },
    {
      value: "email",
      label: t`E-mail`,

      icon: "email",
    },
    { value: "app", label: t`Mobile app`, icon: "app" },
    {
      value: "docs",
      label: t`Documentation site`,

      icon: "chat",
    },
    {
      value: "api",
      label: t`API channel`,

      icon: "api",
    },
  ]
  const mail = inboxes.find((i) => i.id === inboxId)?.purpose === "mail"
  const kind: Kind = mail ? "email" : picked
  const shown = mail ? kinds.filter((k) => k.value === "email") : kinds
  if (apiOpen)
    return (
      <ApiChannelDialog
        inboxId={inboxId}
        onClose={onClose}
        onCreated={() => {
          toast(t`Channel added`)
          onClose()
        }}
      />
    )
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="add-channel">
        <DialogHeader>
          <DialogTitle>
            <Trans>Add a channel</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Pick the product and how customers reach it. The next steps set it up.</Trans>
          </DialogDescription>
        </DialogHeader>
        <Field label={<Trans>Product</Trans>} htmlFor="add-channel-product">
          <ChoiceSelect
            id="add-channel-product"
            label={t`Product`}
            value={inboxId}
            onChange={setInboxId}
            options={inboxes.map((i) => [i.id, i.name] as const)}
            className="w-full"
          />
        </Field>
        <div className="flex flex-col gap-2" role="radiogroup" aria-label={t`Channel`}>
          {shown.map((k) => (
            <button
              key={k.value}
              type="button"
              role="radio"
              aria-checked={kind === k.value}
              onClick={() => setKind(k.value)}
              className={cn(
                "flex items-center gap-3 rounded-xl border px-3.5 py-2.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
                kind === k.value ? "border-foreground bg-surface" : "hover:border-faint"
              )}
              data-testid={`add-channel-${k.value}`}
            >
              <ChannelIcon kind={k.icon} className="size-4 shrink-0 text-muted-foreground" />
              <span className="flex min-w-0 flex-col">
                <span className="font-medium">{k.label}</span>
              </span>
            </button>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            <Trans>Cancel</Trans>
          </Button>
          <Button
            disabled={!inboxId}
            onClick={() => {
              if (kind === "api") return setApiOpen(true)
              navigate(`/setup/${inboxId}/add/${kind}`, {
                state: { back: `${pathname}${search}`, existing: true },
              })
            }}
            data-testid="add-channel-continue"
          >
            <Trans>Continue</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
