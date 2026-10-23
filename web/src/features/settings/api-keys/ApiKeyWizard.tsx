import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { BotIcon, EyeIcon, KeyRoundIcon, PartyPopperIcon, SlidersHorizontalIcon, SparklesIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link, Navigate, useLocation, useNavigate } from "react-router"

import {
  BotAvatar,
  CheckItem,
  CheckList,
  ChoiceCards,
  Dot,
  ErrorLine,
  StepActions,
  StepHeader,
  SummaryList,
  WizardFrame,
} from "@/components/common"
import { CopyButton } from "@/components/common/CopyButton"
import {
  API_KEY_SCOPES,
  formatDateTime,
  OWNER_ONLY_SCOPES,
  useEnumText,
  WORKSPACE_WIDE_SCOPES,
} from "@/components/common/text"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { API_KEY_PATHS, chosenScopes, useApiKeyDraft, type ApiKeyWizardState } from "@/features/settings/api-keys/draft"
import { Field } from "@/features/settings/ui"
import { idempotencyKey } from "@/features/setup/draft"
import { PreviewCaption } from "@/features/setup/previews"
import { api, unwrap, type ApiKeyCreate, type ApiKeyScope } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

type Stage = "name" | "access" | "review" | "done"

const HOME = "/settings/api-keys"

function toggled<T>(list: T[], v: T, on: boolean) {
  const rest = list.filter((x) => x !== v)
  return on ? [...rest, v] : rest
}

