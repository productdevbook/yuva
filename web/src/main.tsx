import { I18nProvider } from "@lingui/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter, Navigate, Route, Routes } from "react-router"

import "./index.css"
import { AppShell, ConversationsPage } from "@/components/AppShell"
import { TooltipProvider } from "@/components/ui/tooltip"
import { activate, i18n, initialLocale } from "@/i18n"
import { ApiError } from "@/lib/api"

activate(initialLocale())

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: (count, err) => !(err instanceof ApiError) && count < 2 } },
})

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <BrowserRouter>
            <Routes>
              <Route element={<AppShell />}>
                <Route index element={<Navigate to="/all" replace />} />
                <Route path=":view" element={<ConversationsPage />} />
              </Route>
            </Routes>
          </BrowserRouter>
        </TooltipProvider>
      </QueryClientProvider>
    </I18nProvider>
  </StrictMode>,
)
