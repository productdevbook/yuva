import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { ArrowRightIcon, BanIcon, MailCheckIcon, MailIcon, PartyPopperIcon } from "lucide-react"
import { useState } from "react"
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from "react-router"

import { ChoiceCards, ErrorLine, Notice, StepActions, StepHeader } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { canSend, InboxField, NameFields, nameTaken, RulesToggles } from "@/features/broadcasts/ListForm"
import { listLink, useInboxChannels, useList, useSetList } from "@/features/broadcasts/queries"
import { NEWSLETTER_PATHS, useNewsletterDraft, type NewsletterState } from "@/features/setup/newsletterDraft"
import { DEFAULT_COLOR, PreviewCaption, SubscribeMock } from "@/features/setup/previews"
import type { SetupState } from "@/features/setup/setup"
import { idempotencyKey } from "@/features/setup/draft"
import { BackHome } from "@/features/setup/BackHome"
import { SetupLayout } from "@/features/setup/SetupLayout"
import { api, ApiError, unwrap, type MailingListCreate } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useChannel, useInboxes } from "@/lib/workspace"

type Stage = "list" | "sender" | "rules" | "done"
const STAGES: Stage[] = ["list", "sender", "rules", "done"]

function useSteps() {
  const { t } = useLingui()
  return [t`List`, t`Sender`, t`Sign-up`, t`Done`]
}

