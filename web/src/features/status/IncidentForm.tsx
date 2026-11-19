import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"
import { useState } from "react"

import { FormCard } from "@/components/common/FormCard"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Field } from "@/features/settings/ui"
import { ChoiceChips } from "@/features/status/Choice"
import { NotifyCheck } from "@/features/status/NotifyCheck"
import { useIncidentMutations, useStatusComponents } from "@/features/status/queries"
import { COMPONENT_STATES, IMPACTS, INCIDENT_STATES, useStatusText } from "@/features/status/text"
import type {
  StatusComponentStateValue,
  StatusIncident,
  StatusIncidentImpact,
  StatusIncidentKind,
  StatusIncidentState,
  StatusPage,
} from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

function toLocalInput(date: Date) {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function defaultWindow() {
  const start = new Date()
  start.setMinutes(0, 0, 0)
  start.setHours(start.getHours() + 1)
  return [toLocalInput(start), toLocalInput(new Date(start.getTime() + 3600_000))] as const
}

export type IncidentPrefill = {
  title: string
  impact: StatusIncidentImpact
  components: Record<string, StatusComponentStateValue>
  message: string
}

export function IncidentForm({
  pages,
  pageId,
  kind,
  edit,
  prefill,
  onClose,
  onDone,
  className,
}: {
  className?: string
  pages: StatusPage[]
  pageId?: string
  kind: StatusIncidentKind
  edit?: StatusIncident
  prefill?: IncidentPrefill
  onClose: () => void
  onDone: (i: StatusIncident) => void
}) {
  const { t } = useLingui()
  const text = useStatusText()
  const inboxes = useInboxes().data ?? []
  const [chosen, setChosen] = useState(edit?.page_id ?? pageId ?? pages[0]?.id ?? "")
  const page = pages.find((p) => p.id === chosen)
  const m = useIncidentMutations(chosen)
  const components = useStatusComponents(chosen).data ?? []
  const [notify, setNotify] = useState(true)
  const maintenance = kind === "maintenance"
  const [title, setTitle] = useState(edit?.title ?? prefill?.title ?? "")
  const [impact, setImpact] = useState<StatusIncidentImpact>(
    edit?.impact ?? prefill?.impact ?? (maintenance ? "none" : "minor")
  )
  const [state, setState] = useState<StatusIncidentState>("investigating")
  const [message, setMessage] = useState(prefill?.message ?? "")
  const [defaults] = useState(defaultWindow)
  const [start, setStart] = useState(edit?.scheduled_start ? toLocalInput(new Date(edit.scheduled_start)) : defaults[0])
  const [end, setEnd] = useState(edit?.scheduled_end ? toLocalInput(new Date(edit.scheduled_end)) : defaults[1])
  const fallback: StatusComponentStateValue = maintenance ? "maintenance" : "major_outage"
  const [touched, setTouched] = useState<Record<string, StatusComponentStateValue>>(
    prefill?.components ?? Object.fromEntries((edit?.components ?? []).map((c) => [c.component_id, c.state]))
  )
  const windowChanged =
    !!edit &&
    maintenance &&
    (new Date(start).getTime() !== Date.parse(edit.scheduled_start ?? "") ||
      new Date(end).getTime() !== Date.parse(edit.scheduled_end ?? ""))
  const reachable = !!page?.published && !!page.email_subscribers.enabled
  const askNotify = !!page && reachable && (!edit || windowChanged)
  const pending = m.create.isPending || m.patch.isPending
  const windowOk = !maintenance || (!!start && !!end && new Date(end) > new Date(start))
  const ok = !!chosen && !!title.trim() && (!!edit || !!message.trim()) && windowOk
  const showProducts = !edit && !prefill && pages.length > 1
  const markName = (p: StatusPage) => inboxes.find((i) => i.id === p.inbox_id)?.name ?? p.name

  const body = () => ({
    title: title.trim(),
    impact,
    components: Object.entries(touched).map(([component_id, s]) => ({ component_id, state: s })),
    ...(maintenance
      ? { scheduled_start: new Date(start).toISOString(), scheduled_end: new Date(end).toISOString() }
      : {}),
  })
  const save = (publish: boolean) => {
    if (!ok) return
    if (edit)
      m.patch.mutate(
        { id: edit.id, ...body(), ...(windowChanged && reachable ? { notify } : {}) },
        { onSuccess: onDone }
      )
    else
      m.create.mutate(
        {
          kind,
          ...body(),
          message: message.trim(),
          publish,
          ...(maintenance ? {} : { state }),
          ...(reachable ? { notify } : {}),
        },
        { onSuccess: onDone }
      )
  }
  const states = INCIDENT_STATES.filter((s) => s !== "resolved").map((s) => [s, text.incident[s]] as const)
  const impacts = IMPACTS.map((s) => [s, text.impact[s]] as const)
  const touchStates = COMPONENT_STATES.filter((s) => s !== "operational").map((s) => [s, text.component[s]] as const)
  return (
    <FormCard
      title={
        edit ? (
          <Trans>Edit details</Trans>
        ) : maintenance ? (
          <Trans>Plan maintenance</Trans>
        ) : (
          <Trans>Open an incident</Trans>
        )
      }
      description={
        !edit && <Trans>A draft is seen only by your team and changes no component until you publish it.</Trans>
      }
      submit={edit ? <Trans>Save</Trans> : <Trans>Save as draft</Trans>}
      ready={ok}
      busy={pending}
      error={m.create.error ?? m.patch.error}
      onSubmit={() => save(false)}
      onCancel={onClose}
      className={className}
      testId="status-incident-dialog"
      footer={
        <>
          {edit ? (
            <Button type="submit" disabled={!ok || pending} data-testid="status-incident-save">
              {pending && <LoaderIcon className="animate-spin" />}
              <Trans>Save</Trans>
            </Button>
          ) : (
            <>
              <Button
                type="button"
                disabled={!ok || pending}
                onClick={() => save(true)}
                data-testid="status-incident-publish"
              >
                {pending && <LoaderIcon className="animate-spin" />}
                <Trans>Publish now</Trans>
              </Button>
              <Button type="submit" variant="outline" disabled={!ok || pending} data-testid="status-incident-draft">
                <Trans>Save as draft</Trans>
              </Button>
            </>
          )}
          <Button type="button" variant="outline" onClick={onClose}>
            <Trans>Cancel</Trans>
          </Button>
          {askNotify && page && (
            <span className="ms-auto">
              <NotifyCheck
                page={page}
                components={Object.keys(touched)}
                checked={notify}
                onChange={setNotify}
                testId="status-incident-notify"
              />
            </span>
          )}
        </>
      }
    >
      {showProducts && (
        <Field label={<Trans>Product</Trans>}>
          <ChoiceChips
            value={chosen}
            onChange={(id) => {
              setChosen(id)
              setTouched({})
            }}
            options={pages.map((p) => [p.id, markName(p)] as const)}
            label={t`Product`}
          />
        </Field>
      )}
      <Field label={<Trans>Title</Trans>} htmlFor="status-incident-title">
        <Input
          id="status-incident-title"
          value={title}
          maxLength={200}
          autoFocus
          required
          placeholder={maintenance ? t`Database upgrade` : t`Elevated API errors`}
          onChange={(e) => setTitle(e.target.value)}
          data-testid="status-incident-title"
        />
      </Field>
      <div className="grid gap-5 sm:grid-cols-2">
        <Field label={<Trans>Impact</Trans>}>
          <ChoiceChips value={impact} onChange={setImpact} options={impacts} label={t`Impact`} />
        </Field>
        {!edit && !maintenance && (
          <Field label={<Trans>State</Trans>}>
            <ChoiceChips value={state} onChange={setState} options={states} label={t`State`} />
          </Field>
        )}
      </div>
      {maintenance && (
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={<Trans>Starts</Trans>} htmlFor="status-incident-start">
            <Input
              id="status-incident-start"
              type="datetime-local"
              value={start}
              required
              onChange={(e) => setStart(e.target.value)}
              data-testid="status-incident-start"
            />
          </Field>
          <Field
            label={<Trans>Ends</Trans>}
            htmlFor="status-incident-end"
            error={!windowOk && start && end ? <Trans>The end must be after the start.</Trans> : undefined}
          >
            <Input
              id="status-incident-end"
              type="datetime-local"
              value={end}
              required
              onChange={(e) => setEnd(e.target.value)}
              data-testid="status-incident-end"
            />
          </Field>
        </div>
      )}
      {components.length > 0 && (
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-2 text-body font-medium">
            <Trans>Components it touches</Trans>
          </legend>
          <ul className="flex flex-col divide-y rounded-xl border">
            {components.map((c) => {
              const on = c.id in touched
              return (
                <li key={c.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2">
                  <Label className="min-w-0 flex-1 basis-32 font-normal">
                    <Checkbox
                      checked={on}
                      onCheckedChange={(v) =>
                        setTouched((cur) => {
                          const next = { ...cur }
                          if (v) next[c.id] = fallback
                          else delete next[c.id]
                          return next
                        })
                      }
                      data-testid="status-incident-component"
                    />
                    <span className="truncate">{c.name}</span>
                  </Label>
                  {on && (
                    <ChoiceChips
                      value={touched[c.id]}
                      onChange={(s) => setTouched((cur) => ({ ...cur, [c.id]: s }))}
                      options={touchStates}
                      label={t`State of ${c.name} while it is open`}
                    />
                  )}
                </li>
              )
            })}
          </ul>
          <p className="text-small text-faint">
            {maintenance ? (
              <Trans>Each component gets this state while the maintenance is open.</Trans>
            ) : (
              <Trans>Each component gets this state while the incident is open.</Trans>
            )}
          </p>
        </fieldset>
      )}
      {!edit && (
        <Field
          label={<Trans>First update</Trans>}
          htmlFor="status-incident-message"
          hint={<Trans>Plain text with links and line breaks, at most 5,000 characters.</Trans>}
        >
          <Textarea
            id="status-incident-message"
            value={message}
            maxLength={5000}
            required
            rows={4}
            onChange={(e) => setMessage(e.target.value)}
            data-testid="status-incident-message"
          />
        </Field>
      )}
      {page && !page.published && (
        <p className="rounded-lg border border-dashed px-3 py-2 text-small text-muted-foreground">
          <Trans>This page is not public yet, so visitors see nothing until you publish the page.</Trans>
        </p>
      )}
    </FormCard>
  )
}
