import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type MemberAddress } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useMyAddresses(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.myAddresses(ws),
    queryFn: () => unwrap(api.GET("/v1/me/addresses")).then((r) => r.items),
    staleTime: 60_000,
    enabled,
  })
}

export function useAddressActions() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = () => qc.invalidateQueries({ queryKey: keys.myAddresses(ws) })
  const add = useMutation({
    mutationFn: (address: string) => unwrap(api.POST("/v1/me/addresses", { body: { address } })),
    onSuccess: refresh,
  })
  const verify = useMutation({
    mutationFn: (v: { id: string; code: string }) =>
      unwrap(
        api.POST("/v1/me/addresses/{memberAddressId}/verify", {
          params: { path: { memberAddressId: v.id } },
          body: { code: v.code },
        })
      ),
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/me/addresses/{memberAddressId}", { params: { path: { memberAddressId: id } } })),
    onSuccess: refresh,
  })
  return { add, verify, remove }
}

export function useMemberAddresses(memberId: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.memberAddresses(ws, memberId),
    queryFn: () =>
      unwrap(api.GET("/v1/members/{memberId}/addresses", { params: { path: { memberId } } })).then((r) => r.items),
    enabled,
  })
}

export function useApproveAddress(memberId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(
        api.POST("/v1/members/{memberId}/addresses/{memberAddressId}/approve", {
          params: { path: { memberId, memberAddressId: id } },
        })
      ),
    onSuccess: (saved: MemberAddress) =>
      qc.setQueryData<MemberAddress[]>(keys.memberAddresses(ws, memberId), (all) =>
        (all ?? []).map((a) => (a.id === saved.id ? saved : a))
      ),
  })
}
