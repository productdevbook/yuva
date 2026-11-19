import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronDownIcon, LoaderIcon } from "lucide-react"
import { useState } from "react"

import { FormCard } from "@/components/common/FormCard"
import { formatDuration } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { look } from "@/features/inbox/PageFrame"
import { Field, ToggleRow } from "@/features/settings/ui"
import { ChoiceChips } from "@/features/status/Choice"
import { HttpDetails, HttpUrlField, newHeaderKey, type HttpForm } from "@/features/status/MonitorHttpFields"
import { useMonitorFieldErrors } from "@/features/status/monitorErrors"
import { useMonitorMutations, usePagesComponents } from "@/features/status/queries"
import { FAILURE_STATES, MONITOR_KINDS, ON_FAILURE, useStatusText } from "@/features/status/text"
import type {
  StatusMonitor,
  StatusMonitorFailureState,
  StatusMonitorKind,
  StatusMonitorOnFailure,
  StatusMonitorWithToken,
  StatusPage,
} from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const INTERVALS = [30, 60, 300, 600, 900, 1800, 3600, 21600, 43200, 86400, 604800]
const OUTSIDE_INTERVALS = [60, 120, 300, 600, 900, 1800, 3600, 21600, 43200, 86400]
const GRACES = [0, 30, 60, 300, 900, 1800, 3600, 21600, 86400]

function withCurrent(list: number[], current: number) {
  return list.includes(current) ? list : [...list, current].sort((a, b) => a - b)
}

