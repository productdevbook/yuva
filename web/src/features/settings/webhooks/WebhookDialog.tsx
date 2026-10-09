import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { useEnumText, WEBHOOK_EVENTS } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useSetEndpoint } from "@/features/settings/webhooks/queries"
import { Field, ToggleRow } from "@/features/settings/ui"
import { api, ApiError, unwrap, type WebhookEndpoint, type WebhookEventType } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"
import { Label } from "@/components/ui/label"

function urlProblem(err: unknown): "private" | "invalid" | null {
  if (!(err instanceof ApiError) || err.status !== 400 || !err.detail) return null
  if (/private|loopback|link-local/.test(err.detail)) return "private"
  if (/\burl\b/.test(err.detail)) return "invalid"
  return null
}

export function WebhookDialog({
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
  const [events, setEvents] = useState<Set<WebhookEventType>>(() => new Set(endpoint?.events ?? ["conversation.created", "message.created", "feedback.created"]))
  const [includeNotes, setIncludeNotes] = useState(endpoint?.include_notes ?? false)
  const [touched, setTouched] = useState(false)
  const chosen = WEBHOOK_EVENTS.filter((e) => events.has(e))
  const save = useMutation({
    mutationFn: (): Promise<{ endpoint: WebhookEndpoint; secret: string | null }> => {
      const body = { url: url.trim(), description: description.trim(), events: chosen, include_notes: includeNotes }
      if (endpoint) {
        return unwrap(api.PATCH("/v1/webhooks/{webhookId}", { params: { path: { webhookId: endpoint.id } }, body })).then((e) => ({ endpoint: e, secret: null }))
      }
      return unwrap(api.POST("/v1/webhooks", { body: { ...body, enabled: true, ...(scope ? { inbox_id: scope } : {}) } }))
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
  const toggle = (ev: WebhookEventType, on: boolean) =>
    setEvents((s) => {
      const next = new Set(s)
      if (on) next.add(ev)
      else next.delete(ev)
      return next
    })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] p-0 sm:max-w-lg">
        <form
          className="flex min-w-0 flex-col"
          onSubmit={(e) => {
            e.preventDefault()
            setTouched(true)
            if (chosen.length > 0) save.mutate()
          }}
        >
          <div className="flex flex-col gap-5 p-6">
            <DialogHeader>
              <DialogTitle>{endpoint ? <Trans>Edit endpoint</Trans> : <Trans>Add endpoint</Trans>}</DialogTitle>
              <DialogDescription>
                <Trans>Yuva sends a signed POST request with a JSON body for every event you pick.</Trans>
              </DialogDescription>
            </DialogHeader>
            <Field
              label={<Trans>URL</Trans>}
              htmlFor="webhook-url"
              error={
                bad &&
                (bad === "private" ? (
                  <Trans>This address is private, loopback or link-local. Webhooks only reach public servers.</Trans>
                ) : (
                  <Trans>Enter an http or https URL without a user name or password.</Trans>
                ))
              }
            >
              <Input
                id="webhook-url"
                type="url"
                required
                maxLength={2000}
                value={url}
                placeholder="https://api.example.com/yuva/webhook"
                onChange={(e) => setUrl(e.target.value)}
                aria-invalid={!!bad || undefined}
                className="font-mono text-caption"
              />
            </Field>
            <Field label={<Trans>Description (optional)</Trans>} htmlFor="webhook-description">
              <Input id="webhook-description" maxLength={200} value={description} placeholder={t`Push notifications`} onChange={(e) => setDescription(e.target.value)} />
            </Field>
            {!inboxId && (
              <Field label={<Trans>Inbox</Trans>} hint={endpoint ? <Trans>The inbox cannot be changed later.</Trans> : undefined}>
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
            <div className="flex min-w-0 flex-col gap-2" role="group" aria-labelledby="webhook-events">
              <span id="webhook-events" className="text-body font-medium">
                <Trans>Events</Trans>
              </span>
              <ul className="divide-y rounded-xl border">
                {WEBHOOK_EVENTS.map((ev) => (
                  <li key={ev}>
                    <Label className="gap-3 px-3.5 py-2.5 font-normal">
                      <Checkbox checked={events.has(ev)} onCheckedChange={(on) => toggle(ev, on)} aria-label={ev} />
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="text-body">{text.event[ev]}</span>
                        <code className="font-mono text-caption text-faint">{ev}</code>
                      </span>
                    </Label>
                  </li>
                ))}
              </ul>
              {touched && chosen.length === 0 && (
                <p role="alert" className="text-caption text-destructive">
                  <Trans>Pick at least one event.</Trans>
                </p>
              )}
            </div>
            <ToggleRow
              title={<Trans>Include notes</Trans>}
              hint={<Trans>Also send message.created for members' internal notes. Internal events are never sent.</Trans>}
            >
              <Switch checked={includeNotes} onCheckedChange={setIncludeNotes} aria-label={t`Include notes`} />
            </ToggleRow>
            {!bad && <ErrorLine error={save.error} />}
          </div>
          <DialogFooter className="sticky bottom-0 border-t bg-card px-6 py-3">
            <Button type="submit" disabled={save.isPending}>
              {endpoint ? <Trans>Save</Trans> : <Trans>Add endpoint</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