function Layout({
  stage,
  name,
  description,
  inboxId,
  back,
  onLeave,
  children,
}: {
  stage: Stage
  name: string
  description?: string
  inboxId: string
  back?: string
  onLeave?: () => void
  children: React.ReactNode
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const steps = useSteps()
  const state = (useLocation().state ?? {}) as NewsletterState
  const index = STAGES.indexOf(stage)
  const [listPath, senderPath] = [NEWSLETTER_PATHS.list, NEWSLETTER_PATHS.sender]
  return (
    <SetupLayout
      step={index + 1}
      steps={steps}
      stepsLabel={t`Newsletter steps`}
      stepHref={(key) => (key === "1" ? listPath : key === "2" ? senderPath : undefined)}
      back={
        index > 0 && stage !== "done"
          ? STAGES[index - 1] && NEWSLETTER_PATHS[STAGES[index - 1] as keyof typeof NEWSLETTER_PATHS]
          : undefined
      }
      leave={
        stage === "done" ? undefined : (
          <Button
            variant="ghost"
            size="sm"
            data-testid="setup-later"
            onClick={() => {
              onLeave?.()
              navigate(back ?? state.back ?? "/setup", { replace: true })
            }}
          >
            <Trans>Cancel</Trans>
          </Button>
        )
      }
      preview={<Preview name={name} description={description} inboxId={inboxId} />}
    >
      {children}
    </SetupLayout>
  )
}

function Preview({ name, description, inboxId }: { name: string; description?: string; inboxId: string }) {
  const inbox = useInboxes().data?.find((i) => i.id === inboxId)
  return (
    <>
      <SubscribeMock name={name} description={description} color={inbox?.branding.color ?? DEFAULT_COLOR} />
      <PreviewCaption>
        <Trans>The subscribe form shows the list’s name and description.</Trans>
      </PreviewCaption>
    </>
  )
}

function NeedsInbox() {
  const { t } = useLingui()
  const steps = useSteps()
  return (
    <SetupLayout
      step={1}
      steps={steps}
      stepsLabel={t`Newsletter steps`}
      preview={<SubscribeMock name="" color={DEFAULT_COLOR} />}
    >
      <StepHeader icon={<MailIcon />} title={<Trans>First, an e-mail address</Trans>}>
        <Trans>
          A mailing list sends in an inbox’s name, through its e-mail channel. Set up e-mail first, then come back here
          for the list.
        </Trans>
      </StepHeader>
      <Button
        size="lg"
        render={<Link to="/setup/start/email" state={{ back: "/setup" } satisfies SetupState} />}
        data-testid="newsletter-setup-email"
      >
        <Trans>Set up e-mail</Trans>
        <ArrowRightIcon className="rtl:rotate-180" />
      </Button>
    </SetupLayout>
  )
}

function useResolvedInbox(draftInbox: string) {
  const [params] = useSearchParams()
  const inboxes = useInboxes().data
  const wanted = params.get("inbox") || draftInbox
  const id = inboxes?.find((i) => i.id === wanted)?.id ?? inboxes?.[0]?.id ?? ""
  return { inboxes, inboxId: id }
}

function Loading() {
  const { t } = useLingui()
  const steps = useSteps()
  const inboxes = useInboxes()
  return (
    <SetupLayout step={1} steps={steps} stepsLabel={t`Newsletter steps`} preview={null}>
      <ErrorLine error={inboxes.error} />
      {!inboxes.error && <Skeleton className="h-48 rounded-2xl" />}
    </SetupLayout>
  )
}

export function NewsletterListStep() {
  const { canManage } = useSession()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as NewsletterState
  const [draft, patch, clear] = useNewsletterDraft()
  const { inboxes, inboxId } = useResolvedInbox(draft.inboxId)
  const [taken, setTaken] = useState(!!state.nameTaken)
  if (!canManage) return <Navigate to="/" replace />
  if (!inboxes) return <Loading />
  if (inboxes.length === 0) return <NeedsInbox />
  const back = state.back ?? (draft.back || undefined)
  return (
    <Layout
      stage="list"
      name={draft.name}
      description={draft.description}
      inboxId={inboxId}
      back={back}
      onLeave={clear}
    >
      <StepHeader title={<Trans>Start your mailing list</Trans>}>
        <Trans>People join it on a subscribe form or are added with their consent. Broadcasts go to lists.</Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        data-testid="setup-newsletter"
        onSubmit={(e) => {
          e.preventDefault()
          if (!draft.name.trim()) return
          patch({ inboxId, back: back ?? "", ...(inboxId !== draft.inboxId ? { channelId: null } : {}) })
          navigate(NEWSLETTER_PATHS.sender, { state: { back } satisfies NewsletterState })
        }}
      >
        {inboxes.length > 1 && <InboxField value={inboxId} onChange={(id) => patch({ inboxId: id })} />}
        <NameFields
          autoFocus
          name={draft.name}
          description={draft.description}
          taken={taken}
          onChange={(p) => {
            setTaken(false)
            patch(p)
          }}
        />
        <StepActions testId="list-continue" disabled={!draft.name.trim()}>
          <Trans>Continue</Trans>
        </StepActions>
      </form>
    </Layout>
  )
}

export function NewsletterSenderStep() {
  const { t } = useLingui()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as NewsletterState
  const [draft, patch, clear] = useNewsletterDraft()
  const { inboxId } = useResolvedInbox(draft.inboxId)
  const channels = useInboxChannels(inboxId)
  if (!canManage) return <Navigate to="/" replace />
  if (!draft.name.trim()) return <Navigate to={NEWSLETTER_PATHS.list} replace state={{ back: state.back }} />
  const back = state.back ?? (draft.back || undefined)
  const emails = (channels.data ?? []).filter((c) => c.kind === "email")
  const sendable = emails.filter(canSend)
  const chosen = draft.channelId ?? sendable[0]?.id ?? ""
  const picked = sendable.some((c) => c.id === chosen) ? chosen : ""
  return (
    <Layout
      stage="sender"
      name={draft.name}
      description={draft.description}
      inboxId={inboxId}
      back={back}
      onLeave={clear}
    >
      <StepHeader title={<Trans>Where does it send from?</Trans>}>
        <Trans>
          Confirmation and welcome mails, and every broadcast to the list, go out through an e-mail channel.
        </Trans>
      </StepHeader>
      {!channels.data ? (
        <>
          <ErrorLine error={channels.error} />
          {!channels.error && <Skeleton className="h-40 rounded-2xl" />}
        </>
      ) : (
        <form
          className="flex flex-col gap-6"
          data-testid="setup-newsletter-sender"
          onSubmit={(e) => {
            e.preventDefault()
            patch({ inboxId, channelId: picked })
            navigate(NEWSLETTER_PATHS.rules, { state: { back } satisfies NewsletterState })
          }}
        >
          {sendable.length === 0 && (
            <Notice tone="warning" icon={MailIcon} data-testid="newsletter-no-sender">
              <p>
                <Trans>
                  This inbox has no e-mail channel that can send yet. You can create the list now; it sends once an
                  e-mail channel with a reply account is picked in its settings.
                </Trans>
              </p>
              <Link
                to={`/setup/${inboxId}/add/email`}
                state={{ back: NEWSLETTER_PATHS.sender } satisfies SetupState}
                className="w-fit text-brand underline-offset-4 hover:underline"
                data-testid="newsletter-setup-email"
              >
                <Trans>Set up e-mail</Trans>
              </Link>
            </Notice>
          )}
          <ChoiceCards
            label={t`Sender`}
            value={picked || "none"}
            onValueChange={(v) => patch({ channelId: v === "none" ? "" : v })}
            items={[
              ...emails.map((c) => ({
                value: c.id,
                icon: <MailIcon />,
                title: c.name,
                hint: c.email?.address,
                disabled: canSend(c) ? undefined : <Trans>No reply account</Trans>,
                testId: `newsletter-sender-${c.id}`,
              })),
              {
                value: "none",
                icon: <BanIcon />,
                title: <Trans>None for now</Trans>,
                hint: <Trans>Pick one later in the list settings.</Trans>,
                testId: "newsletter-sender-none",
              },
            ]}
          />
          <StepActions testId="sender-continue">
            <Trans>Continue</Trans>
          </StepActions>
        </form>
      )}
    </Layout>
  )
}

export function NewsletterRulesStep() {
  const { canManage } = useSession()
  const navigate = useNavigate()
  const state = (useLocation().state ?? {}) as NewsletterState
  const setList = useSetList()
  const [draft, patch, clear] = useNewsletterDraft()
  const { inboxId } = useResolvedInbox(draft.inboxId)
  const [created, setCreated] = useState(false)
  const back = state.back ?? (draft.back || undefined)
  const create = useMutation({
    mutationFn: () => {
      const body: MailingListCreate = {
        inbox_id: inboxId,
        name: draft.name.trim(),
        description: draft.description.trim(),
        double_opt_in: draft.double_opt_in,
        public: draft.public,
        commercial: draft.commercial,
        welcome: draft.welcome,
        channel_id: draft.channelId || undefined,
      }
      return unwrap(
        api.POST("/v1/lists", {
          body,
          params: { header: { "Idempotency-Key": idempotencyKey(draft.idempotencyKey, "list", body) } },
        })
      )
    },
    onSuccess: (l) => {
      setList(l)
      setCreated(true)
      navigate(`/setup/newsletter/${l.id}`, { replace: true, state: { back } satisfies NewsletterState })
      clear()
    },
    onError: (err) => {
      if (!(err instanceof ApiError)) return
      if (err.code === "name_taken")
        navigate(NEWSLETTER_PATHS.list, { replace: true, state: { back, nameTaken: true } satisfies NewsletterState })
      else if (err.code === "email_channel_required")
        navigate(NEWSLETTER_PATHS.sender, { replace: true, state: { back } satisfies NewsletterState })
    },
  })
  if (!canManage) return <Navigate to="/" replace />
  if (!created && !draft.name.trim()) return <Navigate to={NEWSLETTER_PATHS.list} replace state={{ back }} />
  if (!created && draft.channelId === null) return <Navigate to={NEWSLETTER_PATHS.sender} replace state={{ back }} />
  return (
    <Layout
      stage="rules"
      name={draft.name}
      description={draft.description}
      inboxId={inboxId}
      back={back}
      onLeave={clear}
    >
      <StepHeader title={<Trans>How do people sign up?</Trans>}>
        <Trans>These rules apply to the subscribe form. You can change them any time in the list settings.</Trans>
      </StepHeader>
      <form
        className="flex flex-col gap-6"
        data-testid="setup-newsletter-rules"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate()
        }}
      >
        <RulesToggles values={draft} onChange={(p) => patch(p)} />
        <ErrorLine error={nameTaken(create.error) ? null : create.error} />
        <StepActions pending={create.isPending} testId="list-create">
          <Trans>Create list</Trans>
        </StepActions>
      </form>
    </Layout>
  )
}

