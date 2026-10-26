import { Trans, useLingui } from "@lingui/react/macro"
import { MailWarningIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link, useNavigate } from "react-router"

import { CopyButton, ErrorLine, Notice, toast, useConfirm } from "@/components/common"
import { LanguagePicker } from "@/components/common/LanguagePicker"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { NativeSelect } from "@/components/ui/native-select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { canSend } from "@/features/broadcasts/ListForm"
import { useInboxChannels } from "@/features/broadcasts/queries"
import { OwnDomainCard } from "@/features/settings/OwnDomainCard"
import { validSlug } from "@/features/status/CreatePageDialog"
import { useDeleteStatusPage, usePatchStatusPage } from "@/features/status/queries"
import { Card, ChoiceSelect, Field, Section, ToggleRow } from "@/features/settings/ui"
import type { StatusPage, StatusPageUpdate } from "@/lib/api"

const COLOR_RE = /^#[0-9a-fA-F]{6}$/

function zones(current: string) {
  const all =
    typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : ["UTC", "Europe/Istanbul"]
  return all.includes(current) ? all : [current, ...all]
}

function useAutosave(page: StatusPage) {
  const patch = usePatchStatusPage(page.id)
  const [saved, setSaved] = useState(0)
  const save = (body: StatusPageUpdate) => patch.mutate(body, { onSuccess: () => setSaved((n) => n + 1) })
  return { save, patch, saved }
}

type Autosave = ReturnType<typeof useAutosave>

