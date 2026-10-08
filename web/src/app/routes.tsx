import { Navigate, Route, Routes } from "react-router"

import { Gate } from "@/app/Gate"
import { SignInPage } from "@/features/auth/SignInPage"
import { InboxPage, OpenConversation } from "@/features/inbox/InboxPage"
import { ApiKeysSettings } from "@/features/settings/ApiKeysSettings"
import { CannedRepliesSettings } from "@/features/settings/CannedRepliesSettings"
import { InboxesSettings } from "@/features/settings/InboxesSettings"
import { InboxSettings } from "@/features/settings/InboxSettings"
import { LabelsSettings } from "@/features/settings/LabelsSettings"
import { MembersSettings } from "@/features/settings/MembersSettings"
import { NotificationsSettings } from "@/features/settings/NotificationsSettings"
import { ProfileSettings } from "@/features/settings/ProfileSettings"
import { SettingsLayout } from "@/features/settings/SettingsLayout"
import { WebhookSettings, WebhooksSettings } from "@/features/settings/WebhooksSettings"
import { WorkspaceSettings } from "@/features/settings/WorkspaceSettings"

export function AppRoutes() {
  return (
    <Routes>
      <Route path="sign-in" element={<SignInPage />} />
      <Route element={<Gate />}>
        <Route index element={<Navigate to="/all" replace />} />
        <Route path="settings" element={<SettingsLayout />}>
          <Route index element={<Navigate to="profile" replace />} />
          <Route path="profile" element={<ProfileSettings />} />
          <Route path="notifications" element={<NotificationsSettings />} />
          <Route path="workspace" element={<WorkspaceSettings />} />
          <Route path="members" element={<MembersSettings />} />
          <Route path="inboxes" element={<InboxesSettings />} />
          <Route path="inboxes/:inboxId" element={<InboxSettings />} />
          <Route path="labels" element={<LabelsSettings />} />
          <Route path="canned-replies" element={<CannedRepliesSettings />} />
          <Route path="api-keys" element={<ApiKeysSettings />} />
          <Route path="webhooks" element={<WebhooksSettings />} />
          <Route path="webhooks/:webhookId" element={<WebhookSettings />} />
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
