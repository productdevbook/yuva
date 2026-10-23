"use client"

import * as React from "react"
import { MessageSquareIcon, PlusIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemSeparator,
  ItemTitle,
} from "@/components/ui/item"
import type { YuvaChatState } from "@/hooks/use-yuva-chat"
import { cn } from "@/lib/utils"
import { defaultYuvaChatLabels, whenLabel, type YuvaChatLabels } from "@/lib/yuva-chat-utils"

export interface YuvaChatConversationsProps {
  chat: YuvaChatState
  labels?: Partial<YuvaChatLabels>
  locale?: string
  onOpen?: (id: string | null) => void
  className?: string
}

export function YuvaChatConversations({ chat, labels: custom, locale, onOpen, className }: YuvaChatConversationsProps) {
  const labels = React.useMemo(() => ({ ...defaultYuvaChatLabels, ...custom }), [custom])
  const open = (id: string | null) => {
    void chat.open(id)
    onOpen?.(id)
  }
  if (chat.conversations.length === 0) {
    return (
      <Empty className={cn("flex-1", className)}>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <MessageSquareIcon />
          </EmptyMedia>
          <EmptyTitle>{labels.noConversations}</EmptyTitle>
          <EmptyDescription>{labels.noConversationsHint}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button onClick={() => open(null)}>
            <PlusIcon />
            {labels.newConversation}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }
  return (
    <div className={cn("flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-2", className)}>
      <ItemGroup aria-label={labels.conversations}>
        {chat.conversations.map((conversation, index) => (
          <React.Fragment key={conversation.id}>
            {index > 0 && <ItemSeparator />}
            <Item asChild size="sm">
              <button
                type="button"
                className="w-full text-start"
                aria-current={chat.activeId === conversation.id || undefined}
                onClick={() => open(conversation.id)}
              >
                <ItemMedia variant="icon">
                  <MessageSquareIcon />
                </ItemMedia>
                <ItemContent>
                  <ItemTitle className={cn(conversation.unread && "font-semibold")}>
                    {conversation.subject || labels.conversationFallback}
                    {conversation.unread && (
                      <span className="size-2 rounded-full bg-primary" role="img" aria-label={labels.unread} />
                    )}
                  </ItemTitle>
                  {conversation.last_message && (
                    <ItemDescription>
                      {conversation.last_message.redacted ? labels.removed : conversation.last_message.text}
                    </ItemDescription>
                  )}
                </ItemContent>
                <ItemActions className="self-start text-xs text-muted-foreground">
                  {whenLabel(conversation.last_message_at ?? conversation.created_at, locale)}
                </ItemActions>
              </button>
            </Item>
          </React.Fragment>
        ))}
      </ItemGroup>
      {chat.hasMoreConversations && (
        <Button variant="ghost" size="sm" className="self-center" onClick={() => void chat.loadMoreConversations()}>
          {labels.loadMore}
        </Button>
      )}
    </div>
  )
}
