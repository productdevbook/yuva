import { Trans, useLingui } from "@lingui/react/macro"
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  ArrowLeftIcon,
  ChevronRightIcon,
  CircleAlertIcon,
  ClockIcon,
  InfoIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  RotateCwIcon,
  SendIcon,
  Trash2Icon,
  TriangleAlertIcon,
  WebhookIcon,
} from "lucide-react"
import { useState } from "react"
import { Link, Navigate, useLocation, useNavigate, useParams } from "react-router"

import { CopyButton, EmptyState, ErrorLine, SecretDialog, useConfirm } from "@/components/common"
import {
  DELIVERY_STATES,
  formatDateTime,
  formatRelative,
  useEnumText,
  WEBHOOK_EVENTS,
} from "@/components/common/text"
import { Field, PageTitle, Section } from "@/components/settings/SettingsLayout"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  api,
  ApiError,
  unwrap,
  type WebhookAttempt,
  type WebhookDeliveryState,
  type WebhookEndpoint,
  type WebhookEventType,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useInboxes } from "@/lib/queries"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const STANDARD_WEBHOOKS = "https://www.standardwebhooks.com"

function useWebhooks(inboxId?: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.webhookList(ws, inboxId ?? ""),
    queryFn: () =>
      unwrap(api.GET("/v1/webhooks", { params: { query: inboxId ? { inbox_id: inboxId } : {} } })).then((r) => r.items),
  })
}

function useWebhook(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.webhook(ws, id),
    queryFn: () => unwrap(api.GET("/v1/webhooks/{webhookId}", { params: { path: { webhookId: id } } })),
  })
}

function useSetEndpoint() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return (e: WebhookEndpoint) => {
    qc.setQueryData(keys.webhook(ws, e.id), e)
    void qc.invalidateQueries({ queryKey: ["ws", ws, "webhooks", "list"] })
  }
}

export function WebhooksSettings() {
  const { canManage } = useSession()
  if (!canManage) return <Navigate to="/settings/profile" replace />
  return (
    <>
      <PageTitle>
        <Trans>Webhooks</Trans>
      </PageTitle>
      <WebhooksSection />
    </>
  )
}

export function WebhooksSection({ inboxId }: { inboxId?: string }) {
  const { canManage } = useSession()
  const inboxes = useInboxes().data ?? []
  const list = useWebhooks(inboxId)
  const [creating, setCreating] = useState(false)
  const navigate = useNavigate()
  if (!canManage) return null
  return (
    <Section
      title={inboxId ? <Trans>Webhooks</Trans> : <Trans>Endpoints</Trans>}
      description={
        inboxId ? (
          <Trans>Signed HTTP calls to your backend for this inbox's conversations, messages and feedback.</Trans>
        ) : (
          <Trans>
            Signed HTTP calls to your backend when conversations, messages, feedback or contacts change. Endpoints of
            the workspace receive every inbox; inbox endpoints only their inbox.
          </Trans>
        )
      }
      action={
        <Button variant="outline" size="sm" onClick={() => setCreating(true)} data-testid="webhook-add">
          <PlusIcon />
          <Trans>Add endpoint</Trans>
        </Button>
      }
    >
      {list.isPending ? (
        <Skeleton className="h-14 w-full" />
      ) : list.data && list.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Trans>No endpoints yet.</Trans>
        </p>
      ) : (
        <ul className="flex flex-col divide-y rounded-lg border">
          {(list.data ?? []).map((e) => {
            const inbox = e.inbox_id ? inboxes.find((i) => i.id === e.inbox_id) : undefined
            const count = e.events.length
            return (
              <li key={e.id}>
                <Link
                  to={`/settings/webhooks/${e.id}`}
                  className="flex items-center gap-3 px-3 py-2.5 transition-colors hover:bg-muted/60"
                  data-testid="webhook-row"
                >
                  <WebhookIcon className="size-4 shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-xs font-medium">{e.url}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {e.description && <>{e.description} · </>}
                      {!inboxId && <>{inbox ? inbox.name : <Trans>All inboxes</Trans>}{" · "}</>}
                      {count === 1 ? <Trans>1 event</Trans> : <Trans>{count} events</Trans>}
                    </p>
                  </div>
                  <EndpointStatus e={e} />
                  <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                </Link>
              </li>
            )
          })}
        </ul>
      )}
      <ErrorLine error={list.error} />
      <SignatureHint />
      {creating && (
        <WebhookDialog
          endpoint={null}
          inboxId={inboxId}
          onClose={() => setCreating(false)}
          onCreated={(e, secret) => {
            setCreating(false)
            navigate(`/settings/webhooks/${e.id}`, { state: { secret } })
          }}
        />
      )}
    </Section>
  )
}

