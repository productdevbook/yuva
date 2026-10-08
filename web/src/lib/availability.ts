import { useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { toast } from "@/components/common/Toast"
import { api, unwrap, type Availability } from "@/lib/api"
import { meKey } from "@/lib/session"

export function useSetAvailability() {
  const { t } = useLingui()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (value: Availability) => unwrap(api.PATCH("/v1/me", { body: { availability: value } })),
    onSuccess: (data) => {
      qc.setQueryData(meKey, data)
      toast(data.person.availability === "away" ? t`You are away` : t`You are available`)
    },
  })
}
