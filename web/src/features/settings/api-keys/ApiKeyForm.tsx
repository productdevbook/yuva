import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { BotIcon, EyeIcon, SlidersHorizontalIcon, SparklesIcon } from "lucide-react"
import { useState } from "react"

import { CheckItem, CheckList, Dot } from "@/components/common"
import { CodeLine } from "@/components/common/CopyButton"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import { RadioChoices } from "@/components/common/RadioChoices"
import { SecretCard } from "@/components/common/SecretCard"
import {
  API_KEY_PRESET_SCOPES,
  API_KEY_SCOPES,
  OWNER_ONLY_SCOPES,
  useEnumText,
  WORKSPACE_WIDE_SCOPES,
} from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { chosenScopes, type ApiKeyDraft } from "@/features/settings/api-keys/draft"
import { Field } from "@/features/settings/ui"
import { idempotencyKey } from "@/features/setup/draft"
import { api, unwrap, type ApiKeyCreate, type ApiKeyScope } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

function toggled<T>(list: T[], v: T, on: boolean) {
  const rest = list.filter((x) => x !== v)
  return on ? [...rest, v] : rest
}

function localDate(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function ApiKeyForm({ onClose, onCreated }: { onClose: () => void; onCreated: (secret: string) => void }) {
  const { t } = useLingui()
  const text = useEnumText()
  const qc = useQueryClient()
  const { workspaceId: ws, isOwner } = useSession()
  const inboxes = useInboxes().data ?? []
  const [draft, setDraft] = useState<ApiKeyDraft>({
    name: "",
    botName: "",
    avatar: "",
    inboxIds: [],
    preset: "support",
    custom: API_KEY_PRESET_SCOPES.everything,
  })
  const [expiry, setExpiry] = useState("")
  const [today] = useState(() => localDate(new Date()))
  const [key] = useState(() => crypto.randomUUID())
  const patch = (p: Partial<ApiKeyDraft>) => setDraft((d) => ({ ...d, ...p }))
  const limited = draft.inboxIds.length > 0
  const scopes = chosenScopes(draft, isOwner)
  const allowed = (s: ApiKeyScope) =>
    (!limited || !WORKSPACE_WIDE_SCOPES.includes(s)) && (isOwner || !OWNER_ONLY_SCOPES.includes(s))
  const create = useMutation({
    mutationFn: () => {
      const body: ApiKeyCreate = { name: draft.name.trim(), scopes }
      if (limited) body.inbox_ids = draft.inboxIds
      if (expiry) body.expires_at = new Date(`${expiry}T23:59:59`).toISOString()
      if (draft.botName.trim()) body.bot_name = draft.botName.trim()
      if (draft.avatar.trim()) body.bot_avatar_url = draft.avatar.trim()
      return unwrap(
        api.POST("/v1/api-keys", {
          body,
          params: { header: { "Idempotency-Key": idempotencyKey(key, "api-key", body) } },
        })
      )
    },
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: keys.apiKeys(ws) })
      onCreated(r.secret)
    },
  })
  return (
    <FormCard
      title={<Trans>New API key</Trans>}
      description={<Trans>Scopes and inboxes cannot be changed later. For different ones, create a new key.</Trans>}
      submit={<Trans>Create key</Trans>}
      ready={!!draft.name.trim() && scopes.length > 0}
      busy={create.isPending}
      error={create.error}
      onSubmit={() => create.mutate()}
      onCancel={onClose}
      testId="api-key-form"
    >
      <Field
        label={<Trans>Name</Trans>}
        htmlFor="key-name"
        hint={<Trans>Name it after the backend, script or bot that will use it.</Trans>}
      >
        <Input
          id="key-name"
          autoFocus
          required
          maxLength={200}
          value={draft.name}
          placeholder={t`Production backend`}
          onChange={(e) => patch({ name: e.target.value })}
        />
      </Field>
      <FormGrid>
        <Field
          label={<Trans>Bot name (optional)</Trans>}
          htmlFor="key-bot-name"
          hint={<Trans>Shown on its messages. The key's name when empty.</Trans>}
        >
          <Input
            id="key-bot-name"
            maxLength={200}
            value={draft.botName}
            placeholder={draft.name.trim() || t`Billing bot`}
            onChange={(e) => patch({ botName: e.target.value })}
          />
        </Field>
        <Field
          label={<Trans>Bot avatar (optional)</Trans>}
          htmlFor="key-bot-avatar"
          hint={<Trans>An https image URL.</Trans>}
        >
          <Input
            id="key-bot-avatar"
            type="url"
            pattern="https://.*"
            maxLength={2000}
            className="font-mono text-caption"
            value={draft.avatar}
            placeholder="https://"
            onChange={(e) => patch({ avatar: e.target.value })}
          />
        </Field>
      </FormGrid>
      {inboxes.length > 0 && (
        <CheckList
          labelId="key-inboxes"
          label={<Trans>Inboxes</Trans>}
          hint={
            <p className="text-small text-faint">
              {limited ? (
                <Trans>
                  The key sees only these inboxes and cannot manage inboxes, webhooks or the workspace, or read the
                  audit log.
                </Trans>
              ) : (
                <Trans>None picked: the key sees every inbox.</Trans>
              )}
            </p>
          }
        >
          {inboxes.map((i) => (
            <CheckItem
              key={i.id}
              checked={draft.inboxIds.includes(i.id)}
              onChange={(on) => patch({ inboxIds: toggled(draft.inboxIds, i.id, on) })}
              testId="key-inbox"
            >
              <span className="flex items-center gap-2 text-body">
                <Dot color={i.branding.color} className="size-2" />
                <span className="truncate">{i.name}</span>
              </span>
            </CheckItem>
          ))}
        </CheckList>
      )}
      <Field label={<Trans>Access</Trans>}>
        <RadioChoices
          id="key-preset"
          label={t`Access`}
          value={draft.preset}
          onChange={(v) => patch({ preset: v })}
          columns
          items={[
            {
              value: "read",
              icon: <EyeIcon />,
              title: <Trans>Read only</Trans>,
              hint: (
                <Trans>
                  Look at conversations, contacts, inboxes, lists, broadcasts and analytics. Change nothing.
                </Trans>
              ),
              testId: "key-preset-read",
            },
            {
              value: "support",
              icon: <BotIcon />,
              title: <Trans>Support bot</Trans>,
              hint: <Trans>Answer and organise conversations, write notes, keep contacts and labels up to date.</Trans>,
              testId: "key-preset-support",
            },
            {
              value: "everything",
              icon: <SparklesIcon />,
              title: <Trans>Everything</Trans>,
              hint: <Trans>Every scope except sending broadcasts and reading the audit log.</Trans>,
              testId: "key-preset-everything",
            },
            {
              value: "custom",
              icon: <SlidersHorizontalIcon />,
              title: <Trans>Custom</Trans>,
              hint: <Trans>Pick each scope yourself.</Trans>,
              testId: "key-preset-custom",
            },
          ]}
        />
      </Field>
      {draft.preset === "custom" && (
        <CheckList
          labelId="key-scopes"
          label={<Trans>Scopes</Trans>}
          hint={
            scopes.length === 0 && (
              <p role="alert" className="text-small text-destructive">
                <Trans>Pick at least one scope.</Trans>
              </p>
            )
          }
        >
          {API_KEY_SCOPES.map((s) => (
            <CheckItem
              key={s}
              checked={draft.custom.includes(s) && allowed(s)}
              disabled={!allowed(s)}
              onChange={(on) => patch({ custom: toggled(draft.custom, s, on) })}
              testId="key-scope"
            >
              <span className="text-body">{text.scope[s]}</span>
              <code className="font-mono text-caption text-faint">{s}</code>
            </CheckItem>
          ))}
        </CheckList>
      )}
      <Field
        label={<Trans>Expires (optional)</Trans>}
        htmlFor="key-expiry"
        hint={<Trans>The key stops working at the end of this day.</Trans>}
      >
        <Input
          id="key-expiry"
          type="date"
          min={today}
          value={expiry}
          onChange={(e) => setExpiry(e.target.value)}
          className="sm:max-w-48"
        />
      </Field>
    </FormCard>
  )
}

export function ApiKeySecret({ secret, onDone }: { secret: string; onDone: () => void }) {
  const curl = `curl -H "Authorization: Bearer ${secret}" ${window.location.origin}/v1/inboxes`
  return (
    <SecretCard
      title={<Trans>The key is ready</Trans>}
      description={<Trans>Use it as a bearer token against this server.</Trans>}
      label={<Trans>Secret</Trans>}
      secret={secret}
      hint={<Trans>Shown only now. Copy it and store it safely.</Trans>}
      testId="api-key-created"
      actions={
        <Button onClick={onDone} data-testid="api-key-created-done">
          <Trans>Done</Trans>
        </Button>
      }
    >
      <CodeLine value={curl} testId="api-key-curl" />
    </SecretCard>
  )
}
