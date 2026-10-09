import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { SendIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Card, Row, Rows, Section } from "@/features/settings/ui"
import { api, unwrap, type PushSubscriptionItem } from "@/lib/api"
import { pushKeys, pushSupport, subscribe, unsubscribe, useBrowserSubscription, useVapidKey } from "@/lib/push"

function Device({
  s,
  mine,
  tested,
  onTest,
  onRemove,
  testing,
}: {
  s: PushSubscriptionItem
  mine: boolean
  tested: boolean
  onTest: () => void
  onRemove: () => void
  testing: boolean
}) {
  const { t, i18n } = useLingui()
  const name = s.user_agent || t`Unknown device`
  const created = formatDateTime(s.created_at, i18n.locale)
  const delivered = s.last_success_at ? formatDateTime(s.last_success_at, i18n.locale) : null
  const failed = s.last_failure_at && (!s.last_success_at || s.last_failure_at > s.last_success_at) ? s.last_error : null
  return (
    <Row>
      <div className="min-w-0 flex-1">
        <p className="flex flex-wrap items-center gap-2 text-body font-medium">
          <span className="truncate">{name}</span>
          {mine && (
            <span className="rounded-full bg-muted px-2 py-px text-caption font-normal text-muted-foreground">
              <Trans>This device</Trans>
            </span>
          )}
        </p>
        <p className="text-caption text-muted-foreground">
          {delivered ? <Trans>Added {created}, last delivered {delivered}</Trans> : <Trans>Added {created}</Trans>}
        </p>
        {failed && (
          <p className="text-caption text-destructive">
            <Trans>Last push failed: {failed}</Trans>
          </p>
        )}
        {tested && (
          <p role="status" className="text-caption text-success">
            <Trans>Test sent. It should arrive in a few seconds.</Trans>
          </p>
        )}
      </div>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t`Send a test notification to ${name}`}
        title={t`Send a test notification`}
        disabled={testing}
        onClick={onTest}
        data-testid="push-test"
      >
        <SendIcon />
      </Button>
      <Button variant="ghost" size="icon-sm" aria-label={t`Remove ${name}`} onClick={onRemove}>
        <Trash2Icon />
      </Button>
    </Row>
  )
}

export function PushSection() {
  const qc = useQueryClient()
  const [confirm, confirmDialog] = useConfirm()
  const support = pushSupport()
  const vapid = useVapidKey()
  const browser = useBrowserSubscription()
  const [permission, setPermission] = useState(() => ("Notification" in window ? Notification.permission : "default"))
  const [tested, setTested] = useState<string | null>(null)
  const enabled = support === "supported" && vapid.data !== null
  const list = useQuery({ queryKey: pushKeys.subscriptions, queryFn: () => unwrap(api.GET("/v1/me/push-subscriptions")).then((r) => r.items), enabled })
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: pushKeys.subscriptions })
    void qc.invalidateQueries({ queryKey: pushKeys.browser })
  }
  const mine = list.data?.find((s) => !!browser.data && s.endpoint === browser.data)
  const turnOn = useMutation({
    mutationFn: () => subscribe(vapid.data!),
    onSettled: () => {
      setPermission(Notification.permission)
      refresh()
    },
  })
  const turnOff = useMutation({ mutationFn: () => unsubscribe(mine?.id), onSettled: refresh })
  const remove = useMutation({
    mutationFn: (s: PushSubscriptionItem) =>
      s.id === mine?.id
        ? unsubscribe(s.id)
        : unwrap(api.DELETE("/v1/me/push-subscriptions/{pushSubscriptionId}", { params: { path: { pushSubscriptionId: s.id } } })),
    onSettled: refresh,
  })
  const test = useMutation({
    mutationFn: (id: string) => unwrap(api.POST("/v1/me/push-subscriptions/{pushSubscriptionId}/test", { params: { path: { pushSubscriptionId: id } } })),
    onSuccess: (_, id) => {
      setTested(id)
      setTimeout(() => void qc.invalidateQueries({ queryKey: pushKeys.subscriptions }), 4000)
    },
  })

  let status: React.ReactNode
  if (support === "needs-install") {
    status = (
      <Trans>
        On iPhone and iPad, add Yuva to your Home Screen with Share › Add to Home Screen, then open it from there to turn
        on notifications.
      </Trans>
    )
  } else if (support === "unsupported") {
    status = <Trans>This browser cannot receive push notifications.</Trans>
  } else if (vapid.data === null) {
    status = <Trans>This server is not set up to send push notifications.</Trans>
  } else if (permission === "denied") {
    status = <Trans>Notifications are blocked for this site. Allow them in your browser's site settings, then reload.</Trans>
  }

  return (
    <Section title={<Trans>Push notifications</Trans>} description={<Trans>Get notified on your phone or computer, even when the panel is closed.</Trans>}>
      <Card flush>
        {status ? (
          <p className="px-5 py-4 text-body text-muted-foreground" data-testid="push-status">
            {status}
          </p>
        ) : (
          vapid.data &&
          list.data && (
            <div className="flex flex-wrap items-center gap-3 px-5 py-4" data-testid="push-this-device">
              <p className="min-w-0 flex-1 text-body">{mine ? <Trans>On for this device.</Trans> : <Trans>Off for this device.</Trans>}</p>
              {mine ? (
                <Button variant="outline" size="sm" disabled={turnOff.isPending} onClick={() => turnOff.mutate()}>
                  <Trans>Turn off</Trans>
                </Button>
              ) : (
                <Button size="sm" disabled={turnOn.isPending} onClick={() => turnOn.mutate()} data-testid="push-turn-on">
                  <Trans>Turn on for this device</Trans>
                </Button>
              )}
            </div>
          )
        )}
        {enabled && list.data && list.data.length > 0 && (
          <Rows className="border-t" data-testid="push-devices">
            {list.data.map((s) => (
              <Device
                key={s.id}
                s={s}
                mine={s.id === mine?.id}
                tested={tested === s.id}
                testing={test.isPending}
                onTest={() => test.mutate(s.id)}
                onRemove={() =>
                  confirm({
                    title: <Trans>Remove this device?</Trans>,
                    description: <Trans>It will no longer receive notifications.</Trans>,
                    confirm: <Trans>Remove</Trans>,
                    run: () => remove.mutate(s),
                  })
                }
              />
            ))}
          </Rows>
        )}
        {turnOn.data === "denied" && permission !== "denied" && (
          <p role="alert" className="px-5 pb-4 text-body text-muted-foreground">
            <Trans>Notifications were not allowed.</Trans>
          </p>
        )}
        <ErrorLine error={turnOn.error ?? turnOff.error ?? remove.error ?? test.error ?? list.error ?? vapid.error} className="px-5 pb-4" />
      </Card>
      {confirmDialog}
    </Section>
  )
}
