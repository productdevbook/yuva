import "@/features/analytics/i18n"

import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery } from "@tanstack/react-query"
import { ClockIcon, LinkIcon, LockIcon } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { Navigate, Outlet, Route, Routes, useLocation, useNavigate, useParams } from "react-router"

import { EmptyState, ErrorLine, Segmented, YuvaMark } from "@/components/common"
import { LanguageMenu } from "@/components/common/LanguageMenu"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { BreakdownView } from "@/features/analytics/Breakdown"
import { RangePicker } from "@/features/analytics/Controls"
import { EventsView } from "@/features/analytics/Events"
import { GoalsView } from "@/features/analytics/Goals"
import { Overview } from "@/features/analytics/Overview"
import { ShareContext, shareHeaders, type ShareScope } from "@/features/analytics/queries"
import { Realtime } from "@/features/analytics/Realtime"
import { formatInZone, SHARE_VIEWS, useShareViewNames } from "@/features/analytics/Sharing"
import { useViewSearch } from "@/features/analytics/state"
import { api, ApiError, unwrap, type AnalyticsProperty, type AnalyticsShareView } from "@/lib/api"

function asProperty(token: string, p: ShareScope["property"]): AnalyticsProperty {
  return {
    id: token,
    name: p.name,
    public_key: "",
    domains: [],
    platforms: p.platforms,
    timezone: p.timezone,
    currency: p.currency,
    collection: "anonymous",
    consent_features: [],
    audience: "general",
    excluded_paths: [],
    excluded_ips: [],
    path_rules: [],
    tracked_props: [],
    visitors_today: 0,
    created_at: "",
    updated_at: "",
  }
}

function useForcedTheme() {
  const { search } = useLocation()
  const theme = new URLSearchParams(search).get("theme")
  useEffect(() => {
    if (theme !== "light" && theme !== "dark") return
    document.documentElement.classList.toggle("dark", theme === "dark")
  }, [theme])
}

function Frame({ children, footer }: { children: React.ReactNode; footer?: React.ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col bg-background" data-testid="share-page">
      {children}
      <footer className="mx-auto flex w-full max-w-5xl flex-wrap items-center gap-x-4 gap-y-1 px-4 pt-2 pb-6 text-caption text-faint sm:px-6">
        <span className="inline-flex items-center gap-1.5" data-testid="share-footer">
          <YuvaMark className="size-4" />
          <Trans>Shared with Yuva</Trans>
        </span>
        {footer}
      </footer>
    </div>
  )
}

function Message({
  icon,
  title,
  children,
}: {
  icon: typeof LinkIcon
  title: React.ReactNode
  children?: React.ReactNode
}) {
  return (
    <Frame>
      <div className="flex justify-end px-4 pt-4 sm:px-6">
        <LanguageMenu />
      </div>
      <EmptyState icon={icon} title={title}>
        {children}
      </EmptyState>
    </Frame>
  )
}

function Unlock({ token, again, onUnlocked }: { token: string; again: boolean; onUnlocked: (access: string) => void }) {
  const { t } = useLingui()
  const [password, setPassword] = useState("")
  const unlock = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/client/v1/analytics-shares/{token}/access", {
          params: { path: { token } },
          body: { password },
        })
      ),
    onSuccess: (r) => onUnlocked(r.access_token),
  })
  const err = unlock.error instanceof ApiError ? unlock.error : null
  return (
    <Frame>
      <div className="flex justify-end px-4 pt-4 sm:px-6">
        <LanguageMenu />
      </div>
      <main className="flex flex-1 items-start justify-center px-4 pt-[10vh] pb-10">
        <form
          className="flex w-full max-w-[22rem] flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (password && !unlock.isPending) unlock.mutate()
          }}
          data-testid="share-unlock"
        >
          <span className="grid size-11 place-items-center rounded-full border text-faint">
            <LockIcon className="size-5" />
          </span>
          <h1 className="text-page">
            <Trans>This report has a password</Trans>
          </h1>
          <p className="text-body text-muted-foreground">
            {again ? (
              <Trans>Your access ended. Enter the password again to keep reading.</Trans>
            ) : (
              <Trans>Enter the password you were given with the link.</Trans>
            )}
          </p>
          <Input
            type="password"
            autoComplete="current-password"
            autoFocus
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-label={t`Password`}
            placeholder={t`Password`}
            aria-invalid={err?.code === "wrong_password" || undefined}
            data-testid="share-unlock-password"
          />
          {err?.code === "wrong_password" ? (
            <p role="alert" className="text-body text-destructive">
              <Trans>That password is not right.</Trans>
            </p>
          ) : err?.status === 429 ? (
            <p role="alert" className="text-body text-destructive">
              <Trans>Too many tries. Wait a minute and try again.</Trans>
            </p>
          ) : (
            <ErrorLine error={unlock.error} />
          )}
          <Button type="submit" disabled={!password || unlock.isPending} data-testid="share-unlock-submit">
            <Trans>Open report</Trans>
          </Button>
        </form>
      </main>
    </Frame>
  )
}

