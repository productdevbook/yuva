import { useLingui } from "@lingui/react/macro"
import {
  BoldIcon,
  Code2Icon,
  Heading2Icon,
  ImageIcon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  LoaderIcon,
  QuoteIcon,
  SquareCodeIcon,
} from "lucide-react"

import { Kbd } from "@/components/common"
import { alt, mod } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Format } from "@/features/help/markdown"

type Tool = { id: Format | "image"; icon: typeof BoldIcon; label: string; keys?: string[] }

export function EditorToolbar({
  onFormat,
  onImage,
  uploading,
}: {
  onFormat: (f: Format) => void
  onImage: () => void
  uploading: boolean
}) {
  const { t } = useLingui()
  const tools: (Tool | "|")[] = [
    { id: "bold", icon: BoldIcon, label: t`Bold`, keys: [mod, "B"] },
    { id: "italic", icon: ItalicIcon, label: t`Italic`, keys: [mod, "I"] },
    { id: "heading", icon: Heading2Icon, label: t`Heading` },
    "|",
    { id: "link", icon: LinkIcon, label: t`Link`, keys: [alt, "K"] },
    { id: "image", icon: ImageIcon, label: t`Image`, keys: [alt, "I"] },
    "|",
    { id: "bullets", icon: ListIcon, label: t`Bulleted list` },
    { id: "numbers", icon: ListOrderedIcon, label: t`Numbered list` },
    { id: "quote", icon: QuoteIcon, label: t`Quote` },
    "|",
    { id: "code", icon: Code2Icon, label: t`Code` },
    { id: "codeblock", icon: SquareCodeIcon, label: t`Code block` },
  ]
  return (
    <div
      role="toolbar"
      aria-label={t`Formatting`}
      className="flex flex-wrap items-center gap-0.5 rounded-xl border bg-card p-1"
      data-testid="help-toolbar"
    >
      {tools.map((tool, i) =>
        tool === "|" ? (
          <span key={i} className="mx-1 h-5 w-px shrink-0 bg-border" aria-hidden />
        ) : (
          <Tooltip key={tool.id}>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="shrink-0 text-muted-foreground"
                  aria-label={tool.label}
                  disabled={tool.id === "image" && uploading}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => (tool.id === "image" ? onImage() : onFormat(tool.id))}
                  data-testid={`help-tool-${tool.id}`}
                />
              }
            >
              {tool.id === "image" && uploading ? <LoaderIcon className="animate-spin" /> : <tool.icon />}
            </TooltipTrigger>
            <TooltipContent>
              {tool.label}
              {tool.keys && (
                <span className="ms-2 inline-flex gap-1">
                  {tool.keys.map((k) => (
                    <Kbd key={k}>{k}</Kbd>
                  ))}
                </span>
              )}
            </TooltipContent>
          </Tooltip>
        )
      )}
    </div>
  )
}
