import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { BotAvatar, CheckItem, CheckList, Dot, ErrorLine } from "@/components/common"
import { API_KEY_SCOPES, useEnumText, WORKSPACE_WIDE_SCOPES } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Field } from "@/features/settings/ui"
import { api, unwrap, type ApiKey, type ApiKeyCreate, type ApiKeyScope } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

function localDate(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function toggled<T>(set: Set<T>, v: T, on: boolean) {
  const next = new Set(set)
  if (on) next.add(v)
  else next.delete(v)
  return next
}

export function ApiKeyDialog({ apiKey, onClose, onCreated }: { apiKey: ApiKey | null; onClose: () => void; onCreated: (secret: string) => void }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const text = useEnumText()
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data ?? []
  const [name, setName] = useState(apiKey?.name ?? "")
  const [botName, setBotName] = useState(apiKey?.bot_name ?? "")
  const [avatar, setAvatar] = useState(apiKey?.bot_avatar_url ?? "")
  const [scopes, setScopes] = useState<Set<ApiKeyScope>>(() => new Set(API_KEY_SCOPES))
  const [limit, setLimit] = useState<Set<string>>(() => new Set())
  const [expiry, setExpiry] = useState("")
  const [touched, setTouched] = useState(false)
  const limited = limit.size > 0
  const allowed = (s: ApiKeyScope) => !limited || !WORKSPACE_WIDE_SCOPES.includes(s)
  const chosen = API_KEY_SCOPES.filter((s) => scopes.has(s) && allowed(s))
  const save = useMutation({
    mutationFn: async () => {
      const bot = { bot_name: botName.trim(), bot_avatar_url: avatar.trim() }
      if (apiKey) {
        await unwrap(
          api.PATCH("/v1/api-keys/{apiKeyId}", {
            params: { path: { apiKeyId: apiKey.id } },
            body: { name: name.trim(), bot_name: bot.bot_name || null, bot_avatar_url: bot.bot_avatar_url || null },
          }),
        )
        return null
      }
      const body: ApiKeyCreate = { name: name.trim(), scopes: chosen }
      if (limited) body.inbox_ids = inboxes.filter((i) => limit.has(i.id)).map((i) => i.id)
      if (expiry) body.expires_at = new Date(`${expiry}T23:59:59`).toISOString()
      if (bot.bot_name) body.bot_name = bot.bot_name
      if (bot.bot_avatar_url) body.bot_avatar_url = bot.bot_avatar_url
      return (await unwrap(api.POST("/v1/api-keys", { body }))).secret
    },
    onSuccess: (secret) => {
      void qc.invalidateQueries({ queryKey: keys.apiKeys(ws) })
      if (secret) onCreated(secret)
      else onClose()
    },
  })
  const preview = botName.trim() || name.trim() || t`Bot`
  const avatarUrl = avatar.trim()
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] p-0 sm:max-w-lg">
        <form
          className="flex min-w-0 flex-col"
          onSubmit={(e) => {
            e.preventDefault()
            setTouched(true)
            if (apiKey || chosen.length > 0) save.mutate()
          }}
        >
          <div className="flex flex-col gap-5 p-6">
            <DialogHeader>
              <DialogTitle>{apiKey ? <Trans>Edit API key</Trans> : <Trans>New API key</Trans>}</DialogTitle>
              <DialogDescription>
                {apiKey ? (
                  <Trans>Scopes, inboxes and expiry stay as they are. For different ones, create a new key.</Trans>
                ) : (
                  <Trans>Scopes, inboxes and expiry cannot be changed later.</Trans>
                )}
              </DialogDescription>
            </DialogHeader>
            <Field label={<Trans>Name</Trans>} htmlFor="key-name">
              <Input id="key-name" required maxLength={200} value={name} placeholder={t`Production backend`} onChange={(e) => setName(e.target.value)} />
            </Field>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field label={<Trans>Bot name</Trans>} htmlFor="key-bot-name" hint={<Trans>Shown on its messages. The key's name when empty.</Trans>}>
                <Input id="key-bot-name" maxLength={200} value={botName} placeholder={name.trim() || t`Billing bot`} onChange={(e) => setBotName(e.target.value)} />
              </Field>
              <Field label={<Trans>Bot avatar</Trans>} htmlFor="key-bot-avatar" hint={<Trans>An https image URL.</Trans>}>
                <div className="flex items-center gap-2">
                  <BotAvatar name={preview} url={avatarUrl.startsWith("https://") ? avatarUrl : undefined} className="size-8" />
                  <Input
                    id="key-bot-avatar"
                    type="url"
                    pattern="https://.*"
                    maxLength={2000}
                    value={avatar}
                    placeholder="https://"
                    onChange={(e) => setAvatar(e.target.value)}
                    className="min-w-0 font-mono text-caption"
                  />
                </div>
              </Field>
            </div>
            {!apiKey && (
              <>
                <CheckList
                  labelId="key-inboxes"
                  label={<Trans>Inboxes</Trans>}
                  hint={
                    <p className="text-caption text-muted-foreground">
                      {limited ? (
                        <Trans>The key sees only these inboxes and cannot manage inboxes, webhooks or the workspace.</Trans>
                      ) : (
                        <Trans>None picked: the key sees every inbox.</Trans>
                      )}
                    </p>
                  }
                >
                  {inboxes.map((i) => (
                    <CheckItem key={i.id} checked={limit.has(i.id)} onChange={(on) => setLimit((s) => toggled(s, i.id, on))} testId="key-inbox">
                      <span className="flex items-center gap-2 text-body">
                        <Dot color={i.branding.color} className="size-2" />
                        <span className="truncate">{i.name}</span>
                      </span>
                    </CheckItem>
                  ))}
                </CheckList>
                <CheckList
                  labelId="key-scopes"
                  label={<Trans>Scopes</Trans>}
                  hint={
                    touched &&
                    chosen.length === 0 && (
                      <p role="alert" className="text-caption text-destructive">
                        <Trans>Pick at least one scope.</Trans>
                      </p>
                    )
                  }
                >
                  {API_KEY_SCOPES.map((s) => (
                    <CheckItem
                      key={s}
                      checked={scopes.has(s) && allowed(s)}
                      disabled={!allowed(s)}
                      onChange={(on) => setScopes((x) => toggled(x, s, on))}
                      testId="key-scope"
                    >
                      <span className="text-body">{text.scope[s]}</span>
                      <code className="font-mono text-caption text-faint">{s}</code>
                    </CheckItem>
                  ))}
                </CheckList>
                <Field label={<Trans>Expires (optional)</Trans>} htmlFor="key-expiry" hint={<Trans>The key stops working at the end of this day.</Trans>}>
                  <Input id="key-expiry" type="date" min={localDate(new Date())} value={expiry} onChange={(e) => setExpiry(e.target.value)} className="sm:max-w-48" />
                </Field>
              </>
            )}
            <ErrorLine error={save.error} />
          </div>
          <DialogFooter className="sticky bottom-0 border-t bg-card px-6 py-3">
            <Button type="submit" disabled={save.isPending}>
              {apiKey ? <Trans>Save</Trans> : <Trans>Create key</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
