import { Navigate, Route, Routes } from "react-router"

import { Gate } from "@/app/Gate"
import { SignInPage } from "@/features/auth/SignInPage"
import { QueuePage } from "@/features/inbox/QueuePage"
import { ConsentPage } from "@/features/oauth/ConsentPage"
import { ApiKeysPage } from "@/features/settings/api-keys/ApiKeysPage"
import { CannedRepliesPage } from "@/features/settings/canned/CannedRepliesPage"
import { ConnectedAppsPage } from "@/features/settings/connected-apps/ConnectedAppsPage"
import { InboxPage } from "@/features/settings/inboxes/InboxPage"
import { InboxWebhooksPage } from "@/features/settings/inboxes/InboxWebhooksPage"
import { NewInboxPage } from "@/features/settings/inboxes/NewInboxPage"
import { LabelsPage } from "@/features/settings/labels/LabelsPage"
import { MembersPage } from "@/features/settings/members/MembersPage"
import { NotificationsPage } from "@/features/settings/notifications/NotificationsPage"
import { ProfilePage } from "@/features/settings/profile/ProfilePage"
import { SettingsIndex, SettingsLayout } from "@/features/settings/SettingsLayout"
import { WebhookPage } from "@/features/settings/webhooks/WebhookPage"
import { WebhooksPage } from "@/features/settings/webhooks/WebhooksPage"
import { WorkspacePage } from "@/features/settings/workspace/WorkspacePage"

export function AppRoutes() {
  return (
    <Routes>
      <Route path="sign-in" element={<SignInPage />} />
      <Route path="oauth/consent" element={<ConsentPage />} />
      <Route element={<Gate />}>
        <Route index element={<QueuePage />} />
        <Route path="conversations/:conversationId" element={<QueuePage />} />
        <Route path="settings" element={<SettingsLayout />}>
          <Route index element={<SettingsIndex />} />
          <Route path="profile" element={<ProfilePage />} />
          <Route path="notifications" element={<NotificationsPage />} />
          <Route path="workspace" element={<WorkspacePage />} />
          <Route path="members" element={<MembersPage />} />
          <Route path="new/:inboxId?/:channelId?" element={<NewInboxPage />} />
          <Route path="inboxes/:inboxId" element={<InboxPage />} />
          <Route path="inboxes/:inboxId/webhooks" element={<InboxWebhooksPage />} />
          <Route path="labels" element={<LabelsPage />} />
          <Route path="canned-replies" element={<CannedRepliesPage />} />
          <Route path="api-keys" element={<ApiKeysPage />} />
          <Route path="connected-apps" element={<ConnectedAppsPage />} />
          <Route path="webhooks" element={<WebhooksPage />} />
          <Route path="webhooks/:webhookId" element={<WebhookPage />} />
          <Route path="*" element={<Navigate to="/settings" replace />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
