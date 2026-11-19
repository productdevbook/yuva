import "@/features/analytics/i18n"

import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightIcon, CheckCircle2Icon } from "lucide-react"
import { useMemo, useState } from "react"
import { Link, Navigate, useLocation, useNavigate, useParams } from "react-router"

import { ErrorLine, Notice, StepHeader, WizardFrame } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { InstallSnippets } from "@/features/analytics/Install"
import { useCreateProperty, useProperty, useWaitForFirstEvent } from "@/features/analytics/queries"
import { PropertyCard } from "@/features/home/PropertyCard"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { PreviewCaption } from "@/features/setup/previews"
import { BackHome } from "@/features/setup/BackHome"
import type { SetupState } from "@/features/setup/setup"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"
import { timeZones } from "@/lib/utils"

const NO_INBOX = "none"

export function splitLines(s: string) {
  return s
    .split(/[\s,]+/)
    .map((x) => x.trim())
    .filter(Boolean)
}

export function hostOnly(s: string) {
  return s
    .replace(/^[a-z]+:\/\//i, "")
    .replace(/[/:].*$/, "")
    .toLowerCase()
}

export function useZones() {
  return useMemo(() => timeZones(), [])
}

function useSteps() {
  const { t } = useLingui()
  return [t`Website or app`, t`Install`, t`First visit`].map((label, i) => ({ key: String(i + 1), label }))
}

function Preview({ name, host }: { name: string; host: string }) {
  return (
    <>
      <PropertyCard example name={name || "Fieldnote"} host={host || "www.example.com"} />
      <PreviewCaption>
        <Trans>Home shows each website and app like this: visitors today, in the last 30 minutes and this week.</Trans>
      </PreviewCaption>
    </>
  )
}

function NewPropertyStep() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as SetupState
  const steps = useSteps()
  const inboxes = useInboxes().data ?? []
  const zones = useZones()
  const [name, setName] = useState("")
  const [domains, setDomains] = useState("")
  const [inbox, setInbox] = useState(NO_INBOX)
  const [zone, setZone] = useState(() => Intl.DateTimeFormat().resolvedOptions().timeZone)
  const create = useCreateProperty()
  const hosts = splitLines(domains).map(hostOnly).filter(Boolean)
  return (
    <WizardFrame
      current="1"
      steps={steps}
      stepsLabel={t`Analytics steps`}
      preview={<Preview name={name} host={hosts[0] ?? ""} />}
    >
      <StepHeader title={<Trans>Add a website</Trans>}>
        <Trans>
          Count visitors, pages, sources, places and devices without cookies and without storing IP addresses.
        </Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate(
            {
              name: name.trim(),
              domains: hosts,
              inbox_id: inbox === NO_INBOX ? undefined : inbox,
              timezone: zones.includes(zone) ? zone : "UTC",
            },
            { onSuccess: (p) => navigate(`/setup/analytics/${p.id}`, { replace: true, state }) }
          )
        }}
        data-testid="analytics-new"
      >
        <Field label={<Trans>Name</Trans>} htmlFor="analytics-name">
          <Input
            id="analytics-name"
            autoFocus
            value={name}
            maxLength={100}
            onChange={(e) => setName(e.target.value)}
            placeholder={t`For example Fieldnote website`}
            data-testid="analytics-name"
          />
        </Field>
        <Field
          label={<Trans>Domains</Trans>}
          htmlFor="analytics-domains"
          hint={
            <Trans>
              One per line. Pages on other domains cannot send events. *.example.com covers every subdomain.
            </Trans>
          }
        >
          <Textarea
            id="analytics-domains"
            dir="ltr"
            rows={3}
            value={domains}
            onChange={(e) => setDomains(e.target.value)}
            placeholder="www.example.com"
            data-testid="analytics-domains"
          />
        </Field>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field
            label={<Trans>Inbox</Trans>}
            htmlFor="analytics-inbox"
            hint={<Trans>Agents of this inbox can see the numbers.</Trans>}
          >
            <ChoiceSelect
              id="analytics-inbox"
              label={t`Inbox`}
              value={inbox}
              onChange={setInbox}
              options={[[NO_INBOX, t`None, owners and admins only`], ...inboxes.map((i) => [i.id, i.name] as const)]}
              className="w-full"
            />
          </Field>
          <Field
            label={<Trans>Time zone</Trans>}
            htmlFor="analytics-zone"
            hint={<Trans>Where the property’s day starts and ends.</Trans>}
          >
            <Input id="analytics-zone" list="analytics-zones" value={zone} onChange={(e) => setZone(e.target.value)} />
            <datalist id="analytics-zones">
              {zones.map((z) => (
                <option key={z} value={z} />
              ))}
            </datalist>
          </Field>
        </div>
        <ErrorLine error={create.error} />
        <Button
          type="submit"
          size="lg"
          className="self-start"
          disabled={!name.trim() || hosts.length === 0 || create.isPending}
          data-testid="analytics-create"
        >
          <Trans>Add and install</Trans>
          <ArrowRightIcon className="rtl:rotate-180" />
        </Button>
      </form>
    </WizardFrame>
  )
}

