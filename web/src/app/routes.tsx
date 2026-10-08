import { Navigate, Route, Routes } from "react-router"

import { Gate } from "@/app/Gate"
import { SignInPage } from "@/features/auth/SignInPage"
import { InboxPage, OpenConversation } from "@/features/inbox/InboxPage"
import { ApiKeysPage } from "@/features/settings/api-keys/ApiKeysPage"
import { CannedRepliesPage } from "@/features/settings/canned/CannedRepliesPage"
import { ChannelsPage } from "@/features/settings/channels/ChannelsPage"
import { AccessSection } from "@/features/settings/inboxes/AccessSection"
import { AdvancedPage } from "@/features/settings/inboxes/AdvancedPage"
import { GeneralPage } from "@/features/settings/inboxes/GeneralPage"
import { HoursForm } from "@/features/settings/inboxes/HoursForm"
import { InboxesPage } from "@/features/settings/inboxes/InboxesPage"
import { InboxLayout } from "@/features/settings/inboxes/InboxLayout"
import { InboxWebhooksPage } from "@/features/settings/inboxes/InboxWebhooksPage"
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
      <Route element={<Gate />}>
        <Route index element={<Navigate to="/all" replace />} />
        <Route path="settings" element={<SettingsLayout />}>
          <Route index element={<SettingsIndex />} />
          <Route path="profile" element={<ProfilePage />} />
          <Route path="notifications" element={<NotificationsPage />} />
          <Route path="workspace" element={<WorkspacePage />} />
          <Route path="members" element={<MembersPage />} />
          <Route path="inboxes" element={<InboxesPage />} />
          <Route path="inboxes/:inboxId" element={<InboxLayout />}>
            <Route index element={<GeneralPage />} />
            <Route path="hours" element={<HoursForm />} />
            <Route path="channels" element={<ChannelsPage />} />
            <Route path="access" element={<AccessSection />} />
            <Route path="webhooks" element={<InboxWebhooksPage />} />
            <Route path="advanced" element={<AdvancedPage />} />
          </Route>
          <Route path="labels" element={<LabelsPage />} />
          <Route path="canned-replies" element={<CannedRepliesPage />} />
          <Route path="api-keys" element={<ApiKeysPage />} />
          <Route path="webhooks" element={<WebhooksPage />} />
          <Route path="webhooks/:webhookId" element={<WebhookPage />} />
        </Route>
        <Route path="conversations/:conversationId" element={<OpenConversation />} />
        <Route path="inbox/:inboxId/:conversationId?" element={<InboxPage />} />
        <Route path="label/:labelId/:conversationId?" element={<InboxPage />} />
        <Route path="feedback" element={<Navigate to="/feedback/all" replace />} />
        <Route path="feedback/:category/:conversationId?" element={<InboxPage />} />
        <Route path=":view/:conversationId?" element={<InboxPage />} />
      </Route>
    </Routes>
  )
}