function EndpointStatus({ e }: { e: WebhookEndpoint }) {
  if (!e.enabled && e.disabled_reason) {
    return (
      <Badge variant="destructive" data-testid="webhook-status" data-status="disabled">
        <CircleAlertIcon />
        <Trans>Turned off by Yuva</Trans>
      </Badge>
    )
  }
  if (!e.enabled) {
    return (
      <Badge variant="outline" data-testid="webhook-status" data-status="off">
        <Trans>Off</Trans>
      </Badge>
    )
  }
  if (e.failing_since) {
    return (
      <Badge variant="outline" className="border-amber-500/40 text-amber-700 dark:text-amber-400" data-testid="webhook-status" data-status="failing">
        <TriangleAlertIcon />
        <Trans>Failing</Trans>
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="border-success/40 text-success" data-testid="webhook-status" data-status="on">
      <Trans>On</Trans>
    </Badge>
  )
}

function SignatureHint() {
  return (
    <div className="flex gap-2 rounded-lg border bg-muted/40 p-3 text-xs text-muted-foreground" data-testid="signature-hint">
      <InfoIcon className="mt-px size-4 shrink-0" />
      <p>
        <Trans>
          Verify every request before trusting it: the <code className="font-mono">webhook-signature</code> header
          follows the{" "}
          <a href={STANDARD_WEBHOOKS} target="_blank" rel="noreferrer" className="font-medium text-foreground underline underline-offset-2">
            Standard Webhooks
          </a>{" "}
          specification, so any of its libraries can check it with the endpoint's secret. Go backends can use the{" "}
          <code className="font-mono">sdk/go/webhook</code> package of the Yuva repository.
        </Trans>
      </p>
    </div>
  )
}

type UrlProblem = "private" | "invalid" | null

function urlProblem(err: unknown): UrlProblem {
  if (!(err instanceof ApiError) || err.status !== 400 || !err.detail) return null
  if (/private|loopback|link-local/.test(err.detail)) return "private"
  if (/\burl\b/.test(err.detail)) return "invalid"
  return null
}

function WebhookDialog({
  endpoint,
  inboxId,
  onClose,
  onCreated,
}: {
  endpoint: WebhookEndpoint | null
  inboxId?: string
  onClose: () => void
  onCreated?: (e: WebhookEndpoint, secret: string) => void
}) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const setEndpoint = useSetEndpoint()
  const [url, setUrl] = useState(endpoint?.url ?? "")
  const [description, setDescription] = useState(endpoint?.description ?? "")
  const [scope, setScope] = useState(endpoint?.inbox_id ?? inboxId ?? "")
  const [events, setEvents] = useState<Set<WebhookEventType>>(
    () => new Set(endpoint?.events ?? ["conversation.created", "message.created", "feedback.created"]),
  )
  const [includeNotes, setIncludeNotes] = useState(endpoint?.include_notes ?? false)
  const [touched, setTouched] = useState(false)
  const chosen = WEBHOOK_EVENTS.filter((e) => events.has(e))
  const save = useMutation({
    mutationFn: (): Promise<{ endpoint: WebhookEndpoint; secret: string | null }> => {
      const body = {
        url: url.trim(),
        description: description.trim(),
        events: chosen,
        include_notes: includeNotes,
      }
      if (endpoint) {
        return unwrap(
          api.PATCH("/v1/webhooks/{webhookId}", { params: { path: { webhookId: endpoint.id } }, body }),
        ).then((e) => ({ endpoint: e, secret: null }))
      }
      return unwrap(
        api.POST("/v1/webhooks", { body: { ...body, enabled: true, ...(scope ? { inbox_id: scope } : {}) } }),
      )
    },
    onSuccess: (r) => {
      setEndpoint(r.endpoint)
      void qc.invalidateQueries({ queryKey: keys.webhooks(ws) })
      if (r.secret) onCreated?.(r.endpoint, r.secret)
      else onClose()
    },
  })
  const bad = urlProblem(save.error)
  const scopes: Record<string, string> = { "": t`All inboxes` }
  for (const i of inboxes) scopes[i.id] = i.name
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
        <form
          className="flex min-w-0 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            setTouched(true)
            if (chosen.length > 0) save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>{endpoint ? <Trans>Edit endpoint</Trans> : <Trans>Add endpoint</Trans>}</DialogTitle>
            <DialogDescription>
              <Trans>Yuva sends a signed POST request with a JSON body for every event you pick.</Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>URL</Trans>} htmlFor="webhook-url">
            <Input
              id="webhook-url"
              type="url"
              required
              maxLength={2000}
              value={url}
              placeholder="https://api.example.com/yuva/webhook"
              onChange={(e) => setUrl(e.target.value)}
              aria-invalid={!!bad || undefined}
              aria-describedby={bad ? "err-webhook-url" : undefined}
              className="font-mono text-xs"
            />
            {bad && (
              <p id="err-webhook-url" role="alert" className="text-xs text-destructive">
                {bad === "private" ? (
                  <Trans>This address is private, loopback or link-local. Webhooks only reach public servers.</Trans>
                ) : (
                  <Trans>Enter an http or https URL without a user name or password.</Trans>
                )}
              </p>
            )}
          </Field>
          <Field label={<Trans>Description (optional)</Trans>} htmlFor="webhook-description">
            <Input
              id="webhook-description"
              maxLength={200}
              value={description}
              placeholder={t`Push notifications`}
              onChange={(e) => setDescription(e.target.value)}
            />
          </Field>
          {!inboxId && (
            <Field
              label={<Trans>Inbox</Trans>}
              hint={endpoint ? <Trans>The inbox cannot be changed later.</Trans> : undefined}
            >
              <Select value={scope} onValueChange={(v) => setScope(String(v ?? ""))} items={scopes} disabled={!!endpoint}>
                <SelectTrigger className="w-full" aria-label={t`Inbox`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(scopes).map(([id, name]) => (
                    <SelectItem key={id} value={id}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          )}
          <fieldset className="flex min-w-0 flex-col gap-2" aria-describedby={touched && chosen.length === 0 ? "err-webhook-events" : undefined}>
            <legend className="mb-2 text-sm font-medium">
              <Trans>Events</Trans>
            </legend>
            <ul className="flex flex-col divide-y rounded-lg border">
              {WEBHOOK_EVENTS.map((ev) => (
                <li key={ev}>
                  <label className="flex items-center gap-3 px-3 py-2">
                    <Checkbox
                      checked={events.has(ev)}
                      onCheckedChange={(on) =>
                        setEvents((s) => {
                          const next = new Set(s)
                          if (on) next.add(ev)
                          else next.delete(ev)
                          return next
                        })
                      }
                      aria-label={ev}
                    />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="text-sm">{text.event[ev]}</span>
                      <code className="font-mono text-xs text-muted-foreground">{ev}</code>
                    </span>
                  </label>
                </li>
              ))}
            </ul>
            {touched && chosen.length === 0 && (
              <p id="err-webhook-events" role="alert" className="text-xs text-destructive">
                <Trans>Pick at least one event.</Trans>
              </p>
            )}
          </fieldset>
          <label className="flex items-start justify-between gap-3 text-sm">
            <span className="flex flex-col gap-0.5">
              <Trans>Include notes</Trans>
              <span className="text-xs text-muted-foreground">
                <Trans>Also send message.created for members' internal notes. Internal events are never sent.</Trans>
              </span>
            </span>
            <Switch checked={includeNotes} onCheckedChange={setIncludeNotes} aria-label={t`Include notes`} />
          </label>
          {!bad && <ErrorLine error={save.error} />}
          <DialogFooter className="sticky -bottom-6 z-10 -mx-6 -mb-6 border-t bg-popover px-6 py-3">
            <Button type="submit" disabled={save.isPending}>
              {endpoint ? <Trans>Save</Trans> : <Trans>Add endpoint</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function WebhookSettings() {
  const { t, i18n } = useLingui()
  const { webhookId = "" } = useParams()
  const { canManage, workspaceId: ws } = useSession()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const endpoint = useWebhook(webhookId)
  const setEndpoint = useSetEndpoint()
  const location = useLocation()
  const created = (location.state as { secret?: string } | null)?.secret ?? null
  const [editing, setEditing] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const toggle = useMutation({
    mutationFn: (enabled: boolean) =>
      unwrap(api.PATCH("/v1/webhooks/{webhookId}", { params: { path: { webhookId } }, body: { enabled } })),
    onSuccess: setEndpoint,
  })
  const rotate = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/webhooks/{webhookId}/secret", { params: { path: { webhookId } } })),
    onSuccess: (r) => {
      setEndpoint(r.endpoint)
      setSecret(r.secret)
    },
  })
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/webhooks/{webhookId}", { params: { path: { webhookId } } })),
    onSuccess: () => {
      qc.removeQueries({ queryKey: keys.webhook(ws, webhookId) })
      void qc.invalidateQueries({ queryKey: keys.webhooks(ws) })
      navigate("/settings/webhooks")
    },
  })
  if (!canManage) return <Navigate to="/settings/profile" replace />
  const e = endpoint.data
  const inbox = e?.inbox_id ? inboxes.find((i) => i.id === e.inbox_id) : undefined
  const disabledAt = e?.disabled_at ? formatDateTime(e.disabled_at, i18n.locale) : null
  const failingSince = e?.failing_since ? formatDateTime(e.failing_since, i18n.locale) : ""
  const rotatedUntil = e?.secret_rotated_at
    ? formatDateTime(new Date(new Date(e.secret_rotated_at).getTime() + 24 * 3600_000).toISOString(), i18n.locale)
    : null
  return (
    <>
      <div className="flex min-w-0 items-center gap-2">
        <Button variant="ghost" size="icon-sm" render={<Link to="/settings/webhooks" />} aria-label={t`Back`}>
          <ArrowLeftIcon />
        </Button>
        <PageTitle>
          <Trans>Webhook endpoint</Trans>
        </PageTitle>
      </div>
      {endpoint.isPending ? (
        <Skeleton className="h-48 w-full" />
      ) : !e ? (
        <ErrorLine error={endpoint.error} />
      ) : (
        <>
          <Section
            title={<span className="block font-mono text-sm break-all" data-testid="webhook-url">{e.url}</span>}
            description={
              <>
                {e.description && <>{e.description} · </>}
                {inbox ? inbox.name : <Trans>All inboxes</Trans>}
              </>
            }
            action={<EndpointStatus e={e} />}
          >
            {!e.enabled && e.disabled_reason && (
              <div
                role="status"
                className="flex gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm"
                data-testid="webhook-disabled"
              >
                <CircleAlertIcon className="mt-0.5 size-4 shrink-0 text-destructive" />
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="font-medium">
                    {disabledAt ? (
                      <Trans>Yuva turned this endpoint off on {disabledAt}.</Trans>
                    ) : (
                      <Trans>Yuva turned this endpoint off.</Trans>
                    )}
                  </p>
                  <p className="break-words text-muted-foreground">{e.disabled_reason}</p>
                  <p className="text-muted-foreground">
                    <Trans>Turn it on again once the receiver works; send failed deliveries again from the log below.</Trans>
                  </p>
                </div>
              </div>
            )}
            {e.enabled && e.failing_since && (
              <div
                role="status"
                className="flex gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm"
                data-testid="webhook-failing"
              >
                <TriangleAlertIcon className="mt-0.5 size-4 shrink-0 text-amber-600" />
                <p>
                  <Trans>
                    Every attempt has failed since {failingSince}. After 24 hours of
                    failures Yuva turns the endpoint off.
                  </Trans>
                </p>
              </div>
            )}
            <label className="flex items-start justify-between gap-3 text-sm">
              <span className="flex flex-col gap-0.5">
                <Trans>Send events</Trans>
                <span className="text-xs text-muted-foreground">
                  <Trans>While off, no deliveries are made.</Trans>
                </span>
              </span>
              <Switch
                checked={e.enabled}
                disabled={toggle.isPending}
                onCheckedChange={(on) => toggle.mutate(on)}
                aria-label={t`Send events`}
                data-testid="webhook-enabled"
              />
            </label>
            <div className="flex flex-col gap-1.5">
              <p className="text-sm font-medium">
                <Trans>Events</Trans>
              </p>
              <div className="flex flex-wrap gap-1.5">
                {e.events.map((ev) => (
                  <Badge key={ev} variant="secondary" className="font-mono" title={text.event[ev]}>
                    {ev}
                  </Badge>
                ))}
                {e.include_notes && (
                  <Badge variant="outline">
                    <Trans>with notes</Trans>
                  </Badge>
                )}
              </div>
            </div>
            {rotatedUntil && (
              <p className="flex items-start gap-1.5 text-xs text-muted-foreground" data-testid="webhook-rotated">
                <ClockIcon className="mt-px size-3.5 shrink-0" />
                <Trans>The previous secret also signs until {rotatedUntil}.</Trans>
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
                <PencilIcon />
                <Trans>Edit</Trans>
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={rotate.isPending}
                data-testid="webhook-rotate"
                onClick={() =>
                  confirm({
                    title: <Trans>Rotate the signing secret?</Trans>,
                    description: (
                      <Trans>
                        You get a new secret once. For 24 hours every delivery carries two signatures, one with the new
                        and one with the old secret, so you can update your receiver without losing events.
                      </Trans>
                    ),
                    confirm: <Trans>Rotate</Trans>,
                    run: () => rotate.mutate(),
                  })
                }
              >
                <RefreshCwIcon />
                <Trans>Rotate secret</Trans>
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive"
                disabled={remove.isPending}
                onClick={() =>
                  confirm({
                    title: <Trans>Remove this endpoint?</Trans>,
                    description: <Trans>Pending deliveries are dropped with it.</Trans>,
                    confirm: <Trans>Remove</Trans>,
                    run: () => remove.mutate(),
                  })
                }
              >
                <Trash2Icon />
                <Trans>Remove</Trans>
              </Button>
            </div>
            <ErrorLine error={toggle.error ?? rotate.error ?? remove.error} />
          </Section>
          <DeliveriesSection endpoint={e} />
          <AttemptsSection endpoint={e} />
          <SignatureHint />
          {editing && <WebhookDialog endpoint={e} inboxId={e.inbox_id} onClose={() => setEditing(false)} />}
        </>
      )}
      <SecretDialog
        secret={created}
        title={<Trans>Signing secret</Trans>}
        description={<Trans>Copy it now and store it with your receiver; it is not shown again.</Trans>}
        onClose={() => navigate(location.pathname, { replace: true, state: null })}
      />
      <SecretDialog
        secret={secret}
        title={<Trans>New signing secret</Trans>}
        description={
          <Trans>Copy it now and update your receiver; the old secret keeps signing for 24 hours. It is not shown again.</Trans>
        }
        onClose={() => setSecret(null)}
      />
      {confirmDialog}
    </>
  )
}

function useRefreshLog(endpointId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => {
    void qc.invalidateQueries({ queryKey: ["ws", ws, "webhooks", "deliveries", endpointId] })
    void qc.invalidateQueries({ queryKey: keys.webhookAttempts(ws, endpointId) })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "webhooks", "delivery", endpointId] })
    void qc.invalidateQueries({ queryKey: keys.webhook(ws, endpointId) })
  }
}

