import { createContext, useContext } from "react"

import type { Block, Doc } from "@/features/designs/doc"
import type { EmailDesignIssue, EmailImage, EmailThemeColors } from "@/lib/api"

export type DropTarget = { container: string; index: number }

export type View = { phone: boolean; dark: boolean; colors: EmailThemeColors }

export type Designer = {
  doc: Doc
  original: Doc
  locale: string | null
  missing: Set<string>
  locked: boolean
  setText: (id: string, value: string, key?: string) => void
  setLocale: (tag: string | null) => void
  view: View
  selected: string | null
  select: (id: string | null) => void
  setDoc: (fn: (d: Doc) => Doc, key?: string) => void
  update: (id: string, patch: Partial<Block> | ((b: Block) => Block), key?: string) => void
  remove: (id: string) => void
  duplicate: (id: string) => void
  nudge: (id: string, by: -1 | 1) => void
  images: Map<string, EmailImage>
  issues: Map<string, EmailDesignIssue[]>
  readOnly: boolean
  openLibrary: (blockId: string) => void
  drop: DropTarget | null
  dragging: string | null
  toolSlot: HTMLElement | null
}

export const DesignerContext = createContext<Designer | null>(null)

export function useDesigner() {
  const d = useContext(DesignerContext)
  if (!d) throw new Error("useDesigner outside the designer")
  return d
}
