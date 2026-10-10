import { Trans } from "@lingui/react/macro"
import { BellRingIcon, Volume2Icon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Card, Section, ToggleRow } from "@/features/settings/ui"
import { notificationPermission, playAlertSound, setTabAlerts, useTabAlerts } from "@/lib/alerts"

export function TabAlertsSection() {
  const on = useTabAlerts()
  const [permission, setPermission] = useState(notificationPermission)
  const ask = () => {
    void Notification.requestPermission().then(setPermission)
  }
  return (
    <Section
      title={<Trans>In this browser tab</Trans>}
      description={<Trans>Only while the panel is open in this browser. Away, you are alerted only about conversations assigned to you.</Trans>}
    >
      <Card>
        <ToggleRow
          title={<Trans>Alert me while the tab is in the background</Trans>}
          hint={<Trans>A desktop notification and a short sound for new messages in conversations assigned to you or to nobody.</Trans>}
        >
          <Switch checked={on} onCheckedChange={(v) => setTabAlerts(!!v)} data-testid="tab-alerts-toggle" />
        </ToggleRow>
        {on && (
          <div className="flex flex-wrap items-center gap-3" data-testid="tab-alerts-permission">
            <p className="min-w-0 flex-1 text-body text-muted-foreground">
              {permission === "granted" ? (
                <Trans>Desktop notifications are allowed.</Trans>
              ) : permission === "denied" ? (
                <Trans>Desktop notifications are blocked for this site in the browser's settings, so only the sound plays.</Trans>
              ) : permission === "unsupported" ? (
                <Trans>This browser cannot show desktop notifications, so only the sound plays.</Trans>
              ) : (
                <Trans>The browser has to allow desktop notifications first; until then only the sound plays.</Trans>
              )}
            </p>
            {permission === "default" && (
              <Button size="sm" onClick={ask} data-testid="tab-alerts-allow">
                <BellRingIcon />
                <Trans>Allow desktop notifications</Trans>
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={playAlertSound} data-testid="tab-alerts-sound">
              <Volume2Icon />
              <Trans>Play the sound</Trans>
            </Button>
          </div>
        )}
      </Card>
    </Section>
  )
}
