import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useNavigate } from "react-router"

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
import { ColorPicker, PRODUCT_COLORS } from "@/features/settings/inboxes/colors"
import { Field } from "@/features/settings/ui"
import { useInboxDraft } from "@/features/setup/draft"
import { MailboxOwnerChoice } from "@/components/common/MailboxOwnerChoice"
import { PurposeChoice } from "@/components/common/PurposeChoice"
import type { InboxPurpose } from "@/lib/api"
import { cn } from "@/lib/utils"

export function AddProductDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [, patch] = useInboxDraft()
  const [name, setName] = useState("")
  const [color, setColor] = useState(PRODUCT_COLORS[5])
  const [mode, setMode] = useState<"live" | "async">("live")
  const [purpose, setPurpose] = useState<InboxPurpose>("support")
  const [owner, setOwner] = useState<string | undefined>(undefined)
  const modes: { value: "live" | "async"; title: React.ReactNode; hint: React.ReactNode }[] = [
    {
      value: "live",
      title: <Trans>Live chat</Trans>,
      hint: <Trans>The chat shows who is available and typing</Trans>,
    },
    {
      value: "async",
      title: <Trans>Async</Trans>,
      hint: <Trans>The chat shows when you usually reply</Trans>,
    },
  ]
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="add-product">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!name.trim() || (purpose === "mail" && owner === "")) return
            patch({ name: name.trim(), color, mode, purpose, ownerMemberId: purpose === "mail" ? owner : undefined })
            navigate(purpose === "mail" ? "/setup/new/add/email" : "/setup/new/pick", { state: { back: "/settings" } })
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Add a product</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>The next step adds its first channel: website chat, e-mail or a mobile app.</Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Product name</Trans>} htmlFor="add-product-name">
            <Input
              id="add-product-name"
              autoFocus
              required
              maxLength={200}
              value={name}
              placeholder={t`For example Fieldnote`}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label={<Trans>Colour</Trans>}>
            <ColorPicker value={color} onChange={setColor} />
          </Field>
          <PurposeChoice
            value={purpose}
            onChange={(p) => {
              setPurpose(p)
              setOwner(undefined)
            }}
          />
          {purpose === "mail" && <MailboxOwnerChoice value={owner} onChange={setOwner} />}
          {purpose === "support" && (
            <Field label={<Trans>How do you answer?</Trans>}>
              <div className="flex flex-col gap-2" role="radiogroup">
                {modes.map((m) => (
                  <button
                    key={m.value}
                    type="button"
                    role="radio"
                    aria-checked={mode === m.value}
                    onClick={() => setMode(m.value)}
                    className={cn(
                      "flex flex-col rounded-xl border px-3.5 py-2.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
                      mode === m.value ? "border-foreground bg-surface" : "hover:border-faint"
                    )}
                  >
                    <span className="font-medium">{m.title}</span>
                    <span className="text-caption text-faint">{m.hint}</span>
                  </button>
                ))}
              </div>
            </Field>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button
              type="submit"
              disabled={!name.trim() || (purpose === "mail" && owner === "")}
              data-testid="add-product-create"
            >
              <Trans>Create and add a channel</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