function SharedLayout({ scope, expiresAt }: { scope: ShareScope; expiresAt?: string }) {
  const { t, i18n } = useLingui()
  const names = useShareViewNames()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const search = useViewSearch()
  const property = useMemo(() => asProperty(scope.token, scope.property), [scope.token, scope.property])
  const views = SHARE_VIEWS.filter((v) => scope.views.includes(v))
  const last = pathname.split("/")[3] as AnalyticsShareView | undefined
  const current = last && views.includes(last) ? last : (views[0] ?? "overview")
  const expires = expiresAt ? formatInZone(expiresAt, i18n.locale, scope.property.timezone) : ""
  useEffect(() => {
    const before = document.title
    document.title = property.name
    return () => {
      document.title = before
    }
  }, [property.name])
  return (
    <Frame
      footer={
        expires && (
          <span className="inline-flex items-center gap-1.5" data-testid="share-expires">
            <ClockIcon className="size-3.5" />
            <Trans>Available until {expires}</Trans>
          </span>
        )
      }
    >
      <header className="mx-auto flex w-full max-w-5xl items-start justify-between gap-3 px-4 pt-6 sm:px-6">
        <div className="min-w-0">
          <h1 className="text-page break-words" data-testid="share-property-name">
            {property.name}
          </h1>
          <p className="mt-1 text-body text-muted-foreground" title={t`Days are counted in this time zone`}>
            {property.timezone}
          </p>
        </div>
        <LanguageMenu />
      </header>
      <main className="@container mx-auto flex w-full max-w-5xl flex-1 flex-col gap-5 px-4 py-5 sm:px-6">
        {views.length > 1 && (
          <div className="-mx-1 max-w-[calc(100%+0.5rem)] overflow-x-auto px-1 py-1">
            <Segmented
              value={current}
              onChange={(v) => navigate(`/share/${scope.token}${v === views[0] ? "" : `/${v}`}${search}`)}
              label={t`Views`}
              items={views.map((v) => ({ value: v, label: names[v], testId: `share-tab-${v}` }))}
            />
          </div>
        )}
        {current !== "realtime" && (
          <div className="flex flex-wrap items-center gap-2">
            <RangePicker />
          </div>
        )}
        <Outlet context={{ property }} />
      </main>
    </Frame>
  )
}

function viewElement(v: AnalyticsShareView) {
  switch (v) {
    case "overview":
      return <Overview />
    case "realtime":
      return <Realtime />
    case "events":
      return <EventsView />
    case "goals":
      return <GoalsView />
    default:
      return <BreakdownView group={v} />
  }
}

export default function SharePage() {
  const { token = "" } = useParams()
  const [access, setAccess] = useState<string>()
  const [again, setAgain] = useState(false)
  useForcedTheme()
  const page = useQuery({
    queryKey: ["share", token, access ?? "", "page"],
    queryFn: () =>
      unwrap(
        api.GET("/client/v1/analytics-shares/{token}", {
          params: { path: { token } },
          headers: shareHeaders(access),
        })
      ),
    staleTime: 5 * 60_000,
    retry: false,
  })
  const data = page.data
  const scope = useMemo<ShareScope | null>(
    () =>
      data?.property && (!data.password_required || access)
        ? {
            token,
            access,
            views: data.views ?? [],
            property: data.property,
            goals: data.goals ?? [],
            geo: data.geoip,
            locked: () => {
              setAgain(true)
              setAccess(undefined)
            },
          }
        : null,
    [data, token, access]
  )
  if (page.isPending) {
    return (
      <Frame>
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 pt-6 sm:px-6">
          <Skeleton className="h-8 w-56" />
          <Skeleton className="h-64 rounded-2xl" />
        </div>
      </Frame>
    )
  }
  if (page.error) {
    const err = page.error instanceof ApiError ? page.error : null
    if (err?.status === 404) {
      return (
        <Message icon={LinkIcon} title={<Trans>This link is not available</Trans>}>
          <Trans>It may have expired or been revoked. Ask the person who shared it for a new one.</Trans>
        </Message>
      )
    }
    return (
      <Message icon={LinkIcon} title={<Trans>The report could not be loaded</Trans>}>
        {err?.status === 429 ? (
          <Trans>Too many requests from your network. Wait a minute and reload the page.</Trans>
        ) : (
          <ErrorLine error={page.error} />
        )}
        <Button className="mt-3" variant="outline" onClick={() => void page.refetch()}>
          <Trans>Try again</Trans>
        </Button>
      </Message>
    )
  }
  if (!scope) {
    return (
      <Unlock
        token={token}
        again={again}
        onUnlocked={(a) => {
          setAgain(false)
          setAccess(a)
        }}
      />
    )
  }
  const views = SHARE_VIEWS.filter((v) => scope.views.includes(v))
  if (views.length === 0) {
    return <Message icon={LinkIcon} title={<Trans>This link is not available</Trans>} />
  }
  return (
    <ShareContext.Provider value={scope}>
      <Routes>
        <Route element={<SharedLayout scope={scope} expiresAt={data?.expires_at} />}>
          {views.map((v, i) =>
            i === 0 ? (
              <Route key={v} index element={viewElement(v)} />
            ) : (
              <Route key={v} path={v} element={viewElement(v)} />
            )
          )}
          <Route path="*" element={<Navigate to={`/share/${token}`} replace />} />
        </Route>
      </Routes>
    </ShareContext.Provider>
  )
}
