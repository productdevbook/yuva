import { Trans, useLingui } from "@lingui/react/macro"
import { TriangleAlertIcon } from "lucide-react"
import { useRef, useState } from "react"

import { Notice, toast, useConfirm } from "@/components/common"
import { formatDateTime, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { FormBlock } from "@/features/settings/channels/parts"
import { PageSection, usePatchChannel } from "@/features/settings/channels/sections"
import { ChoiceSelect, Field, FormActions, FormCard, ToggleRow } from "@/features/settings/ui"
import type { AppPushInput, Channel } from "@/lib/api"

const GUIDE = "https://github.com/productdevbook/yuva/blob/main/docs/mobile.md#push-sent-by-yuva"

type Environment = "sandbox" | "production"

function SecretInput({
  id,
  label,
  placeholder,
  accept,
  value,
  onChange,
}: {
  id: string
  label: React.ReactNode
  placeholder: string
  accept: string
  value: string
  onChange: (value: string) => void
}) {
  const file = useRef<HTMLInputElement>(null)
  return (
    <Field label={label} htmlFor={id}>
      <Textarea
        id={id}
        rows={5}
        spellCheck={false}
        autoComplete="off"
        dir="ltr"
        className="font-mono text-caption"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <input
        ref={file}
        type="file"
        accept={accept}
        className="hidden"
        tabIndex={-1}
        onChange={(e) => {
          const picked = e.target.files?.[0]
          e.target.value = ""
          if (picked) void picked.text().then(onChange)
        }}
      />
      <div>
        <Button type="button" variant="outline" size="sm" onClick={() => file.current?.click()}>
          <Trans>Upload file</Trans>
        </Button>
      </div>
    </Field>
  )
}

export function PushSection({ channel }: { channel: Channel }) {
  const { t, i18n } = useLingui()
  const errorText = useErrorText()
  const patch = usePatchChannel(channel)
  const [confirm, confirmDialog] = useConfirm()
  const push = channel.app!.push
  const [enabled, setEnabled] = useState(push.enabled)
  const [showContent, setShowContent] = useState(push.show_content)
  const [keyId, setKeyId] = useState(push.apns.key_id ?? "")
  const [teamId, setTeamId] = useState(push.apns.team_id ?? "")
  const [bundleId, setBundleId] = useState(push.apns.bundle_id ?? "")
  const [environment, setEnvironment] = useState<Environment>(push.apns.environment ?? "production")
  const [p8, setP8] = useState("")
  const [replaceP8, setReplaceP8] = useState(false)
  const [account, setAccount] = useState("")
  const [replaceAccount, setReplaceAccount] = useState(false)

  const apnsShown = !push.apns.set || replaceP8
  const fcmShown = !push.fcm.set || replaceAccount
  const apnsTouched =
    p8.trim() !== "" ||
    (push.apns.set &&
      (keyId.trim() !== (push.apns.key_id ?? "") ||
        teamId.trim() !== (push.apns.team_id ?? "") ||
        bundleId.trim() !== (push.apns.bundle_id ?? "") ||
        environment !== (push.apns.environment ?? "production")))
  const dirty = enabled !== push.enabled || showContent !== push.show_content || apnsTouched || account.trim() !== ""

  const reset = () => {
    setP8("")
    setReplaceP8(false)
    setAccount("")
    setReplaceAccount(false)
  }

  const save = () => {
    const input: AppPushInput = { enabled, show_content: showContent }
    if (apnsTouched)
      input.apns = {
        key_id: keyId.trim(),
        team_id: teamId.trim(),
        bundle_id: bundleId.trim(),
        environment,
        ...(p8.trim() ? { private_key: p8.trim() } : {}),
      }
    if (account.trim()) input.fcm = { service_account: account.trim() }
    patch.mutate({ app: { push: input } }, { onSuccess: reset })
  }

  const remove = (input: AppPushInput) =>
    patch.mutate({ app: { push: input } }, { onSuccess: () => toast(t`Saved`), onError: (e) => toast(errorText(e)) })

  return (
    <PageSection
      id="push"
      title={<Trans>Push notifications</Trans>}
      description={
        <Trans>
          Let Yuva send the reply push to your app's users.{" "}
          <a href={GUIDE} target="_blank" rel="noreferrer" className="underline underline-offset-2">
            Read the guide
          </a>
        </Trans>
      }
    >
      <FormCard
        onSubmit={save}
        footer={
          <FormActions
            pending={patch.isPending}
            saved={patch.isSuccess && !dirty}
            error={patch.error}
            disabled={!dirty}
          />
        }
      >
        <ToggleRow
          title={<Trans>Send push notifications from Yuva</Trans>}
          hint={
            <Trans>
              Off: your backend sends its own push from the message webhook, as before. On: Yuva pushes a reply to the
              contact's devices when the app is not connected.
            </Trans>
          }
        >
          <Switch
            checked={enabled}
            onCheckedChange={(v) => {
              patch.reset()
              setEnabled(v)
            }}
            aria-label={t`Send push notifications from Yuva`}
          />
        </ToggleRow>
        <ToggleRow
          title={<Trans>Show message text in the notification</Trans>}
          hint={<Trans>Off: the notification only says there is a new reply, in the contact's language.</Trans>}
        >
          <Switch
            checked={showContent}
            onCheckedChange={(v) => {
              patch.reset()
              setShowContent(v)
            }}
            aria-label={t`Show message text in the notification`}
          />
        </ToggleRow>

        {push.last_error && (
          <Notice tone="warning" icon={TriangleAlertIcon} data-testid="push-last-error">
            <p>{push.last_error}</p>
            {push.last_error_at && (
              <p className="text-small text-faint">{formatDateTime(push.last_error_at, i18n.locale)}</p>
            )}
          </Notice>
        )}

        <FormBlock title={<Trans>iOS (APNs)</Trans>}>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={<Trans>Key ID</Trans>} htmlFor="push-key-id">
              <Input
                id="push-key-id"
                dir="ltr"
                autoComplete="off"
                value={keyId}
                onChange={(e) => setKeyId(e.target.value)}
              />
            </Field>
            <Field label={<Trans>Team ID</Trans>} htmlFor="push-team-id">
              <Input
                id="push-team-id"
                dir="ltr"
                autoComplete="off"
                value={teamId}
                onChange={(e) => setTeamId(e.target.value)}
              />
            </Field>
            <Field label={<Trans>Bundle ID</Trans>} htmlFor="push-bundle-id">
              <Input
                id="push-bundle-id"
                dir="ltr"
                autoComplete="off"
                value={bundleId}
                onChange={(e) => setBundleId(e.target.value)}
              />
            </Field>
            <Field label={<Trans>Environment</Trans>} htmlFor="push-environment">
              <ChoiceSelect<Environment>
                id="push-environment"
                value={environment}
                onChange={setEnvironment}
                options={[
                  ["production", t`Production`],
                  ["sandbox", t`Sandbox (development builds)`],
                ]}
              />
            </Field>
          </div>
          {apnsShown ? (
            <SecretInput
              id="push-p8"
              label={<Trans>Token key (.p8)</Trans>}
              placeholder="-----BEGIN PRIVATE KEY-----"
              accept=".p8,.pem,text/plain"
              value={p8}
              onChange={setP8}
            />
          ) : (
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-body font-medium">
                <Trans>Key set</Trans>
              </span>
              <Button type="button" variant="outline" size="sm" onClick={() => setReplaceP8(true)}>
                <Trans>Replace</Trans>
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  confirm({
                    title: t`Remove the APNs key?`,
                    description: t`iOS devices get no push from Yuva until you add a key again.`,
                    confirm: t`Remove`,
                    run: () => remove({ apns: { private_key: "" } }),
                  })
                }
              >
                <Trans>Remove</Trans>
              </Button>
            </div>
          )}
        </FormBlock>

        <FormBlock title={<Trans>Android (FCM)</Trans>}>
          {fcmShown ? (
            <SecretInput
              id="push-fcm"
              label={<Trans>Service account (JSON)</Trans>}
              placeholder='{"type": "service_account", …}'
              accept=".json,application/json"
              value={account}
              onChange={setAccount}
            />
          ) : (
            <div className="flex flex-wrap items-center gap-3">
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="text-body font-medium">
                  <Trans>Service account set</Trans>
                </span>
                <span className="truncate text-small text-faint" dir="ltr">
                  {push.fcm.project_id}
                </span>
                <span className="truncate text-small text-faint" dir="ltr">
                  {push.fcm.client_email}
                </span>
              </div>
              <Button type="button" variant="outline" size="sm" onClick={() => setReplaceAccount(true)}>
                <Trans>Replace</Trans>
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  confirm({
                    title: t`Remove the FCM service account?`,
                    description: t`Android devices get no push from Yuva until you add one again.`,
                    confirm: t`Remove`,
                    run: () => remove({ fcm: { service_account: "" } }),
                  })
                }
              >
                <Trans>Remove</Trans>
              </Button>
            </div>
          )}
        </FormBlock>
      </FormCard>
      {confirmDialog}
    </PageSection>
  )
}
