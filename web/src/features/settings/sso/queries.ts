import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  api,
  unwrap,
  type SsoDomain,
  type SsoOidcInput,
  type SsoSamlInput,
  type SsoSettings,
  type SsoSettingsUpdate,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useSso(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.sso(ws),
    queryFn: () => unwrap(api.GET("/v1/sso")),
    enabled,
    retry: false,
  })
}

export function useSsoMutations() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const put = (s: SsoSettings) => qc.setQueryData(keys.sso(ws), s)
  const refresh = () => qc.invalidateQueries({ queryKey: keys.sso(ws) })
  return {
    rules: useMutation({
      mutationFn: (body: SsoSettingsUpdate) => unwrap(api.PATCH("/v1/sso", { body })),
      onSuccess: put,
    }),
    saveProvider: useMutation({
      mutationFn: (body: SsoOidcInput) => unwrap(api.PUT("/v1/sso/oidc", { body })),
      onSuccess: put,
    }),
    removeProvider: useMutation({
      mutationFn: () => unwrap(api.DELETE("/v1/sso/oidc")),
      onSuccess: refresh,
    }),
    saveSaml: useMutation({
      mutationFn: (body: SsoSamlInput) => unwrap(api.PUT("/v1/sso/saml", { body })),
      onSuccess: put,
    }),
    removeSaml: useMutation({
      mutationFn: () => unwrap(api.DELETE("/v1/sso/saml")),
      onSuccess: refresh,
    }),
    addDomain: useMutation({
      mutationFn: (domain: string) => unwrap(api.POST("/v1/sso/domains", { body: { domain } })),
      onSuccess: refresh,
    }),
    verifyDomain: useMutation({
      mutationFn: (d: SsoDomain) =>
        unwrap(api.POST("/v1/sso/domains/{ssoDomainId}/verify", { params: { path: { ssoDomainId: d.id } } })),
      onSuccess: refresh,
      onError: refresh,
    }),
    removeDomain: useMutation({
      mutationFn: (d: SsoDomain) =>
        unwrap(api.DELETE("/v1/sso/domains/{ssoDomainId}", { params: { path: { ssoDomainId: d.id } } })),
      onSuccess: refresh,
    }),
  }
}
