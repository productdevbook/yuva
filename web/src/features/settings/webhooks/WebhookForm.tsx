import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { GlobeIcon, InboxIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { FormCard } from "@/components/common/FormCard"
import { RadioChoices } from "@/components/common/RadioChoices"
import { SecretCard } from "@/components/common/SecretCard"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { Field } from "@/features/settings/ui"
import {
  DEFAULT_EVENTS,
  EventPicker,
  sortedEvents,
  UrlProblem,
  urlProblem,
  validUrl,
} from "@/features/settings/webhooks/events"
import { useSetEndpoint } from "@/features/settings/webhooks/queries"
import { idempotencyKey } from "@/features/setup/draft"
import { api, unwrap, type WebhookEventType } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

export type CreatedEndpoint = { id: string; secret: string }

export function WebhookForm({
  lockedInbox,
  onClose,
  onCreated,
}: {
  lockedInbox?: string
  onClose: () => void
  onCreated: (e: CreatedEndpoint) => void
}) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const setEndpoint = useSetEndpoint()
  const [url, setUrl] = useState("")
  const [description, setDescription] = useState("")
  const [scope, setScope] = useState<"all" | "inbox">("all")
  const [picked, setPicked] = useState("")
  const [events, setEvents] = useState<WebhookEventType[]>(DEFAULT_EVENTS)
  const [includeNotes, setIncludeNotes] = useState(false)
  const [touched, setTouched] = useState(false)
  const [problem, setProblem] = useState<"private" | "invalid" | null>(null)
  const [key] = useState(() => crypto.randomUUID())
  const inboxId = lockedInbox ?? (scope === "inbox" ? picked || (inboxes[0]?.id ?? "") : "")
  const lockedName = lockedInbox ? inboxes.find((i) => i.id === lockedInbox)?.name : undefined
  const chosen = sortedEvents(events)
  const urlOk = validUrl(url)
  const shownProblem = problem ?? (touched && !urlOk ? "invalid" : null)
  const create = useMutation({
    mutationFn: () => {
      const body = {
        url: url.trim(),
        description: description.trim(),
        events: chosen,
        include_notes: includeNotes && chosen.includes("message.created"),
        enabled: true,
        ...(inboxId ? { inbox_id: inboxId } : {}),
      }
      return unwrap(
        api.POST("/v1/webhooks", {
          body,
          params: { header: { "Idempotency-Key": idempotencyKey(key, "webhook", body) } },
        })
      )
    },
    onSuccess: (r) => {
      setEndpoint(r.endpoint)
      onCreated({ id: r.endpoint.id, secret: r.secret })
    },
    onError: (err) => setProblem(urlProblem(err)),
  })
  return (
    <FormCard
      title={<Trans>New webhook endpoint</Trans>}
      description={<Trans>Yuva sends a signed POST request with a JSON body for every event you pick.</Trans>}
      submit={<Trans>Add endpoint</Trans>}
      ready={chosen.length > 0 && (scope === "all" || !!inboxId)}
      busy={create.isPending}
      error={problem ? undefined : create.error}
      onSubmit={() => {
        setTouched(true)
        if (urlOk) create.mutate()
      }}
      onCancel={onClose}
      testId="webhook-form"
    >
      <Field
        label={<Trans>URL</Trans>}
        htmlFor="webhook-url"
        error={shownProblem && <UrlProblem problem={shownProblem} />}
      >
        <Input
          id="webhook-url"
          type="url"
          autoFocus
          required
          maxLength={2000}
          className="font-mono text-caption"
          value={url}
          placeholder="https://api.example.com/yuva/webhook"
          onChange={(e) => {
            setProblem(null)
            setUrl(e.target.value)
          }}
          aria-invalid={!!shownProblem || undefined}
          data-testid="webhook-url-input"
        />
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
      {lockedInbox ? (
        lockedName && (
          <p className="text-body text-muted-foreground">
            <Trans>This endpoint receives only the events of {lockedName}.</Trans>
          </p>
        )
      ) : (
        <Field label={<Trans>Inbox</Trans>}>
          <RadioChoices
            id="webhook-scope"
            label={t`Inbox`}
            value={scope}
            onChange={setScope}
            columns
            items={[
              {
                value: "all",
                icon: <GlobeIcon />,
                title: <Trans>All inboxes</Trans>,
                hint: <Trans>Every inbox of the workspace, including ones you add later.</Trans>,
                testId: "webhook-scope-all",
              },
              {
                value: "inbox",
                icon: <InboxIcon />,
                title: <Trans>One inbox</Trans>,
                hint: <Trans>The inbox cannot be changed later.</Trans>,
                testId: "webhook-scope-inbox",
              },
            ]}
          />
          {scope === "inbox" && (
            <NativeSelect
              value={inboxId}
              onChange={(e) => setPicked(e.target.value)}
              aria-label={t`Inbox`}
              className="w-full"
              data-testid="webhook-inbox"
            >
              {inboxes.map((i) => (
                <NativeSelectOption key={i.id} value={i.id}>
                  {i.name}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          )}
        </Field>
      )}
      <div className="flex flex-col gap-2">
        <EventPicker
          events={events}
          includeNotes={includeNotes}
          onEvents={setEvents}
          onIncludeNotes={setIncludeNotes}
        />
        {chosen.length === 0 && (
          <p role="alert" className="text-small text-destructive">
            <Trans>Pick at least one event.</Trans>
          </p>
        )}
      </div>
    </FormCard>
  )
}

export function WebhookSecret({ created, onDone }: { created: CreatedEndpoint; onDone: () => void }) {
  return (
    <SecretCard
      title={<Trans>The endpoint is ready</Trans>}
      description={<Trans>Yuva now sends the events you picked to this address.</Trans>}
      label={<Trans>Signing secret</Trans>}
      secret={created.secret}
      hint={<Trans>Shown only now. Copy it and store it with your receiver.</Trans>}
      testId="webhook-created"
      actions={
        <>
          <Button onClick={onDone} data-testid="webhook-created-done">
            <Trans>Done</Trans>
          </Button>
          <Button variant="outline" render={<Link to={`/settings/webhooks/${created.id}`} />}>
            <Trans>Open the endpoint</Trans>
          </Button>
        </>
      }
    />
  )
}
