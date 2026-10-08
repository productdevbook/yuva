import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, Row, Rows, RowText, Section } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { passkeysSupported, registerPasskey } from "@/lib/passkey"

export function Passkeys() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const [name, setName] = useState("")
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({ queryKey: keys.passkeys, queryFn: () => unwrap(api.GET("/v1/me/passkeys")).then((r) => r.items) })
  const refresh = () => qc.invalidateQueries({ queryKey: keys.passkeys })
  const add = useMutation({
    mutationFn: () => registerPasskey(name.trim()),
    onSuccess: () => {
      setName("")
      void refresh()
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/me/passkeys/{passkeyId}", { params: { path: { passkeyId: id } } })),
    onSuccess: refresh,
  })
  const supported = passkeysSupported()
  return (
    <Section
      title={<Trans>Passkeys</Trans>}
      description={<Trans>Sign in with your device's fingerprint, face or screen lock instead of a code.</Trans>}
    >
      <Card
        flush
        footer={
          supported ? (
            <form
              className="flex w-full flex-col gap-2 sm:flex-row"
              onSubmit={(e) => {
                e.preventDefault()
                add.mutate()
              }}
            >
              <Input
                value={name}
                maxLength={200}
                onChange={(e) => setName(e.target.value)}
                placeholder={t`Name, e.g. Work laptop`}
                aria-label={t`Passkey name`}
                className="sm:max-w-64"
              />
              <Button type="submit" variant="outline" disabled={add.isPending}>
                <Trans>Add a passkey</Trans>
              </Button>
            </form>
          ) : (
            <p className="text-sm text-muted-foreground">
              <Trans>This browser does not support passkeys.</Trans>
            </p>
          )
        }
      >
        {list.data && list.data.length > 0 ? (
          <Rows>
            {list.data.map((p) => {
              const created = formatDateTime(p.created_at, i18n.locale)
              const used = p.last_used_at ? formatDateTime(p.last_used_at, i18n.locale) : null
              return (
                <Row key={p.id}>
                  <RowText
                    title={p.name}
                    detail={used ? <Trans>Added {created}, last used {used}</Trans> : <Trans>Added {created}</Trans>}
                  />
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t`Remove ${p.name}`}
                    onClick={() =>
                      confirm({
                        title: <Trans>Remove this passkey?</Trans>,
                        description: <Trans>You will no longer be able to sign in with it.</Trans>,
                        confirm: <Trans>Remove</Trans>,
                        run: () => remove.mutate(p.id),
                      })
                    }
                  >
                    <Trash2Icon />
                  </Button>
                </Row>
              )
            })}
          </Rows>
        ) : (
          list.data && (
            <p className="px-5 py-4 text-sm text-muted-foreground">
              <Trans>You have no passkeys yet.</Trans>
            </p>
          )
        )}
        {add.error && (
          <p role="alert" className="px-5 pb-4 text-sm text-destructive">
            <Trans>The passkey was not added. Try again.</Trans>
          </p>
        )}
        <ErrorLine error={remove.error} className="px-5 pb-4" />
      </Card>
      {confirmDialog}
    </Section>
  )
}