function localDate(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function Preview({ name, avatar, scopes }: { name: string; avatar: string; scopes: ApiKeyScope[] | null }) {
  const { t } = useLingui()
  const text = useEnumText()
  const url = avatar.trim()
  return (
    <div className="flex w-full max-w-[24rem] flex-col gap-5" aria-hidden="true">
      <div className="flex items-end gap-2.5 rounded-2xl border bg-card p-5 shadow-[0_1px_2px_rgb(15_23_42/0.04),0_24px_60px_-28px_rgb(15_23_42/0.35)]">
        <BotAvatar name={name || t`Bot`} url={url.startsWith("https://") ? url : undefined} className="size-8" />
        <div className="flex min-w-0 flex-col gap-1">
          <span className="truncate text-caption font-medium text-muted-foreground">{name || t`Bot`}</span>
          <Bubble variant="secondary">
            <BubbleContent className="rounded-2xl px-3.5 py-2 text-body">
              <Trans>Thanks for waiting. Your invoice is on its way.</Trans>
            </BubbleContent>
          </Bubble>
        </div>
      </div>
      {scopes && (
        <ul className="flex flex-col gap-1.5 text-small text-muted-foreground">
          {scopes.map((s) => (
            <li key={s} className="flex gap-2">
              <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-brand" />
              <span className="min-w-0">{text.scope[s]}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function Layout({
  stage,
  name,
  avatar,
  scopes,
  onLeave,
  children,
}: {
  stage: Stage
  name: string
  avatar: string
  scopes: ApiKeyScope[] | null
  onLeave?: () => void
  children: React.ReactNode
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as ApiKeyWizardState
  const steps = [
    { key: "name", label: t`Name` },
    { key: "access", label: t`Access` },
    { key: "review", label: t`Review` },
    { key: "done", label: t`Done` },
  ]
  const back = stage === "access" ? API_KEY_PATHS.name : stage === "review" ? API_KEY_PATHS.access : undefined
  return (
    <WizardFrame
      steps={steps}
      current={stage}
      stepHref={(key) =>
        stage === "done"
          ? undefined
          : key === "name" || (key === "access" && stage === "review")
            ? API_KEY_PATHS[key as "name" | "access"]
            : undefined
      }
      stepsLabel={t`Setup steps`}
      back={back}
      leave={
        stage === "done" ? null : (
          <Button
            variant="ghost"
            size="sm"
            data-testid="setup-later"
            onClick={() => {
              onLeave?.()
              navigate(state.back ?? HOME, { replace: true })
            }}
          >
            <Trans>Cancel</Trans>
          </Button>
        )
      }
      preview={
        <>
          <Preview name={name} avatar={avatar} scopes={scopes} />
          <PreviewCaption>
            <Trans>Messages the key sends show its bot name and picture.</Trans>
          </PreviewCaption>
        </>
      }
    >
      {children}
    </WizardFrame>
  )
}

export function ApiKeyNameStep() {
  const { canManage } = useSession()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as ApiKeyWizardState
  const { t } = useLingui()
  const [draft, patch, clear] = useApiKeyDraft()
  if (!canManage) return <Navigate to="/" replace />
  const display = draft.botName.trim() || draft.name.trim()
  return (
    <Layout stage="name" name={display} avatar={draft.avatar} scopes={null} onLeave={clear}>
      <StepHeader title={<Trans>What is this key for?</Trans>}>
        <Trans>Name it after the backend, script or bot that will use it.</Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        data-testid="setup-api-key-name"
        onSubmit={(e) => {
          e.preventDefault()
          if (!draft.name.trim()) return
          navigate(API_KEY_PATHS.access, { state: { back: state.back } })
        }}
      >
        <Field label={<Trans>Name</Trans>} htmlFor="key-name">
          <Input
            id="key-name"
            autoFocus
            required
            maxLength={200}
            className="h-10 rounded-xl"
            value={draft.name}
            placeholder={t`Production backend`}
            onChange={(e) => patch({ name: e.target.value })}
          />
        </Field>
        <Field
          label={<Trans>Bot name (optional)</Trans>}
          htmlFor="key-bot-name"
          hint={<Trans>Shown on its messages. The key's name when empty.</Trans>}
        >
          <Input
            id="key-bot-name"
            maxLength={200}
            className="h-10 rounded-xl"
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
            className="h-10 rounded-xl font-mono text-caption"
            value={draft.avatar}
            placeholder="https://"
            onChange={(e) => patch({ avatar: e.target.value })}
          />
        </Field>
        <StepActions testId="setup-api-key-continue">
          <Trans>Continue</Trans>
        </StepActions>
      </form>
    </Layout>
  )
}

export function ApiKeyAccessStep() {
  const { canManage, isOwner } = useSession()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as ApiKeyWizardState
  const { t } = useLingui()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const [draft, patch, clear] = useApiKeyDraft()
  const [touched, setTouched] = useState(false)
  if (!canManage) return <Navigate to="/" replace />
  if (!draft.name.trim()) return <Navigate to={API_KEY_PATHS.name} replace state={{ back: state.back }} />
  const limited = draft.inboxIds.length > 0
  const scopes = chosenScopes(draft, isOwner)
  const allowed = (s: ApiKeyScope) =>
    (!limited || !WORKSPACE_WIDE_SCOPES.includes(s)) && (isOwner || !OWNER_ONLY_SCOPES.includes(s))
  const display = draft.botName.trim() || draft.name.trim()
  return (
    <Layout stage="access" name={display} avatar={draft.avatar} scopes={scopes} onLeave={clear}>
      <StepHeader title={<Trans>What may it do?</Trans>}>
        <Trans>Scopes and inboxes cannot be changed later. For different ones, create a new key.</Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        data-testid="setup-api-key-access"
        onSubmit={(e) => {
          e.preventDefault()
          setTouched(true)
          if (scopes.length === 0) return
          navigate(API_KEY_PATHS.review, { state: { back: state.back } })
        }}
      >
        <CheckList
          labelId="key-inboxes"
          label={<Trans>Inboxes</Trans>}
          hint={
            <p className="text-caption text-muted-foreground">
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
        <ChoiceCards
          label={t`Access`}
          value={draft.preset}
          onValueChange={(v) => patch({ preset: v as typeof draft.preset })}
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
        {draft.preset === "custom" && (
          <CheckList
            labelId="key-scopes"
            label={<Trans>Scopes</Trans>}
            hint={
              touched &&
              scopes.length === 0 && (
                <p role="alert" className="text-caption text-destructive">
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
        <StepActions testId="setup-api-key-continue">
          <Trans>Continue</Trans>
        </StepActions>
      </form>
    </Layout>
  )
}

export function ApiKeyReviewStep() {
  const { canManage, isOwner, workspaceId: ws } = useSession()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const state = (useLocation().state ?? {}) as ApiKeyWizardState
  const inboxes = useInboxes().data ?? []
  const [draft, , clear] = useApiKeyDraft()
  const [expiry, setExpiry] = useState("")
  const [today] = useState(() => localDate(new Date()))
  const [created, setCreated] = useState(false)
  const [showScopes, setShowScopes] = useState(false)
  const text = useEnumText()
  const scopes = chosenScopes(draft, isOwner)
  const picked = inboxes.filter((i) => draft.inboxIds.includes(i.id))
  const display = draft.botName.trim() || draft.name.trim()
  const create = useMutation({
    mutationFn: () => {
      const body: ApiKeyCreate = { name: draft.name.trim(), scopes }
      if (picked.length > 0) body.inbox_ids = picked.map((i) => i.id)
      if (expiry) body.expires_at = new Date(`${expiry}T23:59:59`).toISOString()
      if (draft.botName.trim()) body.bot_name = draft.botName.trim()
      if (draft.avatar.trim()) body.bot_avatar_url = draft.avatar.trim()
      return unwrap(
        api.POST("/v1/api-keys", {
          body,
          params: { header: { "Idempotency-Key": idempotencyKey(draft.idempotencyKey, "api-key", body) } },
        })
      )
    },
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: keys.apiKeys(ws) })
      setCreated(true)
      navigate(API_KEY_PATHS.done, {
        replace: true,
        state: {
          back: state.back,
          done: {
            name: draft.name.trim(),
            bot: display,
            inboxNames: picked.map((i) => i.name),
            scopes,
            expires: expiry || null,
            secret: r.secret,
          },
        } satisfies ApiKeyWizardState,
      })
      clear()
    },
  })
  if (!canManage) return <Navigate to="/" replace />
  if (!draft.name.trim() && !created) return <Navigate to={API_KEY_PATHS.name} replace state={{ back: state.back }} />
  return (
    <Layout stage="review" name={display} avatar={draft.avatar} scopes={null} onLeave={clear}>
      <StepHeader title={<Trans>Review the key</Trans>}>
        <Trans>The secret is shown once, on the next page.</Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        data-testid="setup-api-key-review"
        onSubmit={(e) => {
          e.preventDefault()
          if (scopes.length > 0) create.mutate()
        }}
      >
        <SummaryList
          items={[
            [<Trans key="n">Name</Trans>, draft.name.trim()],
            [<Trans key="b">Bot</Trans>, display],
            [
              <Trans key="i">Inboxes</Trans>,
              picked.length > 0 ? picked.map((i) => i.name).join(", ") : <Trans key="a">Every inbox</Trans>,
            ],
            [
              <Trans key="s">Scopes</Trans>,
              <button
                key="sv"
                type="button"
                className="text-brand underline-offset-2 hover:underline"
                aria-expanded={showScopes}
                onClick={() => setShowScopes((v) => !v)}
                data-testid="key-scopes-toggle"
              >
                {scopes.length === API_KEY_SCOPES.length ? (
                  <Trans>All scopes</Trans>
                ) : scopes.length === 1 ? (
                  <Trans>1 scope</Trans>
                ) : (
                  <Trans>{scopes.length} scopes</Trans>
                )}
              </button>,
            ],
          ]}
        />
        {showScopes && (
          <ul className="flex flex-col gap-1 rounded-xl border p-3 text-small" data-testid="key-scope-list">
            {scopes.map((s) => (
              <li key={s} className="flex flex-col">
                <span>{text.scope[s]}</span>
                <code className="font-mono text-caption text-faint">{s}</code>
              </li>
            ))}
          </ul>
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
            className="h-10 rounded-xl sm:max-w-48"
          />
        </Field>
        <ErrorLine error={create.error} />
        <StepActions testId="setup-api-key-create" pending={create.isPending}>
          <Trans>Create key</Trans>
        </StepActions>
      </form>
    </Layout>
  )
}

export function ApiKeyDoneStep() {
  const { canManage, workspaceId: ws } = useSession()
  const navigate = useNavigate()
  const location = useLocation()
  const state = (location.state ?? {}) as ApiKeyWizardState
  const done = state.done
  const { i18n } = useLingui()
  const [secret] = useState(done?.secret ?? null)
  const canSend = !!done?.scopes.includes("messages:write")
  const workspace = useQuery({
    queryKey: keys.workspace(ws),
    queryFn: () => unwrap(api.GET("/v1/workspace")),
    enabled: canSend,
  })
  useEffect(() => {
    if (!done?.secret) return
    navigate(location.pathname + location.search, {
      replace: true,
      state: { ...state, done: { ...done, secret: undefined } } satisfies ApiKeyWizardState,
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  if (!canManage) return <Navigate to="/" replace />
  if (!done) return <Navigate to={HOME} replace />
  const origin = window.location.origin
  const curl = `curl -H "Authorization: Bearer ${secret}" ${origin}/v1/inboxes`
  return (
    <Layout stage="done" name={done.bot} avatar="" scopes={null}>
      <StepHeader icon={<PartyPopperIcon />} title={<Trans>The key is ready</Trans>}>
        <Trans>Use it as a bearer token against this server.</Trans>
      </StepHeader>
      <div className="flex flex-col gap-8" data-testid="setup-api-key-done">
        <SummaryList
          items={[
            [<Trans key="n">Name</Trans>, done.name],
            [<Trans key="b">Bot</Trans>, done.bot],
            [
              <Trans key="i">Inboxes</Trans>,
              done.inboxNames.length > 0 ? done.inboxNames.join(", ") : <Trans key="a">Every inbox</Trans>,
            ],
            [
              <Trans key="s">Scopes</Trans>,
              done.scopes.length === 1 ? (
                <Trans key="sv">1 scope</Trans>
              ) : (
                <Trans key="sv">{done.scopes.length} scopes</Trans>
              ),
            ],
            [
              <Trans key="e">Expires</Trans>,
              done.expires ? formatDateTime(`${done.expires}T23:59:59`, i18n.locale) : <Trans key="nv">Never</Trans>,
            ],
          ]}
        />
        {secret ? (
          <div className="flex flex-col gap-3" data-testid="setup-api-key-secret">
            <p className="text-body font-medium">
              <Trans>Secret</Trans>
            </p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 rounded-xl border bg-surface px-3 py-2 font-mono text-caption break-all">
                {secret}
              </code>
              <CopyButton value={secret} />
            </div>
            <p className="text-small text-faint">
              <Trans>Shown only now. Copy it and store it safely.</Trans>
            </p>
            <div className="flex items-start gap-2">
              <code
                className="min-w-0 flex-1 rounded-xl border bg-surface px-3 py-2 font-mono text-caption break-all"
                data-testid="setup-api-key-curl"
              >
                {curl}
              </code>
              <CopyButton value={curl} />
            </div>
          </div>
        ) : (
          <p className="text-body text-muted-foreground" data-testid="setup-api-key-no-secret">
            <Trans>The secret is not shown again. Revoke the key and create a new one if you did not copy it.</Trans>
          </p>
        )}
        {canSend && workspace.data && (
          <p className="text-body text-muted-foreground" data-testid="setup-api-key-bots">
            {workspace.data.bots_may_send ? (
              <Trans>Bots may send in this workspace, so its messages go out at once.</Trans>
            ) : (
              <Trans>Bots may not send in this workspace: the key writes drafts and a member sends them.</Trans>
            )}{" "}
            <Link to={HOME} className="text-brand underline-offset-2 hover:underline">
              <Trans>Change this on the API keys page.</Trans>
            </Link>
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            size="lg"
            onClick={() => navigate(state.back ?? HOME, { replace: true })}
            data-testid="setup-api-key-back"
          >
            <KeyRoundIcon />
            <Trans>Back to API keys</Trans>
          </Button>
          <Button size="lg" variant="outline" render={<Link to={API_KEY_PATHS.name} state={{ back: state.back }} />}>
            <Trans>Create another</Trans>
          </Button>
        </div>
      </div>
    </Layout>
  )
}