function Sender({ channelId }: { channelId: string }) {
  const channel = useChannel(channelId).data
  if (!channel) return <Skeleton className="h-16 rounded-2xl" />
  const address = channel.email?.address ?? channel.name
  return (
    <Notice icon={MailCheckIcon} className="bg-success/8 [&>svg]:text-success" data-testid="newsletter-sender">
      <p className="font-medium text-foreground">
        <Trans>It sends through {address}.</Trans>
      </p>
      <p>
        {canSend(channel) ? (
          <Trans>Confirmation and welcome mails, and every broadcast to the list, go out from this address.</Trans>
        ) : (
          <Trans>This address has no SMTP account yet. Add one in the inbox settings before the first broadcast.</Trans>
        )}
      </p>
    </Notice>
  )
}

export function NewsletterDoneStep({ listId }: { listId: string }) {
  const { t } = useLingui()
  const steps = useSteps()
  const { canManage } = useSession()
  const back = ((useLocation().state ?? {}) as NewsletterState).back
  const list = useList(listId)
  const l = list.data
  const listName = l?.name ?? ""
  if (!canManage) return <Navigate to="/" replace />
  return (
    <SetupLayout
      step={4}
      steps={steps}
      stepsLabel={t`Newsletter steps`}
      preview={l ? <Preview name={l.name} description={l.description} inboxId={l.inbox_id} /> : null}
    >
      <ErrorLine error={list.error} />
      {!l ? (
        !list.error && <Skeleton className="h-40 rounded-2xl" />
      ) : (
        <div className="flex flex-col gap-6" data-testid="newsletter-done">
          <StepHeader icon={<PartyPopperIcon />} title={<Trans>{listName} is ready</Trans>}>
            <Trans>Put its subscribe form on your site, or add people who agreed to hear from you.</Trans>
          </StepHeader>
          {l.channel_id ? (
            <Sender channelId={l.channel_id} />
          ) : (
            <Notice tone="warning" icon={MailIcon} data-testid="newsletter-no-sender">
              <p>
                <Trans>
                  The list has no e-mail channel to send through yet. Set up e-mail with an SMTP account, then pick it
                  in the list settings.
                </Trans>
              </p>
              <Link
                to={`/setup/${l.inbox_id}/add/email`}
                state={{ back: listLink(l.id) } satisfies SetupState}
                className="w-fit text-brand underline-offset-4 hover:underline"
              >
                <Trans>Set up e-mail</Trans>
              </Link>
            </Notice>
          )}
          <div className="flex flex-wrap gap-2">
            <Button size="lg" render={<Link to={listLink(l.id, "form")} />} data-testid="newsletter-form">
              <Trans>Get the subscribe form</Trans>
              <ArrowRightIcon className="rtl:rotate-180" />
            </Button>
            <Button
              size="lg"
              variant="outline"
              render={<Link to={listLink(l.id, "import")} />}
              data-testid="newsletter-import"
            >
              <Trans>Import people</Trans>
            </Button>
            <Button size="lg" variant="outline" render={<Link to={listLink(l.id)} />} data-testid="newsletter-open">
              <Trans>Add a subscriber</Trans>
            </Button>
            <BackHome variant="outline" />
            {back === "/setup" && (
              <Button size="lg" variant="outline" render={<Link to="/setup" />}>
                <Trans>Back to setup</Trans>
              </Button>
            )}
          </div>
        </div>
      )}
    </SetupLayout>
  )
}
