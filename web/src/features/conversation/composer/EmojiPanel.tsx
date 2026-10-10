import { Trans, useLingui } from "@lingui/react/macro"
import { EmojiPicker } from "frimousse"

import { cn } from "@/lib/utils"

export default function EmojiPanel({ onPick }: { onPick: (emoji: string) => void }) {
  const { t } = useLingui()
  return (
    <EmojiPicker.Root locale="en" emojibaseUrl="/emojibase" columns={9} onEmojiSelect={({ emoji }) => onPick(emoji)} className="isolate flex h-[340px] w-full flex-col" data-testid="emoji-picker">
      <EmojiPicker.Search
        name="emoji-search"
        autoFocus
        placeholder={t`Search emoji (in English)`}
        aria-label={t`Search emoji`}
        className="mx-2 mt-2 h-8 shrink-0 rounded-lg border bg-muted px-2.5 text-small outline-none focus-visible:border-brand/45 focus-visible:bg-card"
      />
      <EmojiPicker.Viewport className="relative min-h-0 flex-1 outline-none">
        <EmojiPicker.Loading className="absolute inset-0 grid place-items-center text-small text-faint">
          <Trans>Loading…</Trans>
        </EmojiPicker.Loading>
        <EmojiPicker.Empty className="absolute inset-0 grid place-items-center text-small text-faint">
          <Trans>No emoji found.</Trans>
        </EmojiPicker.Empty>
        <EmojiPicker.List
          className="pb-1.5 select-none"
          components={{
            CategoryHeader: ({ category, ...props }) => (
              <div className="bg-popover px-3 pt-2.5 pb-1 text-caption font-medium text-faint" {...props}>
                {category.label}
              </div>
            ),
            Row: ({ children, ...props }) => (
              <div className="scroll-my-1.5 px-1.5" {...props}>
                {children}
              </div>
            ),
            Emoji: ({ emoji, ...props }) => (
              <button className={cn("grid size-8 place-items-center rounded-md text-xl", emoji.isActive && "bg-muted")} {...props}>
                {emoji.emoji}
              </button>
            ),
          }}
        />
      </EmojiPicker.Viewport>
    </EmojiPicker.Root>
  )
}
