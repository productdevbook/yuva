import { I18nProvider } from "@lingui/react"
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router"

import "./index.css"
import { AppRoutes } from "@/app/routes"
import { SsoRequiredDialog } from "@/app/SsoRequiredDialog"
import { PageViews } from "@/components/common/PageViews"
import { ServiceWorkerBridge } from "@/components/common/ServiceWorkerBridge"
import { TooltipProvider } from "@/components/ui/tooltip"
import { activate, i18n, initialLocale, setLocaleCookieDomain } from "@/i18n"
import { ApiError, versionQuery } from "@/lib/api"
import { startPwa } from "@/lib/pwa"
import { noteSsoRequired } from "@/lib/sso"
import { startTheme } from "@/lib/theme"
import { meKey } from "@/lib/session"

await activate(initialLocale())
startTheme()
startPwa()

const queryClient: QueryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (err, query) => {
      noteSsoRequired(err)
      if (err instanceof ApiError && err.status === 401 && query.queryKey[0] !== meKey[0]) {
        queryClient.setQueryData(meKey, null)
      }
    },
  }),
  mutationCache: new MutationCache({ onError: noteSsoRequired }),
  defaultOptions: { queries: { retry: (count, err) => !(err instanceof ApiError) && count < 2 } },
})

queryClient
  .fetchQuery(versionQuery)
  .then((v) => setLocaleCookieDomain(v.locale_cookie_domain))
  .catch(() => setLocaleCookieDomain(undefined))

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider delay={300}>
          <BrowserRouter>
            <ServiceWorkerBridge />
            <PageViews />
            <AppRoutes />
            <SsoRequiredDialog />
          </BrowserRouter>
        </TooltipProvider>
      </QueryClientProvider>
    </I18nProvider>
  </StrictMode>
)
