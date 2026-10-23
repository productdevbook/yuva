import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { ErrorLine, toast, useConfirm } from "@/components/common"
import { formatDateTime, formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Card, Pill, Row, Rows, RowText, Section } from "@/features/settings/ui"
import { api, unwrap, type Session } from "@/lib/api"
import { keys } from "@/lib/keys"

export function Sessions() {
  const { t, i18n } = useLingui()
  const qc = useQueryClient()
  const [confirm, confirmDialog] = useConfirm()
  const list = useQuery({
    queryKey: keys.sessions,
    queryFn: () => unwrap(api.GET("/v1/me/sessions")).then((r) => r.items),
  })
  const others = (list.data ?? []).filter((s) => !s.current).length
  const signOut = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/me/sessions")),
    onSuccess: () => {
      toast(t`Signed out everywhere else`)
      void qc.invalidateQueries({ queryKey: keys.sessions })
    },
  })
  const method: Record<Session["method"], string> = {
    code: t`E-mailed code`,
    passkey: t`Passkey`,
    sso: t`Single sign-on`,
  }
  return (
    <Section
      title={<Trans>Sessions</Trans>}
      description={
        <Trans>The browsers and devices you are signed in on. Sign out of the rest if you do not know one.</Trans>
      }
    >
      <Card
        flush
        footer={
          <Button
            type="button"
            variant="outline"
            disabled={others === 0 || signOut.isPending}
            data-testid="sessions-sign-out-others"
            onClick={() =>
              confirm({
                title: <Trans>Sign out everywhere else?</Trans>,
                description: <Trans>Every other browser and device is signed out. This one stays signed in.</Trans>,
                confirm: <Trans>Sign out</Trans>,
                run: () => signOut.mutate(),
              })
            }
          >
            <Trans>Sign out everywhere else</Trans>
          </Button>
        }
      >
        {list.data && (
          <Rows>
            {list.data.map((s) => {
              const via = method[s.method]
              const created = formatDateTime(s.created_at, i18n.locale)
              const seen = formatRelative(s.last_seen_at, i18n.locale)
              return (
                <Row key={s.id} data-testid="session-row">
                  <RowText
                    title={
                      <span className="flex flex-wrap items-center gap-2">
                        <span>{s.device ?? <Trans>Unknown device</Trans>}</span>
                        {s.current && (
                          <Pill tone="muted">
                            <Trans>This device</Trans>
                          </Pill>
                        )}
                      </span>
                    }
                    detail={
                      <Trans>
                        {via}, signed in {created}, last active {seen}
                      </Trans>
                    }
                  />
                </Row>
              )
            })}
          </Rows>
        )}
        <ErrorLine error={list.error ?? signOut.error} className="px-5 pb-4" />
      </Card>
      {confirmDialog}
    </Section>
  )
}
