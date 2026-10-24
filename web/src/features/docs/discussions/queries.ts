import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { store } from "@/features/docs/queries"
import {
  api,
  unwrap,
  type Discussion,
  type DiscussionBulkChange,
  type DiscussionDetail,
  type DiscussionTeamSort,
} from "@/lib/api"
import { keys, type DiscussionFilters, type QueueFilters } from "@/lib/keys"
import { useSession } from "@/lib/session"

export type ThreadView = "all" | "unanswered" | "pending" | "locked" | "hidden"

export const THREAD_VIEWS: ThreadView[] = ["all", "unanswered", "pending", "locked", "hidden"]

export const THREAD_SORTS: DiscussionTeamSort[] = ["activity", "new", "top"]

export const useThreadView = store<ThreadView>("all")
export const useThreadSort = store<DiscussionTeamSort>("activity")

export function viewFilters(v: ThreadView): DiscussionFilters {
  switch (v) {
    case "unanswered":
      return { state: "published", answered: false, locked: false }
    case "pending":
      return { pending: true }
    case "locked":
      return { locked: true }
    case "hidden":
      return { state: "hidden" }
    default:
      return {}
  }
}

export function threadLink(id: string) {
  return `/docs/discussions/${id}`
}

export function useDiscussions(f: DiscussionFilters, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.discussionList(ws, f),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/discussions", { params: { query: { ...f, cursor: pageParam, limit: 50 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled,
  })
}

export function useDiscussion(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.discussion(ws, id),
    queryFn: () => unwrap(api.GET("/v1/discussions/{discussionId}", { params: { path: { discussionId: id } } })),
    enabled: !!id,
  })
}

export function useQueue(f: QueueFilters) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.discussionQueue(ws, f),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/discussion-posts", {
          params: {
            query: {
              state: "pending",
              inbox_id: f.inbox_id || undefined,
              site: f.site || undefined,
              page: f.page || undefined,
              cursor: pageParam,
              limit: 50,
            },
          },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

function useRefresh() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return (thread?: Discussion) => {
    if (thread)
      qc.setQueryData<DiscussionDetail>(keys.discussion(ws, thread.id), (old) => old && { ...old, discussion: thread })
    void qc.invalidateQueries({ queryKey: keys.discussions(ws) })
    void qc.invalidateQueries({ queryKey: keys.docs(ws) })
  }
}

export function useUpdateDiscussion(id: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: { title?: string; locked?: boolean }) =>
      unwrap(api.PATCH("/v1/discussions/{discussionId}", { params: { path: { discussionId: id } }, body })),
    onSuccess: (d) => refresh(d),
  })
}

export function useAnswer(id: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (postId: string | null) =>
      postId
        ? unwrap(
            api.PUT("/v1/discussions/{discussionId}/answer", {
              params: { path: { discussionId: id } },
              body: { post_id: postId },
            })
          )
        : unwrap(api.DELETE("/v1/discussions/{discussionId}/answer", { params: { path: { discussionId: id } } })),
    onSuccess: (d) => refresh(d),
  })
}

export function useReply(id: string) {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: { body: string; parent_post_id?: string }) =>
      unwrap(api.POST("/v1/discussions/{discussionId}/posts", { params: { path: { discussionId: id } }, body })),
    onSuccess: () => refresh(),
  })
}

export function useBulkDiscussions() {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (body: DiscussionBulkChange) => unwrap(api.POST("/v1/discussions/bulk", { body })),
    onSuccess: () => refresh(),
  })
}

export type Moderation = "published" | "hidden" | "spam"

export function useModerate() {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: ({ postId, to }: { postId: string; to: Moderation }) =>
      to === "spam"
        ? unwrap(api.POST("/v1/discussion-posts/{postId}/spam", { params: { path: { postId } } }))
        : unwrap(api.PATCH("/v1/discussion-posts/{postId}", { params: { path: { postId } }, body: { state: to } })),
    onSuccess: () => refresh(),
  })
}
