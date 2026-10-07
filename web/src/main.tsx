import { I18nProvider } from "@lingui/react"
import { QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter, Navigate, Route, Routes } from "react-router"

import "./index.css"
import { Gate } from "@/components/AppShell"
import { SignInPage } from "@/components/auth/SignInPage"
import { ServiceWorkerBridge } from "@/components/common/ServiceWorkerBridge"
import { InboxPage, OpenConversation } from "@/components/inbox/InboxPage"
import { ApiKeysSettings } from "@/components/settings/ApiKeysSettings"
import { CannedRepliesSettings } from "@/components/settings/CannedRepliesSettings"
import { InboxSettings } from "@/components/settings/InboxSettings"
import { InboxesSettings } from "@/components/settings/InboxesSettings"
import { LabelsSettings } from "@/components/settings/LabelsSettings"
import { MembersSettings } from "@/components/settings/MembersSettings"
import { NotificationsSettings } from "@/components/settings/NotificationsSettings"
import { ProfileSettings } from "@/components/settings/ProfileSettings"
import { SettingsLayout } from "@/components/settings/SettingsLayout"
import { WebhookSettings, WebhooksSettings } from "@/components/settings/WebhooksSettings"
import { TooltipProvider } from "@/components/ui/tooltip"
import { activate, i18n, initialLocale } from "@/i18n"
import { ApiError } from "@/lib/api"
import { startPwa } from "@/lib/pwa"
import { meKey } from "@/lib/session"

activate(initialLocale())
startPwa()

const queryClient: QueryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (err, query) => {
      if (err instanceof ApiError && err.status === 401 && query.queryKey[0] !== meKey[0]) {
        queryClient.setQueryData(meKey, null)
      }
    },
  }),
  defaultOptions: { queries: { retry: (count, err) => !(err instanceof ApiError) && count < 2 } },
})

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <BrowserRouter>
            <ServiceWorkerBridge />
            <Routes>
              <Route path="sign-in" element={<SignInPage />} />
              <Route element={<Gate />}>
                <Route index element={<Navigate to="/all" replace />} />
                <Route path="settings" element={<SettingsLayout />}>
                  <Route index element={<Navigate to="profile" replace />} />
                  <Route path="profile" element={<ProfileSettings />} />
                  <Route path="notifications" element={<NotificationsSettings />} />
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
          </BrowserRouter>
        </TooltipProvider>
      </QueryClientProvider>
    </I18nProvider>
  </StrictMode>,
)
