import { msg } from "@lingui/core/macro"
import { toast as sonner } from "sonner"

import { i18n } from "@/i18n"

export function toast(text: React.ReactNode, undo?: () => void) {
  sonner.dismiss()
  sonner(text, {
    duration: undo ? 6000 : 2200,
    action: undo ? { label: i18n._(msg`Undo`), onClick: undo } : undefined,
    classNames: { actionButton: "toast-undo" },
  })
}

export { Toaster } from "@/components/ui/sonner"
