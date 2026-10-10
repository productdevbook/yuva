import { Trans } from "@lingui/react/macro"

import { ErrorLine } from "@/components/common"
import { EventsSection } from "@/features/settings/notifications/EventsSection"
import { InboxOverrides } from "@/features/settings/notifications/InboxOverrides"
import { PushSection } from "@/features/settings/notifications/PushSection"
import { TabAlertsSection } from "@/features/settings/notifications/TabAlertsSection"
import { useNotificationSettings } from "@/features/settings/notifications/queries"
import { PageHeader } from "@/features/settings/ui"

export function NotificationsPage() {
  const settings = useNotificationSettings()
  return (
    <>
      <PageHeader title={<Trans>Notifications</Trans>} />
      <PushSection />
      <TabAlertsSection />
      {settings.data && (
        <>
          <EventsSection settings={settings.data} />
          <InboxOverrides settings={settings.data} />
        </>
      )}
      <ErrorLine error={settings.error} />
    </>
  )
}