export function WaitForFirstEvent({ propertyId }: { propertyId: string }) {
  const { i18n } = useLingui()
  const known = useProperty(propertyId).data?.first_event_at
  const wait = useWaitForFirstEvent(propertyId, !known)
  const first = known ?? wait.data?.first_event_at
  if (first) {
    const at = formatDateTime(first, i18n.locale)
    return (
      <Notice icon={CheckCircle2Icon} className="bg-success/8 [&>svg]:text-success" data-testid="first-event">
        <p className="font-medium text-foreground">
          <Trans>The first event arrived at {at}.</Trans>
        </p>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" render={<Link to={`/analytics/${propertyId}/realtime`} />}>
            <Trans>Watch it live</Trans>
          </Button>
          <Button size="sm" variant="outline" render={<Link to={`/analytics/${propertyId}`} />}>
            <Trans>Open the overview</Trans>
          </Button>
        </div>
      </Notice>
    )
  }
  return (
    <Notice data-testid="waiting-first-event">
      <p className="flex items-center gap-2 font-medium text-foreground">
        <Spinner className="size-4" />
        <Trans>Waiting for the first event</Trans>
      </p>
      <p>
        <Trans>
          Open a page of your site with the snippet in place. This turns green as soon as the server has stored the
          first page view. Browsers with Do Not Track or Global Privacy Control send nothing, so use another one to
          test.
        </Trans>
      </p>
      <ErrorLine error={wait.error} />
    </Notice>
  )
}

function InstallPropertyStep({ propertyId }: { propertyId: string }) {
  const { t } = useLingui()
  const steps = useSteps()
  const property = useProperty(propertyId)
  const p = property.data
  const wait = useWaitForFirstEvent(propertyId, !!p && !p.first_event_at)
  const arrived = !!(p?.first_event_at ?? wait.data?.first_event_at)
  return (
    <WizardFrame
      current={arrived ? "3" : "2"}
      steps={steps}
      stepsLabel={t`Analytics steps`}
      preview={<Preview name={p?.name ?? ""} host={p?.domains[0] ?? ""} />}
    >
      {!p ? (
        property.error ? (
          <ErrorLine error={property.error} />
        ) : (
          <Skeleton className="h-48 rounded-2xl" />
        )
      ) : (
        <div className="flex flex-col gap-6" data-testid="analytics-install-page">
          <StepHeader title={<Trans>Install on {p.name}</Trans>}>
            <Trans>
              Pick how the site is built and copy the snippet. The key is public, like a chat key: it only works on the
              property’s domains.
            </Trans>
          </StepHeader>
          <InstallSnippets property={p} />
          <WaitForFirstEvent propertyId={p.id} />
          {arrived && (
            <div>
              <BackHome variant="outline" />
            </div>
          )}
        </div>
      )}
    </WizardFrame>
  )
}

export default function AnalyticsSetup() {
  const { canManage } = useSession()
  const { propertyId } = useParams()
  if (!canManage) return <Navigate to="/" replace />
  return propertyId ? <InstallPropertyStep key={propertyId} propertyId={propertyId} /> : <NewPropertyStep />
}