export function MonitorForm({
  pages,
  pageId,
  edit,
  onClose,
  onCreated,
  className,
}: {
  className?: string
  pages: StatusPage[]
  pageId?: string
  edit?: StatusMonitor
  onClose: () => void
  onCreated: (m: StatusMonitorWithToken) => void
}) {
  const { t, i18n } = useLingui()
  const text = useStatusText()
  const inboxes = useInboxes().data ?? []
  const scoped = edit ? pages.filter((p) => p.id === edit.page_id) : pages
  const all = usePagesComponents(scoped.map((p) => p.id))
  const groups = scoped
    .map((p) => ({
      page: p,
      name: inboxes.find((i) => i.id === p.inbox_id)?.name ?? p.name,
      items: all.filter((c) => c.page_id === p.id),
    }))
    .filter((g) => g.items.length > 0)
  const [kind, setKind] = useState<StatusMonitorKind>(edit?.kind ?? "http")
  const [name, setName] = useState(edit?.name ?? "")
  const [componentId, setComponentId] = useState(edit?.component_id ?? "")
  const picked = componentId || (groups.find((g) => g.page.id === pageId) ?? groups[0])?.items[0]?.id || ""
  const targetPage = all.find((c) => c.id === picked)?.page_id ?? edit?.page_id ?? ""
  const m = useMonitorMutations(targetPage)
  const [interval, setInterval_] = useState(edit?.interval ?? 300)
  const [grace, setGrace] = useState(edit?.grace ?? 60)
  const [outsideInterval, setOutsideInterval] = useState(edit?.interval ? String(edit.interval) : "")
  const [timeout, setTimeout_] = useState(edit?.timeout ?? 10)
  const [tcpHost, setTcpHost] = useState(edit?.tcp?.host ?? "")
  const [tcpPort, setTcpPort] = useState(edit?.tcp?.port ? String(edit.tcp.port) : "")
  const [more, setMore] = useState(false)
  const [http, setHttp] = useState<HttpForm>(() => ({
    url: edit?.http?.url ?? "",
    method: edit?.http?.method ?? "GET",
    headers: (edit?.http?.header_names ?? []).map((name) => ({
      key: newHeaderKey(),
      name,
      value: "",
      stored: true,
      replacing: false,
    })),
    body: edit?.http?.body ?? "",
    statusMin: edit?.http?.expected_status_min ?? 200,
    statusMax: edit?.http?.expected_status_max ?? 299,
    keyword: edit?.http?.keyword ?? "",
    follow: edit?.http?.follow_redirects ?? false,
    certWarn: edit?.http?.cert_warn_days ?? 14,
  }))
  const [failureThresholdSet, setFailureThreshold] = useState(edit?.failure_threshold)
  const [recoveryThresholdSet, setRecoveryThreshold] = useState(edit?.recovery_threshold)
  const [failureState, setFailureState] = useState<StatusMonitorFailureState>(edit?.failure_state ?? "major_outage")
  const [onFailure, setOnFailure] = useState<StatusMonitorOnFailure>(edit?.on_failure ?? "draft")
  const [paused, setPaused] = useState(edit?.paused ?? false)
  const pending = m.create.isPending || m.update.isPending
  const heartbeat = kind === "heartbeat"
  const outside = kind === "http" || kind === "tcp"
  const failureThreshold = failureThresholdSet ?? (outside ? 3 : 1)
  const recoveryThreshold = recoveryThresholdSet ?? (outside ? 2 : 1)
  const fieldErrors = useMonitorFieldErrors()
  const { fields, rest } = fieldErrors(m.create.error ?? m.update.error, kind)
  const clamp = (v: string) => Math.min(10, Math.max(1, Number(v) || 1))

  const outsideBody = () => {
    const headers = http.headers
      .filter((h) => h.name.trim())
      .map((h) => (h.stored && !h.replacing ? { name: h.name } : { name: h.name.trim(), value: h.value }))
    return {
      timeout,
      ...(outsideInterval ? { interval: Number(outsideInterval) } : {}),
      ...(kind === "tcp"
        ? { tcp: { host: tcpHost.trim(), port: Number(tcpPort) } }
        : {
            http: {
              url: http.url.trim(),
              method: http.method,
              headers,
              body: http.body,
              expected_status_min: http.statusMin,
              expected_status_max: http.statusMax,
              keyword: http.keyword,
              follow_redirects: http.follow,
              cert_warn_days: http.certWarn,
            },
          }),
    }
  }

  const submit = () => {
    if (!name.trim() || !picked || !targetPage) return
    const common = {
      name: name.trim(),
      component_id: picked,
      failure_state: failureState,
      on_failure: onFailure,
      failure_threshold: failureThreshold,
      recovery_threshold: recoveryThreshold,
      paused,
      ...(heartbeat ? { interval, grace } : {}),
      ...(outside ? outsideBody() : {}),
    }
    if (edit) m.update.mutate({ id: edit.id, ...common }, { onSuccess: onClose })
    else
      m.create.mutate({ kind, ...common }, { onSuccess: (created) => (created.url ? onCreated(created) : onClose()) })
  }

  const dur = (s: number) => formatDuration(s, i18n.locale)
  const failHint: Record<StatusMonitorOnFailure, string> = {
    none: t`Nothing opens by itself. Open an incident from the monitor when you want one.`,
    draft: t`A draft incident opens for your team to review. Customers see nothing until a member publishes it.`,
    publish: t`An incident is published at once, so customers see it on the page.`,
  }

  return (
    <FormCard
      title={edit ? <Trans>Edit monitor</Trans> : <Trans>Add monitor</Trans>}
      submit={edit ? <Trans>Save</Trans> : <Trans>Add monitor</Trans>}
      ready={
        !(
          !name.trim() ||
          !picked ||
          (kind === "http" && !http.url.trim()) ||
          (kind === "tcp" && (!tcpHost.trim() || !tcpPort))
        )
      }
      busy={pending}
      error={rest}
      onSubmit={submit}
      onCancel={onClose}
      className={className}
      testId="status-monitor-dialog"
      footer={
        <>
          <Button
            type="submit"
            disabled={
              !name.trim() ||
              !picked ||
              (kind === "http" && !http.url.trim()) ||
              (kind === "tcp" && (!tcpHost.trim() || !tcpPort)) ||
              pending
            }
            data-testid="status-monitor-save"
          >
            {pending && <LoaderIcon className="animate-spin" />}
            {edit ? <Trans>Save</Trans> : <Trans>Add monitor</Trans>}
          </Button>
          <Button type="button" variant="outline" onClick={onClose}>
            <Trans>Cancel</Trans>
          </Button>
        </>
      }
    >
      {!edit && (
        <Field
          label={<Trans>Kind</Trans>}
          hint={
            kind === "http" ? (
              <Trans>Yuva requests an address on a schedule and checks the answer.</Trans>
            ) : kind === "tcp" ? (
              <Trans>Yuva opens a connection to a host and port on a schedule.</Trans>
            ) : heartbeat ? (
              <Trans>Your job calls Yuva on a schedule. A missing call fails the monitor.</Trans>
            ) : (
              <Trans>Your own monitoring sends its state, including Alertmanager and Grafana alerts.</Trans>
            )
          }
        >
          <ChoiceChips
            value={kind}
            onChange={setKind}
            label={t`Kind`}
            options={MONITOR_KINDS.map((k) => [k, text.monitorKind[k]] as const)}
          />
        </Field>
      )}
      <Field label={<Trans>Name</Trans>} htmlFor="status-monitor-name">
        <Input
          id="status-monitor-name"
          value={name}
          maxLength={100}
          autoFocus
          required
          placeholder={t`Nightly backup`}
          onChange={(e) => setName(e.target.value)}
          data-testid="status-monitor-name"
        />
      </Field>
      <Field label={<Trans>Component</Trans>} hint={<Trans>The component whose state this monitor sets.</Trans>}>
        <div className="flex flex-col gap-2" role="group" aria-label={t`Component`}>
          {groups.length === 0 && (
            <p className="text-small text-faint">
              <Trans>No components yet. Add one to a status page first.</Trans>
            </p>
          )}
          {groups.map((g) => (
            <div key={g.page.id} className="flex flex-wrap items-center gap-1.5">
              {groups.length > 1 && <span className="me-1 min-w-14 text-caption text-faint">{g.name}</span>}
              {g.items.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  aria-pressed={c.id === picked}
                  onClick={() => setComponentId(c.id)}
                  className={c.id === picked ? look.chipOn : look.chip}
                >
                  {c.name}
                </button>
              ))}
            </div>
          ))}
        </div>
      </Field>
      {kind === "http" && (
        <HttpUrlField value={http} onChange={(p) => setHttp((h) => ({ ...h, ...p }))} errors={fields} />
      )}
      {kind === "tcp" && (
        <div className="grid grid-cols-[1fr_8rem] gap-4 phone:grid-cols-1">
          <Field label={<Trans>Host</Trans>} htmlFor="status-monitor-host" error={fields.host}>
            <Input
              id="status-monitor-host"
              value={tcpHost}
              maxLength={253}
              required
              placeholder="db.example.com"
              aria-invalid={!!fields.host}
              onChange={(e) => setTcpHost(e.target.value)}
              data-testid="status-monitor-host"
            />
          </Field>
          <Field label={<Trans>Port</Trans>} htmlFor="status-monitor-port" error={fields.port}>
            <Input
              id="status-monitor-port"
              type="number"
              inputMode="numeric"
              min={1}
              max={65535}
              required
              placeholder="5432"
              aria-invalid={!!fields.port}
              value={tcpPort}
              onChange={(e) => setTcpPort(e.target.value)}
              data-testid="status-monitor-port"
            />
          </Field>
        </div>
      )}
      {outside && (
        <>
          <Field label={<Trans>Check every</Trans>} error={fields.interval}>
            <ChoiceChips
              value={outsideInterval}
              onChange={setOutsideInterval}
              options={[
                ["", t`As often as allowed`] as const,
                ...withCurrent(OUTSIDE_INTERVALS, Number(outsideInterval) || 0)
                  .filter((s) => s > 0)
                  .map((s) => [String(s), dur(s)] as const),
              ]}
              label={t`Check every`}
            />
          </Field>
          <Field
            label={<Trans>Timeout</Trans>}
            htmlFor="status-monitor-timeout"
            error={fields.timeout}
            hint={<Trans>Seconds, 1 to 30.</Trans>}
            className="max-w-40"
          >
            <Input
              id="status-monitor-timeout"
              type="number"
              inputMode="numeric"
              min={1}
              max={30}
              value={timeout}
              onChange={(e) => setTimeout_(Math.min(30, Math.max(1, Number(e.target.value) || 1)))}
            />
          </Field>
        </>
      )}
      {heartbeat && (
        <>
          <Field label={<Trans>Expected every</Trans>}>
            <ChoiceChips
              value={String(interval)}
              onChange={(v) => setInterval_(Number(v))}
              options={withCurrent(INTERVALS, interval).map((s) => [String(s), dur(s)] as const)}
              label={t`Expected every`}
            />
          </Field>
          <Field label={<Trans>Grace</Trans>} hint={<Trans>How late a beat may be.</Trans>}>
            <ChoiceChips
              value={String(grace)}
              onChange={(v) => setGrace(Number(v))}
              options={withCurrent(GRACES, grace).map((s) => [String(s), s === 0 ? t`None` : dur(s)] as const)}
              label={t`Grace`}
            />
          </Field>
          {!edit && (
            <p className="text-small text-faint">
              <Trans>You get the address to call when you save. It is shown only once.</Trans>
            </p>
          )}
        </>
      )}
      {kind === "push" && !edit && (
        <p className="text-small text-faint">
          <Trans>
            You get the address your alerting calls when you save, for Alertmanager, Grafana or your own script. It is
            shown only once.
          </Trans>
        </p>
      )}
      {kind === "http" && (
        <div className="flex flex-col gap-3">
          <button
            type="button"
            aria-expanded={more}
            onClick={() => setMore((v) => !v)}
            className="inline-flex w-fit items-center gap-1 text-body text-muted-foreground outline-none hover:text-foreground focus-visible:text-foreground"
          >
            <ChevronDownIcon className={cn("size-4 transition-transform", more && "rotate-180")} />
            {more ? <Trans>Hide HTTP details</Trans> : <Trans>Show HTTP details</Trans>}
          </button>
          {more && <HttpDetails value={http} onChange={(p) => setHttp((h) => ({ ...h, ...p }))} errors={fields} />}
        </div>
      )}
      <div className="grid grid-cols-2 gap-4 phone:grid-cols-1">
        <Field
          label={<Trans>Failures to go down</Trans>}
          htmlFor="status-monitor-failures"
          hint={<Trans>In a row, 1 to 10.</Trans>}
        >
          <Input
            id="status-monitor-failures"
            type="number"
            inputMode="numeric"
            min={1}
            max={10}
            value={failureThreshold}
            onChange={(e) => setFailureThreshold(clamp(e.target.value))}
          />
        </Field>
        <Field
          label={<Trans>Successes to recover</Trans>}
          htmlFor="status-monitor-recoveries"
          hint={<Trans>In a row, 1 to 10.</Trans>}
        >
          <Input
            id="status-monitor-recoveries"
            type="number"
            inputMode="numeric"
            min={1}
            max={10}
            value={recoveryThreshold}
            onChange={(e) => setRecoveryThreshold(clamp(e.target.value))}
          />
        </Field>
      </div>
      <Field
        label={<Trans>State while down</Trans>}
        hint={heartbeat || outside ? undefined : <Trans>A pushed state that names its own state keeps it.</Trans>}
      >
        <ChoiceChips
          value={failureState}
          onChange={setFailureState}
          options={FAILURE_STATES.map((s) => [s, text.component[s]] as const)}
          label={t`State while down`}
        />
      </Field>
      <Field label={<Trans>When it goes down</Trans>} hint={failHint[onFailure]}>
        <ChoiceChips
          value={onFailure}
          onChange={setOnFailure}
          options={ON_FAILURE.map((s) => [s, text.onFailure[s]] as const)}
          label={t`When it goes down`}
        />
      </Field>
      <ToggleRow title={<Trans>Paused</Trans>} hint={<Trans>Ignores results and lets the component go back.</Trans>}>
        <Switch checked={paused} onCheckedChange={setPaused} data-testid="status-monitor-paused" />
      </ToggleRow>
    </FormCard>
  )
}
