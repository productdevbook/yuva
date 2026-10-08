import { createContext, createElement, useCallback, useContext, useEffect, useRef, useState, type ReactElement, type ReactNode } from "react";
import type { ClientConversation, ClientMessage } from "./client/api";
import type { MessageInput, YuvaClient } from "./client/client";

const YuvaContext = createContext<YuvaClient | null>(null);

export interface YuvaProviderProps {
  client: YuvaClient;
  connect?: boolean;
  children?: ReactNode;
}

export function YuvaProvider({ client, connect = true, children }: YuvaProviderProps): ReactElement {
  useEffect(() => {
    if (!connect) return;
    client.connect().catch(() => undefined);
    return () => client.disconnect();
  }, [client, connect]);
  return createElement(YuvaContext.Provider, { value: client }, children);
}

export function useYuva(): YuvaClient {
  const client = useContext(YuvaContext);
  if (!client) throw new Error("Yuva: wrap your components in <YuvaProvider client={...}>");
  return client;
}

const time = (value: string) => Date.parse(value);
const byCreated = (a: ClientMessage, b: ClientMessage) => time(a.created_at) - time(b.created_at);
const byActivity = (a: ClientConversation, b: ClientConversation) =>
  time(b.last_message_at ?? b.created_at) - time(a.last_message_at ?? a.created_at);

interface ListState<T> {
  items: T[];
  cursor?: string;
  loading: boolean;
  error: unknown;
}

const initial = { items: [], loading: true, error: null };

export interface ConversationsResult {
  conversations: ClientConversation[];
  loading: boolean;
  error: unknown;
  hasMore: boolean;
  loadMore(): Promise<void>;
  reload(): Promise<void>;
}

export function useConversations(): ConversationsResult {
  const client = useYuva();
  const [state, setState] = useState<ListState<ClientConversation>>(initial);
  const cursor = useRef<string | undefined>(undefined);
  const current = useRef(state);
  current.current = state;

  const load = useCallback(
    async (more: boolean) => {
      setState((s) => ({ ...s, loading: true }));
      try {
        const page = await client.listConversations({ limit: 50, cursor: more ? cursor.current : undefined });
        cursor.current = page.next_cursor;
        setState((s) => {
          const items = new Map((more ? s.items : []).map((c) => [c.id, c]));
          for (const conversation of page.items) items.set(conversation.id, conversation);
          return { items: [...items.values()].sort(byActivity), cursor: page.next_cursor, loading: false, error: null };
        });
      } catch (error) {
        setState((s) => ({ ...s, loading: false, error }));
      }
    },
    [client],
  );

  useEffect(() => {
    void load(false);
    const offSession = client.on("session", ({ renewed }) => void (renewed && load(false)));
    const offEvent = client.on("event", (message) => {
      if (message.type === "resync_required") return void load(false);
      if (message.type === "conversation.created") {
        setState((s) => (s.items.some((c) => c.id === message.data.id) ? s : { ...s, items: [message.data, ...s.items].sort(byActivity) }));
      } else if (message.type === "conversation.updated") {
        setState((s) => ({ ...s, items: s.items.map((c) => (c.id === message.conversation_id ? { ...c, status: message.data.status } : c)) }));
      } else if (message.type === "message.created") {
        const m = message.data;
        if (!current.current.items.some((c) => c.id === m.conversation_id)) return void load(false);
        setState((s) => {
          const items = s.items.map((c) =>
            c.id === m.conversation_id
              ? {
                  ...c,
                  last_message: { id: m.id, author_type: m.author.type, text: m.body.slice(0, 140), created_at: m.created_at },
                  last_message_at: m.created_at,
                  unread: c.unread || m.author.type !== "contact",
                }
              : c,
          );
          return { ...s, items: items.sort(byActivity) };
        });
      }
    });
    return () => {
      offSession();
      offEvent();
    };
  }, [client, load]);

  return {
    conversations: state.items,
    loading: state.loading,
    error: state.error,
    hasMore: !!state.cursor,
    loadMore: () => load(true),
    reload: () => load(false),
  };
}

export interface MessagesResult {
  messages: ClientMessage[];
  loading: boolean;
  error: unknown;
  hasMore: boolean;
  loadOlder(): Promise<void>;
  send(input: MessageInput | string): Promise<ClientMessage>;
  markRead(): Promise<void>;
}

export function useMessages(conversationId: string | null | undefined): MessagesResult {
  const client = useYuva();
  const [state, setState] = useState<ListState<ClientMessage>>(initial);
  const cursor = useRef<string | undefined>(undefined);

  const upsert = useCallback((message: ClientMessage) => {
    setState((s) => {
      const items = s.items.filter((m) => m.id !== message.id && (!message.client_id || m.client_id !== message.client_id));
      return { ...s, items: [...items, message].sort(byCreated) };
    });
  }, []);

  const load = useCallback(
    async (older: boolean) => {
      if (!conversationId) {
        cursor.current = undefined;
        setState({ items: [], loading: false, error: null });
        return;
      }
      if (older && !cursor.current) return;
      setState((s) => ({ ...s, loading: true }));
      try {
        const page = await client.listMessages(conversationId, { order: "desc", limit: 30, cursor: older ? cursor.current : undefined });
        cursor.current = page.next_cursor;
        setState((s) => {
          const items = new Map((older ? s.items : []).map((m) => [m.id, m]));
          for (const message of page.items) items.set(message.id, message);
          return { items: [...items.values()].sort(byCreated), cursor: page.next_cursor, loading: false, error: null };
        });
      } catch (error) {
        setState((s) => ({ ...s, loading: false, error }));
      }
    },
    [client, conversationId],
  );

  useEffect(() => {
    void load(false);
    if (!conversationId) return;
    const offSession = client.on("session", ({ renewed }) => void (renewed && load(false)));
    const offEvent = client.on("event", (message) => {
      if (message.type === "resync_required") void load(false);
      else if ((message.type === "message.created" || message.type === "message.updated") && message.data.conversation_id === conversationId) upsert(message.data);
    });
    return () => {
      offSession();
      offEvent();
    };
  }, [client, conversationId, load, upsert]);

  return {
    messages: state.items,
    loading: state.loading,
    error: state.error,
    hasMore: !!state.cursor,
    loadOlder: () => load(true),
    async send(input) {
      if (!conversationId) throw new Error("Yuva: no conversation; start one with client.startConversation()");
      const message = await client.sendMessage(conversationId, typeof input === "string" ? { body: input } : input);
      upsert(message);
      return message;
    },
    async markRead() {
      if (conversationId) await client.markRead(conversationId);
    },
  };
}