function StateBadge({ state }: { state: WebhookDeliveryState }) {
  const text = useEnumText()
  return (
    <Badge
      variant={state === "failed" ? "destructive" : "outline"}
      className={cn(
        state === "succeeded" && "border-success/40 text-success",
        state === "pending" && "border-amber-500/40 text-amber-700 dark:text-amber-400",
      )}
      data-testid="delivery-state"
      data-state={state}
    >
      {text.delivery[state]}
    </Badge>
  )
}

function Attempts({ n }: { n: number }) {
  return n === 1 ? <Trans>1 attempt</Trans> : <Trans>{n} attempts</Trans>
}

function NextAttempt({ at }: { at: string }) {
  const { i18n } = useLingui()
  const when = formatRelative(at, i18n.locale)
  return (
    <span>
      <Trans>next attempt {when}</Trans>
    </span>
  )
}

function AttemptResult({ a }: { a: WebhookAttempt }) {
  return (
    <span className={cn("font-mono text-xs", a.success ? "text-success" : "text-destructive")}>
      {a.status_code ?? <Trans>no answer</Trans>}
    </span>
  )
}

function DeliveriesSection({ endpoint }: { endpoint: WebhookEndpoint }) {
  const { t, i18n } = useLingui()
  const { workspaceId: ws } = useSession()
  const text = useEnumText()
  const [state, setState] = useState<WebhookDeliveryState | "all">("all")
  const [open, setOpen] = useState<string | null>(null)
  const refresh = useRefreshLog(endpoint.id)
  const list = useInfiniteQuery({
    queryKey: keys.webhookDeliveries(ws, endpoint.id, state),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/webhooks/{webhookId}/deliveries", {
          params: {
            path: { webhookId: endpoint.id },
            query: { cursor: pageParam, limit: 25, ...(state === "all" ? {} : { state }) },
          },
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <Section
      title={<Trans>Deliveries</Trans>}
      description={<Trans>Newest first. Finished deliveries are kept for 7 days.</Trans>}
      action={
        <Button variant="ghost" size="sm" onClick={refresh} disabled={list.isFetching} data-testid="deliveries-refresh">
          <RotateCwIcon className={cn(list.isFetching && "animate-spin")} />
          <Trans>Refresh</Trans>
        </Button>
      }
    >
      <Tabs value={state} onValueChange={(v) => setState(v as WebhookDeliveryState | "all")}>
        <TabsList className="w-full sm:w-fit">
          {(["all", ...DELIVERY_STATES] as const).map((s) => (
            <TabsTrigger key={s} value={s} className="px-2 text-xs">
              {s === "all" ? <Trans>All</Trans> : text.delivery[s]}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      {list.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : items.length === 0 ? (
        <EmptyState icon={SendIcon} title={<Trans>No deliveries</Trans>} className="py-8">
          <Trans>Deliveries show up here once a subscribed event happens.</Trans>
        </EmptyState>
      ) : (
        <ul className="flex flex-col divide-y rounded-lg border">
          {items.map((d) => (
            <li key={d.id}>
              <button
                type="button"
                onClick={() => setOpen(d.id)}
                className="flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors hover:bg-muted/60"
                data-testid="delivery-row"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="flex min-w-0 items-center gap-2">
                    <code className="truncate font-mono text-xs font-medium">{d.event_type}</code>
                    <StateBadge state={d.state} />
                  </span>
                  <span className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                    <time dateTime={d.created_at}>{formatDateTime(d.created_at, i18n.locale)}</time>
                    <span>
                      <Attempts n={d.attempts} />
                    </span>
                    {d.state === "pending" && d.next_attempt_at && <NextAttempt at={d.next_attempt_at} />}
                  </span>
                </div>
                {d.last_attempt && (
                  <span className="flex shrink-0 flex-col items-end gap-0.5" title={t`Last attempt`}>
                    <AttemptResult a={d.last_attempt} />
                    <span className="text-xs text-muted-foreground tabular-nums">{d.last_attempt.latency_ms} ms</span>
                  </span>
                )}
                <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {list.hasNextPage && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start"
          onClick={() => void list.fetchNextPage()}
          disabled={list.isFetchingNextPage}
        >
          <Trans>Load more</Trans>
        </Button>
      )}
      <ErrorLine error={list.error} />
      <DeliverySheet endpoint={endpoint} deliveryId={open} onClose={() => setOpen(null)} />
    </Section>
  )
}

function DeliverySheet({
  endpoint,
  deliveryId,
  onClose,
}: {
  endpoint: WebhookEndpoint
  deliveryId: string | null
  onClose: () => void
}) {
  const { i18n } = useLingui()
  const { workspaceId: ws } = useSession()
  const refresh = useRefreshLog(endpoint.id)
  const delivery = useQuery({
    queryKey: keys.webhookDelivery(ws, endpoint.id, deliveryId ?? ""),
    queryFn: () =>
      unwrap(
        api.GET("/v1/webhooks/{webhookId}/deliveries/{deliveryId}", {
          params: { path: { webhookId: endpoint.id, deliveryId: deliveryId! } },
        }),
      ),
    enabled: !!deliveryId,
  })
  const redeliver = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver", {
          params: { path: { webhookId: endpoint.id, deliveryId: deliveryId! } },
        }),
      ),
    onSuccess: () => {
      refresh()
      setTimeout(refresh, 2000)
    },
  })
  const d = delivery.data
  const payload = d ? JSON.stringify(d.payload, null, 2) : ""
  return (
    <Sheet
      open={!!deliveryId}
      onOpenChange={(o) => {
        if (!o) {
          redeliver.reset()
          onClose()
        }
      }}
    >
      <SheetContent side="right" className="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-xl" data-testid="delivery-sheet">
        <SheetHeader className="border-b pr-12">
          <SheetTitle className="font-mono text-sm">{d?.event_type ?? "…"}</SheetTitle>
          <SheetDescription>
            <Trans>Delivery details</Trans>
          </SheetDescription>
        </SheetHeader>
        {delivery.isPending ? (
          <div className="flex flex-col gap-3 p-4">
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-40 w-full" />
          </div>
        ) : !d ? (
          <ErrorLine error={delivery.error} className="p-4" />
        ) : (
          <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
            <div className="flex flex-wrap items-center gap-2">
              <StateBadge state={d.state} />
              <span className="text-xs text-muted-foreground">
                <Attempts n={d.attempts} />
              </span>
              <Button
                size="sm"
                className="ml-auto"
                onClick={() => redeliver.mutate()}
                disabled={redeliver.isPending}
                data-testid="redeliver"
              >
                <SendIcon />
                <Trans>Redeliver</Trans>
              </Button>
            </div>
            {redeliver.isSuccess && (
              <p role="status" className="text-xs text-success" data-testid="redeliver-queued">
                <Trans>Queued. The new attempt shows up below in a moment.</Trans>
              </p>
            )}
            <ErrorLine error={redeliver.error} className="text-xs" />
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-sm">
              <dt className="text-muted-foreground">webhook-id</dt>
              <dd className="flex min-w-0 items-center gap-2">
                <code className="min-w-0 truncate font-mono text-xs">{d.message_id}</code>
                <CopyButton value={d.message_id} className="h-6 px-1.5 text-xs" />
              </dd>
              <dt className="text-muted-foreground">
                <Trans>Created</Trans>
              </dt>
              <dd>{formatDateTime(d.created_at, i18n.locale)}</dd>
              {d.state === "pending" && d.next_attempt_at && (
                <>
                  <dt className="text-muted-foreground">
                    <Trans>Next attempt</Trans>
                  </dt>
                  <dd>{formatDateTime(d.next_attempt_at, i18n.locale)}</dd>
                </>
              )}
            </dl>
            <div className="flex flex-col gap-2">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-medium">
                  <Trans>Payload</Trans>
                </h3>
                <CopyButton value={payload} />
              </div>
              <pre
                className="max-h-80 overflow-auto rounded-md border bg-muted px-3 py-2 font-mono text-xs leading-5"
                data-testid="delivery-payload"
              >
                {payload}
              </pre>
            </div>
            <div className="flex flex-col gap-2">
              <h3 className="text-sm font-medium">
                <Trans>Attempts</Trans>
              </h3>
              {d.attempt_log.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  <Trans>No attempt yet.</Trans>
                </p>
              ) : (
                <ol className="flex flex-col divide-y rounded-lg border" data-testid="attempt-log">
                  {d.attempt_log.map((a) => (
                    <li key={a.id} className="flex flex-col gap-1 px-3 py-2">
                      <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                        <AttemptResult a={a} />
                        <span className="text-muted-foreground tabular-nums">{a.latency_ms} ms</span>
                        <time className="text-muted-foreground" dateTime={a.attempted_at}>
                          {formatDateTime(a.attempted_at, i18n.locale)}
                        </time>
                        {a.manual && (
                          <Badge variant="secondary" className="h-4 text-[10px]">
                            <Trans>manual</Trans>
                          </Badge>
                        )}
                      </span>
                      {a.error && <span className="text-xs break-words text-destructive">{a.error}</span>}
                      {a.response_body && (
                        <pre className="max-h-24 overflow-auto rounded bg-muted px-2 py-1 font-mono text-[11px] break-all whitespace-pre-wrap text-muted-foreground">
                          {a.response_body}
                        </pre>
                      )}
                    </li>
                  ))}
                </ol>
              )}
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}

function AttemptsSection({ endpoint }: { endpoint: WebhookEndpoint }) {
  const { i18n } = useLingui()
  const { workspaceId: ws } = useSession()
  const [open, setOpen] = useState<string | null>(null)
  const list = useQuery({
    queryKey: keys.webhookAttempts(ws, endpoint.id),
    queryFn: () =>
      unwrap(
        api.GET("/v1/webhooks/{webhookId}/attempts", {
          params: { path: { webhookId: endpoint.id }, query: { limit: 100 } },
        }),
      ).then((r) => r.items),
  })
  return (
    <Section
      title={<Trans>Attempt log</Trans>}
      description={<Trans>Every request to this endpoint, newest first. The newest 100 are kept.</Trans>}
    >
      {list.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : !list.data || list.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Trans>No attempts yet.</Trans>
        </p>
      ) : (
        <div className="max-h-96 overflow-auto rounded-lg border">
          <table className="w-full text-left text-xs" data-testid="attempts-table">
            <thead className="sticky top-0 bg-muted text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium whitespace-nowrap">
                  <Trans>Time</Trans>
                </th>
                <th className="px-3 py-2 font-medium whitespace-nowrap">
                  <Trans>Status</Trans>
                </th>
                <th className="px-3 py-2 text-right font-medium whitespace-nowrap">
                  <Trans>Latency</Trans>
                </th>
                <th className="px-3 py-2 font-medium whitespace-nowrap">
                  <Trans>Error or answer</Trans>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {list.data.map((a) => (
                <tr
                  key={a.id}
                  className="cursor-pointer align-top hover:bg-muted/60"
                  onClick={() => setOpen(a.delivery_id)}
                  data-testid="attempt-row"
                >
                  <td className="px-3 py-1.5 whitespace-nowrap">
                    {formatDateTime(a.attempted_at, i18n.locale)}
                    {a.manual && (
                      <Badge variant="secondary" className="ml-1.5 h-4 text-[10px]">
                        <Trans>manual</Trans>
                      </Badge>
                    )}
                  </td>
                  <td className="px-3 py-1.5">
                    <AttemptResult a={a} />
                  </td>
                  <td className="px-3 py-1.5 text-right whitespace-nowrap tabular-nums">{a.latency_ms} ms</td>
                  <td className="max-w-64 min-w-40 truncate px-3 py-1.5 text-muted-foreground" title={a.error ?? a.response_body}>
                    {a.error || a.response_body || "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <ErrorLine error={list.error} />
      <DeliverySheet endpoint={endpoint} deliveryId={open} onClose={() => setOpen(null)} />
    </Section>
  )
}
