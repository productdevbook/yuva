import {
  Columns2Icon,
  HeadingIcon,
  ImageIcon,
  MessageSquareTextIcon,
  MinusIcon,
  MousePointerClickIcon,
  MoveVerticalIcon,
  PanelBottomIcon,
  Share2Icon,
  SquareDashedIcon,
  TextIcon,
} from "lucide-react"

import type { BlockType } from "@/features/designs/doc"

const ICONS: Record<BlockType, React.ComponentType<{ className?: string }>> = {
  section: SquareDashedIcon,
  columns: Columns2Icon,
  heading: HeadingIcon,
  text: TextIcon,
  image: ImageIcon,
  button: MousePointerClickIcon,
  divider: MinusIcon,
  spacer: MoveVerticalIcon,
  social: Share2Icon,
  footer: PanelBottomIcon,
  message: MessageSquareTextIcon,
}

export function BlockIcon({ type, className }: { type: BlockType; className?: string }) {
  const Icon = ICONS[type]
  return <Icon className={className} />
}
