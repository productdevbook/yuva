import { useQuery } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useGrants() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.oauthGrants(ws),
    queryFn: () => unwrap(api.GET("/v1/oauth/grants")).then((r) => r.items),
    refetchOnWindowFocus: "always",
  })
}