function Commit({
  id,
  label,
  hint,
  value,
  onCommit,
  error,
  multiline,
  type,
  placeholder,
  maxLength,
  testId,
  spellCheck,
}: {
  id: string
  label: React.ReactNode
  hint?: React.ReactNode
  value: string
  onCommit: (v: string) => void
  error?: (v: string) => React.ReactNode
  multiline?: boolean
  type?: string
  placeholder?: string
  maxLength?: number
  testId: string
  spellCheck?: boolean
}) {
  const [text, setText] = useState(value)
  const [seen, setSeen] = useState(value)
  if (value !== seen) {
    setSeen(value)
    setText(value)
  }
  const problem = error?.(text)
  const commit = () => {
    if (text !== value && !problem) onCommit(text)
  }
  return (
    <Field label={label} htmlFor={id} hint={hint} error={problem || undefined}>
      {multiline ? (
        <Textarea
          id={id}
          value={text}
          maxLength={maxLength}
          onChange={(e) => setText(e.target.value)}
          onBlur={commit}
          data-testid={testId}
        />
      ) : (
        <Input
          id={id}
          type={type}
          value={text}
          maxLength={maxLength}
          placeholder={placeholder}
          spellCheck={spellCheck}
          autoCapitalize={spellCheck === false ? "none" : undefined}
          aria-invalid={!!problem}
          onChange={(e) => setText(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
          data-testid={testId}
        />
      )}
    </Field>
  )
}

function PageCard({ page, auto }: { page: StatusPage; auto: Autosave }) {
  const { save } = auto
  const link = (v: string) => (v.trim() ? v.trim() : null)
  const address = page.url
  return (
    <Section title={<Trans>Page</Trans>}>
      <Card>
        <Commit
          id="status-name"
          label={<Trans>Name</Trans>}
          value={page.name}
          maxLength={100}
          error={(v) => (v.trim() ? undefined : <Trans>Give the page a name.</Trans>)}
          onCommit={(v) => save({ name: v.trim() })}
          testId="status-setting-name"
        />
        <Commit
          id="status-description"
          label={<Trans>Description</Trans>}
          hint={<Trans>Shown under the name on the public page.</Trans>}
          value={page.description}
          multiline
          maxLength={1000}
          onCommit={(v) => save({ description: v.trim() })}
          testId="status-setting-description"
        />
        <Commit
          id="status-slug"
          label={<Trans>Address</Trans>}
          value={page.slug}
          maxLength={48}
          spellCheck={false}
          hint={<Trans>The page is at {address}. The old address keeps redirecting if you change it.</Trans>}
          error={(v) =>
            v && !validSlug(v) ? <Trans>Use 3 to 48 letters, numbers and inner hyphens.</Trans> : undefined
          }
          onCommit={(v) => save({ slug: v.toLowerCase() })}
          testId="status-setting-slug"
        />
        <div className="grid gap-5 sm:grid-cols-2">
          <Commit
            id="status-logo"
            label={<Trans>Logo URL</Trans>}
            hint={<Trans>An https image.</Trans>}
            type="url"
            placeholder="https://"
            value={page.logo_url ?? ""}
            onCommit={(v) => save({ logo_url: link(v) })}
            testId="status-setting-logo"
          />
          <Commit
            id="status-logo-dark"
            label={<Trans>Dark mode logo URL</Trans>}
            hint={<Trans>Shown instead of the logo in dark mode.</Trans>}
            type="url"
            placeholder="https://"
            value={page.logo_dark_url ?? ""}
            onCommit={(v) => save({ logo_dark_url: link(v) })}
            testId="status-setting-logo-dark"
          />
          <Accent page={page} onCommit={(v) => save({ accent_color: link(v) })} />
          <Commit
            id="status-site"
            label={<Trans>Site link</Trans>}
            hint={<Trans>Your product.</Trans>}
            type="url"
            placeholder="https://"
            value={page.site_url ?? ""}
            onCommit={(v) => save({ site_url: link(v) })}
            testId="status-setting-site"
          />
          <Commit
            id="status-support"
            label={<Trans>Support link</Trans>}
            hint={<Trans>A page or a mailto: link where customers get help.</Trans>}
            placeholder="https://"
            value={page.support_url ?? ""}
            onCommit={(v) => save({ support_url: link(v) })}
            testId="status-setting-support"
          />
        </div>
      </Card>
    </Section>
  )
}

function Accent({ page, onCommit }: { page: StatusPage; onCommit: (v: string) => void }) {
  const { t } = useLingui()
  const value = page.accent_color ?? ""
  const [color, setColor] = useState(value)
  const [seen, setSeen] = useState(value)
  if (value !== seen) {
    setSeen(value)
    setColor(value)
  }
  const ok = !color || COLOR_RE.test(color)
  const commit = () => {
    if (color !== value && ok) onCommit(color)
  }
  return (
    <Field
      label={<Trans>Accent colour</Trans>}
      htmlFor="status-color"
      error={!ok ? <Trans>Use a colour like #2563eb.</Trans> : undefined}
    >
      <div className="flex items-center gap-2">
        <input
          type="color"
          aria-label={t`Pick a colour`}
          value={COLOR_RE.test(color) ? color : "#2563eb"}
          onChange={(e) => setColor(e.target.value)}
          onBlur={commit}
          className="size-9 shrink-0 cursor-pointer rounded-xl border border-input bg-background p-1"
        />
        <Input
          id="status-color"
          value={color}
          maxLength={7}
          placeholder="#2563eb"
          spellCheck={false}
          aria-invalid={!ok}
          onChange={(e) => setColor(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
          data-testid="status-setting-color"
        />
      </div>
    </Field>
  )
}

function DisplayCard({ page, auto }: { page: StatusPage; auto: Autosave }) {
  const { t } = useLingui()
  const { save, patch } = auto
  return (
    <Section title={<Trans>Display</Trans>}>
      <Card>
        <Field
          label={<Trans>Language</Trans>}
          htmlFor="status-locale"
          hint={<Trans>The language of the page’s words.</Trans>}
        >
          <LanguagePicker
            id="status-locale"
            value={page.locale}
            onChange={(locale) => locale !== page.locale && save({ locale })}
            label={t`Language`}
            className="sm:max-w-xs"
            testId="status-setting-locale"
          />
        </Field>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field
            label={<Trans>Time zone</Trans>}
            htmlFor="status-timezone"
            hint={<Trans>The zone of the day bars.</Trans>}
          >
            <NativeSelect
              id="status-timezone"
              className="w-full"
              value={page.timezone}
              onChange={(e) => save({ timezone: e.target.value })}
              data-testid="status-setting-timezone"
            >
              {zones(page.timezone).map((z) => (
                <option key={z} value={z}>
                  {z}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Commit
            id="status-days"
            label={<Trans>Days of history</Trans>}
            hint={<Trans>7 to 90.</Trans>}
            type="number"
            value={String(page.history_days)}
            error={(v) =>
              Number.isInteger(Number(v)) && Number(v) >= 7 && Number(v) <= 90 ? undefined : (
                <Trans>Use a whole number from 7 to 90.</Trans>
              )
            }
            onCommit={(v) => save({ history_days: Number(v) })}
            testId="status-setting-days"
          />
        </div>
        <ToggleRow title={<Trans>Show uptime</Trans>} hint={<Trans>A percentage next to each component.</Trans>}>
          <Switch
            checked={page.show_uptime}
            disabled={patch.isPending}
            onCheckedChange={(show_uptime) => save({ show_uptime })}
            data-testid="status-setting-uptime"
          />
        </ToggleRow>
        <ToggleRow
          title={<Trans>Show response times</Trans>}
          hint={<Trans>The last 24 hours of components with an http or tcp monitor.</Trans>}
        >
          <Switch
            checked={page.show_response_times}
            disabled={patch.isPending}
            onCheckedChange={(show_response_times) => save({ show_response_times })}
            data-testid="status-setting-response-times"
          />
        </ToggleRow>
        <ToggleRow
          title={<Trans>Show the name next to the logo</Trans>}
          hint={<Trans>Turn off when the logo already contains the name.</Trans>}
        >
          <Switch
            checked={page.show_name}
            disabled={patch.isPending}
            onCheckedChange={(show_name) => save({ show_name })}
            data-testid="status-setting-show-name"
          />
        </ToggleRow>
        <ToggleRow
          title={<Trans>Show feed, badge and embed</Trans>}
          hint={<Trans>The Follow and embed section and the feed and badge links.</Trans>}
        >
          <Switch
            checked={page.show_follow}
            disabled={patch.isPending}
            onCheckedChange={(show_follow) => save({ show_follow })}
            data-testid="status-setting-follow"
          />
        </ToggleRow>
      </Card>
    </Section>
  )
}

function EmailSubscribers({ page }: { page: StatusPage }) {
  const { t } = useLingui()
  const patch = usePatchStatusPage(page.id)
  const channels = (useInboxChannels(page.inbox_id).data ?? []).filter(canSend)
  const sub = page.email_subscribers
  const options: [string, string][] = channels.map((c) => [c.id, c.email ? `${c.name} · ${c.email.address}` : c.name])
  const channelId = sub.channel_id ?? ""
  if (channelId && !options.some(([id]) => id === channelId)) options.unshift([channelId, channelId])
  const warnings: Record<(typeof sub.warnings)[number], string> = {
    no_deliverability_check: t`The sending domain has not passed a check of SPF, DKIM and DMARC yet. Notices may land in spam.`,
    no_postal_address: t`The inbox names no postal address for the sender. Add one in the inbox settings.`,
    dkim_unaligned: t`Mail is not signed with a key of the sender’s own domain, which Gmail and Yahoo expect.`,
  }
  return (
    <Section
      title={<Trans>E-mail subscribers</Trans>}
      description={
        <Trans>Visitors subscribe on the page and get a mail when an incident or maintenance changes.</Trans>
      }
    >
      <Card>
        <ToggleRow
          title={<Trans>Send notices by e-mail</Trans>}
          hint={<Trans>Subscribers confirm their address by mail before they get notices.</Trans>}
        >
          <Switch
            checked={sub.enabled}
            disabled={patch.isPending}
            onCheckedChange={(enabled) =>
              patch.mutate({
                email_subscribers: {
                  enabled,
                  ...(enabled && !sub.channel_id && channels.length === 1 ? { channel_id: channels[0].id } : {}),
                },
              })
            }
            data-testid="status-setting-subscribers"
          />
        </ToggleRow>
        <Field
          label={<Trans>Sends its mail from</Trans>}
          htmlFor="status-channel"
          hint={
            channels.length === 0 ? (
              <Trans>This inbox has no e-mail channel that can send. Add an SMTP account in the inbox settings.</Trans>
            ) : (
              <Trans>The inbox’s e-mail channel that sends confirmations and notices.</Trans>
            )
          }
        >
          <ChoiceSelect
            id="status-channel"
            value={channelId}
            onChange={(id) => patch.mutate({ email_subscribers: { channel_id: id } })}
            options={options}
            placeholder={t`Choose a channel`}
            label={t`Sends its mail from`}
            disabled={patch.isPending || options.length === 0}
          />
        </Field>
        {sub.enabled && !sub.sender_ready && (
          <Notice tone="warning" icon={MailWarningIcon} data-testid="status-subscribers-no-sender">
            <p>
              <Trans>
                No channel that can send is chosen. Until there is one, nobody gets a confirmation or a notice.
              </Trans>
            </p>
            <Link
              to={`/settings/inboxes/${page.inbox_id}`}
              className="w-fit text-brand underline-offset-4 hover:underline"
            >
              <Trans>Open the inbox settings</Trans>
            </Link>
          </Notice>
        )}
        {sub.warnings.map((w) => (
          <Notice key={w} tone="warning" icon={MailWarningIcon} data-testid="status-subscribers-warning">
            <p>{warnings[w]}</p>
          </Notice>
        ))}
        <ErrorLine error={patch.error} />
      </Card>
    </Section>
  )
}

function Address({ page }: { page: StatusPage }) {
  return (
    <Section title={<Trans>Public address</Trans>}>
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          {page.published ? (
            <a
              href={page.url}
              target="_blank"
              rel="noreferrer noopener"
              className="min-w-0 flex-1 break-all text-body text-brand underline-offset-4 hover:underline"
              data-testid="status-public-url"
            >
              {page.url}
            </a>
          ) : (
            <span className="min-w-0 flex-1 break-all text-body text-muted-foreground" data-testid="status-public-url">
              {page.url}
            </span>
          )}
          <CopyButton value={page.url} />
        </div>
        {!page.published && (
          <p className="text-small text-faint">
            <Trans>The address answers “not found” until you publish the page.</Trans>
          </p>
        )}
      </Card>
    </Section>
  )
}

function Danger({ page }: { page: StatusPage }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [confirm, dialog] = useConfirm()
  const remove = useDeleteStatusPage(page.id)
  return (
    <>
      <Section title={<Trans>Delete</Trans>}>
        <Card tone="danger">
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-0 flex-1 text-body">
              <Trans>
                Deletes the page with its components, their history, incidents and updates. The public address stops
                working.
              </Trans>
            </p>
            <Button
              variant="destructive"
              size="sm"
              disabled={remove.isPending}
              onClick={() =>
                confirm({
                  title: <Trans>Delete {page.name}?</Trans>,
                  description: <Trans>Components, history, incidents and updates are deleted with it.</Trans>,
                  confirm: <Trans>Delete page</Trans>,
                  run: () =>
                    remove.mutate(undefined, {
                      onSuccess: () => {
                        toast(t`Status page deleted`)
                        navigate("/status")
                      },
                    }),
                })
              }
              data-testid="status-delete"
            >
              <Trans>Delete page</Trans>
            </Button>
          </div>
          <ErrorLine error={remove.error} />
        </Card>
      </Section>
      {dialog}
    </>
  )
}

export function SettingsTab({ page }: { page: StatusPage }) {
  const auto = useAutosave(page)
  const domainSave = usePatchStatusPage(page.id)
  const [shown, setShown] = useState(false)
  const [seen, setSeen] = useState(0)
  if (auto.saved !== seen) {
    setSeen(auto.saved)
    setShown(true)
  }
  useEffect(() => {
    if (!shown) return
    const timer = window.setTimeout(() => setShown(false), 2500)
    return () => window.clearTimeout(timer)
  }, [shown, seen])
  return (
    <div className="flex flex-col gap-6" data-testid="status-settings">
      <div className="relative -mb-6 h-0">
        <div className="absolute inset-x-0 bottom-1 flex items-center justify-end">
          {auto.patch.error ? (
            <ErrorLine error={auto.patch.error} />
          ) : (
            shown && (
              <span className="text-body text-muted-foreground" role="status" data-testid="status-saved">
                <Trans>Saved</Trans>
              </span>
            )
          )}
        </div>
      </div>
      <PageCard page={page} auto={auto} />
      <DisplayCard page={page} auto={auto} />
      <EmailSubscribers page={page} />
      <OwnDomainCard
        domain={page.custom_domain}
        enabled={!!page.own_domains}
        example="status.example.com"
        tid="status-domain"
        save={domainSave}
      />
      <Address page={page} />
      <Danger page={page} />
    </div>
  )
}
