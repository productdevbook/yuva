import { Navigate, Route, Routes } from "react-router"

import { Gate } from "@/app/Gate"
import { Layout } from "@/app/Layout"
import { SignInPage } from "@/features/auth/SignInPage"
import { ContactPage } from "@/features/contact/ContactPage"
import { ContactsHome } from "@/features/contact/ContactsColumn"
import { ChatsPage } from "@/features/inbox/ChatsPage"
import { ConsentPage } from "@/features/oauth/ConsentPage"
import { ApiKeysPage } from "@/features/settings/api-keys/ApiKeysPage"
import { AppearancePage } from "@/features/settings/appearance/AppearancePage"
import { CannedRepliesPage } from "@/features/settings/canned/CannedRepliesPage"
import { AssistantPage } from "@/features/settings/connected-apps/AssistantPage"
import { ConnectedAppsPage } from "@/features/settings/connected-apps/ConnectedAppsPage"
import { InboxPage } from "@/features/settings/inboxes/InboxPage"
import { InboxWebhooksPage } from "@/features/settings/inboxes/InboxWebhooksPage"
import { LabelsPage } from "@/features/settings/labels/LabelsPage"
import { MembersPage } from "@/features/settings/members/MembersPage"
import { NotificationsPage } from "@/features/settings/notifications/NotificationsPage"
import { ProfilePage } from "@/features/settings/profile/ProfilePage"
import { MembersReport, RatingsReport, ReportsPane, TodayReport } from "@/features/reports/ReportPages"
import { SettingsHome, SettingsPane } from "@/features/settings/SettingsColumn"
import { WebhookPage } from "@/features/settings/webhooks/WebhookPage"
import { WebhooksPage } from "@/features/settings/webhooks/WebhooksPage"
import { WorkspacePage } from "@/features/settings/workspace/WorkspacePage"
import { SetupPage } from "@/features/setup/SetupPage"
import { MemberPage } from "@/features/team/MemberPage"
import { TeamHome } from "@/features/team/TeamColumn"

export function AppRoutes() {
  return (
    <Routes>
      <Route path="sign-in" element={<SignInPage />} />
      <Route path="oauth/consent" element={<ConsentPage />} />
      <Route element={<Gate />}>
        <Route path="setup" element={<SetupPage />} />
        <Route path="setup/:inboxId" element={<SetupPage />} />
        <Route path="setup/:inboxId/add/:kind" element={<SetupPage />} />
        <Route path="setup/:inboxId/:channelId" element={<SetupPage />} />
        <Route path="setup/:inboxId/:channelId/wait" element={<SetupPage wait />} />
        <Route element={<Layout />}>
          <Route index element={<ChatsPage />} />
          <Route path="conversations/:conversationId" element={<ChatsPage />} />
          <Route path="mentions" element={<ChatsPage />} />
          <Route path="contacts" element={<ContactsHome />} />
          <Route path="contacts/:contactId" element={<ContactPage />} />
          <Route path="team" element={<TeamHome />} />
          <Route path="team/:memberId" element={<MemberPage />} />
          <Route path="reports" element={<ReportsPane />}>
            <Route index element={<Navigate to="today" replace />} />
            <Route path="today" element={<TodayReport />} />
            <Route path="ratings" element={<RatingsReport />} />
            <Route path="members" element={<MembersReport />} />
            <Route path="*" element={<Navigate to="/reports" replace />} />
          </Route>
          <Route path="settings">
            <Route index element={<SettingsHome />} />
            <Route element={<SettingsPane />}>
              <Route path="profile" element={<ProfilePage />} />
              <Route path="notifications" element={<NotificationsPage />} />
              <Route path="appearance" element={<AppearancePage />} />
              <Route path="workspace" element={<WorkspacePage />} />
              <Route path="members" element={<MembersPage />} />
              <Route path="inboxes/:inboxId" element={<InboxPage />} />
              <Route path="inboxes/:inboxId/webhooks" element={<InboxWebhooksPage />} />
              <Route path="labels" element={<LabelsPage />} />
              <Route path="canned-replies" element={<CannedRepliesPage />} />
              <Route path="api-keys" element={<ApiKeysPage />} />
              <Route path="connected-apps" element={<ConnectedAppsPage />} />
              <Route path="connected-apps/:assistant" element={<AssistantPage />} />
              <Route path="webhooks" element={<WebhooksPage />} />
              <Route path="webhooks/:webhookId" element={<WebhookPage />} />
            </Route>
            <Route path="new/*" element={<Navigate to="/setup" replace />} />
            <Route path="*" element={<Navigate to="/settings" replace />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Route>
    </Routes>
  )
}
