import { I18nProvider } from "@lingui/react"
import { QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router"

import "./index.css"
import { AppRoutes } from "@/app/routes"
import { ServiceWorkerBridge } from "@/components/common/ServiceWorkerBridge"
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
        <TooltipProvider delay={300}>
          <BrowserRouter>
            <ServiceWorkerBridge />
            <AppRoutes />
          </BrowserRouter>
        </TooltipProvider>
      </QueryClientProvider>
    </I18nProvider>
  </StrictMode>,
)
